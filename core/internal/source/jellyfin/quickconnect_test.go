package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

// qcFake is a server with Quick Connect, answering from hand-written
// fixtures (no real server was available to record them). Zero status
// values answer from the fixture.
type qcFake struct {
	*httptest.Server
	mu       sync.Mutex
	paths    []string
	auths    []string
	status   map[string]int
	approved bool
	// getOnly answers Initiate with 405 unless it is a GET, like Jellyfin 10.8.
	getOnly bool
}

func newQCFake(t *testing.T) *qcFake {
	f := &qcFake{status: map[string]int{}}
	fixture := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		require.NoError(t, err)
		return data
	}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		f.auths = append(f.auths, r.Header.Get("Authorization"))
		code, approved := f.status[r.URL.Path], f.approved
		f.mu.Unlock()
		if f.getOnly && r.URL.Path == "/QuickConnect/Initiate" && r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if code != 0 {
			w.WriteHeader(code)
			return
		}
		switch r.URL.Path {
		case "/QuickConnect/Enabled":
			_, _ = w.Write(fixture("qc_enabled.json"))
		case "/QuickConnect/Initiate":
			_, _ = w.Write(fixture("qc_initiate.json"))
		case "/QuickConnect/Connect":
			if approved {
				_, _ = w.Write(fixture("qc_state_authenticated.json"))
				return
			}
			_, _ = w.Write(fixture("qc_state_waiting.json"))
		case "/Users/AuthenticateWithQuickConnect":
			_, _ = w.Write(fixture("qc_auth_result.json"))
		case "/System/Info/Public":
			_, _ = w.Write([]byte(`{"ProductName":"Jellyfin Server","Version":"10.10.0"}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *qcFake) count(path string) (n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.paths {
		if p == path {
			n++
		}
	}
	return n
}

func (f *qcFake) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.paths)
}

func (f *qcFake) set(path string, code int) {
	f.mu.Lock()
	f.status[path] = code
	f.mu.Unlock()
}

func newQCSource(t *testing.T, f *qcFake, secrets source.Secrets) *Source {
	lib := source.Library{ID: "qc", Kind: Kind, Name: "QC", Config: map[string]any{"url": f.URL, "auth": "quickConnect"}}
	src, err := Factory(lib, source.Env{Secrets: secrets, CacheDir: t.TempDir(), Version: "1.2.3"})
	require.NoError(t, err)
	s := src.(*Source)
	s.client.http = f.Client()
	return s
}

func TestQuickConnectSignsIn(t *testing.T) {
	f := newQCFake(t)
	secrets := memSecrets{}
	s := newQCSource(t, f, secrets)
	ctx := context.Background()

	on, err := s.QuickConnectEnabled(ctx)
	require.NoError(t, err)
	assert.True(t, on)

	code, secret, err := s.QuickConnectStart(ctx)
	require.NoError(t, err)
	assert.Equal(t, "482917", code)
	assert.Len(t, secret, 64)
	for _, part := range []string{`DeviceId="dplayer-qc"`, `Client="Dux Player"`, `Version="1.2.3"`, `Device="Linux"`} {
		assert.Contains(t, f.auths[1], part, "Initiate needs the device in the header")
	}
	assert.NotContains(t, f.auths[1], "Token=")

	ok, err := s.QuickConnectPoll(ctx, secret)
	require.NoError(t, err)
	assert.False(t, ok)
	f.mu.Lock()
	f.approved = true
	f.mu.Unlock()
	ok, err = s.QuickConnectPoll(ctx, secret)
	require.NoError(t, err)
	assert.True(t, ok)

	user, err := s.QuickConnectFinish(ctx, secret)
	require.NoError(t, err)
	assert.Equal(t, "dani", user)
	assert.Equal(t, "qctoken1", secrets["qc/token"])
	assert.Equal(t, userID, secrets["qc/userId"])
	assert.Equal(t, f.URL+"\x00dani", secrets["qc/sessionFor"])
	_, hasPassword := secrets["qc/password"]
	assert.False(t, hasPassword, "no password is stored")

	// The stored session is used by a library with that user name.
	lib := source.Library{ID: "qc", Kind: Kind, Name: "QC", Config: map[string]any{"url": f.URL, "auth": "quickConnect", "username": "dani"}}
	again, err := Factory(lib, source.Env{Secrets: secrets, CacheDir: t.TempDir()})
	require.NoError(t, err)
	tok, uid := again.(*Source).client.session()
	assert.Equal(t, "qctoken1", tok)
	assert.Equal(t, userID, uid)
}

func TestQuickConnectFailures(t *testing.T) {
	tests := []struct {
		name string
		path string
		code int
		call func(*Source) error
		want error
	}{
		{"off at Initiate", "/QuickConnect/Initiate", 401, func(s *Source) error { _, _, err := s.QuickConnectStart(context.Background()); return err }, source.ErrQuickConnectOff},
		{"off while polling", "/QuickConnect/Connect", 401, func(s *Source) error { _, err := s.QuickConnectPoll(context.Background(), "x"); return err }, source.ErrQuickConnectOff},
		{"expired while polling", "/QuickConnect/Connect", 404, func(s *Source) error { _, err := s.QuickConnectPoll(context.Background(), "x"); return err }, source.ErrQuickConnectExpired},
		{"expired at the exchange", "/Users/AuthenticateWithQuickConnect", 404, func(s *Source) error { _, err := s.QuickConnectFinish(context.Background(), "x"); return err }, source.ErrQuickConnectExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newQCFake(t)
			f.set(tt.path, tt.code)
			err := tt.call(newQCSource(t, f, memSecrets{}))
			assert.ErrorIs(t, err, tt.want)
		})
	}

	t.Run("a server error is not the feature being off", func(t *testing.T) {
		f := newQCFake(t)
		f.set("/QuickConnect/Connect", 500)
		_, err := newQCSource(t, f, memSecrets{}).QuickConnectPoll(context.Background(), "x")
		require.Error(t, err)
		assert.NotErrorIs(t, err, source.ErrQuickConnectOff)
		assert.NotErrorIs(t, err, source.ErrQuickConnectExpired)
	})
}

func TestQuickConnectStartRetriesWithGETOn405(t *testing.T) {
	f := newQCFake(t)
	f.getOnly = true
	code, secret, err := newQCSource(t, f, memSecrets{}).QuickConnectStart(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, code)
	assert.NotEmpty(t, secret)

	f = newQCFake(t)
	f.getOnly = true
	f.set("/QuickConnect/Initiate", 401)
	_, _, err = newQCSource(t, f, memSecrets{}).QuickConnectStart(context.Background())
	assert.ErrorIs(t, err, source.ErrQuickConnectOff)
}

func TestQuickConnectEnabledReading(t *testing.T) {
	f := newQCFake(t)
	s := newQCSource(t, f, memSecrets{})
	ctx := context.Background()
	for _, code := range []int{401, 404} {
		f.set("/QuickConnect/Enabled", code)
		on, err := s.QuickConnectEnabled(ctx)
		require.NoError(t, err, "%d", code)
		assert.False(t, on, "%d is off", code)
	}
	f.set("/QuickConnect/Enabled", 500)
	_, err := s.QuickConnectEnabled(ctx)
	assert.Error(t, err)
}

// The capability follows what the server said at the last ping; a failed
// read keeps it, and a refused sign-in does not hide it.
func TestPingDecidesTheQuickConnectCapability(t *testing.T) {
	f := newQCFake(t)
	s := newQCSource(t, f, memSecrets{})
	ctx := context.Background()
	assert.Contains(t, source.Capabilities(s), source.CapQuickConnect)

	_, err := s.Ping(ctx)
	require.ErrorIs(t, err, source.ErrSignInNeeded, "no session yet")
	assert.Contains(t, source.Capabilities(s), source.CapQuickConnect, "on, and a refused sign-in doesn't hide it")

	f.set("/QuickConnect/Enabled", 500)
	_, _ = s.Ping(ctx)
	assert.Contains(t, source.Capabilities(s), source.CapQuickConnect, "a failed read keeps it")

	f.set("/QuickConnect/Enabled", 0)
	f.mu.Lock()
	f.status["/QuickConnect/Enabled"] = 401
	f.mu.Unlock()
	_, _ = s.Ping(ctx)
	assert.NotContains(t, source.Capabilities(s), source.CapQuickConnect)
}

// A library without a password can't sign in again: a refused token
// is one request, then nothing, until a new token or a Ping.
func TestRefusedTokenWithoutPasswordIsOneRequest(t *testing.T) {
	f := newQCFake(t)
	secrets := memSecrets{"qc/token": "old", "qc/userId": userID, "qc/sessionFor": f.URL + "\x00dani"}
	lib := source.Library{ID: "qc", Kind: Kind, Name: "QC", Config: map[string]any{"url": f.URL, "auth": "quickConnect", "username": "dani"}}
	src, err := Factory(lib, source.Env{Secrets: secrets, CacheDir: t.TempDir()})
	require.NoError(t, err)
	s := src.(*Source)
	s.client.http = f.Client()
	ctx := context.Background()

	_, err = s.Album(ctx, "a1")
	require.ErrorIs(t, err, source.ErrSignInNeeded)
	assert.ErrorIs(t, err, source.ErrSignInRefused)
	assert.Equal(t, 1, f.total(), "the refused request is the only one")

	_, err = s.Artists(ctx)
	assert.ErrorIs(t, err, source.ErrSignInNeeded)
	_, err = s.StreamURL(ctx, "x")
	assert.ErrorIs(t, err, source.ErrSignInNeeded)
	_, _, err = s.client.Image(ctx, "x", 0)
	assert.ErrorIs(t, err, source.ErrSignInNeeded)
	assert.Equal(t, 1, f.total(), "later calls send nothing")
	assert.Zero(t, f.count("/Users/AuthenticateByName"), "no password to send")

	_, err = s.Ping(ctx)
	assert.ErrorIs(t, err, source.ErrSignInRefused)
	assert.Positive(t, f.count("/Users/"+userID), "Ping asks the server again")
}

// A password account whose password was changed fails one sign-in, not one
// per call: each failed sign-in counts toward the server's lockout.
func TestRefusedPasswordIsOneSignIn(t *testing.T) {
	f := newQCFake(t)
	f.set("/Users/AuthenticateByName", 401)
	lib := source.Library{ID: "pw", Kind: Kind, Name: "PW", Config: map[string]any{"url": f.URL, "username": "dani"}}
	src, err := Factory(lib, source.Env{Secrets: memSecrets{"pw/password": "changed"}, CacheDir: t.TempDir()})
	require.NoError(t, err)
	s := src.(*Source)
	s.client.http = f.Client()

	for range 3 {
		_, err = s.Artists(context.Background())
		assert.ErrorIs(t, err, source.ErrSignInRefused)
	}
	assert.Equal(t, 1, f.count("/Users/AuthenticateByName"))
}

// Calls that run together share the one failed sign-in.
func TestRefusedPasswordIsOneSignInWhenCallsOverlap(t *testing.T) {
	f := newQCFake(t)
	f.set("/Users/AuthenticateByName", 401)
	lib := source.Library{ID: "pw", Kind: Kind, Name: "PW", Config: map[string]any{"url": f.URL, "username": "dani"}}
	src, err := Factory(lib, source.Env{Secrets: memSecrets{"pw/password": "changed"}, CacheDir: t.TempDir()})
	require.NoError(t, err)
	s := src.(*Source)
	s.client.http = f.Client()

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Artists(context.Background())
			assert.ErrorIs(t, err, source.ErrSignInRefused)
		}()
	}
	wg.Wait()
	assert.Equal(t, 1, f.count("/Users/AuthenticateByName"))
	assert.True(t, s.SignInRefused(), "the manager can see the refusal")
}

// Reading a library's session stops at the first read that fails, since each
// read of a locked keyring can ask the user to unlock it.
type countingSecrets struct{ reads *int }

func (c countingSecrets) Get(string, string) (string, error) {
	*c.reads++
	return "", errors.New("the keyring is locked")
}
func (countingSecrets) Set(string, string, string) error { return nil }
func (countingSecrets) Delete(string) error              { return nil }

func TestQuickConnectFactoryReadsKeyringOnceWhenLocked(t *testing.T) {
	lib := source.Library{ID: "qc", Kind: Kind, Name: "QC", Config: map[string]any{"url": "http://127.0.0.1:1", "auth": "quickConnect", "username": "dani"}}
	reads := 0
	_, err := Factory(lib, source.Env{Secrets: countingSecrets{&reads}})
	require.Error(t, err)
	assert.Equal(t, 1, reads)
}

// A keyring that can't be read is not a missing session: the library
// reports the keyring, not "sign in again".
type lockedSecrets struct{ err error }

func (l lockedSecrets) Get(string, string) (string, error) { return "", l.err }
func (lockedSecrets) Set(string, string, string) error     { return nil }
func (lockedSecrets) Delete(string) error                  { return nil }

func TestQuickConnectFactoryKeyring(t *testing.T) {
	lib := source.Library{ID: "qc", Kind: Kind, Name: "QC", Config: map[string]any{"url": "http://127.0.0.1:1", "auth": "quickConnect", "username": "dani"}}
	locked := errors.New("the keyring is locked")
	_, err := Factory(lib, source.Env{Secrets: lockedSecrets{locked}})
	assert.ErrorIs(t, err, locked)

	_, err = Factory(lib, source.Env{Secrets: lockedSecrets{source.ErrNoSecret}})
	assert.NoError(t, err, "nothing stored is no session, not a failure")

	lib.Config["auth"] = ""
	_, err = Factory(lib, source.Env{Secrets: lockedSecrets{source.ErrNoSecret}})
	assert.Error(t, err, "a password library still needs its password")
}
