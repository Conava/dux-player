package cache

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

func (e *env) ref(id string) source.Ref { return source.Ref{Library: e.lib, ID: id} }

func (e *env) page(t *testing.T, offset, limit int, excludedOnly bool) (int, []string) {
	t.Helper()
	p, err := e.c.Tracks(context.Background(), e.lib, offset, limit, excludedOnly)
	require.NoError(t, err)
	out := []string{}
	for _, it := range p.Items {
		id := it.Track.ID
		if it.Excluded {
			id = "!" + id
		}
		out = append(out, id)
	}
	return p.Total, out
}

// An excluded track is fetched by no rule and no play until it is included
// again; a plain delete leaves the rules to fetch it again.
func TestDeleteAndExclude(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		exclude bool
		after   []string
	}{
		{"exclude", true, []string{"b"}},
		{"delete only", false, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := setup(t, "", nil, nil)
			e.setRules(t, album("A1"), played(Limit{Songs: 10}))
			e.sync(t)
			require.ElementsMatch(t, []string{"a", "b"}, cached(t, e.c))

			require.NoError(t, e.c.Delete(ctx, e.ref("a"), tt.exclude))
			assert.Equal(t, []string{"b"}, cached(t, e.c))
			e.sync(t)
			e.play(t, "a")
			assert.ElementsMatch(t, tt.after, cached(t, e.c))
			if !tt.exclude {
				return
			}
			assert.Equal(t, 1, e.hits("a"), "nothing downloaded it again")
			st, err := e.c.Status(ctx)
			require.NoError(t, err)
			require.NotEmpty(t, st.Rules)
			assert.Equal(t, 1, st.Rules[0].Tracks, "the album rule no longer counts it")
			total, items := e.page(t, 0, 0, false)
			assert.Equal(t, 2, total)
			assert.Equal(t, []string{"!a", "b"}, items)
			p, err := e.c.Tracks(ctx, e.lib, 0, 0, false)
			require.NoError(t, err)
			assert.Empty(t, p.Items[0].Rules)
			assert.Equal(t, []RuleRef{{Key: "album:A1", Kind: KindAlbum, ID: "A1"}}, p.Items[1].Rules)

			require.NoError(t, e.c.Include(ctx, e.ref("a")))
			e.settle(t)
			assert.ElementsMatch(t, []string{"a", "b"}, cached(t, e.c))
		})
	}
}

// Including a track that isn't excluded starts no sync.
func TestIncludeWithoutExclusionSyncsNothing(t *testing.T) {
	e := setup(t, "", nil, nil)
	e.setRules(t, album("A1"))
	require.NoError(t, e.c.Include(context.Background(), e.ref("a")))
	e.settle(t)
	assert.Empty(t, cached(t, e.c))
}

func TestExcludeNeedsACopy(t *testing.T) {
	e := setup(t, "", nil, nil)
	require.Error(t, e.c.Delete(context.Background(), e.ref("a"), true))
	total, _ := e.page(t, 0, 0, true)
	assert.Zero(t, total)
}

// Clearing keeps the exclusions; keeping the track by hand lifts one.
func TestExclusionOutlivesClearUntilKept(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "", nil, nil)
	e.setRules(t, album("A1"))
	e.sync(t)
	require.NoError(t, e.c.Delete(ctx, e.ref("a"), true))

	require.NoError(t, e.c.Clear(ctx, e.lib))
	e.sync(t)
	assert.Equal(t, []string{"b"}, cached(t, e.c))

	require.NoError(t, keep(e, true, "a"))
	e.drain(t)
	assert.ElementsMatch(t, []string{"a", "b"}, cached(t, e.c))
	total, _ := e.page(t, 0, 0, true)
	assert.Zero(t, total)
}

func TestTracksPaging(t *testing.T) {
	e := setup(t, "", nil, nil)
	for i := range 503 {
		e.insert(t, fmt.Sprintf("t%03d", i), 1, 0)
		_, err := e.c.db.Exec("UPDATE entries SET used = ? WHERE id = ?", i, fmt.Sprintf("t%03d", i))
		require.NoError(t, err)
	}
	require.NoError(t, e.c.Delete(context.Background(), e.ref("t000"), true))
	require.NoError(t, e.c.Delete(context.Background(), e.ref("t001"), true))

	tests := []struct {
		name          string
		offset, limit int
		excludedOnly  bool
		total, n      int
		first, last   string
	}{
		{"default limit", 0, 0, false, 503, 100, "!t001", "t405"},
		{"clamped", 0, 1000, false, 503, 500, "!t001", "t005"},
		{"negative offset", -5, 3, false, 503, 3, "!t001", "t502"},
		{"past the exclusions", 2, 2, false, 503, 2, "t502", "t501"},
		{"last page", 500, 100, false, 503, 3, "t004", "t002"},
		{"excluded only", 0, 0, true, 2, 2, "!t001", "!t000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, items := e.page(t, tt.offset, tt.limit, tt.excludedOnly)
			assert.Equal(t, tt.total, total)
			require.Len(t, items, tt.n)
			assert.Equal(t, tt.first, items[0])
			assert.Equal(t, tt.last, items[len(items)-1])
		})
	}
	st, err := e.c.Status(context.Background())
	require.NoError(t, err)
	require.Len(t, st.Libraries, 1)
	assert.Equal(t, 2, st.Libraries[0].Excluded)
}

func TestRemovedLibraryLosesItsExclusions(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "", nil, nil)
	e.setRules(t, album("A1"))
	e.sync(t)
	require.NoError(t, e.c.Delete(ctx, e.ref("a"), true))
	require.NoError(t, e.m.Remove(ctx, e.lib))
	e.c.LibrariesChanged(ctx)
	n := -1
	require.NoError(t, e.c.db.QueryRow("SELECT count(*) FROM excluded").Scan(&n))
	assert.Zero(t, n)
	assert.Empty(t, e.c.excluded)
}

// The prefetch leaves excluded tracks to stream.
func TestPrefetchSkipsExcluded(t *testing.T) {
	e, ctx := pf(t)
	e.setRules(t, album("A1"))
	e.sync(t)
	require.NoError(t, e.c.Delete(ctx, e.ref("b"), true))
	e.c.SetPrefetch(ctx, 2, false)
	e.at(ctx, player.StatePlaying, 0, queueOf(e.lib, player.RepeatOff, "a", "b", "c"))
	assert.Equal(t, []string{"c"}, e.ahead())
}

// An excluded track leaves its place in a mostPlayed rule to the next one.
func TestMostPlayedSkipsExcluded(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "", nil, nil)
	e.c.cfg.MostPlayed = func(context.Context) ([]source.Track, error) {
		return []source.Track{{Library: e.lib, ID: "p"}, {Library: e.lib, ID: "q"}}, nil
	}
	e.setRules(t, Rule{Kind: KindMostPlayed, Limit: &Limit{Songs: 1}})
	e.sync(t)
	require.Equal(t, []string{"p"}, cached(t, e.c))
	require.NoError(t, e.c.Delete(ctx, e.ref("p"), true))
	e.drain(t)
	assert.Equal(t, []string{"q"}, cached(t, e.c))
}
