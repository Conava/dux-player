// Package scrobble reports plays to a server track's own server and to the
// ListenBrainz and Last.fm accounts set up here. Each library turns either
// destination off on its own, so a server that forwards plays to the same
// accounts doesn't count them twice.
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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/atomicfile"
	"github.com/Conava/dux-player/core/internal/lastfm"
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
	// Counted, when set, is told of each play a library's server took.
	Counted func(ctx context.Context, library, id string, at time.Time)
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
	ListenBrainz        Account  `json:"listenbrainz"`
	LastFM              Account  `json:"lastfm"`
	NoServerLibraries   []string `json:"noServerLibraries"`
	NoAccountsLibraries []string `json:"noAccountsLibraries"`
}

type Status struct {
	Settings
	Queued int `json:"queued"`
	// LastFMBuiltIn: the app carries its own Last.fm API account, so signing in needs no key.
	LastFMBuiltIn bool `json:"lastfmBuiltIn"`
}

type Service struct {
	cfg   Config
	lb    listenBrainz
	fm    lastFM
	queue *queue

	mu       sync.Mutex
	settings Settings
	// raw is the settings file as read, so a save keeps keys this version doesn't know.
	raw     map[string]json.RawMessage
	fmToken string // a Last.fm token waiting for the user's approval
	// fmBuiltIn: fmToken came from the built-in account, not a key in the keyring.
	fmBuiltIn bool

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
	s.raw = map[string]json.RawMessage{}
	if data, err := os.ReadFile(filepath.Join(cfg.ConfigDir, settingsFile)); err == nil {
		if s.settings, s.raw, err = loadSettings(data); err != nil {
			s.settings, s.raw = Settings{}, map[string]json.RawMessage{}
			setAside(filepath.Join(cfg.ConfigDir, settingsFile))
		}
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
	st := Status{Settings: s.settings, LastFMBuiltIn: lastfm.APIKey != "" && lastfm.APISecret != ""}
	st.NoServerLibraries = slices.Clone(s.settings.NoServerLibraries)
	st.NoAccountsLibraries = slices.Clone(s.settings.NoAccountsLibraries)
	st.LastFM.AwaitingApproval = s.fmToken != ""
	s.mu.Unlock()
	if st.NoServerLibraries == nil {
		st.NoServerLibraries = []string{}
	}
	if st.NoAccountsLibraries == nil {
		st.NoAccountsLibraries = []string{}
	}
	st.Queued = s.queue.len()
	return st
}

func (s *Service) publish() { s.cfg.Publish(Topic, s.Status()) }

// update changes the settings under the lock, saves them and publishes.
func (s *Service) update(fn func(*Settings)) error {
	s.mu.Lock()
	next := s.settings
	next.NoServerLibraries = slices.Clone(s.settings.NoServerLibraries)
	next.NoAccountsLibraries = slices.Clone(s.settings.NoAccountsLibraries)
	fn(&next)
	raw, err := mergeSettings(s.raw, next)
	var data []byte
	if err == nil {
		data, err = json.MarshalIndent(raw, "", "  ")
	}
	if err == nil {
		err = atomicfile.WriteFile(filepath.Join(s.cfg.ConfigDir, settingsFile), append(data, '\n'), 0o600)
	}
	if err == nil {
		s.settings, s.raw = next, raw
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.publish()
	return nil
}

// loadSettings reads the settings file. Before the server and the accounts
// were separate switches, disabledLibraries reported nowhere and
// serverOnlyLibraries only to the server.
func loadSettings(data []byte) (Settings, map[string]json.RawMessage, error) {
	var st Settings
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return st, nil, err
	}
	if raw == nil {
		raw = map[string]json.RawMessage{}
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, nil, err
	}
	var off, serverOnly []string
	for key, list := range map[string]*[]string{"disabledLibraries": &off, "serverOnlyLibraries": &serverOnly} {
		if v, ok := raw[key]; ok {
			if err := json.Unmarshal(v, list); err != nil {
				return st, nil, fmt.Errorf("%s: %w", key, err)
			}
			delete(raw, key)
		}
	}
	for _, id := range off {
		st.NoServerLibraries = switchOff(st.NoServerLibraries, id, true)
	}
	for _, id := range append(off, serverOnly...) {
		st.NoAccountsLibraries = switchOff(st.NoAccountsLibraries, id, true)
	}
	return st, raw, nil
}

// mergeSettings lays the known settings over the file as read.
func mergeSettings(old map[string]json.RawMessage, st Settings) (map[string]json.RawMessage, error) {
	known, err := json.Marshal(st)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(known, &fields); err != nil {
		return nil, err
	}
	raw := make(map[string]json.RawMessage, len(old)+len(fields))
	for k, v := range old {
		raw[k] = v
	}
	for k, v := range fields {
		raw[k] = v
	}
	return raw, nil
}

// switchOff puts id in the list of libraries with a destination off, or takes it out.
func switchOff(list []string, id string, off bool) []string {
	list = slices.DeleteFunc(list, func(x string) bool { return x == id })
	if off {
		list = append(list, id)
	}
	return list
}

func listenOf(t source.Track, at time.Time) Listen {
	l := Listen{Library: t.Library, ID: t.ID, Artist: t.Artist, Title: t.Title, Album: t.Album, Duration: t.Duration, PlayedAt: at.UTC()}
	var names []string
	for _, a := range t.Artists {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	if len(names) > 0 {
		l.Artist, l.Credit = names[0], strings.Join(names, ", ")
	}
	return l
}

// targets says where a track's plays go: its server when it scrobbles, and
// the enabled accounts, each unless the library turned it off.
func (s *Service) targets(src source.Source, library string) []string {
	var out []string
	scrobbles := slices.Contains(source.Capabilities(src), source.CapScrobble)
	s.mu.Lock()
	defer s.mu.Unlock()
	if scrobbles && !slices.Contains(s.settings.NoServerLibraries, library) {
		out = append(out, targetLibrary)
	}
	if slices.Contains(s.settings.NoAccountsLibraries, library) {
		return out
	}
	if a := s.settings.ListenBrainz; a.Connected && a.Enabled {
		out = append(out, ListenBrainz)
	}
	if a := s.settings.LastFM; a.Connected && a.Enabled {
		out = append(out, LastFM)
	}
	return out
}

// silent: the library has both destinations off, so its plays aren't even queued.
func (s *Service) silent(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Contains(s.settings.NoServerLibraries, id) && slices.Contains(s.settings.NoAccountsLibraries, id)
}

// switchedOff: the library turned this destination off since the play was queued.
func (s *Service) switchedOff(library, target string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch target {
	case targetLibrary:
		return slices.Contains(s.settings.NoServerLibraries, library)
	case ListenBrainz, LastFM:
		return slices.Contains(s.settings.NoAccountsLibraries, library)
	}
	return false
}

func (s *Service) report(t source.Track, nowPlaying bool, at time.Time) {
	if s.silent(t.Library) || t.Title == "" {
		return
	}
	li := listenOf(t, at)
	src, err := s.cfg.Libraries.Get(t.Library)
	if err != nil {
		// The library is down (server, unmounted drive): decide later.
		if !nowPlaying {
			s.queue.add(pending{Target: targetUnknown, Listen: li})
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
	for _, target := range s.targets(src, li.Library) {
		err := s.send(s.ctx, src, target, li, nowPlaying)
		switch {
		case err == nil:
			sent = true
			if !nowPlaying {
				log.Infof("scrobbled %q to %s", li.Title, target)
			}
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
		err := sc.Scrobble(ctx, li.ID, li.PlayedAt)
		if err == nil && s.cfg.Counted != nil {
			s.cfg.Counted(ctx, li.Library, li.ID, li.PlayedAt)
		}
		return err
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

// apiKeys is the user's own Last.fm API account from the keyring, else the built-in one.
func (s *Service) apiKeys() (lastFMKeys, error) {
	key, kerr := s.cfg.Secrets.Get(secretsFM, "apiKey")
	secret, serr := s.cfg.Secrets.Get(secretsFM, "apiSecret")
	if kerr == nil && serr == nil && key != "" && secret != "" {
		return lastFMKeys{key: key, secret: secret}, nil
	}
	if lastfm.APIKey != "" && lastfm.APISecret != "" {
		return lastFMKeys{key: lastfm.APIKey, secret: lastfm.APISecret}, nil
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
		if s.switchedOff(p.Library, p.Target) {
			done = append(done, p)
			continue
		}
		src, err := s.cfg.Libraries.Get(p.Library)
		if err != nil {
			continue
		}
		if p.Target == targetUnknown || p.Target == targetLibrary && !slices.Contains(source.Capabilities(src), source.CapScrobble) {
			// Queued while its library was down, so where it goes is decided now.
			done = append(done, p)
			s.deliver(src, p.Listen, false)
			continue
		}
		err = s.send(s.ctx, src, p.Target, p.Listen, false)
		if err != nil && !permanent(err) {
			break
		}
		if err != nil {
			log.Warnf("queued scrobble to %s refused, dropped: %v", p.Target, err)
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
