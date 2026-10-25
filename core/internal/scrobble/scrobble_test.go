package scrobble

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

type memSecrets map[string]string

func (m memSecrets) Get(lib, key string) (string, error) {
	v, ok := m[lib+"/"+key]
	if !ok {
		return "", errors.New("no secret")
	}
	return v, nil
}
func (m memSecrets) Set(lib, key, value string) error { m[lib+"/"+key] = value; return nil }
func (m memSecrets) Delete(lib string) error {
	for k := range m {
		if len(k) > len(lib) && k[:len(lib)+1] == lib+"/" {
			delete(m, k)
		}
	}
	return nil
}

type localSource struct{ id string }

func (s localSource) Library() source.Library                             { return source.Library{ID: s.id} }
func (s localSource) Track(context.Context, string) (source.Track, error) { return source.Track{}, nil }
func (s localSource) StreamURL(context.Context, string) (string, error)   { return "/music/x.flac", nil }
func (s localSource) Close() error                                        { return nil }

type serverSource struct {
	localSource
	mu    sync.Mutex
	plays []string
}

func (s *serverSource) Ping(context.Context) (source.ServerInfo, error) {
	return source.ServerInfo{}, nil
}
func (s *serverSource) NowPlaying(context.Context, string) error { return nil }
func (s *serverSource) Scrobble(_ context.Context, id string, _ time.Time) error {
	s.mu.Lock()
	s.plays = append(s.plays, id)
	s.mu.Unlock()
	return nil
}

// A server that can't scrobble is still a server.
type mutedServer struct{ localSource }

func (mutedServer) Ping(context.Context) (source.ServerInfo, error) { return source.ServerInfo{}, nil }

type libs map[string]source.Source

func (l libs) Get(id string) (source.Source, error) {
	if s, ok := l[id]; ok {
		return s, nil
	}
	return nil, errors.New("library is down")
}

// fakeServices records what reaches ListenBrainz and Last.fm.
type fakeServices struct {
	mu      sync.Mutex
	lb, fm  []string
	offline bool
}

func (f *fakeServices) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/1/validate-token", func(w http.ResponseWriter, r *http.Request) {
		valid := r.Header.Get("Authorization") == "Token good"
		_ = json.NewEncoder(w).Encode(map[string]any{"valid": valid, "user_name": "ann"})
	})
	mux.HandleFunc("/1/submit-listens", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ListenType string `json:"listen_type"`
			Payload    []struct {
				Meta struct {
					Track string `json:"track_name"`
				} `json:"track_metadata"`
			} `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.offline {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		f.lb = append(f.lb, body.ListenType+" "+body.Payload[0].Meta.Track)
	})
	mux.HandleFunc("/2.0/", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		p := r.Form
		// Last.fm's signature, computed here on its own.
		var names []string
		for k := range p {
			if k != "format" && k != "api_sig" {
				names = append(names, k)
			}
		}
		sort.Strings(names)
		s := ""
		for _, k := range names {
			s += k + p.Get(k)
		}
		sum := md5.Sum([]byte(s + "shh"))
		if p.Get("api_sig") != hex.EncodeToString(sum[:]) {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": 13, "message": "Invalid method signature supplied"})
			return
		}
		switch p.Get("method") {
		case "auth.getToken":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "tok"})
		case "auth.getSession":
			_ = json.NewEncoder(w).Encode(map[string]any{"session": map[string]any{"name": "bob", "key": "sess"}})
		case "track.scrobble", "track.updateNowPlaying":
			if p.Get("sk") != "sess" {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": 9, "message": "Invalid session key"})
				return
			}
			f.mu.Lock()
			f.fm = append(f.fm, p.Get("method")+" "+p.Get("track"))
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	})
	return mux
}

func (f *fakeServices) setOffline(v bool) {
	f.mu.Lock()
	f.offline = v
	f.mu.Unlock()
}

func (f *fakeServices) take() (lb, fm []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	lb, fm = f.lb, f.fm
	f.lb, f.fm = nil, nil
	return lb, fm
}

func newTestService(t *testing.T, l libs, f *fakeServices, dir string) *Service {
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	return New(context.Background(), Config{ConfigDir: dir, StateDir: dir, Libraries: l, Secrets: secrets(t, dir),
		Publish: func(string, any) {}, ListenBrainzURL: srv.URL, LastFMURL: srv.URL + "/2.0/", LastFMAuthURL: "https://auth.example/"})
}

var secretStores = map[string]memSecrets{}

func secrets(t *testing.T, dir string) memSecrets {
	if s, ok := secretStores[dir]; ok {
		return s
	}
	s := memSecrets{}
	secretStores[dir] = s
	t.Cleanup(func() { delete(secretStores, dir) })
	return s
}

func track(lib, title string) source.Track {
	return source.Track{Library: lib, ID: title, Title: title, Artist: "x", Duration: 200}
}

func TestPlaysGoToTheRightPlace(t *testing.T) {
	ctx := context.Background()
	f := &fakeServices{}
	server := &serverSource{localSource: localSource{"srv"}}
	s := newTestService(t, libs{"loc": localSource{"loc"}, "srv": server, "mute": mutedServer{localSource{"mute"}}}, f, t.TempDir())

	require.Error(t, s.ConnectListenBrainz(ctx, "bad"))
	require.NoError(t, s.ConnectListenBrainz(ctx, "good"))
	page, err := s.StartLastFM(ctx, "key", "shh")
	require.NoError(t, err)
	assert.Equal(t, "https://auth.example/?api_key=key&token=tok", page)
	require.NoError(t, s.FinishLastFM(ctx))
	assert.Equal(t, "bob", s.Status().LastFM.User)

	s.report(track("loc", "local song"), false, time.Now())
	s.report(track("srv", "server song"), false, time.Now())
	s.report(track("mute", "unscrobbled server song"), false, time.Now())
	lb, fm := f.take()
	assert.Equal(t, []string{"single local song"}, lb)
	assert.Equal(t, []string{"track.scrobble local song"}, fm)
	assert.Equal(t, []string{"server song"}, server.plays, "server tracks go to their server only")

	require.NoError(t, s.SetLibrary("loc", false))
	s.report(track("loc", "muted library"), false, time.Now())
	lb, fm = f.take()
	assert.Empty(t, lb)
	assert.Empty(t, fm)
}

func TestFailedPlaysWaitAndSurviveARestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	f := &fakeServices{}
	l := libs{"loc": localSource{"loc"}}
	s := newTestService(t, l, f, dir)
	require.NoError(t, s.ConnectListenBrainz(ctx, "good"))

	f.setOffline(true)
	s.report(track("loc", "one"), false, time.Now())
	s.report(track("loc", "two"), false, time.Now())
	s.report(track("gone", "on a library that is down"), false, time.Now())
	assert.Equal(t, 3, s.Status().Queued)

	f.setOffline(false)
	// A new service retries the saved queue at start.
	s = newTestService(t, l, f, dir)
	assert.Eventually(t, func() bool { return s.Status().Queued == 1 }, 5*time.Second, 10*time.Millisecond,
		"sent what it could; the down library's play waits for it")
	lb, _ := f.take()
	assert.Equal(t, []string{"single one", "single two"}, lb)

	q := openQueue(dir + "/" + queueFile)
	for range maxQueued + 5 {
		q.add(pending{Target: ListenBrainz})
	}
	assert.Equal(t, maxQueued, q.len())
}

// Now playing goes out once per track start, also when repeat one starts
// the track over; played plays come from the player's played event.
func TestNowPlaying(t *testing.T) {
	f := &fakeServices{}
	s := newTestService(t, libs{"loc": localSource{"loc"}}, f, t.TempDir())
	require.NoError(t, s.ConnectListenBrainz(context.Background(), "good"))
	a, b := track("loc", "a"), track("loc", "b")
	s.PlayerStatus(player.Status{State: player.StatePaused, Track: &a})
	s.PlayerStatus(player.Status{State: player.StatePlaying, Track: &a})
	s.PlayerStatus(player.Status{State: player.StatePaused, Track: &a, Position: 10})
	s.PlayerStatus(player.Status{State: player.StatePlaying, Track: &a, Position: 10})
	for pos := 11.0; pos < 60; pos++ {
		s.Position(pos)
	}
	s.Position(0.5)
	s.PlayerStatus(player.Status{State: player.StatePlaying, Track: &b})
	s.Played(b, time.Now())
	var got []string
	assert.Eventually(t, func() bool {
		lb, _ := f.take()
		got = append(got, lb...)
		return len(got) >= 4
	}, 5*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	late, _ := f.take()
	got = append(got, late...)
	sort.Strings(got)
	assert.Equal(t, []string{"playing_now a", "playing_now a", "playing_now b", "single b"}, got)
}

// The build's Last.fm API account is used unless the user set their own.
func TestLastFMKeysPreferTheUsersOwn(t *testing.T) {
	old, oldSecret := LastFMAPIKey, LastFMAPISecret
	t.Cleanup(func() { LastFMAPIKey, LastFMAPISecret = old, oldSecret })
	sec := memSecrets{}
	s := &Service{cfg: Config{Secrets: sec}}

	LastFMAPIKey, LastFMAPISecret = "", ""
	_, err := s.apiKeys()
	assert.Error(t, err, "no key anywhere")

	LastFMAPIKey, LastFMAPISecret = "app", "appsecret"
	k, err := s.apiKeys()
	require.NoError(t, err)
	assert.Equal(t, lastFMKeys{key: "app", secret: "appsecret"}, k)

	sec[secretsFM+"/apiKey"], sec[secretsFM+"/apiSecret"] = "mine", "minesecret"
	k, err = s.apiKeys()
	require.NoError(t, err)
	assert.Equal(t, lastFMKeys{key: "mine", secret: "minesecret"}, k)
}
