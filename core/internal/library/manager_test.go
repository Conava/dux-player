package library

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/keyring"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

type fakeServer struct {
	lib  source.Library
	down *atomic.Bool
	ping chan struct{}
}

func (s fakeServer) Library() source.Library                             { return s.lib }
func (s fakeServer) Track(context.Context, string) (source.Track, error) { return source.Track{}, nil }
func (s fakeServer) StreamURL(context.Context, string) (string, error)   { return "http://srv/x", nil }
func (s fakeServer) Close() error                                        { return nil }
func (s fakeServer) Ping(context.Context) (source.ServerInfo, error) {
	<-s.ping
	if s.down.Load() {
		return source.ServerInfo{}, errors.New("connection refused")
	}
	return source.ServerInfo{}, nil
}

// A failed track makes the queue skip its server's tracks, quietly until the
// ping confirms the server is down, then with one notice; a server that
// answers only loses that one track.
func TestFailedTrackChecksItsServer(t *testing.T) {
	for _, down := range []bool{true, false} {
		ctx := context.Background()
		store, err := Open(ctx, filepath.Join(t.TempDir(), "library.db"))
		require.NoError(t, err)
		t.Cleanup(func() { store.Close() })
		lib := source.Library{ID: "srv", Kind: "fake", Name: "Server", Enabled: true}
		require.NoError(t, store.AddLibrary(ctx, lib))
		var isDown atomic.Bool
		ping := make(chan struct{})
		m := NewManager(store, source.Registry{"fake": func(l source.Library, _ source.Env) (source.Source, error) {
			return &fakeServer{lib: l, down: &isDown, ping: ping}, nil
		}}, source.Env{})
		require.NoError(t, m.Load(ctx))
		m.awaitAll(ctx)
		var mu sync.Mutex
		var notices []string
		m.SetPublish(func(topic string, data any) {
			if topic == player.TopicError {
				mu.Lock()
				notices = append(notices, data.(map[string]string)["reason"])
				mu.Unlock()
			}
		})
		ref := source.Ref{Library: "srv", ID: "a"}
		require.False(t, m.Skip(ref))

		isDown.Store(down)
		rechecked := make(chan struct{})
		m.Failed(ref, func() { close(rechecked) })
		assert.True(t, m.Skip(ref), "skipped while the ping runs")
		mu.Lock()
		assert.Empty(t, notices, "no notice before the ping answers")
		mu.Unlock()
		close(ping)
		select {
		case <-rechecked:
		case <-time.After(5 * time.Second):
			t.Fatal("no recheck")
		}
		assert.Equal(t, down, m.Skip(ref), "down=%v", down)
		m.Skip(ref)
		mu.Lock()
		assert.Len(t, notices, map[bool]int{true: 1, false: 0}[down], "down=%v", down)
		mu.Unlock()
	}
}

type memSecrets struct{ sync.Map }

func (m *memSecrets) Get(lib, key string) (string, error) {
	v, ok := m.Load(lib + "/" + key)
	if !ok {
		return "", errors.New("no " + key)
	}
	return v.(string), nil
}
func (m *memSecrets) Set(lib, key, v string) error { m.Store(lib+"/"+key, v); return nil }
func (m *memSecrets) Delete(lib string) error {
	m.Range(func(k, _ any) bool {
		if strings.HasPrefix(k.(string), lib+"/") {
			m.Map.Delete(k)
		}
		return true
	})
	return nil
}

func (m *memSecrets) has(lib string) bool {
	found := false
	m.Range(func(k, _ any) bool {
		found = found || strings.HasPrefix(k.(string), lib+"/")
		return true
	})
	return found
}

// The stored password must not be sent to a server the user just typed in.
func TestMovingAServerNeedsThePassword(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	ping := make(chan struct{})
	close(ping)
	var down atomic.Bool
	m := NewManager(store, source.Registry{"fake": func(l source.Library, _ source.Env) (source.Source, error) {
		return &fakeServer{lib: l, down: &down, ping: ping}, nil
	}}, source.Env{Secrets: &memSecrets{}})
	lib, err := m.Add(ctx, "fake", "S", map[string]any{"url": "https://a.example:4533"}, map[string]string{"password": "pw"})
	require.NoError(t, err)

	_, err = m.Update(ctx, lib.ID, "", map[string]any{"url": "https://a.example:4533/navidrome"}, nil)
	require.NoError(t, err, "same server, other path")
	_, err = m.Update(ctx, lib.ID, "", map[string]any{"url": "https://b.example"}, nil)
	require.Error(t, err)
	_, err = m.Update(ctx, lib.ID, "", map[string]any{"url": "https://b.example"}, map[string]string{"password": "pw2"})
	require.NoError(t, err)
}

// A library that can't be built leaves nothing behind (no stored config, no
// secrets); a bad edit changes nothing; removing takes its secrets along.
func TestManagerEditsAndSecrets(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	ping := make(chan struct{})
	close(ping)
	var down atomic.Bool
	secrets := &memSecrets{}
	m := NewManager(store, source.Registry{"fake": func(l source.Library, env source.Env) (source.Source, error) {
		if pw, _ := env.Secrets.Get(l.ID, "password"); pw == "wrong" {
			return nil, errors.New("refused")
		}
		return &fakeServer{lib: l, down: &down, ping: ping}, nil
	}}, source.Env{Secrets: secrets})

	_, err = m.Add(ctx, "fake", "Bad", map[string]any{"url": "https://a.example"}, map[string]string{"password": "wrong"})
	require.Error(t, err)
	libs, err := store.Libraries(ctx)
	require.NoError(t, err)
	assert.Empty(t, libs)
	empty := true
	secrets.Range(func(any, any) bool { empty = false; return false })
	assert.True(t, empty, "the failed add's secrets are gone")

	lib, err := m.Add(ctx, "fake", "Good", map[string]any{"url": "https://a.example"}, map[string]string{"password": "pw"})
	require.NoError(t, err)
	_, err = m.Update(ctx, lib.ID, "Renamed", map[string]any{"url": "https://a.example/x"}, map[string]string{"password": "wrong"})
	require.Error(t, err)
	got, err := m.Library(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "Good", got.Name)
	assert.Equal(t, "https://a.example", got.Config["url"])
	pw, err := secrets.Get(lib.ID, "password")
	require.NoError(t, err)
	assert.Equal(t, "pw", pw, "a refused edit keeps the stored password")

	require.NoError(t, m.Remove(ctx, lib.ID))
	assert.False(t, secrets.has(lib.ID))
	_, err = m.Get(lib.ID)
	require.ErrorIs(t, err, source.ErrNotFound)
}

// Startup doesn't wait for a library whose build blocks (a keyring
// prompt); using that library waits for it.
func TestLoadBuildsInTheBackground(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	require.NoError(t, store.AddLibrary(ctx, source.Library{ID: "srv", Kind: "fake", Name: "S", Enabled: true}))
	release := make(chan struct{})
	ping := make(chan struct{})
	close(ping)
	var down atomic.Bool
	m := NewManager(store, source.Registry{"fake": func(l source.Library, _ source.Env) (source.Source, error) {
		<-release
		return &fakeServer{lib: l, down: &down, ping: ping}, nil
	}}, source.Env{})

	loaded := make(chan error)
	go func() { loaded <- m.Load(ctx) }()
	select {
	case err := <-loaded:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Load waited for the build")
	}
	assert.Empty(t, m.Unavailable(ctx, source.Ref{Library: "srv", ID: "a"}), "a starting library isn't skipped")
	got := make(chan error)
	go func() {
		_, err := m.Tracks(ctx, []source.Ref{{Library: "srv", ID: "a"}, {Library: "srv", ID: "b"}})
		got <- err
	}()
	close(release)
	select {
	case err := <-got:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("lookup never finished")
	}
}

// A library that fails to build shows a fixed reason, never the raw error.
func TestBuildReason(t *testing.T) {
	tests := []struct {
		err  error
		code string
	}{
		{fmt.Errorf("password for x: keyring: %w", keyring.ErrLocked), CodeKeyringLocked},
		{fmt.Errorf("password for x: %w", keyring.ErrUnavailable), CodeNoKeyring},
		{errors.New("dial tcp 10.0.0.1:443: secret detail"), CodeOpenFailed},
	}
	for _, tt := range tests {
		r := buildReason(tt.err)
		assert.Equal(t, tt.code, r.Code)
		assert.NotContains(t, r.Message, "secret detail")
	}
}
