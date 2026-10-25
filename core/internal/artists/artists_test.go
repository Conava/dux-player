package artists

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/playlists"
	"github.com/Conava/dux-player/core/internal/source"
)

var png = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

// fakeWeb answers like MusicBrainz, Wikidata, Wikipedia and Commons for
// "Known" and nothing else. down makes every reply a 503.
type fakeWeb struct {
	mu   sync.Mutex
	hits map[string]int
	down bool
}

func (f *fakeWeb) setDown(down bool) {
	f.mu.Lock()
	f.down = down
	f.mu.Unlock()
}

func (f *fakeWeb) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits[r.URL.Path]++
	down := f.down
	f.mu.Unlock()
	if down {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	q := r.URL.Query()
	switch r.URL.Path {
	case "/ws/2/artist/":
		if q.Get("query") == `artist:"Known"` {
			reply(map[string]any{"artists": []any{map[string]any{"id": "mb-known", "name": "Known", "score": 100}}})
			return
		}
		reply(map[string]any{"artists": []any{}})
	case "/ws/2/artist/mb-known":
		reply(map[string]any{"relations": []any{map[string]any{"type": "wikidata", "url": map[string]any{"resource": "https://www.wikidata.org/wiki/Q1"}}}})
	case "/wiki/Special:EntityData/Q1.json":
		reply(map[string]any{"entities": map[string]any{"Q1": map[string]any{
			"claims":    map[string]any{"P18": []any{map[string]any{"mainsnak": map[string]any{"datavalue": map[string]any{"value": "Known.png"}}}}},
			"sitelinks": map[string]any{"enwiki": map[string]any{"title": "Known (band)"}},
		}}})
	case "/wiki/Special:FilePath/Known.png":
		_, _ = w.Write(png)
	case "/api/rest_v1/page/summary/Known_(band)":
		reply(map[string]any{"extract": "Known is a band.", "content_urls": map[string]any{"desktop": map[string]any{"page": "https://en.wikipedia.org/wiki/Known"}}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// fakeLib is a server library with one artist and two playlists.
type fakeLib struct {
	source.Source
	info      source.ArtistInfo
	playlists []source.Playlist
	fetched   atomic.Int32
	// hang, when set, holds every playlist request until it closes.
	hang chan struct{}
	// fail makes listing the playlists fail.
	fail   bool
	listed atomic.Int32
}

func (f *fakeLib) Library() source.Library { return source.Library{ID: "srv"} }
func (f *fakeLib) ArtistInfo(context.Context, string) (source.ArtistInfo, error) {
	return f.info, nil
}
func (f *fakeLib) Playlists(context.Context) ([]source.Playlist, error) {
	f.listed.Add(1)
	if f.hang != nil {
		<-f.hang
	}
	if f.fail {
		return nil, errors.New("server down")
	}
	out := make([]source.Playlist, len(f.playlists))
	for i, p := range f.playlists {
		p.Tracks = nil
		out[i] = p
	}
	return out, nil
}
func (f *fakeLib) Playlist(_ context.Context, id string) (source.Playlist, error) {
	if f.hang != nil {
		<-f.hang
	}
	f.fetched.Add(1)
	for _, p := range f.playlists {
		if p.ID == id {
			return p, nil
		}
	}
	return source.Playlist{}, source.ErrNotFound
}

type noSecrets struct{}

func (noSecrets) Get(string, string) (string, error) { return "", source.ErrNotFound }
func (noSecrets) Set(string, string, string) error   { return nil }
func (noSecrets) Delete(string) error                { return nil }

func newService(t *testing.T, lib *fakeLib, online bool) (*Service, *fakeWeb) {
	web := &fakeWeb{hits: map[string]int{}}
	srv := httptest.NewServer(web)
	t.Cleanup(srv.Close)
	s := New(Config{
		Get:      func(string) (source.Source, error) { return lib, nil },
		Online:   func() bool { return online },
		Secrets:  noSecrets{},
		CacheDir: t.TempDir(),
	})
	s.online.mbGap = 0
	s.online.ep = Endpoints{MusicBrainz: srv.URL, Wikidata: srv.URL, Wikipedia: func(string) string { return srv.URL },
		Commons: srv.URL}
	return s, web
}

func TestOnlineArtistInfo(t *testing.T) {
	ctx := context.Background()
	lib := &fakeLib{info: source.ArtistInfo{Artist: source.Artist{Library: "srv", ID: "a", Name: "Known"}}}
	s, web := newService(t, lib, true)

	info, err := s.Info(ctx, "srv", "a")
	require.NoError(t, err)
	assert.Equal(t, "Known is a band.", info.Biography)
	assert.Equal(t, &source.Link{Name: "Wikipedia", URL: "https://en.wikipedia.org/wiki/Known"}, info.BiographySource)
	assert.Equal(t, "mb-known", info.MusicBrainzID)

	path, from, err := s.Image(ctx, "srv", "a", "Known", "", 0)
	require.NoError(t, err)
	assert.Equal(t, "Wikimedia Commons", from)
	assert.Equal(t, filepath.Join(s.online.dir, cacheKey("Known")+".png"), path)

	_, _, err = s.Image(ctx, "srv", "c", "Nobody", "", 0)
	require.EqualError(t, err, "no image")
	before := web.hits["/ws/2/artist/"]
	_, _, err = s.Image(ctx, "srv", "c", "Nobody", "", 0)
	require.EqualError(t, err, "no image")
	_, _, err = s.Image(ctx, "srv", "a", "Known", "", 0)
	require.NoError(t, err)
	assert.Equal(t, before, web.hits["/ws/2/artist/"], "found and not found are both cached")

	web.setDown(true)
	_, _, err = s.Image(ctx, "srv", "d", "Later", "", 0)
	require.Error(t, err)
	web.setDown(false)
	_, _, err = s.Image(ctx, "srv", "d", "Later", "", 0)
	require.EqualError(t, err, "no image", "a failed request is not cached as not found")
	assert.Equal(t, before+2, web.hits["/ws/2/artist/"])
}

func TestOnlineOff(t *testing.T) {
	ctx := context.Background()
	lib := &fakeLib{info: source.ArtistInfo{Artist: source.Artist{Library: "srv", ID: "a", Name: "Known"}}}
	s, web := newService(t, lib, false)
	info, err := s.Info(ctx, "srv", "a")
	require.NoError(t, err)
	assert.Empty(t, info.Biography)
	_, _, err = s.Image(ctx, "srv", "a", "Known", "", 0)
	assert.EqualError(t, err, "no image")
	assert.Empty(t, web.hits, "nothing goes online")
}

func TestArtistPlaylists(t *testing.T) {
	ctx := context.Background()
	ann := []source.ArtistRef{{ID: "ann", Name: "Ann"}, {ID: "bob", Name: "Bob"}}
	lib := &fakeLib{playlists: []source.Playlist{
		{ID: "p1", Name: "Mix", TrackCount: 2, Tracks: []source.Track{{Library: "srv", ID: "1", Artists: ann}, {Library: "srv", ID: "2", ArtistID: "cy"}}},
		{ID: "p2", Name: "Other", TrackCount: 1, Tracks: []source.Track{{Library: "srv", ID: "3", ArtistID: "cy"}}},
	}}
	s, _ := newService(t, lib, false)
	store, err := playlists.Open(ctx, filepath.Join(t.TempDir(), "playlists.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	s.cfg.Local = func() *playlists.Store { return store }
	// a local track of another library, matched by name
	_, err = store.Create(ctx, "Local", []source.Track{{Library: "loc", ID: "x", Artist: "bob"}})
	require.NoError(t, err)
	_, err = store.Create(ctx, "Empty", nil)
	require.NoError(t, err)

	hits, err := s.Playlists(ctx, "srv", "bob", "Bob")
	require.NoError(t, err)
	var names []string
	for _, h := range hits {
		names = append(names, h.Name)
		assert.Equal(t, 1, h.Matches)
	}
	assert.Equal(t, []string{"Local", "Mix"}, names)

	_, err = s.Playlists(ctx, "srv", "bob", "Bob")
	require.NoError(t, err)
	assert.Equal(t, int32(2), lib.fetched.Load(), "unchanged server playlists aren't fetched again")
}

// Once indexed, a visit answers from the index while the server hangs; a
// playlist edited through the player starts a refresh.
func TestArtistPlaylistsDontWaitForTheServer(t *testing.T) {
	ctx := context.Background()
	lib := &fakeLib{playlists: []source.Playlist{
		{ID: "p1", Name: "Mix", TrackCount: 1, Tracks: []source.Track{{Library: "srv", ID: "1", ArtistID: "bob"}}},
	}}
	s, _ := newService(t, lib, false)
	hits, err := s.Playlists(ctx, "srv", "bob", "")
	require.NoError(t, err)
	require.Len(t, hits, 1)

	lib.hang = make(chan struct{})
	// The refresh ends, index written, before the temp dir is removed.
	t.Cleanup(func() {
		s.mu.Lock()
		running := s.index["srv"].running
		s.mu.Unlock()
		close(lib.hang)
		if running != nil {
			<-running
		}
	})
	s.Invalidate("srv", "p1")
	done := make(chan []PlaylistHit)
	go func() {
		hits, _ := s.Playlists(ctx, "srv", "bob", "")
		done <- hits
	}()
	select {
	case hits := <-done:
		assert.Len(t, hits, 1)
	case <-time.After(indexWait / 2):
		t.Fatal("the visit waited for the server")
	}
	s.mu.Lock()
	assert.NotNil(t, s.index["srv"].running, "the edit started a refresh")
	s.mu.Unlock()
}

// A failed refresh isn't retried on every visit, and an edit made while a
// refresh ran is refetched by the next one.
func TestArtistPlaylistRefreshes(t *testing.T) {
	ctx := context.Background()
	t.Run("failure backs off", func(t *testing.T) {
		lib := &fakeLib{fail: true}
		s, _ := newService(t, lib, false)
		for range 3 {
			_, err := s.Playlists(ctx, "srv", "bob", "")
			require.NoError(t, err)
		}
		assert.Equal(t, int32(1), lib.listed.Load())
	})
	t.Run("edit during a refresh", func(t *testing.T) {
		lib := &fakeLib{playlists: []source.Playlist{
			{ID: "p1", Name: "Mix", TrackCount: 1, Tracks: []source.Track{{Library: "srv", ID: "1", ArtistID: "bob"}}},
		}}
		s, _ := newService(t, lib, false)
		lib.hang = make(chan struct{})
		go func() { _, _ = s.Playlists(ctx, "srv", "bob", "") }()
		require.Eventually(t, func() bool { return lib.listed.Load() == 1 }, time.Second, time.Millisecond)
		s.mu.Lock()
		running := s.index["srv"].running
		s.mu.Unlock()
		s.Invalidate("srv", "p1")
		close(lib.hang)
		<-running
		s.mu.Lock()
		assert.Zero(t, s.index["srv"].Listed, "the edit is refetched on the next visit")
		s.mu.Unlock()
	})
}
