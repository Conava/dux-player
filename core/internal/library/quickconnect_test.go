package library

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/keyring"
	"github.com/Conava/dux-player/core/internal/source"
)

// qcServer is a server source with Quick Connect whose answers the test
// scripts.
type qcServer struct {
	lib    source.Library
	env    source.Env
	script *qcScript
}

type qcScript struct {
	mu       sync.Mutex
	pingErr  error
	pings    int
	polls    []error // errors to answer with, in order; then approved
	pollNum  int
	startErr error
	finished []string // library ids that traded a secret
	// callRefused is what the source reports after a call met a refusal.
	callRefused bool
	// finishWait, when set, is closed as the exchange starts and holds it
	// until the request is cancelled.
	finishWait chan struct{}
}

func (s *qcServer) Library() source.Library                             { return s.lib }
func (s *qcServer) Track(context.Context, string) (source.Track, error) { return source.Track{}, nil }
func (s *qcServer) StreamURL(context.Context, string) (string, error)   { return "", nil }
func (s *qcServer) Close() error                                        { return nil }

func (s *qcServer) Ping(context.Context) (source.ServerInfo, error) {
	s.script.mu.Lock()
	defer s.script.mu.Unlock()
	s.script.pings++
	return source.ServerInfo{}, s.script.pingErr
}

func (s *qcServer) QuickConnectEnabled(context.Context) (bool, error) { return true, nil }
func (s *qcServer) QuickConnectStart(context.Context) (string, string, error) {
	s.script.mu.Lock()
	defer s.script.mu.Unlock()
	return "482917", "secret", s.script.startErr
}
func (s *qcServer) QuickConnectPoll(context.Context, string) (bool, error) {
	s.script.mu.Lock()
	defer s.script.mu.Unlock()
	i := s.script.pollNum
	s.script.pollNum++
	if i < len(s.script.polls) {
		return false, s.script.polls[i]
	}
	return true, nil
}
func (s *qcServer) SignInRefused() bool {
	s.script.mu.Lock()
	defer s.script.mu.Unlock()
	return s.script.callRefused
}

func (s *qcServer) QuickConnectFinish(ctx context.Context, _ string) (string, error) {
	if s.script.finishWait != nil {
		close(s.script.finishWait)
		<-ctx.Done()
		return "", ctx.Err()
	}
	s.script.mu.Lock()
	s.script.finished = append(s.script.finished, s.lib.ID)
	s.script.mu.Unlock()
	user := "dani"
	err := errors.Join(s.env.Secrets.Set(s.lib.ID, "token", "tok"), s.env.Secrets.Set(s.lib.ID, "userId", "uid"),
		s.env.Secrets.Set(s.lib.ID, "sessionFor", "owner"))
	return user, err
}

// plainServer has no Quick Connect.
type plainServer struct{ fakeServer }

type qcEvents struct {
	ch chan map[string]any
}

func (e *qcEvents) publish(topic string, data any) {
	ev, ok := data.(map[string]any)
	if topic != TopicLibraries || !ok || ev["type"] != "quickConnect" {
		return
	}
	e.ch <- ev
}

func (e *qcEvents) next(t *testing.T) map[string]any {
	t.Helper()
	// "approved" is the step before the end and not what the tests look at.
	for {
		select {
		case ev := <-e.ch:
			if ev["state"] != QuickConnectApproved {
				return ev
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no quickConnect event")
			return nil
		}
	}
}

func (e *qcEvents) none(t *testing.T) {
	t.Helper()
	select {
	case ev := <-e.ch:
		t.Fatalf("unexpected event %v", ev)
	case <-time.After(150 * time.Millisecond):
	}
}

func newQCManager(t *testing.T) (*Manager, *qcScript, *memSecrets, *qcEvents) {
	t.Helper()
	secrets := &memSecrets{}
	m, script, ev := newQCManagerWith(t, secrets)
	return m, script, secrets, ev
}

// failSecrets fails the write of one key.
type failSecrets struct {
	*memSecrets
	key string
	err error
}

func (f failSecrets) Set(lib, key, v string) error {
	if key == f.key {
		return f.err
	}
	return f.memSecrets.Set(lib, key, v)
}

func newQCManagerWith(t *testing.T, keys source.Secrets) (*Manager, *qcScript, *qcEvents) {
	t.Helper()
	qcEvery := quickConnectEvery
	quickConnectEvery = 10 * time.Millisecond
	t.Cleanup(func() { quickConnectEvery = qcEvery })
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	script := &qcScript{}
	ping := make(chan struct{})
	close(ping)
	m := NewManager(store, source.Registry{
		"qc": func(l source.Library, env source.Env) (source.Source, error) {
			return &qcServer{lib: l, env: env, script: script}, nil
		},
		"plain": func(l source.Library, _ source.Env) (source.Source, error) {
			return &plainServer{fakeServer{lib: l, down: new(atomic.Bool), ping: ping}}, nil
		},
	}, source.Env{Secrets: keys})
	ev := &qcEvents{ch: make(chan map[string]any, 8)}
	m.SetPublish(ev.publish)
	return m, script, ev
}

func TestQuickConnectAddsTheLibraryOnApproval(t *testing.T) {
	ctx := context.Background()
	m, script, secrets, ev := newQCManager(t)

	session, code, err := m.QuickConnectStart(ctx, "", "qc", "Home", map[string]any{"url": "https://a.example"})
	require.NoError(t, err)
	assert.Equal(t, "482917", code)

	done := ev.next(t)
	assert.Equal(t, QuickConnectDone, done["state"])
	assert.Equal(t, session, done["session"])
	id := done["library"].(string)

	lib, err := m.Library(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Home", lib.Name)
	assert.Equal(t, "dani", lib.Config["username"])
	assert.Equal(t, "quickConnect", lib.Config["auth"])
	assert.Equal(t, "https://a.example", lib.Config["url"])
	assert.Equal(t, []string{id}, script.finished, "the code was traded under the id the library got")
	tok, err := secrets.Get(id, "token")
	require.NoError(t, err)
	assert.Equal(t, "tok", tok)
	_, err = secrets.Get(id, "password")
	assert.ErrorIs(t, err, source.ErrNoSecret)
}

func TestQuickConnectEndings(t *testing.T) {
	netErr := &net.OpError{Op: "dial", Err: errors.New("refused")}
	tests := []struct {
		name  string
		polls []error
		want  string
		code  string
	}{
		{"expired", []error{source.ErrQuickConnectExpired}, QuickConnectExpired, ""},
		{"turned off", []error{nil, source.ErrQuickConnectOff}, QuickConnectOff, ""},
		{"three failures in a row", []error{netErr, netErr, netErr}, QuickConnectFailed, CodeServerUnreachable},
		{"failures that don't follow each other", []error{netErr, netErr, nil, netErr, netErr}, QuickConnectDone, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, script, _, ev := newQCManager(t)
			script.polls = tt.polls
			_, _, err := m.QuickConnectStart(context.Background(), "", "qc", "Home", map[string]any{"url": "https://a.example"})
			require.NoError(t, err)
			got := ev.next(t)
			assert.Equal(t, tt.want, got["state"])
			if tt.code != "" {
				assert.Equal(t, tt.code, got["errorCode"])
			}
			libs, err := m.store.Libraries(context.Background())
			require.NoError(t, err)
			assert.Len(t, libs, map[bool]int{true: 1, false: 0}[tt.want == QuickConnectDone])
		})
	}
}

func TestQuickConnectTimesOut(t *testing.T) {
	life := quickConnectLifetime
	quickConnectLifetime = 100 * time.Millisecond
	t.Cleanup(func() { quickConnectLifetime = life })
	m, script, _, ev := newQCManager(t)
	script.polls = make([]error, 1000)
	_, _, err := m.QuickConnectStart(context.Background(), "", "qc", "Home", map[string]any{"url": "https://a.example"})
	require.NoError(t, err)
	assert.Equal(t, QuickConnectExpired, ev.next(t)["state"])
}

func TestQuickConnectCancelAndReplace(t *testing.T) {
	m, script, _, ev := newQCManager(t)
	script.polls = make([]error, 100000)
	ctx := context.Background()
	cfg := map[string]any{"url": "https://a.example"}

	first, _, err := m.QuickConnectStart(ctx, "", "qc", "One", cfg)
	require.NoError(t, err)
	m.QuickConnectCancel(first)
	ev.none(t)

	_, _, err = m.QuickConnectStart(ctx, "", "qc", "Two", cfg)
	require.NoError(t, err)
	second, _, err := m.QuickConnectStart(ctx, "", "qc", "Three", cfg)
	require.NoError(t, err)
	script.mu.Lock()
	script.polls = nil
	script.mu.Unlock()
	got := ev.next(t)
	assert.Equal(t, second, got["session"], "only the latest request finishes")
	ev.none(t)
	libs, err := m.store.Libraries(ctx)
	require.NoError(t, err)
	require.Len(t, libs, 1)
	assert.Equal(t, "Three", libs[0].Name)
}

func TestQuickConnectStartRefusals(t *testing.T) {
	m, script, _, _ := newQCManager(t)
	ctx := context.Background()
	script.startErr = source.ErrQuickConnectOff
	_, _, err := m.QuickConnectStart(ctx, "", "qc", "Home", map[string]any{"url": "https://a.example"})
	var r Reason
	require.ErrorAs(t, err, &r)
	assert.Equal(t, CodeQuickConnectOff, r.Code)

	_, _, err = m.QuickConnectStart(ctx, "", "plain", "Home", map[string]any{"url": "https://a.example"})
	assert.Error(t, err, "a kind without Quick Connect")
}

// Signing in again keeps the library and its id, and drops the old password.
func TestQuickConnectSignInAgain(t *testing.T) {
	ctx := context.Background()
	m, _, secrets, ev := newQCManager(t)
	lib, err := m.Add(ctx, "qc", "Home", map[string]any{"url": "https://a.example", "username": "dani"}, map[string]string{"password": "old"})
	require.NoError(t, err)

	_, _, err = m.QuickConnectStart(ctx, lib.ID, "", "", nil)
	require.NoError(t, err)
	got := ev.next(t)
	assert.Equal(t, QuickConnectDone, got["state"])
	assert.Equal(t, lib.ID, got["library"])

	again, err := m.Library(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "quickConnect", again.Config["auth"])
	_, err = secrets.Get(lib.ID, "password")
	assert.ErrorIs(t, err, source.ErrNoSecret, "the password is gone")
	tok, err := secrets.Get(lib.ID, "token")
	require.NoError(t, err)
	assert.Equal(t, "tok", tok)
	libs, err := m.store.Libraries(ctx)
	require.NoError(t, err)
	assert.Len(t, libs, 1)
}

func TestQuickConnectLibraryNeedsANewSignInAtANewAddress(t *testing.T) {
	ctx := context.Background()
	m, _, _, _ := newQCManager(t)
	lib, err := m.Add(ctx, "qc", "Home", map[string]any{"url": "https://a.example", "username": "dani", "auth": "quickConnect"}, map[string]string{"token": "t"})
	require.NoError(t, err)
	_, err = m.Update(ctx, lib.ID, "", map[string]any{"url": "https://b.example"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Quick Connect")
	_, err = m.Update(ctx, lib.ID, "", map[string]any{"url": "https://b.example"}, map[string]string{"token": "t2"})
	assert.NoError(t, err)
}

// A refused sign-in reads "sign in again", not "server unreachable", and
// using the library does not ask the server again.
func TestRefusedSignInIsItsOwnState(t *testing.T) {
	ctx := context.Background()
	m, script, _, _ := newQCManager(t)
	lib, err := m.Add(ctx, "qc", "Home", map[string]any{"url": "https://a.example", "username": "dani", "auth": "quickConnect"}, map[string]string{"token": "t"})
	require.NoError(t, err)

	state := func() string {
		infos, err := m.List(ctx)
		require.NoError(t, err)
		require.Len(t, infos, 1)
		return infos[0].ErrorCode
	}
	script.mu.Lock()
	script.pingErr = source.ErrSignInNeeded
	script.mu.Unlock()
	m.retry(ctx, lib.ID)
	assert.Equal(t, CodeSignInExpired, state())
	assert.True(t, m.Skip(source.Ref{Library: lib.ID, ID: "x"}), "its tracks are passed over")

	script.mu.Lock()
	pings := script.pings
	script.mu.Unlock()
	m.mu.Lock()
	m.tried[lib.ID] = time.Now().Add(-time.Hour)
	m.mu.Unlock()
	_, err = m.Get(lib.ID)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	script.mu.Lock()
	assert.Equal(t, pings, script.pings, "using a refused library doesn't ping it again")
	script.pingErr = &net.OpError{Op: "dial", Err: errors.New("refused")}
	script.mu.Unlock()

	m.retry(ctx, lib.ID)
	assert.Equal(t, CodeServerUnreachable, state(), "a network failure is still unreachable")
	script.mu.Lock()
	script.pingErr = nil
	script.mu.Unlock()
	m.retry(ctx, lib.ID)
	assert.Empty(t, state())
}

// A server that said nothing about it, but whose calls met a refusal, is
// looked at once, and a library already known as refused is not.
func TestRefusalSeenByACallIsLookedAt(t *testing.T) {
	ctx := context.Background()
	m, script, _, _ := newQCManager(t)
	lib, err := m.Add(ctx, "qc", "Home", map[string]any{"url": "https://a.example", "username": "dani", "auth": "quickConnect"}, map[string]string{"token": "t"})
	require.NoError(t, err)
	m.awaitAll(ctx)
	script.mu.Lock()
	script.callRefused, script.pingErr = true, source.ErrSignInNeeded
	script.mu.Unlock()
	pings := func() int {
		script.mu.Lock()
		defer script.mu.Unlock()
		return script.pings
	}
	before := pings()
	old := func() {
		m.mu.Lock()
		m.tried[lib.ID] = time.Now().Add(-time.Hour)
		m.mu.Unlock()
	}
	old()
	_, err = m.Get(lib.ID)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		infos, err := m.List(ctx)
		return err == nil && len(infos) == 1 && infos[0].ErrorCode == CodeSignInExpired
	}, 2*time.Second, 10*time.Millisecond)

	old()
	after := pings()
	_, err = m.Get(lib.ID)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, after, pings(), "a refusal the ping found is not asked again on use")
	assert.Greater(t, after, before)
}

// A network change doesn't send a known-bad password again; startup does.
func TestNetworkChangeSkipsARefusedPassword(t *testing.T) {
	ctx := context.Background()
	m, script, _, _ := newQCManager(t)
	_, err := m.Add(ctx, "qc", "Home", map[string]any{"url": "https://a.example", "username": "dani"}, map[string]string{"password": "pw"})
	require.NoError(t, err)
	m.awaitAll(ctx)
	script.mu.Lock()
	script.pingErr = source.ErrSignInRefused
	script.mu.Unlock()
	m.Recheck(ctx)
	script.mu.Lock()
	before := script.pings
	script.mu.Unlock()

	m.RecheckNetwork(ctx)
	script.mu.Lock()
	assert.Equal(t, before, script.pings)
	script.mu.Unlock()
	m.Recheck(ctx)
	script.mu.Lock()
	assert.Equal(t, before+1, script.pings)
	script.mu.Unlock()
}

// Storing the session can fail on the keyring side only: that reads as the
// keyring, whatever the error says, and leaves nothing behind.
func TestQuickConnectStoreFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"keyring locked", fmt.Errorf("prompt dismissed: %w", keyring.ErrLocked), CodeKeyringLocked},
		{"no keyring", keyring.ErrUnavailable, CodeNoKeyring},
		{"any other failure", errors.New("collection missing"), CodeOpenFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secrets := failSecrets{&memSecrets{}, "sessionFor", tt.err}
			m, _, ev := newQCManagerWith(t, secrets)
			_, _, err := m.QuickConnectStart(context.Background(), "", "qc", "Home", map[string]any{"url": "https://a.example"})
			require.NoError(t, err)
			got := ev.next(t)
			assert.Equal(t, QuickConnectFailed, got["state"])
			assert.Equal(t, tt.want, got["errorCode"])
			assert.False(t, secrets.has(got["library"].(string)), "no token is left under a library that was never added")
		})
	}
}

// A cancel during the final exchange ends the request without a word.
func TestQuickConnectCancelDuringTheExchangeIsSilent(t *testing.T) {
	ctx := context.Background()
	m, script, _, ev := newQCManager(t)
	script.finishWait = make(chan struct{})
	session, _, err := m.QuickConnectStart(ctx, "", "qc", "Home", map[string]any{"url": "https://a.example"})
	require.NoError(t, err)
	<-script.finishWait
	m.QuickConnectCancel(session)
	for {
		select {
		case e := <-ev.ch:
			require.Equal(t, QuickConnectApproved, e["state"])
			continue
		case <-time.After(150 * time.Millisecond):
		}
		break
	}
	libs, err := m.store.Libraries(ctx)
	require.NoError(t, err)
	assert.Empty(t, libs)
}

// Only a Quick Connect request makes a Quick Connect library: a bare
// switch would leave one with no session, or without its password.
func TestQuickConnectAuthNeedsASession(t *testing.T) {
	ctx := context.Background()
	m, _, secrets, _ := newQCManager(t)
	qc := map[string]any{"url": "https://a.example", "username": "dani", "auth": "quickConnect"}

	_, err := m.Add(ctx, "qc", "Home", qc, nil)
	assert.Error(t, err)

	lib, err := m.Add(ctx, "qc", "Pw", map[string]any{"url": "https://a.example", "username": "dani"}, map[string]string{"password": "pw"})
	require.NoError(t, err)
	_, err = m.Update(ctx, lib.ID, "", map[string]any{"auth": "quickConnect"}, nil)
	assert.Error(t, err)
	pw, err := secrets.Get(lib.ID, "password")
	require.NoError(t, err)
	assert.Equal(t, "pw", pw)
}

// A second or third key failing to store leaves no first one behind.
func TestAddDropsSecretsWhenOneFailsToStore(t *testing.T) {
	secrets := failSecrets{&memSecrets{}, "userId", errors.New("no space")}
	m, _, _ := newQCManagerWith(t, secrets)
	_, err := m.Add(context.Background(), "qc", "Home", map[string]any{"url": "https://a.example", "username": "d"},
		map[string]string{"token": "t", "userId": "u"})
	require.Error(t, err)
	secrets.memSecrets.Range(func(k, _ any) bool {
		t.Errorf("left behind: %v", k)
		return true
	})
}

func TestQuickConnectKindsNeedNoServer(t *testing.T) {
	m, _, _, _ := newQCManager(t)
	assert.Equal(t, []string{"qc"}, m.QuickConnectKinds())
}
