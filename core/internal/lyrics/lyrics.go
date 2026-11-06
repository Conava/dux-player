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
	"io/fs"
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
	next       *source.Track
	warm       []warmed // newest last
	warming    string   // key of the track being prefetched
	warmCancel context.CancelFunc
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
	s := &Service{cfg: cfg, baseCtx: ctx, providers: openProviders(filepath.Join(cfg.ConfigDir, providersFile))}
	s.client = s.newClient(s.cacheDir())
	s.wasOn = s.enabled()
	go s.client.Prune()
	if err := os.RemoveAll(s.aheadDir()); err != nil {
		log.Warnf("lyrics: %v", err)
	}
	return s
}

func (s *Service) newClient(dir string) *duxlyrics.Client {
	return duxlyrics.New(duxlyrics.Options{
		CacheDir:  dir,
		UserAgent: "dplayer/" + s.cfg.Version + " (+https://github.com/Conava/dux-player)",
		Resolve:   s.resolve,
	})
}

func (s *Service) cacheDir() string { return filepath.Join(s.cfg.CacheDir, "lyrics") }
func (s *Service) aheadDir() string { return filepath.Join(s.cfg.CacheDir, "lyrics-next") }

func (s *Service) enabled() bool { return s.cfg.Enabled == nil || s.cfg.Enabled() }

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
	stale := s.shown != key(s.track) || s.last.State == "error"
	last := s.last
	s.mu.Unlock()
	if stale {
		s.refresh()
		return
	}
	s.cfg.Publish(Topic, last)
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
		s.last = Event{State: "idle"}
		s.mu.Unlock()
		s.cfg.Publish(Topic, s.last)
		return
	}
	ctx, cancel := context.WithCancel(s.baseCtx)
	s.cancel = cancel
	if res := s.chosen(key(t)); res != nil {
		ev := Event{Library: t.Library, ID: t.ID, State: "ready", Result: res, Chosen: true}
		s.last = ev
		s.mu.Unlock()
		cancel()
		s.cfg.Publish(Topic, ev)
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
		s.cfg.Publish(Topic, ev)
		s.adopt(dir)
		s.prefetch()
		return
	}
	s.last = Event{Library: t.Library, ID: t.ID, State: "loading"}
	s.mu.Unlock()
	s.cfg.Publish(Topic, s.last)

	go func() {
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
	}()
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
	s.cfg.Publish(Topic, ev)
}

func retimed(a, b *duxlyrics.Result) bool {
	if a.Source != b.Source || len(a.Synced) != len(b.Synced) {
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
	go func() {
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
	}()
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
	on := s.enabled()
	s.mu.Lock()
	changed := s.wasOn != on
	s.wasOn = on
	s.mu.Unlock()
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

// Choices lists what every enabled source has for a track.
func (s *Service) Choices(ctx context.Context, ref source.Ref) ([]Choice, error) {
	t, err := s.trackOf(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := []Choice{}
	for _, r := range s.gather(ctx, s.client, t) {
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
	results := s.gather(ctx, s.client, t)
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
	var server chan answer
	if serverID != "" {
		sreq := req
		sreq.Providers = []duxlyrics.Provider{serverID}
		server = make(chan answer, 1)
		go func() {
			res, err := s.client.Lookup(ctx, sreq)
			server <- answer{res, err}
		}()
	}
	providers := make(chan answer, 1)
	go func() {
		res, err := s.client.Lookup(ctx, req)
		providers <- answer{res, err}
	}()
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
	req := duxlyrics.Request{
		Title: t.Title, Artist: t.Artist, Album: t.Album,
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
	results := s.gather(ctx, client, t)
	if len(results) == 0 {
		return nil
	}
	return align(s.providers.enabled(), results)
}

func (s *Service) gather(ctx context.Context, client *duxlyrics.Client, t source.Track) []*duxlyrics.Result {
	order := s.providers.enabled()
	req, serverID := s.request(ctx, t, order)
	ids := slices.Clone(req.Providers)
	if serverID != "" {
		ids = append(ids, serverID)
	}
	results := make([]*duxlyrics.Result, len(ids))
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
			res, err := client.Lookup(ctx, one)
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
	return slices.DeleteFunc(results, func(r *duxlyrics.Result) bool { return r == nil })
}

type answer struct {
	res *duxlyrics.Result
	err error
}

// tier ranks a result as dux-go does: word sync 2, line sync (or
// instrumental) 1, plain 0, nothing -1.
func tier(a answer) int {
	switch {
	case a.err != nil || a.res == nil || !a.res.Found:
		return -1
	case slices.ContainsFunc(a.res.Synced, func(l duxlyrics.Line) bool { return len(l.Words) > 0 }):
		return 2
	case len(a.res.Synced) > 0 || a.res.Instrumental:
		return 1
	}
	return 0
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

// rank is p's place in order; a sidecar file comes before everything.
func rank(order []duxlyrics.Provider, p duxlyrics.Provider) int {
	if p == duxlyrics.Sidecar {
		return -1
	}
	if i := slices.Index(order, p); i >= 0 {
		return i
	}
	return len(order)
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
// Offline, lookups use the cache only instead of waiting out timeouts.
func online() bool {
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
