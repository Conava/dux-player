package cache

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/source"
)

// server is a remote library whose every track is 40 bytes of its id.
type server struct {
	lib source.Library
	url string
	*catalog
}

// catalog is what the fake server lists; fail makes every listing fail.
type catalog struct {
	mu        sync.Mutex
	albums    map[string][]string
	artists   map[string][]string
	playlists map[string][]string
	favs      []string
	fail      bool
	// audio maps a track id to a file served instead of the 40 bytes.
	audio map[string]string
	// hits counts the downloads of each track; a track in hold is sent
	// only once its channel closes.
	hits map[string]int
	hold map[string]chan struct{}
	// broken answers this many downloads of a track with an error.
	broken map[string]int
}

func (s *server) tracks(ids []string) []source.Track {
	out := []source.Track{}
	for _, id := range ids {
		out = append(out, source.Track{Library: s.lib.ID, ID: id, Title: id})
	}
	return out
}

func (s *server) err() error {
	if s.fail {
		return errors.New("server down")
	}
	return nil
}

func (s *server) Library() source.Library { return s.lib }
func (s *server) Close() error            { return nil }
func (s *server) Track(_ context.Context, id string) (source.Track, error) {
	return source.Track{Library: s.lib.ID, ID: id, Title: id}, nil
}
func (s *server) StreamURL(_ context.Context, id string) (string, error) {
	return s.url + "/" + id, nil
}
func (s *server) Ping(context.Context) (source.ServerInfo, error)         { return source.ServerInfo{}, nil }
func (s *server) SetFavorite(context.Context, string, string, bool) error { return nil }
func (s *server) Favorites(context.Context) (source.SearchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return source.SearchResult{Tracks: s.tracks(s.favs)}, s.err()
}
func (s *server) Artists(context.Context) ([]source.Artist, error) { return nil, nil }
func (s *server) Albums(_ context.Context, artist string) ([]source.Album, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []source.Album
	for id := range s.albums {
		if artist == "" {
			out = append(out, source.Album{ID: id})
		}
	}
	for _, id := range s.artists[artist] {
		out = append(out, source.Album{ID: id})
	}
	return out, s.err()
}
func (s *server) Album(_ context.Context, id string) (source.Album, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return source.Album{ID: id, Tracks: s.tracks(s.albums[id])}, s.err()
}
func (s *server) Search(context.Context, string, int) (source.SearchResult, error) {
	return source.SearchResult{}, nil
}
func (s *server) Playlists(context.Context) ([]source.Playlist, error) { return nil, nil }
func (s *server) Playlist(_ context.Context, id string) (source.Playlist, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return source.Playlist{ID: id, Tracks: s.tracks(s.playlists[id])}, s.err()
}

type noSecrets struct{}

func (noSecrets) Get(string, string) (string, error) { return "", nil }
func (noSecrets) Set(string, string, string) error   { return nil }
func (noSecrets) Delete(string) error                { return nil }

type env struct {
	c   *Cache
	m   *library.Manager
	lib string
	cat *catalog
}

func newCatalog() *catalog {
	return &catalog{
		albums:    map[string][]string{"A1": {"a", "b"}, "A2": {"c"}},
		artists:   map[string][]string{"R": {"A2"}},
		playlists: map[string][]string{"P": {"b", "d"}},
		favs:      []string{"e"},
		hits:      map[string]int{},
		hold:      map[string]chan struct{}{},
		broken:    map[string]int{},
	}
}

// setup opens a cache over one fake server library; dir holds the cache,
// libCfg is the library's config and prep runs before the cache opens, for
// migrations.
func setup(t *testing.T, dir string, libCfg map[string]any, prep func(lib string)) *env {
	t.Helper()
	ctx := context.Background()
	cat := newCatalog()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cat.mu.Lock()
		file := cat.audio[r.URL.Path[1:]]
		cat.hits[r.URL.Path[1:]]++
		hold := cat.hold[r.URL.Path[1:]]
		broken := cat.broken[r.URL.Path[1:]] > 0
		if broken {
			cat.broken[r.URL.Path[1:]]--
		}
		cat.mu.Unlock()
		if broken {
			http.Error(w, "broken", http.StatusInternalServerError)
			return
		}
		if hold != nil {
			select {
			case <-hold:
			case <-r.Context().Done():
				return
			}
		}
		if file != "" {
			http.ServeFile(w, r, file)
			return
		}
		_, _ = w.Write([]byte(strings.Repeat(r.URL.Path[1:], 40)[:40]))
	}))
	t.Cleanup(srv.Close)
	store, err := library.Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	reg := source.Registry{"fake": func(lib source.Library, _ source.Env) (source.Source, error) {
		return &server{lib: lib, url: srv.URL, catalog: cat}, nil
	}}
	m := library.NewManager(store, reg, source.Env{Secrets: noSecrets{}})
	if libCfg == nil {
		libCfg = map[string]any{}
	}
	lib, err := m.Add(ctx, "fake", "Srv", map[string]any{}, nil)
	require.NoError(t, err)
	for k, v := range libCfg {
		require.NoError(t, m.SetConfig(ctx, lib.ID, k, v))
	}
	if dir == "" {
		dir = filepath.Join(t.TempDir(), "tracks")
	}
	if prep != nil {
		prep(lib.ID)
	}
	c, err := Open(ctx, Config{Dir: dir, ConfigDir: t.TempDir(), Libraries: m})
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })
	m.SetOffline(c)
	c.limit = 1000
	return &env{c: c, m: m, lib: lib.ID, cat: cat}
}

func (e *env) setRules(t *testing.T, rules ...Rule) {
	t.Helper()
	require.NoError(t, e.m.SetConfig(context.Background(), e.lib, configKey, map[string]any{"rules": rules}))
}

// drain downloads everything queued, as Run would.
func (e *env) drain(t *testing.T) {
	t.Helper()
	for {
		e.c.mu.Lock()
		if len(e.c.queue) == 0 {
			e.c.mu.Unlock()
			return
		}
		j := e.c.queue[0]
		e.c.queue = e.c.queue[1:]
		delete(e.c.queued, j.track.Ref())
		e.c.mu.Unlock()
		require.NoError(t, e.c.fetch(context.Background(), j))
	}
}

func (e *env) sync(t *testing.T) {
	t.Helper()
	e.c.Sync(context.Background(), e.lib, all)
	e.drain(t)
}

func cached(t *testing.T, c *Cache) []string {
	ids, err := column(context.Background(), c.db, "SELECT id FROM entries")
	require.NoError(t, err)
	if ids == nil {
		ids = []string{}
	}
	return ids
}

func (e *env) play(t *testing.T, id string) {
	t.Helper()
	e.c.Played(context.Background(), source.Track{Library: e.lib, ID: id})
	e.drain(t)
}

func album(id string) Rule    { return Rule{Kind: KindAlbum, ID: id} }
func playlist(id string) Rule { return Rule{Kind: KindPlaylist, ID: id} }
func played(l Limit) Rule     { return Rule{Kind: KindPlayed, Limit: &l} }

// Removing a rule, or a track leaving a rule's scope, deletes only the
// copies no other rule holds.
func TestRuleCoverage(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		rules  []Rule
		before func(e *env)
		change func(t *testing.T, e *env)
		want   []string
	}{
		{name: "another rule still holds a track", rules: []Rule{album("A1"), playlist("P")},
			change: func(t *testing.T, e *env) { require.NoError(t, e.c.RemoveRule(ctx, e.lib, playlist("P"))) },
			want:   []string{"a", "b"}},
		{name: "a removed rule's own tracks go", rules: []Rule{album("A1")},
			change: func(t *testing.T, e *env) { require.NoError(t, e.c.RemoveRule(ctx, e.lib, album("A1"))) },
			want:   []string{}},
		{name: "a track leaving a playlist goes", rules: []Rule{playlist("P")},
			change: func(t *testing.T, e *env) { e.cat.playlists["P"] = []string{"b"}; e.sync(t) },
			want:   []string{"b"}},
		{name: "an unfavorited track goes", rules: []Rule{{Kind: KindFavorites}, album("A1")},
			change: func(t *testing.T, e *env) { e.cat.favs = []string{"a"}; e.c.Favorited(ctx, e.lib) },
			want:   []string{"a", "b"}},
		{name: "a failing server shrinks nothing", rules: []Rule{playlist("P"), {Kind: KindEverything}},
			change: func(t *testing.T, e *env) { e.cat.fail = true; e.sync(t) },
			want:   []string{"a", "b", "c", "d"}},
		{name: "played keeps what it holds", rules: []Rule{album("A1"), played(Limit{Songs: 10})},
			before: func(e *env) { e.c.Played(ctx, source.Track{Library: e.lib, ID: "a"}) },
			change: func(t *testing.T, e *env) { require.NoError(t, e.c.RemoveRule(ctx, e.lib, album("A1"))) },
			want:   []string{"a"}},
		{name: "removing played deletes what only it held", rules: []Rule{album("A2"), played(Limit{Songs: 10})},
			before: func(e *env) {
				e.c.Played(ctx, source.Track{Library: e.lib, ID: "c"})
				e.c.Played(ctx, source.Track{Library: e.lib, ID: "x"})
			},
			change: func(t *testing.T, e *env) { require.NoError(t, e.c.RemoveRule(ctx, e.lib, Rule{Kind: KindPlayed})) },
			want:   []string{"c"}},
		{name: "an artist rule keeps its albums", rules: []Rule{{Kind: KindArtist, ID: "R"}, album("A1")},
			change: func(t *testing.T, e *env) { require.NoError(t, e.c.RemoveRule(ctx, e.lib, album("A1"))) },
			want:   []string{"c"}},
		{name: "a kept local playlist holds its server tracks", rules: []Rule{album("A1")},
			before: func(e *env) {
				e.c.cfg.LocalPlaylist = func(context.Context, string) ([]source.Track, error) {
					return []source.Track{{Library: e.lib, ID: "a"}, {Library: "elsewhere", ID: "z"}}, nil
				}
				require.NoError(t, e.c.writeKey("localPlaylists", []string{"L"}))
			},
			change: func(t *testing.T, e *env) { require.NoError(t, e.c.RemoveRule(ctx, e.lib, album("A1"))) },
			want:   []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := setup(t, "", nil, nil)
			e.setRules(t, tt.rules...)
			if tt.before != nil {
				tt.before(e)
			}
			e.sync(t)
			tt.change(t, e)
			e.drain(t)
			assert.ElementsMatch(t, tt.want, cached(t, e.c))
		})
	}
}

// The size limit evicts only dynamic copies, least recently played first;
// a pinned copy that doesn't fit is dropped and stops syncing.
func TestEviction(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		limit    int64
		rules    []Rule
		plays    []string
		want     []string
		full     bool
		overPins bool
	}{
		{name: "least recently played goes first", limit: 120, rules: []Rule{album("A2"), played(Limit{Songs: 10})},
			plays: []string{"x", "y", "z"}, want: []string{"c", "y", "z"}},
		{name: "pinned copies never go for played ones", limit: 80, rules: []Rule{album("A1"), played(Limit{Songs: 10})},
			plays: []string{"x"}, want: []string{"a", "b"}},
		{name: "a pinned copy that doesn't fit stops syncing", limit: 80, rules: []Rule{album("A1"), album("A2")},
			want: []string{"a", "b"}, full: true},
		{name: "a played track already pinned stays", limit: 80, rules: []Rule{album("A1"), played(Limit{Songs: 10})},
			plays: []string{"a"}, want: []string{"a", "b"}},
		{name: "a played track a rule pins stays over the limit", limit: 80, rules: []Rule{album("A1"), album("A2"), played(Limit{Songs: 10})},
			plays: []string{"c"}, want: []string{"a", "b", "c"}, full: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := setup(t, "", nil, nil)
			e.c.limit = tt.limit
			e.setRules(t, tt.rules...)
			e.sync(t)
			for _, id := range tt.plays {
				e.play(t, id)
				_, err := e.c.db.Exec("UPDATE entries SET played = played - 10 WHERE played > 0")
				require.NoError(t, err)
			}
			assert.ElementsMatch(t, tt.want, cached(t, e.c))
			st, err := e.c.Status(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.full, st.Full)
		})
	}
}

func TestLowerLimitKeepsPinned(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "", nil, nil)
	e.setRules(t, album("A1"), played(Limit{Songs: 10}))
	e.sync(t)
	e.play(t, "x")
	e.c.limit = 40
	require.NoError(t, e.c.evict(ctx, source.Ref{}, false))
	assert.ElementsMatch(t, []string{"a", "b"}, cached(t, e.c))
	st, err := e.c.Status(ctx)
	require.NoError(t, err)
	assert.True(t, st.PinnedOverLimit)
}

// insert adds a copy without a file, size and last play given.
func (e *env) insert(t *testing.T, id string, size, played int64) {
	t.Helper()
	_, err := e.c.db.Exec(`INSERT INTO entries (library_id, id, file, size, added, used, track, played)
		VALUES (?, ?, ?, ?, 1, 1, '{}', ?)`, e.lib, id, filepath.Join(t.TempDir(), id), size, played)
	require.NoError(t, err)
}

// Dynamic rules hold up to their own limit, a song count or a size, and let
// go of what falls out.
func TestDynamicLimits(t *testing.T) {
	const mb = 1 << 20
	tests := []struct {
		name          string
		kind          string
		before, after Limit
		top           [2][]string
		want          []string
	}{
		{name: "played by songs", kind: KindPlayed, before: Limit{Songs: 3}, after: Limit{Songs: 2}, want: []string{"q", "r"}},
		{name: "played by size", kind: KindPlayed, before: Limit{Bytes: 30 * mb}, after: Limit{Bytes: 20 * mb}, want: []string{"q", "r"}},
		{name: "most played by songs", kind: KindMostPlayed, before: Limit{Songs: 3}, after: Limit{Songs: 2},
			top: [2][]string{{"p", "q", "r"}, {"r", "p", "q"}}, want: []string{"r", "p"}},
		{name: "most played by size", kind: KindMostPlayed, before: Limit{Bytes: 30 * mb}, after: Limit{Bytes: 20 * mb},
			top: [2][]string{{"p", "q", "r"}, {"r", "p", "q"}}, want: []string{"r", "p"}},
		{name: "most played moves with the counts", kind: KindMostPlayed, before: Limit{Songs: 2}, after: Limit{Songs: 2},
			top: [2][]string{{"p", "q"}, {"q", "r"}}, want: []string{"q", "r"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			e := setup(t, "", nil, nil)
			e.c.limit = 1 << 30
			for i, id := range []string{"p", "q", "r"} {
				e.insert(t, id, 8*mb, int64(100+i))
			}
			top := tt.top[0]
			e.c.cfg.MostPlayed = func(context.Context) ([]source.Track, error) {
				var out []source.Track
				for _, id := range top {
					out = append(out, source.Track{Library: e.lib, ID: id}, source.Track{Library: "other", ID: id})
				}
				return out, nil
			}
			e.setRules(t, Rule{Kind: tt.kind, Limit: &tt.before})
			e.c.refresh(ctx, e.lib, true)
			require.ElementsMatch(t, []string{"p", "q", "r"}, cached(t, e.c))

			top = tt.top[1]
			e.setRules(t, Rule{Kind: tt.kind, Limit: &tt.after})
			e.c.refresh(ctx, e.lib, true)
			e.drain(t)
			assert.ElementsMatch(t, tt.want, cached(t, e.c))
		})
	}
}

func TestMigrateConfig(t *testing.T) {
	tests := []struct {
		name string
		old  map[string]any
		want []Rule
		keep bool
	}{
		{name: "played", old: map[string]any{"mode": "played"}, want: []Rule{played(Limit{Bytes: 5 << 30})}},
		{name: "favorites with playlists", old: map[string]any{"mode": "favorites", "playlists": []any{"P", "Q"}},
			want: []Rule{{Kind: KindFavorites}, playlist("P"), playlist("Q")}},
		{name: "everything", old: map[string]any{"mode": "all", "playlists": []any{}}, want: []Rule{{Kind: KindEverything}}},
		{name: "chosen playlists only", old: map[string]any{"mode": "playlists", "playlists": []any{"P"}}, want: []Rule{playlist("P")}},
		{name: "off", old: map[string]any{"mode": "off"}, want: []Rule{}},
		{name: "already rules", old: map[string]any{"rules": []any{}, "mode": "all"}, keep: true},
		{name: "nothing set", old: nil, keep: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := migrateConfig(tt.old, 5<<30)
			require.Equal(t, !tt.keep, ok)
			if tt.keep {
				return
			}
			assert.NotContains(t, got, "mode")
			assert.NotContains(t, got, "playlists")
			assert.Equal(t, tt.want, got["rules"])
		})
	}
}

// Moving an older cache to rules deletes none of its copies, also those no
// rule covers.
func TestMigrationKeepsCopies(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "tracks")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "x"), 0o700))
	var files []string
	old := func(lib string) {
		db, err := library.OpenDB(ctx, filepath.Join(dir, "index.db"), migrations[:1])
		require.NoError(t, err)
		defer db.Close()
		for _, id := range []string{"b", "z"} {
			f := filepath.Join(dir, "x", id)
			require.NoError(t, os.WriteFile(f, []byte("old"), 0o600))
			files = append(files, f)
			_, err := db.Exec(`INSERT INTO entries (library_id, id, file, size, added, used, track) VALUES (?, ?, ?, 3, 1, 50, '{}')`,
				lib, id, f)
			require.NoError(t, err)
		}
	}
	e := setup(t, dir, map[string]any{configKey: map[string]any{"mode": "favorites", "playlists": []any{"P"}, "extra": true}}, old)
	lib, err := e.m.Library(ctx, e.lib)
	require.NoError(t, err)
	cfg := lib.Config[configKey].(map[string]any)
	assert.Equal(t, true, cfg["extra"], "unknown keys survive")
	assert.Len(t, parseRules(cfg), 2)

	var playedAt int64
	require.NoError(t, e.c.db.QueryRow("SELECT played FROM entries WHERE id = 'z'").Scan(&playedAt))
	assert.Equal(t, int64(50), playedAt, "an old copy counts as played when last used")

	e.sync(t)
	assert.ElementsMatch(t, []string{"b", "d", "e", "z"}, cached(t, e.c))
	for _, f := range files {
		assert.FileExists(t, f)
	}
}

func TestSweepDropsLeftovers(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "", nil, nil)
	require.NoError(t, e.c.fetch(ctx, job{track: source.Track{Library: e.lib, ID: "a"}}))
	st, ok := e.c.Cached(ctx, source.Ref{Library: e.lib, ID: "a"})
	require.True(t, ok)
	f := st.URL
	part := filepath.Join(filepath.Dir(f), "x.part")
	require.NoError(t, os.WriteFile(part, []byte("half"), 0o600))
	probe := filepath.Join(e.c.cfg.Dir, "probe.part")
	require.NoError(t, os.WriteFile(probe, []byte("half"), 0o600))
	e.c.sweep(ctx)
	assert.NoFileExists(t, part)
	assert.NoFileExists(t, probe)
	assert.FileExists(t, f)
	require.NoError(t, os.Remove(f))
	_, ok = e.c.Cached(ctx, source.Ref{Library: e.lib, ID: "a"})
	assert.False(t, ok, "a copy deleted behind our back is forgotten")
}

// Removing a rule stops its queued downloads, and removing the library
// drops its copies.
func TestRemoveStopsQueueAndLibrary(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "", nil, nil)
	e.setRules(t, album("A1"))
	e.c.Sync(ctx, e.lib, all)
	require.NoError(t, e.c.RemoveRule(ctx, e.lib, album("A1")))
	e.c.mu.Lock()
	assert.Empty(t, e.c.queue)
	e.c.mu.Unlock()

	require.NoError(t, e.c.fetch(ctx, job{track: source.Track{Library: e.lib, ID: "a"}}))
	st, ok := e.c.Cached(ctx, source.Ref{Library: e.lib, ID: "a"})
	require.True(t, ok)
	f := st.URL
	require.NoError(t, e.m.Remove(ctx, e.lib))
	assert.Empty(t, cached(t, e.c))
	assert.NoFileExists(t, f)
}

// Server tracks without ReplayGain values that aren't cached: the first
// play has no gain, so a loud and a quiet track play as they are; a probe
// of the stream's start gives each its gain for the rest of the track and
// later plays.
func TestProbeEvensOutUncachedServerTracks(t *testing.T) {
	meter := loudness.Find()
	if !meter.Available() {
		t.Skip("ffmpeg not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	e := setup(t, "", nil, nil)
	e.c.cfg.Meter = meter
	dir := t.TempDir()
	audio := map[string]string{}
	for id, vol := range map[string]string{"loud": "18dB", "quiet": "6dB"} {
		audio[id] = filepath.Join(dir, id+".flac")
		gen := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "sine=f=440:r=8000:d=20",
			"-af", "volume="+vol, audio[id])
		require.NoError(t, gen.Run())
	}
	e.cat.mu.Lock()
	e.cat.audio = audio
	e.cat.mu.Unlock()

	gains := map[string]float64{}
	for id := range audio {
		ref := source.Ref{Library: e.lib, ID: id}
		st, err := e.m.Stream(ctx, ref)
		require.NoError(t, err)
		assert.Nil(t, st.GainDB, "nothing is known before the probe")
		done := make(chan float64, 1)
		e.m.Probe(ctx, ref, func(g float64) { done <- g })
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("the probe did not finish")
		}
		st, err = e.m.Stream(ctx, ref)
		require.NoError(t, err)
		require.NotNil(t, st.GainDB, "a later play has the probed gain")
		gains[id] = *st.GainDB
	}
	assert.InDelta(t, 12, gains["quiet"]-gains["loud"], 1, "the quiet track is raised by what it lacks")
}

// Dropping stale coverage needs the library's rules: one that went away
// mid-sync keeps what its rules hold.
func TestDropStaleNeedsTheRules(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		change func(t *testing.T, e *env)
		lost   []string
	}{
		{"library disabled", func(t *testing.T, e *env) { require.NoError(t, e.m.SetEnabled(ctx, e.lib, false)) }, nil},
		{"rule removed", func(t *testing.T, e *env) { e.setRules(t) }, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := setup(t, "", nil, nil)
			e.setRules(t, album("A1"))
			e.sync(t)
			tt.change(t, e)
			lost, err := e.c.dropStale(ctx, e.lib)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.lost, lost)
		})
	}
}

// A server that stops sending mid-file frees the worker long before the
// whole download's timeout.
func TestStalledDownloadGivesUp(t *testing.T) {
	e := setup(t, "", nil, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	e.c.stall = 50 * time.Millisecond
	src := &server{lib: source.Library{ID: e.lib}, url: srv.URL, catalog: e.cat}
	start := time.Now()
	_, err := e.c.download(context.Background(), src, "a", filepath.Join(t.TempDir(), "a"), 0)
	require.ErrorIs(t, err, errStalled)
	assert.Less(t, time.Since(start), 5*time.Second)
}

// An unreadable cache.json is moved aside, not overwritten.
func TestUnreadableSettingsAreKept(t *testing.T) {
	e := setup(t, "", nil, nil)
	path := limitPath(e.c.cfg.ConfigDir)
	require.NoError(t, os.WriteFile(path, []byte(`{"localPlaylists": ["L"`), 0o600))
	require.NoError(t, e.c.SetLimit(context.Background(), minLimit))
	bad, err := os.ReadFile(path + ".bad")
	require.NoError(t, err)
	assert.Equal(t, `{"localPlaylists": ["L"`, string(bad))
	assert.Equal(t, int64(minLimit), loadLimit(e.c.cfg.ConfigDir))
}
