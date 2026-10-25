package library

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	mathrand "math/rand/v2"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/keyring"
	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/shuffle"
	"github.com/Conava/dux-player/core/internal/source"
)

// Info is a library as the UI sees it: what it is, what it can do, and why
// it isn't working if it isn't. Available is false while its tracks can't
// play (Error says why), and for a library that is turned off.
type Info struct {
	source.Library
	Capabilities []string `json:"capabilities"`
	// Notices are the messages of NoticeCodes, for older UIs.
	Notices     []string        `json:"notices"`
	NoticeCodes []source.Notice `json:"noticeCodes"`
	Available   bool            `json:"available"`
	// Error is ErrorCode in plain English.
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
	// LastScan is the unix time of the last full scan of an indexed library.
	LastScan int64 `json:"lastScan,omitempty"`
	// Server is what the last successful ping learned about the server.
	Server *source.ServerInfo `json:"server,omitempty"`
}

// Reason says why a library or track can't play: a stable Code the UI
// words, and Message, plain English for logs and the CLI. Zero when it can.
type Reason struct{ Code, Message string }

// Error makes a Reason an error that IPC replies with its code.
func (r Reason) Error() string { return r.Message }

// Codes of Reason.
const (
	CodeServerUnreachable  = "serverUnreachable"
	CodeKeyringLocked      = "keyringLocked"
	CodeNoKeyring          = "noKeyring"
	CodeOpenFailed         = "openFailed"
	CodeFolderMissing      = "folderMissing"
	CodeFoldersMissing     = "foldersMissing"
	CodeTrackFolderMissing = "trackFolderMissing"
	CodeLibraryOff         = "libraryOff"
)

// Codes of a failed libraries.test (and serverUnreachable).
const (
	CodeSignInRefused        = "signInRefused"
	CodePasswordNeedsHTTPS   = "passwordNeedsHttps"
	CodeCertificateInvalid   = "certificateInvalid"
	CodeBadAddress           = "badAddress"
	CodeCredentialsInAddress = "credentialsInAddress"
	CodeRedirected           = "redirected"
	CodeTestFailed           = "testFailed"
	// CodeScanFailed is the code of a scanFailed event.
	CodeScanFailed = "scanFailed"
)

// testFailure words why a server test failed; the raw error goes to the log.
func testFailure(err error) Reason {
	var cert *tls.CertificateVerificationError
	var op *net.OpError
	var dns *net.DNSError
	switch {
	case errors.Is(err, source.ErrSignInRefused):
		return Reason{CodeSignInRefused, "The server refused the username or password"}
	case errors.Is(err, source.ErrPasswordNeedsHTTPS):
		return Reason{CodePasswordNeedsHTTPS, "Signing in with the password itself needs an https server address"}
	case errors.Is(err, source.ErrCredentialsInAddress):
		return Reason{CodeCredentialsInAddress, "Put the username and password in their own fields, not in the server address"}
	case errors.Is(err, source.ErrBadAddress):
		return Reason{CodeBadAddress, "That isn't an http(s) server address"}
	case errors.Is(err, source.ErrRedirected):
		return Reason{CodeRedirected, "The server redirected to another address; use that address as the server"}
	case errors.As(err, &cert):
		return Reason{CodeCertificateInvalid, "The server's certificate isn't trusted"}
	case errors.As(err, &op), errors.As(err, &dns), errors.Is(err, context.DeadlineExceeded), errors.Is(err, os.ErrDeadlineExceeded):
		return unreachable
	}
	return Reason{CodeTestFailed, "Couldn't connect to this server"}
}

var (
	TrackMissing = Reason{CodeTrackFolderMissing, "Folder not found. It was removed or its drive isn't mounted."}
	libraryOff   = Reason{CodeLibraryOff, "Library turned off or removed"}
	unreachable  = Reason{CodeServerUnreachable, "Server unreachable"}
)

// buildReason words a library that failed to build; the error itself goes
// to the log.
func buildReason(err error) Reason {
	switch {
	case errors.Is(err, keyring.ErrLocked):
		return Reason{CodeKeyringLocked, "The system keyring is locked"}
	case errors.Is(err, keyring.ErrUnavailable):
		return Reason{CodeNoKeyring, "No system keyring is running"}
	}
	return Reason{CodeOpenFailed, "Couldn't open the library"}
}

// Availability is implemented by sources that know on their own when they,
// or single tracks, can't play (a local library whose folders are gone).
// Both are called often and must not block.
type Availability interface {
	Unavailable() Reason
	TrackUnavailable(ctx context.Context, id string) Reason
}

// Offline holds copies of remote tracks on disk, so they play while their
// server is unreachable.
type Offline interface {
	// Cached returns the file of a cached track with its measured gain and
	// silence.
	Cached(ctx context.Context, ref source.Ref) (player.Stream, bool)
	// Buffered returns the prefetched copy of an upcoming track.
	Buffered(ctx context.Context, ref source.Ref) (player.Stream, bool)
	// ProbedGain is the gain Probe measured for a track that isn't cached.
	ProbedGain(ctx context.Context, ref source.Ref) (float64, bool)
	// Probe measures a track's gain from the start of its stream.
	Probe(ctx context.Context, ref source.Ref) (float64, error)
	// Snapshot is the track as it was when it was cached.
	Snapshot(ctx context.Context, ref source.Ref) (source.Track, bool)
	// Clear removes a library's copies.
	Clear(ctx context.Context, libID string) error
}

const (
	// A broken library is tried again on use at most this often.
	retryEvery  = 10 * time.Second
	pingTimeout = 15 * time.Second
	// Capabilities a server reveals during requests are published after
	// this quiet time.
	capsDelay = 2 * time.Second
)

var _ player.Availability = (*Manager)(nil)

// Manager builds a source for every configured library and routes track
// lookups to the right one.
type Manager struct {
	store    *Store
	registry source.Registry
	env      source.Env
	builtin  map[string]source.Source
	publish  atomic.Pointer[func(topic string, data any)]
	offline  atomic.Pointer[Offline]

	mu       sync.RWMutex
	sources  map[string]source.Source
	errs     map[string]error
	scanning map[string]bool
	// down holds the libraries whose server didn't answer the last ping.
	down map[string]bool
	// suspect holds servers being pinged after a track of theirs failed to
	// load; the queue passes over their tracks until the ping answers.
	suspect  map[string]bool
	tried    map[string]time.Time
	checking map[string]bool
	// reported is each library's status as last published; noticed marks
	// libraries whose skipped tracks were already announced.
	reported  map[string]string
	noticed   map[string]bool
	servers   map[string]source.ServerInfo
	capsTimer *time.Timer
	// starting holds the libraries whose first build still runs; it closes
	// when the build is done.
	starting map[string]chan struct{}
}

func NewManager(store *Store, registry source.Registry, env source.Env, builtin ...source.Source) *Manager {
	m := &Manager{
		store:    store,
		registry: registry,
		env:      env,
		builtin:  make(map[string]source.Source),
		sources:  make(map[string]source.Source),
		errs:     make(map[string]error),
		scanning: make(map[string]bool),
		down:     make(map[string]bool),
		suspect:  make(map[string]bool),
		tried:    make(map[string]time.Time),
		checking: make(map[string]bool),
		reported: make(map[string]string),
		noticed:  make(map[string]bool),
		servers:  make(map[string]source.ServerInfo),
		starting: make(map[string]chan struct{}),
	}
	for _, b := range builtin {
		m.builtin[b.Library().ID] = b
	}
	return m
}

func (m *Manager) Kinds() []string { return m.registry.Kinds() }

// Load builds every enabled library in the background, since a server
// library waits for the keyring. Until its build is done a library counts
// as available, and lookups in it wait for the build. One broken library
// does not stop the others; its error shows in List.
func (m *Manager) Load(ctx context.Context) error {
	libs, err := m.store.Libraries(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, lib := range libs {
		if !lib.Enabled {
			continue
		}
		done := make(chan struct{})
		m.starting[lib.ID] = done
		go func() {
			m.build(lib)
			m.mu.Lock()
			delete(m.starting, lib.ID)
			m.mu.Unlock()
			close(done)
			m.PublishChanges(context.Background())
		}()
	}
	return nil
}

// await waits for a library's first build, if it still runs.
func (m *Manager) await(ctx context.Context, id string) {
	m.mu.RLock()
	done := m.starting[id]
	m.mu.RUnlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (m *Manager) awaitAll(ctx context.Context) {
	m.mu.RLock()
	ids := make([]string, 0, len(m.starting))
	for id := range m.starting {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		m.await(ctx, id)
	}
}

// build (re)creates a library's source. Building may wait for the keyring,
// so it runs outside the lock.
func (m *Manager) build(lib source.Library) {
	var src source.Source
	var err error
	if lib.Enabled {
		src, err = m.registry.Build(lib, m.env)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.sources[lib.ID]; ok {
		_ = old.Close()
		delete(m.sources, lib.ID)
	}
	delete(m.errs, lib.ID)
	delete(m.down, lib.ID)
	m.tried[lib.ID] = time.Now()
	switch {
	case !lib.Enabled:
	case err != nil:
		log.Warnf("library %s (%s): %v", lib.Name, lib.Kind, err)
		m.errs[lib.ID] = err
	default:
		m.sources[lib.ID] = src
	}
}

func (m *Manager) List(ctx context.Context) ([]Info, error) {
	libs, err := m.store.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	scans, err := m.store.LastScans(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Info, 0, len(libs))
	for _, lib := range libs {
		info := Info{Library: lib, Capabilities: []string{}, Notices: []string{}, NoticeCodes: []source.Notice{}}
		if src, ok := m.sources[lib.ID]; ok {
			info.Capabilities = source.Capabilities(src)
			if n, ok := src.(source.Notices); ok {
				for _, x := range n.Notices() {
					info.Notices = append(info.Notices, x.Message)
					info.NoticeCodes = append(info.NoticeCodes, x)
				}
			}
		}
		r := m.reasonLocked(lib.ID)
		info.Error, info.ErrorCode = r.Message, r.Code
		info.Available = lib.Enabled && r.Code == ""
		info.LastScan = scans[lib.ID]
		if srv, ok := m.servers[lib.ID]; ok {
			info.Server = &srv
		}
		out = append(out, info)
	}
	return out, nil
}

// reasonLocked says why a configured library can't play; zero when it can.
func (m *Manager) reasonLocked(id string) Reason {
	if err := m.errs[id]; err != nil {
		return buildReason(err)
	}
	if m.down[id] {
		return unreachable
	}
	src, ok := m.sources[id]
	if !ok {
		return Reason{}
	}
	if a, ok := src.(Availability); ok {
		return a.Unavailable()
	}
	return Reason{}
}

// Add stores a new library and builds it. Secrets go to the secret store,
// never into the config.
func (m *Manager) Add(ctx context.Context, kind, name string, config map[string]any, secrets map[string]string) (source.Library, error) {
	if _, ok := m.registry[kind]; !ok {
		return source.Library{}, fmt.Errorf("unknown library kind %q (known: %s)", kind, strings.Join(m.Kinds(), ", "))
	}
	name = source.Clean(name)
	if name == "" {
		return source.Library{}, errors.New("a library needs a name")
	}
	lib := source.Library{ID: source.NewID(), Kind: kind, Name: name, Enabled: true, Config: config}
	for k, v := range secrets {
		if err := m.env.Secrets.Set(lib.ID, k, v); err != nil {
			return source.Library{}, fmt.Errorf("store %s: %w", k, err)
		}
	}
	probe, err := m.registry.Build(lib, m.env)
	if err != nil {
		_ = m.env.Secrets.Delete(lib.ID)
		return source.Library{}, err
	}
	_ = probe.Close()
	if err := m.store.AddLibrary(ctx, lib); err != nil {
		_ = m.env.Secrets.Delete(lib.ID)
		return source.Library{}, err
	}
	m.build(lib)
	return lib, nil
}

func (m *Manager) Remove(ctx context.Context, id string) error {
	m.await(ctx, id)
	if err := m.store.RemoveLibrary(ctx, id); err != nil {
		return err
	}
	m.mu.Lock()
	if src, ok := m.sources[id]; ok {
		_ = src.Close()
	}
	delete(m.sources, id)
	delete(m.errs, id)
	delete(m.down, id)
	m.mu.Unlock()
	if off := m.offline.Load(); off != nil {
		if err := (*off).Clear(ctx, id); err != nil {
			log.Warnf("clear the cache of library %s: %v", id, err)
		}
	}
	return m.env.Secrets.Delete(id)
}

// Get returns the working source of a library. Using a broken library is
// what retries it: one that failed to build is rebuilt now, one whose server
// was down is pinged in the background.
func (m *Manager) Get(id string) (source.Source, error) {
	if b, ok := m.builtin[id]; ok {
		return b, nil
	}
	m.await(context.Background(), id)
	m.watchCaps()
	m.mu.RLock()
	src, ok := m.sources[id]
	_, failed := m.errs[id]
	due := (failed || m.down[id]) && !m.checking[id] && time.Since(m.tried[id]) >= retryEvery
	m.mu.RUnlock()
	switch {
	case due && failed:
		m.retry(context.Background(), id)
	case due:
		go m.retry(context.Background(), id)
	}
	if ok {
		return src, nil
	}
	return m.get(id)
}

// get is Get without the retry, for the player's loop, which must not wait.
func (m *Manager) get(id string) (source.Source, error) {
	if b, ok := m.builtin[id]; ok {
		return b, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if src, ok := m.sources[id]; ok {
		return src, nil
	}
	if err := m.errs[id]; err != nil {
		return nil, fmt.Errorf("library %s is not working: %w", id, err)
	}
	return nil, fmt.Errorf("library %s: %w", id, source.ErrNotFound)
}

// Server says whether a library is a server (its source pings) without
// waiting on anything.
func (m *Manager) Server(id string) bool {
	src, err := m.get(id)
	_, ok := src.(source.Pinger)
	return err == nil && ok
}

// trackLookups bounds the lookups Tracks runs at once; a server answers
// each track with a request of its own.
const trackLookups = 8

// Tracks resolves refs to tracks, in order; the first failing ref's error
// wins. A cached remote track resolves from its cached copy when its
// library can't answer.
func (m *Manager) Tracks(ctx context.Context, refs []source.Ref) ([]source.Track, error) {
	out := make([]source.Track, len(refs))
	errs := make([]error, len(refs))
	sem := make(chan struct{}, trackLookups)
	var wg sync.WaitGroup
	for i, ref := range refs {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()
			out[i], errs[i] = m.track(ctx, ref)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (m *Manager) track(ctx context.Context, ref source.Ref) (source.Track, error) {
	src, err := m.Get(ref.Library)
	if err == nil {
		var t source.Track
		if t, err = src.Track(ctx, ref.ID); err == nil {
			return t, nil
		}
	}
	if off := m.offline.Load(); off != nil && !errors.Is(err, source.ErrNotFound) {
		if snap, ok := (*off).Snapshot(ctx, ref); ok {
			return snap, nil
		}
	}
	return source.Track{}, err
}

// Stream implements player.Resolver. A cached or prefetched copy wins over
// the server.
func (m *Manager) Stream(ctx context.Context, ref source.Ref) (player.Stream, error) {
	if off := m.offline.Load(); off != nil {
		if st, ok := (*off).Cached(ctx, ref); ok {
			return st, nil
		}
		if st, ok := (*off).Buffered(ctx, ref); ok {
			return st, nil
		}
	}
	m.await(ctx, ref.Library)
	src, err := m.get(ref.Library)
	if err != nil {
		return player.Stream{}, err
	}
	url, err := src.StreamURL(ctx, ref.ID)
	if err != nil {
		return player.Stream{}, err
	}
	m.mu.Lock()
	delete(m.noticed, ref.Library)
	m.mu.Unlock()
	st := player.Stream{URL: url}
	if l, ok := src.(source.Loudness); ok {
		if gain, ok := l.LoudnessGain(ctx, ref.ID); ok {
			st.GainDB = &gain
		}
		st.Silence = l.Silence(ctx, ref.ID)
	}
	if off := m.offline.Load(); off != nil && st.GainDB == nil {
		if gain, ok := (*off).ProbedGain(ctx, ref); ok {
			st.GainDB = &gain
		}
	}
	return st, nil
}

// Probe implements player.Prober for server tracks.
func (m *Manager) Probe(ctx context.Context, ref source.Ref, done func(gain float64)) {
	off := m.offline.Load()
	if off == nil {
		return
	}
	// A source that measures its own files does it better, from the whole
	// file.
	src, err := m.Get(ref.Library)
	if err != nil {
		return
	}
	if _, ok := src.(source.Loudness); ok {
		return
	}
	go func() {
		gain, err := (*off).Probe(ctx, ref)
		if err != nil {
			log.Debugf("loudness probe: %v", err)
			return
		}
		done(gain)
	}()
}

// Unavailable says why a track can't play right now, or "" when it can.
func (m *Manager) Unavailable(ctx context.Context, ref source.Ref) Reason {
	if _, ok := m.builtin[ref.Library]; ok {
		return Reason{}
	}
	m.mu.RLock()
	src, built := m.sources[ref.Library]
	reason := m.reasonLocked(ref.Library)
	_, failed := m.errs[ref.Library]
	_, starting := m.starting[ref.Library]
	m.mu.RUnlock()
	if !built && !failed && !starting {
		reason = libraryOff
	}
	if reason.Code == "" {
		if a, ok := src.(Availability); ok {
			reason = a.TrackUnavailable(ctx, ref.ID)
		}
	}
	if reason.Code == "" || m.cached(ctx, ref) {
		return Reason{}
	}
	return reason
}

func (m *Manager) cached(ctx context.Context, ref source.Ref) bool {
	off := m.offline.Load()
	if off == nil {
		return false
	}
	if _, ok := (*off).Cached(ctx, ref); ok {
		return true
	}
	_, ok := (*off).Buffered(ctx, ref)
	return ok
}

// Skip implements player.Availability: the queue passes over tracks that
// can't play, and the first one of each library says why.
func (m *Manager) Skip(ref source.Ref) bool {
	ctx := context.Background()
	reason := m.Unavailable(ctx, ref)
	if reason.Code == "" {
		m.mu.RLock()
		suspect := m.suspect[ref.Library]
		m.mu.RUnlock()
		// Quietly: the notice waits for the ping to confirm the server is down.
		return suspect && !m.cached(ctx, ref)
	}
	m.mu.Lock()
	first := !m.noticed[ref.Library]
	m.noticed[ref.Library] = true
	m.mu.Unlock()
	if first {
		name := ref.Library
		if lib, err := m.library(context.Background(), ref.Library); err == nil {
			name = lib.Name
		}
		m.emit(player.TopicError, map[string]string{"message": fmt.Sprintf("skipping tracks from %s: %s", name, reason.Message),
			"library": ref.Library, "reason": reason.Message, "code": reason.Code})
	}
	return true
}

// Failed implements player.Availability. A server whose track didn't load
// is pinged at once instead of at the next network change, so a server that
// went down has its tracks skipped instead of failing one by one.
func (m *Manager) Failed(ref source.Ref, recheck func()) {
	src, err := m.get(ref.Library)
	if _, server := src.(source.Pinger); err != nil || !server {
		return
	}
	m.mu.Lock()
	pending := m.suspect[ref.Library]
	m.suspect[ref.Library] = true
	m.mu.Unlock()
	if pending {
		return
	}
	go func() {
		m.retry(context.Background(), ref.Library)
		m.mu.Lock()
		delete(m.suspect, ref.Library)
		m.mu.Unlock()
		recheck()
	}()
}

const maxDelta = 1000

// SetPublish connects the manager to the event bus: library status changes
// and index changes from the folder watcher are published from then on.
func (m *Manager) SetPublish(publish func(topic string, data any)) {
	m.publish.Store(&publish)
	m.store.OnChange(func(libID string, d *Delta) {
		ev := map[string]any{"type": "indexChanged", "library": libID}
		// Past maxDelta ids the client reloads anyway.
		if d != nil && len(d.Added)+len(d.Changed)+len(d.Removed) <= maxDelta {
			ev["added"], ev["changed"], ev["removed"] = d.Added, d.Changed, d.Removed
		}
		m.emit(TopicLibraries, ev)
		m.PublishChanges(context.Background())
	})
}

// SetOffline makes cached copies of remote tracks playable.
func (m *Manager) SetOffline(o Offline) { m.offline.Store(&o) }

const TopicLibraries = "libraries"

func (m *Manager) emit(topic string, data any) {
	if p := m.publish.Load(); p != nil {
		(*p)(topic, data)
	}
}

// PublishChanges sends libraries {type: changed} when any library's status,
// capabilities or notices differ from what was last published.
func (m *Manager) PublishChanges(ctx context.Context) {
	infos, err := m.List(ctx)
	if err != nil {
		return
	}
	now := make(map[string]string, len(infos))
	for _, info := range infos {
		now[info.ID] = fmt.Sprint(info.Enabled, info.Available, info.Error, info.Capabilities, info.Notices)
	}
	m.mu.Lock()
	changed := len(now) != len(m.reported)
	for id, st := range now {
		if m.reported[id] != st {
			changed = true
			delete(m.noticed, id)
		}
	}
	m.reported = now
	m.mu.Unlock()
	if changed {
		m.emit(TopicLibraries, map[string]any{"type": "changed"})
	}
}

// watchCaps publishes capability changes a little after requests, since a
// server reveals what it supports while answering them.
func (m *Manager) watchCaps() {
	if m.publish.Load() == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.capsTimer == nil {
		m.capsTimer = time.AfterFunc(capsDelay, func() { m.PublishChanges(context.Background()) })
		return
	}
	m.capsTimer.Reset(capsDelay)
}

// Recheck retries every enabled library that failed to build and pings
// every server, then publishes what changed. It runs at startup and when
// the network changes.
func (m *Manager) Recheck(ctx context.Context) {
	libs, err := m.store.Libraries(ctx)
	if err != nil {
		return
	}
	var wg sync.WaitGroup
	for _, lib := range libs {
		if lib.Enabled {
			wg.Add(1)
			go func() {
				defer wg.Done()
				m.check(ctx, lib)
			}()
		}
	}
	wg.Wait()
	m.PublishChanges(ctx)
}

// Retry checks one library again now, as when the user opens it.
func (m *Manager) Retry(ctx context.Context, id string) (Info, error) {
	m.retry(ctx, id)
	infos, err := m.List(ctx)
	if err != nil {
		return Info{}, err
	}
	for _, info := range infos {
		if info.ID == id {
			return info, nil
		}
	}
	return Info{}, fmt.Errorf("library %s: %w", id, source.ErrNotFound)
}

func (m *Manager) retry(ctx context.Context, id string) {
	lib, err := m.library(ctx, id)
	if err != nil || !lib.Enabled {
		return
	}
	m.check(ctx, lib)
	m.PublishChanges(ctx)
}

// check rebuilds a library that failed to build and pings its server.
func (m *Manager) check(ctx context.Context, lib source.Library) {
	m.await(ctx, lib.ID)
	m.mu.Lock()
	if m.checking[lib.ID] {
		m.mu.Unlock()
		return
	}
	m.checking[lib.ID] = true
	_, failed := m.errs[lib.ID]
	src, built := m.sources[lib.ID]
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.checking, lib.ID)
		m.mu.Unlock()
	}()
	if failed || !built {
		m.build(lib)
		m.mu.RLock()
		src, built = m.sources[lib.ID]
		m.mu.RUnlock()
	}
	p, ok := src.(source.Pinger)
	if !built || !ok {
		return
	}
	pctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	srv, err := p.Ping(pctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tried[lib.ID] = time.Now()
	if m.sources[lib.ID] != src {
		return
	}
	if err != nil {
		log.Infof("library %s is unavailable: %v", lib.Name, err)
		m.down[lib.ID] = true
		return
	}
	delete(m.down, lib.ID)
	m.servers[lib.ID] = srv
}

// SetEnabled turns a library on or off. A library that is off has no
// source: no watcher, no requests, nothing cached for it.
func (m *Manager) SetEnabled(ctx context.Context, id string, enabled bool) error {
	m.await(ctx, id)
	if err := m.store.SetEnabled(ctx, id, enabled); err != nil {
		return err
	}
	lib, err := m.library(ctx, id)
	if err != nil {
		return err
	}
	m.build(lib)
	if enabled {
		m.check(ctx, lib)
	}
	m.PublishChanges(ctx)
	return nil
}

// Update renames a library and changes its config keys and secrets, then
// rebuilds it. A nil config value removes the key.
func (m *Manager) Update(ctx context.Context, id, name string, config map[string]any, secrets map[string]string) (source.Library, error) {
	m.await(ctx, id)
	lib, err := m.library(ctx, id)
	if err != nil {
		return lib, err
	}
	if name = source.Clean(name); name != "" {
		lib.Name = name
	}
	if lib.Config == nil {
		lib.Config = map[string]any{}
	}
	oldURL, _ := lib.Config["url"].(string)
	for k, v := range config {
		if v == nil {
			delete(lib.Config, k)
			continue
		}
		lib.Config[k] = v
	}
	// The stored password must not follow the library to another server.
	if newURL, _ := lib.Config["url"].(string); otherHost(oldURL, newURL) && secrets["password"] == "" {
		return lib, errors.New("a new server address needs the password again")
	}
	// Built with the new values first, so a bad edit changes nothing.
	probe, err := m.registry.Build(lib, source.Env{Secrets: overlay{m.env.Secrets, secrets}, CacheDir: m.env.CacheDir,
		Version: m.env.Version})
	if err != nil {
		return lib, err
	}
	_ = probe.Close()
	for k, v := range secrets {
		if err := m.env.Secrets.Set(id, k, v); err != nil {
			return lib, fmt.Errorf("store %s: %w", k, err)
		}
	}
	if err := m.store.UpdateLibrary(ctx, lib); err != nil {
		return lib, err
	}
	m.build(lib)
	m.check(ctx, lib)
	m.PublishChanges(ctx)
	return lib, nil
}

func otherHost(a, b string) bool {
	ua, erra := url.Parse(a)
	ub, errb := url.Parse(b)
	if erra != nil || errb != nil {
		return a != b
	}
	return !strings.EqualFold(ua.Hostname(), ub.Hostname()) || ua.Port() != ub.Port()
}

// Test builds a library that isn't stored and pings its server, for
// "Test connection" before adding. Nothing reaches the keyring or the index.
func (m *Manager) Test(ctx context.Context, kind string, config map[string]any, secrets map[string]string) (source.ServerInfo, error) {
	if _, ok := m.registry[kind]; !ok {
		return source.ServerInfo{}, fmt.Errorf("unknown library kind %q (known: %s)", kind, strings.Join(m.Kinds(), ", "))
	}
	dir, err := os.MkdirTemp("", "dplayer-test-")
	if err != nil {
		return source.ServerInfo{}, err
	}
	defer os.RemoveAll(dir)
	lib := source.Library{ID: "test-" + source.NewID(), Kind: kind, Name: "test", Enabled: true, Config: config}
	src, err := m.registry.Build(lib, source.Env{Secrets: overlay{nil, secrets}, CacheDir: dir, Version: m.env.Version})
	if err != nil {
		log.Infof("test %s server: %v", kind, err)
		return source.ServerInfo{}, testFailure(err)
	}
	defer src.Close()
	p, ok := src.(source.Pinger)
	if !ok {
		return source.ServerInfo{}, fmt.Errorf("a %s library has no server to test", kind)
	}
	pctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	info, err := p.Ping(pctx)
	if err != nil {
		log.Infof("test %s server: %v", kind, err)
		return info, testFailure(err)
	}
	return info, nil
}

// overlay serves the given secrets first and keeps writes in memory, so a
// trial build never touches the keyring.
type overlay struct {
	base source.Secrets
	vals map[string]string
}

func (o overlay) Get(lib, key string) (string, error) {
	if v, ok := o.vals[key]; ok {
		return v, nil
	}
	if o.base == nil {
		return "", fmt.Errorf("no %s given", key)
	}
	return o.base.Get(lib, key)
}

func (o overlay) Set(_, key, value string) error {
	if o.vals != nil {
		o.vals[key] = value
	}
	return nil
}

func (o overlay) Delete(string) error { return nil }

// SetConfig changes one key of a library's config without rebuilding it.
func (m *Manager) SetConfig(ctx context.Context, id, key string, value any) error {
	return m.store.SetConfig(ctx, id, key, value)
}

// Cached says whether a track has a copy in the offline cache, with the
// gain measured for it.
func (m *Manager) Cached(ctx context.Context, ref source.Ref) (bool, *float64) {
	off := m.offline.Load()
	if off == nil {
		return false, nil
	}
	st, ok := (*off).Cached(ctx, ref)
	return ok, st.GainDB
}

func (m *Manager) Library(ctx context.Context, id string) (source.Library, error) {
	return m.library(ctx, id)
}

func (m *Manager) library(ctx context.Context, id string) (source.Library, error) {
	libs, err := m.store.Libraries(ctx)
	if err != nil {
		return source.Library{}, err
	}
	for _, lib := range libs {
		if lib.ID == id {
			return lib, nil
		}
	}
	return source.Library{}, fmt.Errorf("library %s: %w", id, source.ErrNotFound)
}

// Meter measures loudness; it may be unavailable.
func (m *Manager) Meter() *loudness.Meter { return m.env.Meter }

// Store is the shared index, for features that look tracks up by path.
func (m *Manager) Store() *Store { return m.store }

// Working returns the working sources, ordered by library name.
func (m *Manager) Working(ctx context.Context) ([]source.Source, error) {
	m.awaitAll(ctx)
	libs, err := m.store.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []source.Source
	for _, lib := range libs {
		if src, ok := m.sources[lib.ID]; ok {
			out = append(out, src)
		}
	}
	return out, nil
}

// Find returns the library whose id or name (case-insensitive) is key.
func (m *Manager) Find(ctx context.Context, key string) (source.Source, error) {
	if src, err := m.Get(key); err == nil {
		return src, nil
	}
	srcs, err := m.Working(ctx)
	if err != nil {
		return nil, err
	}
	for _, src := range srcs {
		if strings.EqualFold(src.Library().Name, key) {
			return src, nil
		}
	}
	return nil, fmt.Errorf("no working library named %q", key)
}

// Favorites gathers favorites from every library that has them. A library
// that fails is skipped, so one offline server doesn't empty the list.
func (m *Manager) Favorites(ctx context.Context) (source.SearchResult, error) {
	srcs, err := m.Working(ctx)
	if err != nil {
		return source.SearchResult{}, err
	}
	res := source.SearchResult{Artists: []source.Artist{}, Albums: []source.Album{}, Tracks: []source.Track{}}
	for _, src := range srcs {
		f, ok := src.(source.Favorites)
		if !ok {
			continue
		}
		r, err := f.Favorites(ctx)
		if err != nil {
			log.Warnf("favorites of %s: %v", src.Library().Name, err)
			continue
		}
		res.Artists = append(res.Artists, r.Artists...)
		res.Albums = append(res.Albums, r.Albums...)
		res.Tracks = append(res.Tracks, r.Tracks...)
	}
	return res, nil
}

// Shuffle is what `dplayer shuffle` plays: either a fixed list (favorites,
// a playlist), shuffled whole, or a feed that draws over whole libraries as
// the queue runs.
type Shuffle struct {
	Tracks []source.Track
	Feed   player.Feed
	Origin player.Origin
}

// pools turns libraries into weighted pools for the shuffler. Counting a
// server can take a request, so this runs outside the player loop.
func pools(ctx context.Context, srcs []source.Source) []shuffle.Pool {
	var out []shuffle.Pool
	for _, src := range srcs {
		r, ok := src.(source.RandomTracks)
		if !ok {
			continue
		}
		n, err := r.TrackCount(ctx)
		if err != nil {
			log.Warnf("track count of %s: %v", src.Library().Name, err)
			continue
		}
		out = append(out, shuffle.Pool{Name: src.Library().Name, Total: n, Fetch: r.RandomTracks})
	}
	return out
}

func newShuffler(p []shuffle.Pool) *shuffle.Shuffler {
	return shuffle.New(p, mathrand.New(mathrand.NewPCG(mathrand.Uint64(), mathrand.Uint64())))
}

// ShuffleSource resolves `dplayer shuffle`: every library's tracks, all
// favorites, one library, or one server playlist by name.
func (m *Manager) ShuffleSource(ctx context.Context, from, name string) (Shuffle, error) {
	switch from {
	case "tracks":
		srcs, err := m.Working(ctx)
		if err != nil {
			return Shuffle{}, err
		}
		return Shuffle{Feed: newShuffler(pools(ctx, srcs)), Origin: player.Origin{Kind: "tracks", Name: "All tracks"}}, nil
	case "favorites":
		// Favorites are listed in full, so shuffling their union is already
		// uniform over all of them.
		f, err := m.Favorites(ctx)
		return Shuffle{Tracks: f.Tracks, Origin: player.Origin{Kind: "favorites", Name: "Favorites"}}, err
	case "library":
		src, err := m.Find(ctx, name)
		if err != nil {
			return Shuffle{}, err
		}
		if _, ok := src.(source.RandomTracks); !ok {
			return Shuffle{}, fmt.Errorf("library %s can't be shuffled as a whole", src.Library().Name)
		}
		lib := src.Library()
		return Shuffle{Feed: newShuffler(pools(ctx, []source.Source{src})),
			Origin: player.Origin{Kind: "library", Library: lib.ID, ID: lib.ID, Name: lib.Name}}, nil
	case "playlist":
		srcs, err := m.Working(ctx)
		if err != nil {
			return Shuffle{}, err
		}
		for _, src := range srcs {
			pls, ok := src.(source.Playlists)
			if !ok {
				continue
			}
			list, err := pls.Playlists(ctx)
			if err != nil {
				continue
			}
			for _, pl := range list {
				if strings.EqualFold(pl.Name, name) || pl.ID == name {
					full, err := pls.Playlist(ctx, pl.ID)
					return Shuffle{Tracks: full.Tracks, Origin: player.Origin{Kind: "playlist", Library: pl.Library, ID: pl.ID, Name: pl.Name}}, err
				}
			}
		}
		return Shuffle{}, fmt.Errorf("no playlist named %q", name)
	}
	return Shuffle{}, fmt.Errorf("shuffle what? tracks, favorites, library NAME or playlist NAME")
}

// ErrScanning refuses a scan of a library that is already scanning.
var ErrScanning = errors.New("already scanning")

// Scan rescans a library; a second scan of the same library is refused
// while one runs.
func (m *Manager) Scan(ctx context.Context, id string, progress func(done, total int)) (source.ScanStats, error) {
	src, err := m.Get(id)
	if err != nil {
		return source.ScanStats{}, err
	}
	sc, ok := src.(source.Scanner)
	if !ok {
		return source.ScanStats{}, fmt.Errorf("library %s can't be scanned", id)
	}
	m.mu.Lock()
	if m.scanning[id] {
		m.mu.Unlock()
		return source.ScanStats{}, fmt.Errorf("library %s: %w", id, ErrScanning)
	}
	m.scanning[id] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.scanning, id)
		m.mu.Unlock()
	}()
	stats, err := sc.Scan(ctx, progress)
	if err == nil {
		err = m.store.SetScanned(ctx, id)
	}
	return stats, err
}
