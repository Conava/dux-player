// Package scrobble reports plays. A server track goes only to its own server
// (which may forward it), so nothing counts twice; any other track goes to
// the ListenBrainz and Last.fm accounts set up here.
package scrobble

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/atomicfile"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

const (
	Topic        = "scrobble"
	settingsFile = "scrobble.json"
	queueFile    = "scrobble-queue.json"

	// Keyring entries live under these ids, apart from the libraries'.
	secretsLB = "scrobble-listenbrainz"
	secretsFM = "scrobble-lastfm"
)

type Libraries interface {
	Get(id string) (source.Source, error)
}

type Config struct {
	ConfigDir, StateDir, Version string
	Libraries                    Libraries
	Secrets                      source.Secrets
	Publish                      func(topic string, data any)
	// Endpoints, for tests; empty means the real services.
	HTTP                                      *http.Client
	ListenBrainzURL, LastFMURL, LastFMAuthURL string
}

// Account is one service on the Scrobbling page.
type Account struct {
	Connected bool   `json:"connected"`
	Enabled   bool   `json:"enabled"`
	User      string `json:"user,omitempty"`
	// AwaitingApproval: the Last.fm auth page is open, finishAuth comes next.
	AwaitingApproval bool `json:"awaitingApproval,omitempty"`
}

type Settings struct {
	ListenBrainz      Account  `json:"listenbrainz"`
	LastFM            Account  `json:"lastfm"`
	DisabledLibraries []string `json:"disabledLibraries"`
}

type Status struct {
	Settings
	Queued int `json:"queued"`
}

type Service struct {
	cfg   Config
	lb    listenBrainz
	fm    lastFM
	queue *queue

	mu       sync.Mutex
	settings Settings
	fmToken  string // a Last.fm token waiting for the user's approval

	// now is the track last announced as playing; nowPos its position.
	now      *source.Track
	nowPos   float64
	flushing atomic.Bool
	ctx      context.Context
}

func New(ctx context.Context, cfg Config) *Service {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: callTimeout, CheckRedirect: source.SameOrigin}
	}
	if cfg.ListenBrainzURL == "" {
		cfg.ListenBrainzURL = "https://api.listenbrainz.org"
	}
	if cfg.LastFMURL == "" {
		cfg.LastFMURL = "https://ws.audioscrobbler.com/2.0/"
	}
	if cfg.LastFMAuthURL == "" {
		cfg.LastFMAuthURL = "https://www.last.fm/api/auth/"
	}
	s := &Service{
		cfg:   cfg,
		ctx:   ctx,
		lb:    listenBrainz{base: cfg.ListenBrainzURL, version: cfg.Version, http: cfg.HTTP},
		fm:    lastFM{base: cfg.LastFMURL, authBase: cfg.LastFMAuthURL, http: cfg.HTTP},
		queue: openQueue(filepath.Join(cfg.StateDir, queueFile)),
	}
	if data, err := os.ReadFile(filepath.Join(cfg.ConfigDir, settingsFile)); err == nil && json.Unmarshal(data, &s.settings) != nil {
		setAside(filepath.Join(cfg.ConfigDir, settingsFile))
	}
	if s.queue.len() > 0 {
		go s.Flush()
	}
	return s
}

// Played reports a play that counts: the player's played event, the same
// rule the play history follows.
func (s *Service) Played(t source.Track, at time.Time) { go s.report(t, false, at) }

// PlayerStatus and Position announce "now playing" when a track starts
// playing, and again when repeat one starts it over. Both return at once.
func (s *Service) PlayerStatus(st player.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.Track == nil {
		s.now = nil
		return
	}
	s.nowPos = st.Position
	if st.State != player.StatePlaying || s.now != nil && s.now.Ref() == st.Track.Ref() {
		return
	}
	t := *st.Track
	s.now = &t
	go s.report(t, true, time.Time{})
}

func (s *Service) Position(pos float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.now != nil && pos < 3 && s.nowPos-pos > 15 {
		go s.report(*s.now, true, time.Time{})
	}
	s.nowPos = pos
}

func (s *Service) Status() Status {
	s.mu.Lock()
	st := Status{Settings: s.settings}
	st.DisabledLibraries = slices.Clone(s.settings.DisabledLibraries)
	st.LastFM.AwaitingApproval = s.fmToken != ""
	s.mu.Unlock()
	if st.DisabledLibraries == nil {
		st.DisabledLibraries = []string{}
	}
	st.Queued = s.queue.len()
	return st
}

func (s *Service) publish() { s.cfg.Publish(Topic, s.Status()) }

// update changes the settings under the lock, saves them and publishes.
func (s *Service) update(fn func(*Settings)) error {
	s.mu.Lock()
	next := s.settings
	next.DisabledLibraries = slices.Clone(s.settings.DisabledLibraries)
	fn(&next)
	data, err := json.MarshalIndent(next, "", "  ")
	if err == nil {
		err = atomicfile.WriteFile(filepath.Join(s.cfg.ConfigDir, settingsFile), append(data, '\n'), 0o600)
	}
	if err == nil {
		s.settings = next
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.publish()
	return nil
}

func listenOf(t source.Track, at time.Time) Listen {
	return Listen{Library: t.Library, ID: t.ID, Artist: t.Artist, Title: t.Title, Album: t.Album, Duration: t.Duration, PlayedAt: at.UTC()}
}

// targets says where a track's plays go: its server when it scrobbles,
// nowhere for a server that doesn't, else the enabled accounts.
func (s *Service) targets(src source.Source) []string {
	if slices.Contains(source.Capabilities(src), source.CapScrobble) {
		return []string{targetLibrary}
	}
	if _, server := src.(source.Pinger); server {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	if a := s.settings.ListenBrainz; a.Connected && a.Enabled {
		out = append(out, ListenBrainz)
	}
	if a := s.settings.LastFM; a.Connected && a.Enabled {
		out = append(out, LastFM)
	}
	return out
}

func (s *Service) libraryEnabled(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !slices.Contains(s.settings.DisabledLibraries, id)
}

func (s *Service) report(t source.Track, nowPlaying bool, at time.Time) {
	if !s.libraryEnabled(t.Library) || t.Title == "" {
		return
	}
	li := listenOf(t, at)
	src, err := s.cfg.Libraries.Get(t.Library)
	if err != nil {
		// The library is down (server, unmounted drive): decide later.
		if !nowPlaying {
			s.queue.add(pending{Target: targetLibrary, Listen: li})
			s.publish()
		}
		return
	}
	s.deliver(src, li, nowPlaying)
}

// deliver sends a play to every target of its library and queues what
// failed for a reason that may pass.
func (s *Service) deliver(src source.Source, li Listen, nowPlaying bool) {
	sent, queued := false, false
	for _, target := range s.targets(src) {
		err := s.send(s.ctx, src, target, li, nowPlaying)
		switch {
		case err == nil:
			sent = true
		case nowPlaying:
			log.Debugf("now playing to %s: %v", target, err)
		case permanent(err):
			log.Warnf("scrobble to %s refused: %v", target, err)
		default:
			log.Infof("scrobble to %s failed, queued: %v", target, err)
			s.queue.add(pending{Target: target, Listen: li})
			queued = true
		}
	}
	if queued {
		s.publish()
	}
	if sent && s.queue.len() > 0 {
		go s.Flush()
	}
}

func (s *Service) send(ctx context.Context, src source.Source, target string, li Listen, nowPlaying bool) error {
	switch target {
	case targetLibrary:
		sc, ok := src.(source.Scrobbler)
		if !ok {
			return permanentError{errors.New("library doesn't scrobble")}
		}
		if nowPlaying {
			return sc.NowPlaying(ctx, li.ID)
		}
		return sc.Scrobble(ctx, li.ID, li.PlayedAt)
	case ListenBrainz:
		token, err := s.cfg.Secrets.Get(secretsLB, "token")
		if err != nil {
			return err
		}
		return s.lb.submit(ctx, token, li, nowPlaying)
	case LastFM:
		k, err := s.fmKeys()
		if err != nil {
			return err
		}
		return s.fm.submit(ctx, k, li, nowPlaying)
	}
	return permanentError{fmt.Errorf("unknown target %q", target)}
}

func (s *Service) fmKeys() (lastFMKeys, error) {
	k, err := s.apiKeys()
	if err != nil {
		return k, err
	}
	k.session, err = s.cfg.Secrets.Get(secretsFM, "session")
	return k, err
}

// apiKeys is the user's Last.fm API account from the keyring.
func (s *Service) apiKeys() (lastFMKeys, error) {
	key, kerr := s.cfg.Secrets.Get(secretsFM, "apiKey")
	secret, serr := s.cfg.Secrets.Get(secretsFM, "apiSecret")
	if kerr == nil && serr == nil && key != "" && secret != "" {
		return lastFMKeys{key: key, secret: secret}, nil
	}
	return lastFMKeys{}, errors.Join(kerr, serr, errors.New("no Last.fm API key"))
}

// Flush retries the queued plays in order. It stops at the first failure
// that may pass (still offline) and drops plays that were refused.
func (s *Service) Flush() {
	if !s.flushing.CompareAndSwap(false, true) {
		return
	}
	defer s.flushing.Store(false)
	var done []pending
	sent := false
	for _, p := range s.queue.snapshot() {
		src, err := s.cfg.Libraries.Get(p.Library)
		if err != nil {
			continue
		}
		if p.Target == targetLibrary && !slices.Contains(source.Capabilities(src), source.CapScrobble) {
			// Queued while its library was down; it turned out not to be a
			// scrobbling server.
			done = append(done, p)
			s.deliver(src, p.Listen, false)
			continue
		}
		err = s.send(s.ctx, src, p.Target, p.Listen, false)
		if err != nil && !permanent(err) {
			break
		}
		// Saved as it goes, so a kill mid-flush resends at most a few.
		if done = append(done, p); len(done) >= 20 {
			s.queue.remove(done)
			done, sent = nil, true
		}
	}
	if len(done) > 0 || sent {
		s.queue.remove(done)
		s.publish()
	}
}
