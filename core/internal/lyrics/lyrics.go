// Package lyrics finds lyrics for the playing track through dux-go/lyrics:
// sidecar files next to local tracks, the track's own server when it serves
// lyrics, then the configured providers.
package lyrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
	duxlyrics "github.com/duxlabs-dev/dux-go/lyrics"

	"github.com/Conava/dux-player/core/internal/atomicfile"
	"github.com/Conava/dux-player/core/internal/source"
)

const (
	// Topic carries the current track's lyrics. It is not in the default
	// subscription: lyrics are looked up only while a client subscribes.
	Topic = "lyrics"
	// TopicProviders carries the provider list after a change.
	TopicProviders = "lyricsProviders"

	serverPrefix = "server:"
	// ServerSource is Result.source for lyrics from the track's own server.
	ServerSource = "server"
)

// ServerLyrics is a library whose server serves lyrics for its tracks
// (OpenSubsonic songLyrics, Jellyfin). Empty lyrics mean none.
type ServerLyrics interface {
	Lyrics(ctx context.Context, id string) (*duxlyrics.Lyrics, error)
}

// Libraries is the part of the library manager lyrics need.
type Libraries interface {
	Get(id string) (source.Source, error)
	Tracks(ctx context.Context, refs []source.Ref) ([]source.Track, error)
}

// Event is the Topic payload. Result is the dux-go Result JSON unchanged.
type Event struct {
	Library string            `json:"library"`
	ID      string            `json:"id"`
	State   string            `json:"state"` // idle, loading, ready, error
	Result  *duxlyrics.Result `json:"result"`
	Error   string            `json:"error,omitempty"`
	// Chosen says the user picked these lyrics for this track.
	Chosen bool `json:"chosen,omitempty"`
	// Offset is the user's timing nudge in seconds, later when positive; Result already has it.
	Offset float64 `json:"offset,omitempty"`
}

// Choice is one source's lyrics for a track, for the user to pick from.
type Choice struct {
	Source string `json:"source"`
	Name   string `json:"name"`
	Timing string `json:"timing"` // word, line, plain
}

type Config struct {
	CacheDir, ConfigDir, DataDir, Version string
	Libraries                             Libraries
	Publish                               func(topic string, data any)
	HasSubscriber                         func(topic string) bool
	// Enabled is the lyrics setting; nil is on.
	Enabled func() bool
	// AudioTiming says whether word timing is fitted to the audio; nil is off.
	AudioTiming func() bool
	// Keep is the setting to fetch lyrics for kept tracks; nil is on.
	Keep func() bool
	// HasCopy says whether a server track still has a copy on this device;
	// known is false when that can't be told.
	HasCopy func(ctx context.Context, ref source.Ref) (has, known bool)
	// AudioFile finds a track's audio on this device.
	AudioFile func(ctx context.Context, ref source.Ref) (string, bool)
	// Decode reads a file as stereo floats at FitRate.
	Decode func(ctx context.Context, path string, read func(io.Reader) error) error
}

type Service struct {
	cfg       Config
	client    *duxlyrics.Client
	providers *providerStore

	mu      sync.Mutex
	track   *source.Track
	shown   string // key of the track the last event belongs to
	last    Event
	cancel  context.CancelFunc
	baseCtx context.Context

	wasOn      bool
	wasKeep    bool
	wasFitting bool
	next       *source.Track
	warm       []warmed // newest last
	warming    string   // key of the track being prefetched
	warmCancel context.CancelFunc

	offsetMu sync.Mutex // one nudge reads and writes the file at a time

	stop   context.CancelFunc
	workMu sync.Mutex // keeps spawn from racing Close's wait
	work   sync.WaitGroup
}

// maxWarmed keeps the next track's result while the track after it is
// prefetched: the queue can report the new next before the player changes.
const maxWarmed = 3

// warmed is a prefetched result. Its cache entries wait in dir until the
// track plays, so a track that never does leaves nothing behind.
type warmed struct {
	key string
	res *duxlyrics.Result
	dir string
}

func New(ctx context.Context, cfg Config) *Service {
	ctx, stop := context.WithCancel(ctx)
	s := &Service{cfg: cfg, baseCtx: ctx, stop: stop, providers: openProviders(filepath.Join(cfg.ConfigDir, providersFile))}
	s.client = s.newClient(s.cacheDir())
	s.wasOn, s.wasFitting, s.wasKeep = s.enabled(), s.fitting(), cfg.Keep == nil || cfg.Keep()
	s.spawn(func() { s.refreshKept(); s.client.Prune() })
	s.spawn(func() { pruneFits(filepath.Join(cfg.CacheDir, "lyrics-fit")) })
	if err := os.RemoveAll(s.aheadDir()); err != nil {
		log.Warnf("lyrics: %v", err)
	}
	return s
}

// spawn runs f in the background for Close to wait on, and reports false
// without running it once the service has stopped.
func (s *Service) spawn(f func()) bool {
	s.workMu.Lock()
	defer s.workMu.Unlock()
	if s.baseCtx.Err() != nil {
		return false
	}
	s.work.Go(f)
	return true
}

// closeWait bounds Close: the cache walks and the prune don't stop when
// cancelled, and shutdown must not wait for a whole walk.
const closeWait = 3 * time.Second

// Close cancels the lookups and waits up to closeWait for the service's
// goroutines. A dux-go lookup that was already running isn't cancelled and
// may still write its cache entry after Close returns.
func (s *Service) Close() {
	s.workMu.Lock()
	s.stop()
	s.workMu.Unlock()
	done := make(chan struct{})
	go func() {
		s.work.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(closeWait):
		log.Infof("lyrics: stopped waiting for background work after %s", closeWait)
	}
}

func (s *Service) newClient(dir string) *duxlyrics.Client {
	return duxlyrics.New(duxlyrics.Options{
		CacheDir:  dir,
		UserAgent: "dplayer/" + s.cfg.Version + " (+https://github.com/Conava/dux-player)",
		Resolve:   s.resolve,
	})
}

// fitMaxAge is how long a measured shift is kept: about as long as the lyrics
// cache keeps the lyrics it was measured for.
const fitMaxAge = 90 * 24 * time.Hour

func pruneFits(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Warnf("lyrics: can't read the measured shifts: %v", err)
		}
		return
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > fitMaxAge {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				log.Warnf("lyrics: can't remove an old measured shift: %v", err)
			}
		}
	}
}

func (s *Service) keptDir() string  { return filepath.Join(s.cfg.CacheDir, "lyrics-kept") }
func (s *Service) cacheDir() string { return filepath.Join(s.cfg.CacheDir, "lyrics") }
func (s *Service) aheadDir() string { return filepath.Join(s.cfg.CacheDir, "lyrics-next") }

func (s *Service) enabled() bool { return s.cfg.Enabled == nil || s.cfg.Enabled() }

func (s *Service) fitting() bool { return s.cfg.AudioTiming != nil && s.cfg.AudioTiming() }

func hashed(k string) string {
	sum := sha256.Sum256([]byte(k))
	return hex.EncodeToString(sum[:])
}

func key(t *source.Track) string {
	if t == nil {
		return ""
	}
	return t.Library + ":" + t.ID
}

// TrackChanged is called with the playing track (nil when none). Lyrics are
// fetched only while someone subscribes to Topic.
func (s *Service) TrackChanged(t *source.Track) {
	s.mu.Lock()
	if key(t) == key(s.track) {
		s.mu.Unlock()
		return
	}
	s.track = t
	s.mu.Unlock()
	if s.cfg.HasSubscriber(Topic) {
		s.refresh()
	}
}

// Subscribed runs after any subscribe: a new lyrics subscriber gets the
// current state, fetching it if nobody was watching when the track changed.
func (s *Service) Subscribed() {
	if !s.cfg.HasSubscriber(Topic) {
		return
	}
	s.mu.Lock()
	// An idle result for a track is what lyrics being off left behind.
	stale := s.shown != key(s.track) || s.last.State == "error" || (s.last.State == "idle" && s.track != nil)
	last := s.last
	s.mu.Unlock()
	if stale {
		s.refresh()
		return
	}
	s.publish(last)
	s.prefetch()
}

func (s *Service) refresh() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	t := s.track
	s.shown = key(t)
	if t == nil || !s.enabled() {
		ev := Event{State: "idle"}
		s.last = ev
		s.mu.Unlock()
		s.publish(ev)
		return
	}
	ctx, cancel := context.WithCancel(s.baseCtx)
	s.cancel = cancel
	if res := s.chosen(key(t)); res != nil {
		ev := Event{Library: t.Library, ID: t.ID, State: "ready", Result: res, Chosen: true}
		s.last = ev
		s.mu.Unlock()
		cancel()
		s.publish(ev)
		s.prefetch()
		return
	}
	if w := s.warmedLocked(key(t)); w != nil {
		ev := Event{Library: t.Library, ID: t.ID, State: "ready", Result: w.res}
		s.last = ev
		dir := w.dir
		w.dir = ""
		s.mu.Unlock()
		cancel()
		s.publish(ev)
		s.adopt(dir)
		s.prefetch()
		return
	}
	ev := Event{Library: t.Library, ID: t.ID, State: "loading"}
	s.last = ev
	s.mu.Unlock()
	s.publish(ev)

	s.spawn(func() {
		defer cancel()
		res, err := s.Lookup(ctx, *t)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Infof("lyrics for %q: %v", t.Title, err)
			s.show(ctx, t, Event{Library: t.Library, ID: t.ID, State: "error", Error: err.Error()})
			return
		}
		s.show(ctx, t, Event{Library: t.Library, ID: t.ID, State: "ready", Result: res})
		// Shown first so the view isn't held up; the slower sources may still improve it.
		if best := s.Best(ctx, *t); best != nil && ctx.Err() == nil {
			s.remember(warmed{key: key(t), res: best})
			if retimed(res, best) {
				s.show(ctx, t, Event{Library: t.Library, ID: t.ID, State: "ready", Result: best})
			}
		}
		s.prefetch()
	})
}

// publish sends ev with the track's timing nudge applied. Results are kept
// without it, so a nudge never compounds and the caches stay valid.
func (s *Service) publish(ev Event) {
	if ev.Result != nil && ev.ID != "" {
		if ev.Offset = s.offset(ev.Library + ":" + ev.ID); ev.Offset != 0 {
			ev.Result = shifted(ev.Result, ev.Offset)
		}
	}
	s.cfg.Publish(Topic, ev)
}

// show publishes ev unless the view moved on from t.
func (s *Service) show(ctx context.Context, t *source.Track, ev Event) {
	s.mu.Lock()
	if s.shown != key(t) || ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	s.last = ev
	s.mu.Unlock()
	s.publish(ev)
}

func retimed(a, b *duxlyrics.Result) bool {
	if a.Source != b.Source || len(a.Synced) != len(b.Synced) || tier(answer{res: a}) != tier(answer{res: b}) {
		return true
	}
	return len(a.Synced) > 0 && a.Synced[0].Start != b.Synced[0].Start
}

// NextChanged is called with the track that plays after the current one.
func (s *Service) NextChanged(t *source.Track) {
	s.mu.Lock()
	if key(t) == key(s.next) {
		s.mu.Unlock()
		return
	}
	s.next = t
	s.mu.Unlock()
	s.prefetch()
}

// prefetch finds the next track's lyrics ahead of a skip, only while someone
// watches lyrics.
func (s *Service) prefetch() {
	if !s.cfg.HasSubscriber(Topic) || !s.enabled() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.next
	k := key(t)
	if t == nil || k == key(s.track) || k == s.warming || s.warmedLocked(k) != nil || s.chosen(k) != nil {
		return
	}
	if s.warmCancel != nil {
		s.warmCancel()
	}
	ctx, cancel := context.WithCancel(s.baseCtx)
	s.warming, s.warmCancel = k, cancel
	s.spawn(func() {
		defer cancel()
		dir := filepath.Join(s.aheadDir(), hashed(k))
		best := s.best(ctx, s.newClient(dir), *t)
		if best != nil && ctx.Err() == nil {
			s.remember(warmed{key: k, res: best, dir: dir})
		} else if err := os.RemoveAll(dir); err != nil {
			log.Warnf("lyrics: %v", err)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.warming == k {
			s.warming = ""
		}
	})
}

func (s *Service) warmedLocked(k string) *warmed {
	for i := range s.warm {
		if s.warm[i].key == k {
			return &s.warm[i]
		}
	}
	return nil
}

func (s *Service) remember(w warmed) {
	s.mu.Lock()
	var dropped []string
	s.warm = slices.DeleteFunc(s.warm, func(old warmed) bool {
		if old.key != w.key {
			return false
		}
		dropped = append(dropped, old.dir)
		return true
	})
	s.warm = append(s.warm, w)
	if len(s.warm) > maxWarmed {
		dropped = append(dropped, s.warm[0].dir)
		s.warm = slices.Delete(s.warm, 0, 1)
	}
	s.mu.Unlock()
	for _, dir := range dropped {
		discard(dir)
	}
}

func discard(dir string) {
	if dir == "" {
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		log.Warnf("lyrics: %v", err)
	}
}

// adopt moves a prefetched track's cache entries into the real cache: it was played.
func (s *Service) adopt(dir string) {
	if dir == "" {
		return
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		to := filepath.Join(s.cacheDir(), rel)
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		return os.Rename(path, to)
	})
	if err != nil {
		log.Warnf("lyrics: keeping prefetched lyrics: %v", err)
	}
	discard(dir)
}

// forget drops what was found under the old settings.
func (s *Service) forget() {
	s.mu.Lock()
	if s.warmCancel != nil {
		s.warmCancel()
	}
	old := s.warm
	s.warm, s.warming = nil, ""
	s.mu.Unlock()
	for _, w := range old {
		discard(w.dir)
	}
}

// SettingsChanged follows the lyrics setting.
func (s *Service) SettingsChanged() {
	on, fitting, keep := s.enabled(), s.fitting(), s.cfg.Keep == nil || s.cfg.Keep()
	s.mu.Lock()
	keptChanged := s.wasOn != on || s.wasKeep != keep
	changed := s.wasOn != on || s.wasFitting != fitting
	s.wasOn, s.wasFitting, s.wasKeep = on, fitting, keep
	s.mu.Unlock()
	if keptChanged {
		s.refreshKept()
	}
	if !changed {
		return
	}
	s.forget()
	if s.cfg.HasSubscriber(Topic) {
		s.refresh()
	}
}

func (s *Service) chosenPath(k string) string {
	return filepath.Join(s.cfg.DataDir, "lyrics-chosen", hashed(k)+".json")
}

// chosen is what the user picked for this track, kept apart from the cache
// so it never expires and works offline.
func (s *Service) chosen(k string) *duxlyrics.Result {
	data, err := os.ReadFile(s.chosenPath(k))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		log.Warnf("lyrics: %v", err)
		return nil
	}
	var res duxlyrics.Result
	if err := json.Unmarshal(data, &res); err != nil {
		log.Warnf("lyrics: %s is unreadable: %v", s.chosenPath(k), err)
		return nil
	}
	return &res
}

// The timing nudge: maxNudge bounds it and nudgeStep is what it rounds to, in seconds.
const (
	maxNudge  = 10.0
	nudgeStep = 0.5
)

func (s *Service) offsetPath(k string) string {
	return filepath.Join(s.cfg.DataDir, "lyrics-offset", hashed(k)+".json")
}

// offset is the timing nudge the user gave a track, within maxNudge; 0 without one or when the file is bad.
func (s *Service) offset(k string) float64 {
	data, err := os.ReadFile(s.offsetPath(k))
	if errors.Is(err, fs.ErrNotExist) {
		return 0
	}
	if err != nil {
		log.Warnf("lyrics: %v", err)
		return 0
	}
	var v float64
	if err := json.Unmarshal(data, &v); err != nil {
		log.Warnf("lyrics: %s is unreadable, ignoring it", s.offsetPath(k))
		return 0
	}
	return max(-maxNudge, min(maxNudge, v))
}

// SetOffset sets the playing track's lyrics timing to seconds, later when
// positive, or moves it by seconds when relative, and returns the new offset.
func (s *Service) SetOffset(seconds float64, relative bool) (float64, error) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, errors.New("the offset must be a number of seconds")
	}
	if !s.enabled() {
		return 0, errOff
	}
	s.mu.Lock()
	k := key(s.track)
	s.mu.Unlock()
	if k == "" {
		return 0, errors.New("nothing is playing")
	}
	s.offsetMu.Lock()
	v, err := s.storeOffset(k, seconds, relative)
	s.offsetMu.Unlock()
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	last := s.last
	s.mu.Unlock()
	if last.Library+":"+last.ID == k && s.cfg.HasSubscriber(Topic) {
		s.publish(last)
	}
	return v, nil
}

func (s *Service) storeOffset(k string, v float64, relative bool) (float64, error) {
	path := s.offsetPath(k)
	if relative {
		v += s.offset(k)
	}
	v = max(-maxNudge, min(maxNudge, math.Round(v/nudgeStep)*nudgeStep))
	if v == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return 0, err
		}
		return 0, nil
	}
	data, err := json.Marshal(v)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o700)
	}
	if err == nil {
		err = atomicfile.WriteFile(path, data, 0o600)
	}
	return v, err
}

// Choices lists what every enabled source has for a track.
func (s *Service) Choices(ctx context.Context, ref source.Ref) ([]Choice, error) {
	t, err := s.trackOf(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := []Choice{}
	results, loose := s.gather(ctx, s.client, t)
	for _, r := range append(results, loose...) {
		// a lyrics file beside the track answers for every provider
		seen := slices.ContainsFunc(out, func(c Choice) bool { return c.Source == string(r.Source) })
		if !r.Found || r.Instrumental || seen {
			continue
		}
		c := Choice{Source: string(r.Source), Name: r.Attribution.Name, Timing: "plain"}
		if name, ok := providerNames[r.Source]; ok && c.Name == "" {
			c.Name = name
		}
		switch tier(answer{res: r}) {
		case 2:
			c.Timing = "word"
		case 1:
			c.Timing = "line"
		}
		out = append(out, c)
	}
	return out, nil
}

// Choose keeps one source's lyrics for a track from now on; an empty source
// goes back to automatic.
func (s *Service) Choose(ctx context.Context, ref source.Ref, src string) error {
	t, err := s.trackOf(ctx, ref)
	if err != nil {
		return err
	}
	path := s.chosenPath(key(&t))
	if src == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		s.reshow(key(&t))
		return nil
	}
	results, loose := s.gather(ctx, s.client, t)
	results = append(results, loose...)
	i := slices.IndexFunc(results, func(r *duxlyrics.Result) bool { return string(r.Source) == src && r.Found })
	if i < 0 {
		return fmt.Errorf("%s has no lyrics for this track", src)
	}
	data, err := json.Marshal(results[i])
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := atomicfile.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	s.reshow(key(&t))
	return nil
}

func (s *Service) reshow(k string) {
	s.mu.Lock()
	s.warm = slices.DeleteFunc(s.warm, func(w warmed) bool { return w.key == k })
	current := key(s.track) == k
	s.mu.Unlock()
	if current && s.cfg.HasSubscriber(Topic) {
		s.refresh()
	}
}

// Get looks up the lyrics of any track.
func (s *Service) Get(ctx context.Context, ref source.Ref) (*duxlyrics.Result, error) {
	t, err := s.trackOf(ctx, ref)
	if err != nil {
		return nil, err
	}
	return s.Lookup(ctx, t)
}

var errOff = errors.New("lyrics are turned off")

func (s *Service) trackOf(ctx context.Context, ref source.Ref) (source.Track, error) {
	if !s.enabled() {
		return source.Track{}, errOff
	}
	tracks, err := s.cfg.Libraries.Tracks(ctx, []source.Ref{ref})
	if err != nil {
		return source.Track{}, err
	}
	if len(tracks) == 0 {
		return source.Track{}, source.ErrNotFound
	}
	return tracks[0], nil
}

// syncGrace is how long a synced server result waits for a provider that
// might have word sync, as dux-go waits within one lookup.
const syncGrace = 2 * time.Second

// Lookup asks sidecars, the cache, the track's server and the providers.
// The server is asked apart from the providers: within one dux-go lookup
// its plain text won once the others took longer than dux-go's sync grace,
// so slower synced lyrics never showed.
func (s *Service) Lookup(ctx context.Context, t source.Track) (*duxlyrics.Result, error) {
	order := s.providers.enabled()
	req, serverID := s.request(ctx, t, order)
	ask := func(look func() (*duxlyrics.Result, error)) chan answer {
		out := make(chan answer, 1)
		ran := s.spawn(func() {
			res, err := look()
			out <- answer{res, err}
		})
		if !ran {
			out <- answer{err: context.Canceled}
		}
		return out
	}
	var server chan answer
	if serverID != "" {
		sreq := req
		sreq.Providers = []duxlyrics.Provider{serverID}
		server = ask(func() (*duxlyrics.Result, error) { return s.client.Lookup(ctx, sreq) })
	}
	providers := ask(func() (*duxlyrics.Result, error) {
		res, _, err := find(ctx, s.client, req, t.Album, false)
		return res, err
	})
	if server == nil {
		a := <-providers
		return a.res, a.err
	}
	srv := <-server
	if srv.err == nil && srv.res.Found {
		srv.res.Source = ServerSource
	}
	var prov answer
	switch tier(srv) {
	case 2:
		return srv.res, nil
	case 1:
		select {
		case prov = <-providers:
		case <-time.After(syncGrace):
			return srv.res, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	default:
		prov = <-providers
	}
	return pick(order, srv, prov)
}

// request is the lookup for t, and the provider id of its server's lyrics
// when it has them and they are on.
func (s *Service) request(ctx context.Context, t source.Track, order []duxlyrics.Provider) (duxlyrics.Request, duxlyrics.Provider) {
	// The album is left out: tags often name a single or a compilation, and
	// providers then return a different cut's lyrics or none.
	req := duxlyrics.Request{
		Title: t.Title, Artist: t.Artist,
		Duration:  time.Duration(t.Duration * float64(time.Second)),
		Providers: slices.DeleteFunc(slices.Clone(order), func(p duxlyrics.Provider) bool { return p == ServerSource }),
		CacheOnly: !online(),
	}
	src, err := s.cfg.Libraries.Get(t.Library)
	if err != nil {
		return req, ""
	}
	if _, remote := src.(source.Pinger); !remote {
		// Only local sources: a server's stream URL carries its token.
		if path, err := src.StreamURL(ctx, t.ID); err == nil && filepath.IsAbs(path) {
			req.FileURL = (&url.URL{Scheme: "file", Path: path}).String()
		}
	}
	if _, ok := src.(ServerLyrics); ok && slices.Contains(order, ServerSource) {
		return req, duxlyrics.Provider(serverPrefix + t.Library + ":" + t.ID)
	}
	return req, ""
}

// Best asks every enabled source on its own, so a fast one can't cut a slow
// one off, and each answer lands in the cache. It waits for the slowest, so
// it runs beside what is on screen. Nil when no source could answer.
func (s *Service) Best(ctx context.Context, t source.Track) *duxlyrics.Result {
	return s.best(ctx, s.client, t)
}

func (s *Service) best(ctx context.Context, client *duxlyrics.Client, t source.Track) *duxlyrics.Result {
	results, loose := s.gather(ctx, client, t)
	if len(results)+len(loose) == 0 {
		return nil
	}
	return decide(s.providers.enabled(), results, loose, s.recording(ctx, t))
}

// trackFits measures t's answers against its local copy, decoded at most
// once, and keeps every measure in the fit cache.
type trackFits struct {
	s     *Service
	t     source.Track
	audio func() ([]float64, bool)
}

// recording is nil when fitting is off or the audio can't be read.
func (s *Service) recording(ctx context.Context, t source.Track) recording {
	if !s.fitting() || s.cfg.AudioFile == nil || s.cfg.Decode == nil {
		return nil
	}
	return trackFits{s: s, t: t, audio: sync.OnceValues(func() ([]float64, bool) { return s.audioAttacks(ctx, t) })}
}

func (f trackFits) measure(r *duxlyrics.Result, words bool) (fitResult, bool) {
	name, starts := fitName(f.t, r, words, "")
	return f.fitted(r.Source, name, starts, words)
}

func (f trackFits) measureLaid(words, laid *duxlyrics.Result) (fitResult, bool) {
	name, starts := fitName(f.t, laid, true, words.Source)
	return f.fitted(words.Source, name, starts, true)
}

// fitted is the cached or newly measured fit of starts from src. A lyrics
// file beside the track is the user's own and never measured, and too few
// starts fit anywhere.
func (f trackFits) fitted(src duxlyrics.Provider, name string, starts []float64, words bool) (fitResult, bool) {
	least := fitMinLines
	if words {
		least = fitMinWords
	}
	if src == duxlyrics.Sidecar || len(starts) < least {
		return fitResult{}, false
	}
	if known, ok := f.s.knownFit(name); ok {
		return known, true
	}
	found, ok := f.audio()
	if !ok {
		return fitResult{}, false
	}
	var known fitResult
	if words {
		shift, confidence := fit(starts, found, fitReach)
		known = fitResult{Shift: shift, Confident: confidence >= fitConfidence}
	} else {
		known.Shift, known.Confident = fitLines(starts, found)
	}
	log.Debugf("lyrics: %q from %s fits the audio at %+.2f s (words %v): %v", f.t.Title, src, known.Shift, words, known.Confident)
	f.s.storeFit(name, known)
	return known, true
}

// fitName names a fit in the cache and returns the starts it fits. The
// timing is part of the name: a source that changes it is fitted anew. Words
// laid on r's lines from laidFrom are named apart from both sources' own fits,
// with all their starts, as a change to either source can move any of them.
func fitName(t source.Track, r *duxlyrics.Result, words bool, laidFrom duxlyrics.Provider) (string, []float64) {
	starts, kind := lineStarts(r), "lines/"
	if words {
		starts, kind = wordStarts(r), ""
	}
	if len(starts) == 0 {
		return "", nil
	}
	name := fmt.Sprintf("%s%s/%s/%d/%.2f", kind, key(&t), r.Source, len(starts), starts[0])
	if laidFrom != "" {
		name = fmt.Sprintf("laid/%s/%s/%.12s", laidFrom, name, hashed(fmt.Sprintf("%.2f", starts)))
	}
	return name, starts
}

func (s *Service) fitPath(name string) string {
	return filepath.Join(s.cfg.CacheDir, "lyrics-fit", hashed(name)+".json")
}

func (s *Service) knownFit(name string) (fitResult, bool) {
	var known fitResult
	data, err := os.ReadFile(s.fitPath(name))
	if err != nil || json.Unmarshal(data, &known) != nil {
		return fitResult{}, false
	}
	return known, true
}

func (s *Service) storeFit(name string, known fitResult) {
	path := s.fitPath(name)
	data, err := json.Marshal(known)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o700)
	}
	if err == nil {
		err = atomicfile.WriteFile(path, data, 0o600)
	}
	if err != nil {
		log.Warnf("lyrics: %v", err)
	}
}

// audioAttacks decodes the track's local copy, when there is one.
func (s *Service) audioAttacks(ctx context.Context, t source.Track) ([]float64, bool) {
	file, ok := s.cfg.AudioFile(ctx, t.Ref())
	if !ok {
		return nil, false
	}
	var found []float64
	err := s.cfg.Decode(ctx, file, func(in io.Reader) error {
		var err error
		found, err = attacks(in)
		return err
	})
	if err != nil {
		log.Debugf("lyrics: fitting %q to its audio: %v", t.Title, err)
		return nil, false
	}
	return found, true
}

// keep links the cache entries written since start into the track's folder
// in keptDir, so refreshKept can protect them from the 90-day prune of the
// lyrics cache. Entries that were already cached before start are not captured.
func (s *Service) keep(ref source.Ref, start time.Time) {
	root := s.cacheDir()
	dir := filepath.Join(s.keptDir(), hashed(ref.Library+":"+ref.ID))
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		if info, err := d.Info(); err != nil || info.ModTime().Before(start) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		to := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err == nil {
			_ = os.Remove(to)
			_ = os.Link(path, to)
		}
		return nil
	})
	if _, err := os.Stat(dir); err == nil {
		data, _ := json.Marshal(ref)
		_ = os.WriteFile(filepath.Join(dir, keptRef), data, 0o600)
	}
}

// keptRef names the file in a kept folder that says which track it belongs to.
const keptRef = "track"

// unkeep drops what keep linked for a track.
func (s *Service) unkeep(ref source.Ref) {
	_ = os.RemoveAll(filepath.Join(s.keptDir(), hashed(ref.Library+":"+ref.ID)))
}

// refreshKept touches the cache entry of every kept track (restoring it from
// keptDir when missing) so the prune, which goes by modification time, spares it.
// Folders of tracks no longer on this device, and all of them while lyrics or
// keeping them is off, are removed.
func (s *Service) refreshKept() {
	now := time.Now()
	folders, _ := os.ReadDir(s.keptDir())
	for _, f := range folders {
		dir := filepath.Join(s.keptDir(), f.Name())
		var ref source.Ref
		data, err := os.ReadFile(filepath.Join(dir, keptRef))
		if err == nil {
			err = json.Unmarshal(data, &ref)
		}
		if err != nil || !s.enabled() || s.cfg.Keep != nil && !s.cfg.Keep() {
			_ = os.RemoveAll(dir)
			continue
		}
		if s.cfg.HasCopy != nil {
			if has, known := s.cfg.HasCopy(s.baseCtx, ref); known && !has {
				_ = os.RemoveAll(dir)
				continue
			}
		}
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Name() == keptRef {
				return nil
			}
			rel, _ := filepath.Rel(dir, path)
			to := filepath.Join(s.cacheDir(), rel)
			if _, err := os.Stat(to); err != nil {
				if os.MkdirAll(filepath.Dir(to), 0o755) == nil {
					_ = os.Link(path, to)
				}
			}
			_ = os.Chtimes(to, now, now)
			_ = os.Chtimes(path, now, now)
			return nil
		})
	}
}

// Stored follows a track kept on this device: its lyrics are fetched to keep,
// and the track on screen can now be fitted. Others are fitted when they play,
// so a large download doesn't decode every track.
func (s *Service) Stored(ctx context.Context, t source.Track, keepLyrics bool) {
	if keepLyrics {
		defer s.keep(t.Ref(), time.Now())
	} else {
		s.unkeep(t.Ref())
	}
	s.mu.Lock()
	current := key(s.track) == key(&t)
	shown := s.last.Result
	s.mu.Unlock()
	if !s.enabled() || !keepLyrics && !current || s.chosen(key(&t)) != nil {
		return
	}
	if !current {
		s.gather(ctx, s.client, t)
		return
	}
	if !keepLyrics && !s.cfg.HasSubscriber(Topic) {
		return
	}
	best := s.Best(ctx, t)
	if best == nil || !s.cfg.HasSubscriber(Topic) {
		return
	}
	s.remember(warmed{key: key(&t), res: best})
	if shown == nil || retimed(shown, best) {
		s.show(ctx, &t, Event{Library: t.Library, ID: t.ID, State: "ready", Result: best})
	}
}

// gather asks every source on its own. Loose answers, found only without the
// track's length, come apart from the others.
func (s *Service) gather(ctx context.Context, client *duxlyrics.Client, t source.Track) (results, loose []*duxlyrics.Result) {
	order := s.providers.enabled()
	req, serverID := s.request(ctx, t, order)
	ids := slices.Clone(req.Providers)
	if serverID != "" {
		ids = append(ids, serverID)
	}
	results = make([]*duxlyrics.Result, len(ids))
	loose = make([]*duxlyrics.Result, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			one := req
			one.Providers = []duxlyrics.Provider{id}
			if id == serverID {
				one.FileURL = ""
			}
			var res *duxlyrics.Result
			var err error
			if id == serverID {
				res, err = client.Lookup(ctx, one)
			} else {
				res, loose[i], err = find(ctx, client, one, t.Album, true)
			}
			if err != nil {
				log.Debugf("lyrics for %q from %s: %v", t.Title, id, err)
				return
			}
			if id == serverID && res.Found {
				res.Source = ServerSource
			}
			results[i] = res
		}()
	}
	wg.Wait()
	isNil := func(r *duxlyrics.Result) bool { return r == nil }
	return slices.DeleteFunc(results, isNil), slices.DeleteFunc(loose, isNil)
}

// pick keeps the better synced of the server's and the providers' answers;
// between equals the one earlier in order wins.
func pick(order []duxlyrics.Provider, srv, prov answer) (*duxlyrics.Result, error) {
	ts, tp := tier(srv), tier(prov)
	switch {
	case ts > tp:
		return srv.res, nil
	case tp > ts:
		return prov.res, nil
	case ts >= 0 && slices.Index(order, ServerSource) < rank(order, prov.res.Source):
		return srv.res, nil
	case prov.err != nil && srv.err == nil:
		return srv.res, nil
	}
	return prov.res, prov.err
}

// resolve turns "server:<library>:<track>" into a Source asking that server;
// the track id rides in the provider id because a Fetcher only sees artist
// and title.
// Dux-go keeps a rate bucket per provider id, so each server track
// looked up leaks a small entry; an opaque-id Request field upstream fixes it.
func (s *Service) resolve(id duxlyrics.Provider) (duxlyrics.Source, bool) {
	rest, ok := strings.CutPrefix(string(id), serverPrefix)
	if !ok {
		return duxlyrics.Source{}, false
	}
	lib, track, ok := strings.Cut(rest, ":")
	if !ok {
		return duxlyrics.Source{}, false
	}
	src, err := s.cfg.Libraries.Get(lib)
	if err != nil {
		return duxlyrics.Source{}, false
	}
	sl, ok := src.(ServerLyrics)
	if !ok {
		return duxlyrics.Source{}, false
	}
	return duxlyrics.Source{
		Attribution: duxlyrics.Attribution{Name: src.Library().Name},
		WordSync:    false,
		Fetch: func(ctx context.Context, _ duxlyrics.Request) (*duxlyrics.Lyrics, error) {
			l, err := sl.Lyrics(ctx, track)
			if err == nil && l == nil {
				l = &duxlyrics.Lyrics{}
			}
			return l, err
		},
	}, true
}

// online is a cheap guess: some interface is up with a routable address.
// Offline, lookups use the cache only instead of waiting out timeouts. A
// variable so tests don't depend on the machine's network.
var online = func() bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return true
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.IsGlobalUnicast() {
				return true
			}
		}
	}
	return false
}

var errUnknownProvider = errors.New("unknown lyrics provider")
