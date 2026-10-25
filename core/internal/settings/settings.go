// Package settings holds the playback settings in settings.json, apart from
// the UI's ui-settings.json, because the daemon applies them with no window
// open. Unknown keys are kept on write.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/Conava/dux-player/core/internal/atomicfile"
	"github.com/Conava/dux-player/core/internal/player"
)

const FileName = "settings.json"

type Settings struct {
	PreviousThreshold  float64 `json:"previousThreshold"`
	AllowVolumeBoost   bool    `json:"allowVolumeBoost"`
	GaplessMode        string  `json:"gaplessMode"`
	ReplayGainMode     string  `json:"replayGainMode"`
	KeepPlayingOnClose bool    `json:"keepPlayingOnClose"`
	// Speed is the playback rate, pitch kept.
	Speed        float64   `json:"speed"`
	Equalizer    Equalizer `json:"equalizer"`
	Crossfade    Crossfade `json:"crossfade"`
	AutoContinue string    `json:"autoContinue"`
	// HistoryMonths is how long plays are kept.
	HistoryMonths int `json:"historyMonths"`
	// MpvIdleExit is how many minutes mpv stays with nothing playing; 0
	// keeps it running.
	MpvIdleExit int `json:"mpvIdleExit"`
	// ArtistInfoOnline looks up artist images and biographies online.
	ArtistInfoOnline bool `json:"artistInfoOnline"`
	ShowTrayIcon     bool `json:"showTrayIcon"`
	// VolumeTarget is what the volume controls drive: the player ("app") or
	// the default output ("system").
	VolumeTarget string `json:"volumeTarget"`
	// SkipSilenceStart and SkipSilenceEnd skip a track's leading and
	// trailing silence; measuring it needs ffmpeg.
	SkipSilenceStart bool `json:"skipSilenceStart"`
	SkipSilenceEnd   bool `json:"skipSilenceEnd"`
	// PrefetchTracks is how many upcoming server tracks are downloaded
	// ahead, 0-3; PrefetchOnMetered keeps doing it on a metered connection.
	PrefetchTracks    int  `json:"prefetchTracks"`
	PrefetchOnMetered bool `json:"prefetchOnMetered"`
}

// defaultMpvIdleExit frees mpv's memory between listening sessions.
const defaultMpvIdleExit = 5

// EQBands are the equalizer's center frequencies in Hz.
var EQBands = [10]int{31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}

// Equalizer has 10 bands at EQBands, in dB. It runs after mpv, in PipeWire
// (internal/devices), so the player never sees it.
type Equalizer struct {
	Enabled bool        `json:"enabled"`
	Preset  string      `json:"preset"`
	Bands   [10]float64 `json:"bands"`
}

const maxBandGain = 12

// Crossfade blends each track into the next over Seconds. With AlbumGapless,
// consecutive tracks of one album play gapless instead.
type Crossfade struct {
	Enabled      bool    `json:"enabled"`
	Seconds      float64 `json:"seconds"`
	AlbumGapless bool    `json:"albumGapless"`
}

const minCrossfade, maxCrossfade = 1, 12

// Presets are the equalizer presets; "custom" keeps the bands as set.
var Presets = map[string][10]float64{
	"flat":       {},
	"bass":       {6, 5, 4, 2, 0, 0, 0, 0, 0, 0},
	"treble":     {0, 0, 0, 0, 0, 1, 2, 4, 5, 6},
	"vocal":      {-2, -1, 0, 2, 4, 4, 3, 1, 0, -1},
	"rock":       {5, 4, 2, -1, -2, -1, 2, 3, 4, 4},
	"pop":        {-1, 1, 3, 4, 3, 0, -1, -1, 0, 1},
	"jazz":       {3, 2, 1, 2, -1, -1, 0, 1, 2, 3},
	"classical":  {4, 3, 2, 1, -1, -1, 0, 2, 3, 4},
	"electronic": {5, 4, 1, 0, -2, 1, 0, 1, 4, 5},
}

func Defaults() Settings {
	return Settings{
		PreviousThreshold:  5,
		GaplessMode:        "weak",
		ReplayGainMode:     "track",
		KeepPlayingOnClose: true,
		Speed:              1,
		Equalizer:          Equalizer{Preset: "flat"},
		Crossfade:          Crossfade{Seconds: 5, AlbumGapless: true},
		AutoContinue:       player.ContinueOff,
		HistoryMonths:      12,
		MpvIdleExit:        defaultMpvIdleExit,
		ArtistInfoOnline:   true,
		ShowTrayIcon:       true,
		VolumeTarget:       player.VolumeApp,
		PrefetchTracks:     1,
	}
}

// PlayerOptions maps the settings onto what the player hands to mpv.
func (s Settings) PlayerOptions() player.Options {
	o := player.Options{PreviousThreshold: s.PreviousThreshold, VolumeMax: 100, Gapless: s.GaplessMode, ReplayGain: s.ReplayGainMode}
	if s.AllowVolumeBoost {
		o.VolumeMax = 130
	}
	if s.ReplayGainMode == "off" {
		o.ReplayGain = "no"
	}
	o.Speed, o.AutoContinue = s.Speed, s.AutoContinue
	if s.Crossfade.Enabled {
		o.Crossfade = s.Crossfade.Seconds
	}
	o.AlbumGapless = s.Crossfade.AlbumGapless
	o.IdleExit = time.Duration(s.MpvIdleExit) * time.Minute
	o.VolumeTarget = s.VolumeTarget
	o.SkipSilenceStart, o.SkipSilenceEnd = s.SkipSilenceStart, s.SkipSilenceEnd
	return o
}

// Store is the settings file plus its current, validated values.
type Store struct {
	path string
	mu   sync.Mutex
	cur  Settings
	raw  map[string]json.RawMessage
}

// Open reads the file. A missing file means defaults; a broken one is an
// error, so a typo doesn't silently reset everything.
func Open(configDir string) (*Store, error) {
	s := &Store{path: filepath.Join(configDir, FileName), cur: Defaults(), raw: map[string]json.RawMessage{}}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.raw); err != nil {
		return nil, fmt.Errorf("%s: %w", s.path, err)
	}
	for k, v := range s.raw {
		var val any
		if json.Unmarshal(v, &val) != nil {
			continue
		}
		next, err := apply(s.cur, k, val)
		if err != nil {
			// Unknown keys are kept for newer versions; bad values fall back.
			continue
		}
		s.cur = next
	}
	return s, nil
}

func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// Set validates and applies changes, then writes the file. Nothing is
// applied if any value is invalid.
func (s *Store) Set(changes map[string]any) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cur
	for k, v := range changes {
		var err error
		if next, err = apply(next, k, v); err != nil {
			return s.cur, err
		}
	}
	raw := make(map[string]json.RawMessage, len(s.raw))
	for k, v := range s.raw {
		raw[k] = v
	}
	known, err := json.Marshal(next)
	if err != nil {
		return s.cur, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(known, &fields); err != nil {
		return s.cur, err
	}
	for k, v := range fields {
		raw[k] = v
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return s.cur, err
	}
	if err := atomicfile.WriteFile(s.path, append(data, '\n'), 0o600); err != nil {
		return s.cur, err
	}
	s.cur, s.raw = next, raw
	return next, nil
}

// The CLI sends strings, the UI JSON types; both are accepted.
func toBool(key string, v any) (bool, error) {
	switch b := v.(type) {
	case bool:
		return b, nil
	case string:
		if parsed, err := strconv.ParseBool(b); err == nil {
			return parsed, nil
		}
	}
	return false, fmt.Errorf("%s must be true or false", key)
}

// toFloat rejects NaN and infinities, which pass every range check and
// can't be written as JSON.
func toFloat(key string, v any) (float64, error) {
	f, ok := v.(float64)
	if s, isText := v.(string); isText {
		parsed, err := strconv.ParseFloat(s, 64)
		f, ok = parsed, err == nil
	}
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	return f, nil
}

func oneOf(key string, v any, allowed ...string) (string, error) {
	if s, ok := v.(string); ok {
		for _, a := range allowed {
			if s == a {
				return s, nil
			}
		}
	}
	return "", fmt.Errorf("%s must be one of %v", key, allowed)
}

func apply(s Settings, key string, v any) (Settings, error) {
	var err error
	switch key {
	case "previousThreshold":
		var f float64
		if f, err = toFloat(key, v); err == nil && (f < 0 || f > 60) {
			err = fmt.Errorf("%s must be between 0 and 60 seconds", key)
		}
		s.PreviousThreshold = f
	case "allowVolumeBoost":
		s.AllowVolumeBoost, err = toBool(key, v)
	case "gaplessMode":
		s.GaplessMode, err = oneOf(key, v, "weak", "yes", "no")
	case "replayGainMode":
		s.ReplayGainMode, err = oneOf(key, v, "track", "album", "off")
	case "keepPlayingOnClose":
		s.KeepPlayingOnClose, err = toBool(key, v)
	case "speed":
		var f float64
		if f, err = toFloat(key, v); err == nil && (f < 0.5 || f > 2) {
			err = fmt.Errorf("%s must be between 0.5 and 2", key)
		}
		s.Speed = f
	case "autoContinue":
		s.AutoContinue, err = oneOf(key, v, player.ContinueOff, player.ContinueRandom, player.ContinueSimilar)
	case "historyMonths":
		var f float64
		if f, err = toFloat(key, v); err == nil && (f != float64(int(f)) || f < 1 || f > 120) {
			err = fmt.Errorf("%s must be a whole number from 1 to 120", key)
		}
		s.HistoryMonths = int(f)
	case "mpvIdleExit":
		var f float64
		if f, err = toFloat(key, v); err == nil && (f != float64(int(f)) || f < 0 || f > 1440) {
			err = fmt.Errorf("%s must be a whole number of minutes from 0 to 1440", key)
		}
		s.MpvIdleExit = int(f)
	case "artistInfoOnline":
		s.ArtistInfoOnline, err = toBool(key, v)
	case "showTrayIcon":
		s.ShowTrayIcon, err = toBool(key, v)
	case "skipSilenceStart":
		s.SkipSilenceStart, err = toBool(key, v)
	case "skipSilenceEnd":
		s.SkipSilenceEnd, err = toBool(key, v)
	case "prefetchTracks":
		var f float64
		if f, err = toFloat(key, v); err == nil && (f != float64(int(f)) || f < 0 || f > 3) {
			err = fmt.Errorf("%s must be a whole number from 0 to 3", key)
		}
		s.PrefetchTracks = int(f)
	case "prefetchOnMetered":
		s.PrefetchOnMetered, err = toBool(key, v)
	case "volumeTarget":
		s.VolumeTarget, err = oneOf(key, v, player.VolumeApp, player.VolumeSystem)
	case "equalizer":
		s.Equalizer, err = toEqualizer(s.Equalizer, v)
	case "crossfade":
		s.Crossfade, err = toCrossfade(s.Crossfade, v)
	default:
		err = fmt.Errorf("unknown setting %q", key)
	}
	return s, err
}

// object takes an object, or its JSON text from the CLI.
func object(key string, v any) (map[string]any, error) {
	if text, ok := v.(string); ok {
		var m map[string]any
		if json.Unmarshal([]byte(text), &m) == nil {
			return m, nil
		}
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	return nil, fmt.Errorf("%s must be an object", key)
}

// toCrossfade merges a partial {enabled, seconds, albumGapless} object.
func toCrossfade(cur Crossfade, v any) (Crossfade, error) {
	m, err := object("crossfade", v)
	if err != nil {
		return cur, err
	}
	next := cur
	if e, ok := m["enabled"]; ok {
		if next.Enabled, err = toBool("crossfade.enabled", e); err != nil {
			return cur, err
		}
	}
	if e, ok := m["albumGapless"]; ok {
		if next.AlbumGapless, err = toBool("crossfade.albumGapless", e); err != nil {
			return cur, err
		}
	}
	if e, ok := m["seconds"]; ok {
		f, err := toFloat("crossfade.seconds", e)
		if err != nil || f < minCrossfade || f > maxCrossfade {
			return cur, fmt.Errorf("crossfade.seconds must be between %d and %d", minCrossfade, maxCrossfade)
		}
		next.Seconds = f
	}
	return next, nil
}

// toEqualizer merges a partial {enabled, preset, bands} object (or its JSON
// text, from the CLI) into cur. Picking a preset sets its bands; setting
// bands without a preset makes it custom.
func toEqualizer(cur Equalizer, v any) (Equalizer, error) {
	m, err := object("equalizer", v)
	if err != nil {
		return cur, errors.New("equalizer must be an object {enabled, preset, bands}")
	}
	next := cur
	if e, ok := m["enabled"]; ok {
		b, err := toBool("equalizer.enabled", e)
		if err != nil {
			return cur, err
		}
		next.Enabled = b
	}
	if raw, ok := m["bands"]; ok {
		list, ok := raw.([]any)
		if !ok || len(list) != len(next.Bands) {
			return cur, fmt.Errorf("equalizer.bands must be %d numbers", len(next.Bands))
		}
		for i, b := range list {
			g, err := toFloat("equalizer.bands", b)
			if err != nil || g < -maxBandGain || g > maxBandGain {
				return cur, fmt.Errorf("equalizer.bands must be between -%d and %d dB", maxBandGain, maxBandGain)
			}
			next.Bands[i] = g
		}
		next.Preset = "custom"
	}
	if raw, ok := m["preset"]; ok {
		name, _ := raw.(string)
		bands, known := Presets[name]
		switch {
		case name == "custom":
		case !known:
			return cur, fmt.Errorf("equalizer.preset %q is unknown", name)
		default:
			next.Bands = bands
		}
		next.Preset = name
	}
	return next, nil
}
