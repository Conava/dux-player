package engine

import (
	"math"
	"strconv"
	"strings"

	"go.senan.xyz/taglib"
)

// tagGain is a file's ReplayGain tags in dB, peaks linear; nil when absent.
type tagGain struct {
	track, album, trackPeak, albumPeak *float64
}

// readTags reads a local file's ReplayGain tags, or R128 tags (Opus), which
// are relative to -23 LUFS where ReplayGain's reference is 5 dB louder.
func readTags(path string) tagGain {
	tags, err := taglib.ReadTags(path)
	if err != nil {
		return tagGain{}
	}
	get := func(key string) string {
		if v := tags[key]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	g := tagGain{
		track:     number(get("REPLAYGAIN_TRACK_GAIN"), -60, 60),
		album:     number(get("REPLAYGAIN_ALBUM_GAIN"), -60, 60),
		trackPeak: number(get("REPLAYGAIN_TRACK_PEAK"), 0, 10),
		albumPeak: number(get("REPLAYGAIN_ALBUM_PEAK"), 0, 10),
	}
	if r := number(get("R128_TRACK_GAIN"), -32768, 32767); g.track == nil && r != nil {
		v := *r/256 + 5
		g.track = &v
	}
	if r := number(get("R128_ALBUM_GAIN"), -32768, 32767); g.album == nil && r != nil {
		v := *r/256 + 5
		g.album = &v
	}
	return g
}

func number(s string, lo, hi float64) *float64 {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "dB"))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < lo || v > hi {
		return nil
	}
	return &v
}

// replayGain is the linear factor for mode (track, album or no), as mpv
// picks it: the file's tags, then fallback (dB), lowered so a known peak
// doesn't clip.
func replayGain(mode string, t tagGain, fallback *float64) float64 {
	if mode != "track" && mode != "album" {
		return 1
	}
	gain, peak := t.track, t.trackPeak
	if mode == "album" && t.album != nil {
		gain, peak = t.album, t.albumPeak
	}
	if gain == nil {
		gain, peak = fallback, nil
	}
	if gain == nil {
		return 1
	}
	f := math.Pow(10, *gain/20)
	if peak != nil && *peak > 0 {
		f = min(f, 1 / *peak)
	}
	return f
}

// volumeFactor is mpv's volume curve: cubic, so 50% sounds half as loud.
func volumeFactor(volume float64, muted bool) float64 {
	if muted {
		return 0
	}
	v := max(volume, 0) / 100
	return v * v * v
}
