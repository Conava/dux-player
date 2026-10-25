// Package lyrics finds lyrics for the playing track through dux-go/lyrics:
// sidecar files next to local tracks, the track's own server when it serves
// lyrics, then the configured providers.
package lyrics

import (
	"context"
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
	duxlyrics "github.com/duxlabs-dev/dux-go/lyrics"

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
}

type Config struct {
	CacheDir, ConfigDir, Version string
	Libraries                    Libraries
	Publish                      func(topic string, data any)
	HasSubscriber                func(topic string) bool
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
}

func New(ctx context.Context, cfg Config) *Service {
	s := &Service{cfg: cfg, baseCtx: ctx, providers: openProviders(filepath.Join(cfg.ConfigDir, providersFile))}
	s.client = duxlyrics.New(duxlyrics.Options{
		CacheDir:  filepath.Join(cfg.CacheDir, "lyrics"),
		UserAgent: "dplayer/" + cfg.Version + " (+https://github.com/Conava/dux-player)",
		Resolve:   s.resolve,
	})
	go s.client.Prune()
	return s
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
}

func (s *Service) refresh() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	t := s.track
	s.shown = key(t)
	if t == nil {
		s.last = Event{State: "idle"}
		s.mu.Unlock()
		s.cfg.Publish(Topic, s.last)
		return
	}
	ctx, cancel := context.WithCancel(s.baseCtx)
	s.cancel = cancel
	s.last = Event{Library: t.Library, ID: t.ID, State: "loading"}
	s.mu.Unlock()
	s.cfg.Publish(Topic, s.last)

	go func() {
		defer cancel()
		res, err := s.Lookup(ctx, *t)
		ev := Event{Library: t.Library, ID: t.ID, State: "ready", Result: res}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Infof("lyrics for %q: %v", t.Title, err)
			ev = Event{Library: t.Library, ID: t.ID, State: "error", Error: err.Error()}
		}
		s.mu.Lock()
		if s.shown != key(t) || ctx.Err() != nil {
			s.mu.Unlock()
			return
		}
		s.last = ev
		s.mu.Unlock()
		s.cfg.Publish(Topic, ev)
	}()
}

// Get looks up the lyrics of any track.
func (s *Service) Get(ctx context.Context, ref source.Ref) (*duxlyrics.Result, error) {
	tracks, err := s.cfg.Libraries.Tracks(ctx, []source.Ref{ref})
	if err != nil {
		return nil, err
	}
	if len(tracks) == 0 {
		return nil, source.ErrNotFound
	}
	return s.Lookup(ctx, tracks[0])
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
	req := duxlyrics.Request{
		Title: t.Title, Artist: t.Artist, Album: t.Album,
		Duration:  time.Duration(t.Duration * float64(time.Second)),
		Providers: slices.DeleteFunc(slices.Clone(order), func(p duxlyrics.Provider) bool { return p == ServerSource }),
		CacheOnly: !online(),
	}
	var server chan answer
	src, err := s.cfg.Libraries.Get(t.Library)
	if err == nil {
		if _, ok := src.(ServerLyrics); ok && slices.Contains(order, ServerSource) {
			sreq := req
			sreq.Providers = []duxlyrics.Provider{duxlyrics.Provider(serverPrefix + t.Library + ":" + t.ID)}
			server = make(chan answer, 1)
			go func() {
				res, err := s.client.Lookup(ctx, sreq)
				server <- answer{res, err}
			}()
		}
		if _, remote := src.(source.Pinger); !remote {
			// Only local sources: a server's stream URL carries its token.
			if path, err := src.StreamURL(ctx, t.ID); err == nil && filepath.IsAbs(path) {
				req.FileURL = (&url.URL{Scheme: "file", Path: path}).String()
			}
		}
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
