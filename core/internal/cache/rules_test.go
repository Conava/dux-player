package cache

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/source"
)

// addServer adds another fake server library over the env's catalog.
func (e *env) addServer(t *testing.T, name string) string {
	t.Helper()
	lib, err := e.m.Add(context.Background(), "fake", name, map[string]any{}, nil)
	require.NoError(t, err)
	return lib.ID
}

// copies lists the cached tracks as library/id.
func copies(t *testing.T, c *Cache) []string {
	t.Helper()
	refs, err := column(context.Background(), c.db, "SELECT library_id || '/' || id FROM entries")
	require.NoError(t, err)
	if refs == nil {
		refs = []string{}
	}
	return refs
}

// settle waits for the syncs a rule change started and downloads what they
// queued.
func (e *env) settle(t *testing.T) {
	t.Helper()
	e.c.work.Wait()
	e.drain(t)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	m := map[string]any{}
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

// The rules move from the library configs to cache.json with their scope;
// the configs keep only what this version doesn't know, a copy covered
// before stays covered, and a second open changes nothing.
func TestMigrationMovesRulesToCacheJSON(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "tracks")
	var second, file string
	prep := func(e *env) {
		second = e.addServer(t, "Old")
		require.NoError(t, e.m.SetConfig(ctx, second, configKey, map[string]any{"mode": "favorites", "other": "x"}))
		require.NoError(t, os.WriteFile(limitPath(e.cfgDir), []byte(`{"future": 1, "limitBytes": 200000000}`), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "x"), 0o700))
		db, err := library.OpenDB(ctx, filepath.Join(dir, "index.db"), migrations)
		require.NoError(t, err)
		defer db.Close()
		file = filepath.Join(dir, "x", "a")
		require.NoError(t, os.WriteFile(file, []byte("old"), 0o600))
		_, err = db.Exec(`INSERT INTO entries (library_id, id, file, size, added, used, track) VALUES (?, 'a', ?, 3, 1, 1, '{}')`, e.lib, file)
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO covers (library_id, id, rule, static) VALUES (?, 'a', 'album:A1', 1)`, e.lib)
		require.NoError(t, err)
	}
	e := setup(t, dir, map[string]any{configKey: map[string]any{"rules": []any{album("A1")}, "extra": true}}, prep)

	assert.ElementsMatch(t, []Rule{{Kind: KindAlbum, ID: "A1", Library: e.lib}, {Kind: KindFavorites, Library: second}}, e.c.ruleList)
	saved := readJSON(t, limitPath(e.cfgDir))
	assert.Equal(t, float64(1), saved["future"], "unknown keys in cache.json survive")
	assert.Len(t, saved["rules"], 2)
	for id, want := range map[string]any{e.lib: map[string]any{"extra": true}, second: map[string]any{"other": "x"}} {
		lib, err := e.m.Library(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, lib.Config[configKey])
	}

	e.sync(t)
	assert.Contains(t, copies(t, e.c), e.lib+"/a")
	assert.FileExists(t, file)
	assert.Zero(t, e.cat.hits["a"], "the covered copy isn't fetched again")
	keys, err := column(ctx, e.c.db, "SELECT rule FROM covers WHERE id = 'a'")
	require.NoError(t, err)
	assert.Equal(t, []string{"album:A1"}, keys, "a rule keeps its covers key")

	before, err := os.ReadFile(limitPath(e.cfgDir))
	require.NoError(t, err)
	again, err := Open(ctx, Config{Dir: dir, ConfigDir: e.cfgDir, Libraries: e.m})
	require.NoError(t, err)
	defer again.Close()
	after, err := os.ReadFile(limitPath(e.cfgDir))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
	assert.ElementsMatch(t, e.c.ruleList, again.ruleList)
}

// A favorites rule for all servers covers every enabled server library,
// also one added after it.
func TestAllServersRuleCoversEveryLibrary(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "", nil, nil)
	b := e.addServer(t, "B")
	off := e.addServer(t, "Off")
	require.NoError(t, e.m.SetEnabled(ctx, off, false))
	e.c.LibrariesChanged(ctx)

	require.NoError(t, e.c.AddRule(ctx, "", Rule{Kind: KindFavorites}))
	e.settle(t)
	assert.ElementsMatch(t, []string{e.lib + "/e", b + "/e"}, copies(t, e.c))

	later := e.addServer(t, "Later")
	e.c.LibrariesChanged(ctx)
	e.drain(t)
	assert.ElementsMatch(t, []string{e.lib + "/e", b + "/e", later + "/e"}, copies(t, e.c))

	refs, err := e.c.List(ctx, b)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, []RuleRef{{Key: "all:favorites", Kind: KindFavorites, Global: true}}, refs[0].Rules)
	st, err := e.c.Status(ctx)
	require.NoError(t, err)
	require.Len(t, st.Rules, 1)
	assert.Equal(t, 3, st.Rules[0].Cached, "an all-servers rule counts the copies of every library")
}

// Played for all servers keeps the most recently played copies of every
// library together: a play in one library pushes out a copy in another.
func TestAllServersPlayedRanksAcrossLibraries(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "", nil, nil)
	b := e.addServer(t, "B")
	require.NoError(t, e.c.AddRule(ctx, "", played(Limit{Songs: 2})))
	e.settle(t)
	for _, ref := range []source.Ref{{Library: e.lib, ID: "x"}, {Library: e.lib, ID: "z"}, {Library: b, ID: "y"}} {
		e.c.Played(ctx, source.Track{Library: ref.Library, ID: ref.ID})
		e.drain(t)
		_, err := e.c.db.Exec("UPDATE entries SET played = played - 10 WHERE played > 0")
		require.NoError(t, err)
	}
	assert.ElementsMatch(t, []string{e.lib + "/z", b + "/y"}, copies(t, e.c))
}

// Removing a library drops the rules scoped to it, not those for all
// servers.
func TestRemovedLibraryLosesItsRules(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "", nil, nil)
	b := e.addServer(t, "B")
	e.setRules(t, album("A1"), Rule{Kind: KindAlbum, ID: "A2", Library: b}, Rule{Kind: KindEverything, Library: b})
	require.NoError(t, e.c.AddRule(ctx, "", Rule{Kind: KindFavorites}))
	e.settle(t)

	require.NoError(t, e.m.Remove(ctx, b))
	e.c.LibrariesChanged(ctx)
	want := []Rule{{Kind: KindAlbum, ID: "A1", Library: e.lib}, {Kind: KindFavorites}}
	assert.ElementsMatch(t, want, e.c.ruleList)
	saved, _ := parseRules(readJSON(t, limitPath(e.cfgDir))["rules"], "")
	assert.ElementsMatch(t, want, saved)
}

// Moving a rule to all servers and back keeps the copies both scopes hold:
// none is deleted and fetched again.
func TestScopeChangeKeepsOverlappingCopies(t *testing.T) {
	ctx := context.Background()
	everywhere := ""
	tests := []struct {
		name    string
		rule    Rule
		play    string
		id      string
		offline bool
	}{
		{name: "favorites", rule: Rule{Kind: KindFavorites}, id: "e"},
		{name: "favorites with the server down", rule: Rule{Kind: KindFavorites}, id: "e", offline: true},
		{name: "played", rule: played(Limit{Songs: 5}), play: "x", id: "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := setup(t, "", nil, nil)
			e.setRules(t, tt.rule)
			e.sync(t)
			if tt.play != "" {
				e.play(t, tt.play)
			}
			require.Equal(t, []string{tt.id}, cached(t, e.c))
			e.cat.fail = tt.offline

			require.NoError(t, e.c.UpdateRule(ctx, e.lib, tt.rule.Kind, "", nil, &everywhere))
			e.settle(t)
			assert.Equal(t, []Rule{{Kind: tt.rule.Kind, Limit: tt.rule.Limit}}, e.c.ruleList)
			assert.Equal(t, []string{tt.id}, cached(t, e.c))

			require.NoError(t, e.c.UpdateRule(ctx, "", tt.rule.Kind, "", nil, &e.lib))
			e.settle(t)
			assert.Equal(t, []string{tt.id}, cached(t, e.c))
			assert.Equal(t, 1, e.cat.hits[tt.id], "the copy was never fetched again")
			keys, err := column(ctx, e.c.db, "SELECT rule FROM covers WHERE id = ?", tt.id)
			require.NoError(t, err)
			assert.Equal(t, []string{tt.rule.Kind}, keys)
		})
	}
}

// Before the play counts are read, as at the start with the servers offline,
// a refresh neither empties mostPlayed nor misses a new top track.
func TestMostPlayedBeforeTheFirstSync(t *testing.T) {
	tests := []struct {
		name   string
		broken bool
		queued []string
	}{
		{name: "play counts read on demand", queued: []string{"r"}},
		{name: "play counts unreadable", broken: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := setup(t, "", nil, nil)
			e.c.cfg.MostPlayed = func(context.Context) ([]source.Track, error) {
				if tt.broken {
					return nil, errors.New("history down")
				}
				return []source.Track{{Library: e.lib, ID: "p"}, {Library: e.lib, ID: "q"}, {Library: e.lib, ID: "r"}}, nil
			}
			require.NoError(t, e.c.editRules(func([]Rule) ([]Rule, error) {
				return []Rule{{Kind: KindMostPlayed, Limit: &Limit{Songs: 5}}}, nil
			}))
			for _, id := range []string{"p", "q"} {
				e.insert(t, id, 40, 0)
				_, err := e.c.db.Exec("INSERT INTO covers (library_id, id, rule, static) VALUES (?, ?, 'all:mostPlayed', 0)", e.lib, id)
				require.NoError(t, err)
			}
			require.NoError(t, keep(e, true, "k"))
			assert.ElementsMatch(t, []string{"p", "q"}, cached(t, e.c))
			var queued []string
			e.c.mu.Lock()
			for _, j := range e.c.queue {
				if j.track.ID != "k" {
					queued = append(queued, j.track.ID)
				}
			}
			e.c.mu.Unlock()
			assert.Equal(t, tt.queued, queued)
		})
	}
}

// Rule entries and fields this version can't read are saved again as they
// are, also those moved out of a library config.
func TestUnknownRulesSurvive(t *testing.T) {
	ctx := context.Background()
	prep := func(e *env) {
		saved := `{"rules": [{"kind": "future"}, {"kind": "favorites", "library": "` + e.lib + `", "color": "red"}]}`
		require.NoError(t, os.WriteFile(limitPath(e.cfgDir), []byte(saved), 0o600))
		require.NoError(t, e.m.SetConfig(ctx, e.lib, configKey, map[string]any{"rules": []any{map[string]any{"kind": "later", "x": 1}}}))
	}
	e := setup(t, "", nil, prep)
	require.NoError(t, e.c.AddRule(ctx, e.lib, album("A1")))
	e.settle(t)
	rules := readJSON(t, limitPath(e.cfgDir))["rules"]
	assert.ElementsMatch(t, []any{
		map[string]any{"kind": "future"},
		map[string]any{"kind": "favorites", "library": e.lib, "color": "red"},
		map[string]any{"kind": "later", "x": float64(1), "library": e.lib},
		map[string]any{"kind": "album", "id": "A1", "library": e.lib},
	}, rules)
}

// A move that stopped after writing cache.json is finished without
// repeating its rules.
func TestInterruptedMigrationAddsNoDuplicates(t *testing.T) {
	ctx := context.Background()
	prep := func(e *env) {
		saved := `{"rules": [{"kind": "album", "id": "A1", "library": "` + e.lib + `"}]}`
		require.NoError(t, os.WriteFile(limitPath(e.cfgDir), []byte(saved), 0o600))
	}
	e := setup(t, "", map[string]any{configKey: map[string]any{"rules": []any{album("A1")}, "extra": true}}, prep)
	assert.Equal(t, []Rule{{Kind: KindAlbum, ID: "A1", Library: e.lib}}, e.c.ruleList)
	assert.Len(t, readJSON(t, limitPath(e.cfgDir))["rules"], 1)
	lib, err := e.m.Library(ctx, e.lib)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"extra": true}, lib.Config[configKey])
}

// An all-servers rule keeps a library's copies while that library is
// missing from the server list, as after a failed read of its rules.
func TestAllServersRuleNeedsTheLibraryListed(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "", nil, nil)
	require.NoError(t, e.c.AddRule(ctx, "", played(Limit{Songs: 10})))
	e.settle(t)
	e.play(t, "x")
	rules, _, ok := e.c.rules(ctx, e.lib)
	require.True(t, ok)
	e.c.recompute(ctx, e.lib, rules, nil)
	assert.Equal(t, []string{"x"}, cached(t, e.c))
}

// A turned-off library's rules keep the names of what they hold: albums and
// artists from the library's index, a server playlist from its last sync.
func TestRuleNamesOfATurnedOffLibrary(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "", nil, nil)
	batch := e.m.Store().BeginCatalog(e.lib, 1, 0)
	require.NoError(t, batch.PutServerAlbum(ctx, source.Album{ID: "A1", Name: "First Light"}, false))
	require.NoError(t, batch.PutServerArtist(ctx, source.Artist{ID: "R", Name: "Artist R"}, false))
	require.NoError(t, batch.Commit())
	for _, r := range []Rule{album("A1"), {Kind: KindArtist, ID: "R"}, playlist("P")} {
		require.NoError(t, e.c.AddRule(ctx, e.lib, r))
	}
	e.settle(t)
	require.NoError(t, e.m.SetEnabled(ctx, e.lib, false))
	e.c.LibrariesChanged(ctx)
	e.settle(t)

	st, err := e.c.Status(ctx)
	require.NoError(t, err)
	names := map[string]string{}
	for _, r := range st.Rules {
		names[r.Kind] = r.Name
	}
	assert.Equal(t, map[string]string{KindAlbum: "First Light", KindArtist: "Artist R", KindPlaylist: "List P"}, names)
}
