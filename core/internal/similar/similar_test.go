package similar

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

type fakeCatalog struct{ tracks []source.Track }

func (f fakeCatalog) TracksByArtists(_ context.Context, lib string, ids []string, limit int) ([]source.Track, error) {
	var out []source.Track
	for _, t := range f.tracks {
		if t.Library != lib || len(out) == limit {
			continue
		}
		for _, a := range t.Artists {
			if slices.Contains(ids, a.ID) {
				out = append(out, t)
				break
			}
		}
	}
	return out, nil
}

func (f fakeCatalog) Collaborators(_ context.Context, lib string, ids []string, limit int) ([]Collaborator, error) {
	counts := map[string]int{}
	for _, t := range f.tracks {
		if t.Library != lib || !slices.ContainsFunc(t.Artists, func(a source.ArtistRef) bool { return slices.Contains(ids, a.ID) }) {
			continue
		}
		for _, a := range t.Artists {
			if !slices.Contains(ids, a.ID) {
				counts[a.ID]++
			}
		}
	}
	var out []Collaborator
	for _, id := range sortedKeys(counts) {
		if len(out) < limit {
			out = append(out, Collaborator{ID: id, Count: counts[id]})
		}
	}
	return out, nil
}

func (f fakeCatalog) TracksByGenre(_ context.Context, lib, genre string, limit int) ([]source.Track, error) {
	var out []source.Track
	for _, t := range f.tracks {
		if t.Library == lib && strings.EqualFold(t.Genre, genre) && len(out) < limit {
			out = append(out, t)
		}
	}
	return out, nil
}

func track(id string, artists ...string) source.Track {
	t := source.Track{Library: "L", ID: id}
	for _, a := range artists {
		t.Artists = append(t.Artists, source.ArtistRef{ID: a, Name: a})
	}
	return t
}

// library has count tracks by each of the artists, ids "<artist>-<i>".
func library(count int, artists ...string) []source.Track {
	var out []source.Track
	for _, a := range artists {
		for i := range count {
			out = append(out, track(fmt.Sprintf("%s-%d", a, i), a))
		}
	}
	return out
}

func byArtist(ts []source.Track) map[string]int {
	n := map[string]int{}
	for _, t := range ts {
		for _, a := range t.Artists {
			n[a.ID]++
		}
	}
	return n
}

func ten() []string {
	var a []string
	for i := range 10 {
		a = append(a, fmt.Sprintf("a%d", i))
	}
	return a
}

func TestPick(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	blocking := func(context.Context, source.Track) ([]source.Track, error) {
		<-release
		return []source.Track{track("late", "z")}, nil
	}
	serverTracks := func(context.Context, source.Track) ([]source.Track, error) {
		return []source.Track{track("s1", "x"), track("s2", "y")}, nil
	}
	failing := func(context.Context, source.Track) ([]source.Track, error) {
		return nil, errors.New("not implemented")
	}

	tests := []struct {
		name  string
		in    Input
		n     int
		check func(t *testing.T, got []source.Track)
	}{
		{
			name: "every artist of every seed is a source",
			in: Input{
				Seeds:   []source.Track{track("a1-0", "a1"), track("duet", "a3", "a4"), track("a2-0", "a2")},
				Catalog: fakeCatalog{library(4, "a1", "a2", "a3", "a4", "other")},
			},
			n: 12,
			check: func(t *testing.T, got []source.Track) {
				counts := byArtist(got)
				for _, a := range []string{"a1", "a2", "a3", "a4"} {
					assert.Positive(t, counts[a], a)
				}
				assert.Zero(t, counts["other"])
			},
		},
		{
			name: "the per-artist cap holds",
			in: Input{
				Seeds:   firstTracks(ten()...),
				Catalog: fakeCatalog{library(10, ten()...)},
				Profile: Close,
			},
			n: 10,
			check: func(t *testing.T, got []source.Track) {
				require.Len(t, got, 10)
				for a, c := range byArtist(got) {
					assert.LessOrEqual(t, c, 2, a)
				}
			},
		},
		{
			name: "collaborators widen the reach",
			in: Input{
				Seeds:   []source.Track{track("a1-0", "a1")},
				Catalog: fakeCatalog{append(library(3, "a1", "c1"), track("feat", "a1", "c1"))},
			},
			n: 10,
			check: func(t *testing.T, got []source.Track) {
				assert.Len(t, got, 6)
				assert.Equal(t, 4, byArtist(got)["c1"])
			},
		},
		{
			name: "the server's tracks are used",
			in:   Input{Seeds: []source.Track{track("a1-0", "a1")}, Catalog: fakeCatalog{}, Server: serverTracks},
			n:    5,
			check: func(t *testing.T, got []source.Track) {
				assert.ElementsMatch(t, []string{"s1", "s2"}, ids(got))
			},
		},
		{
			name: "a failing server is ignored",
			in:   Input{Seeds: []source.Track{track("a1-0", "a1")}, Catalog: fakeCatalog{library(4, "a1")}, Server: failing},
			n:    5,
			check: func(t *testing.T, got []source.Track) {
				assert.Len(t, got, 3)
			},
		},
		{
			name: "a server past its timeout is ignored",
			in: Input{
				Seeds: []source.Track{track("a1-0", "a1")}, Catalog: fakeCatalog{library(4, "a1")},
				Server: blocking, ServerTimeout: 20 * time.Millisecond,
			},
			n: 5,
			check: func(t *testing.T, got []source.Track) {
				assert.Len(t, got, 3)
			},
		},
		{
			name: "queued and skipped tracks are never returned",
			in: Input{
				Seeds:   []source.Track{track("a1-0", "a1")},
				Catalog: fakeCatalog{library(5, "a1")},
				Queued:  map[source.Ref]bool{{Library: "L", ID: "a1-1"}: true},
				Skip:    func(t source.Track) bool { return t.ID == "a1-2" },
			},
			n: 5,
			check: func(t *testing.T, got []source.Track) {
				assert.ElementsMatch(t, []string{"a1-3", "a1-4"}, ids(got))
			},
		},
		{name: "exactly 1", in: Input{Seeds: []source.Track{track("a0-0", "a0"), track("a5-0", "a5")}, Catalog: fakeCatalog{library(10, ten()...)}}, n: 1, check: wantLen(1)},
		{name: "exactly 5", in: Input{Seeds: []source.Track{track("a0-0", "a0"), track("a5-0", "a5")}, Catalog: fakeCatalog{library(10, ten()...)}}, n: 5, check: wantLen(5)},
		{name: "exactly 25", in: Input{Seeds: []source.Track{track("a0-0", "a0"), track("a5-0", "a5")}, Catalog: fakeCatalog{library(20, ten()...)}, Profile: Adventurous}, n: 25, check: wantLen(25)},
		{name: "fewer when candidates run out", in: Input{Seeds: []source.Track{track("a0-0", "a0")}, Catalog: fakeCatalog{library(4, "a0")}}, n: 25, check: wantLen(3)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Rand = rand.New(rand.NewPCG(1, 2))
			start := time.Now()
			got := Pick(context.Background(), tt.in, tt.n)
			assert.Less(t, time.Since(start), time.Second)
			tt.check(t, got)
		})
	}
}

func TestPickTwiceInASession(t *testing.T) {
	cat := fakeCatalog{library(10, "a1", "a2", "a3")}
	seeds := []source.Track{track("a1-0", "a1"), track("a2-0", "a2"), track("a3-0", "a3")}
	queued := map[source.Ref]bool{}
	for _, s := range seeds {
		queued[s.Ref()] = true
	}
	rnd := rand.New(rand.NewPCG(3, 4))

	first := Pick(context.Background(), Input{Seeds: seeds, Queued: queued, Catalog: cat, Rand: rnd}, 5)
	require.Len(t, first, 5)
	for _, tr := range first {
		queued[tr.Ref()] = true
	}
	// The newest seeds are all by one artist; the earlier ones still count.
	session := append(append(seeds, first...), track("x", "a3"))
	second := Pick(context.Background(), Input{Seeds: session, Queued: queued, Catalog: cat, Rand: rnd}, 5)

	require.Len(t, second, 5)
	for _, tr := range second {
		assert.False(t, queued[tr.Ref()], tr.ID)
	}
	counts := byArtist(second)
	assert.Positive(t, counts["a1"]+counts["a2"])
}

func firstTracks(artists ...string) []source.Track {
	var out []source.Track
	for _, a := range artists {
		out = append(out, track(a+"-0", a))
	}
	return out
}

// fakePlaylists holds every track of each playlist; PlaylistsWith returns
// those holding a ref, with at most limit tracks.
type fakePlaylists map[string][]source.Track

func (f fakePlaylists) PlaylistsWith(_ context.Context, refs []source.Ref, limit int) (map[string]Playlist, error) {
	out := map[string]Playlist{}
	for id, ts := range f {
		var seeds []source.Ref
		for _, r := range refs {
			if slices.ContainsFunc(ts, func(t source.Track) bool { return t.Ref() == r }) {
				seeds = append(seeds, r)
			}
		}
		if len(seeds) > 0 {
			out[id] = Playlist{Len: len(ts), Seeds: seeds, Tracks: source.Window(ts, 0, limit)}
		}
	}
	return out, nil
}

func rock(ts ...source.Track) []source.Track {
	for i := range ts {
		ts[i].Genre = "Rock"
	}
	return ts
}

// TestPickOdds draws many batches with one fixed Rand and checks how often
// tracks come up.
func TestPickOdds(t *testing.T) {
	// Ten session artists with nine unplayed tracks each, among 300 tracks
	// of the same genre by other artists.
	session := rock(library(10, "s0", "s1", "s2", "s3", "s4", "s5", "s6", "s7", "s8", "s9")...)
	var seeds []source.Track
	for i := 0; i < len(session); i += 10 {
		seeds = append(seeds, session[i])
	}
	var others []source.Track
	for i := range 300 {
		others = append(others, track(fmt.Sprintf("g%d", i), fmt.Sprintf("g%d", i)))
	}
	genreLib := fakeCatalog{append(session, rock(others...)...)}
	fromSession := func(share float64) func(t *testing.T, picked map[string]int, total int) {
		return func(t *testing.T, picked map[string]int, total int) {
			n := 0
			for id, c := range picked {
				if id[0] == 's' {
					n += c
				}
			}
			assert.GreaterOrEqual(t, float64(n)/float64(total), share)
		}
	}

	seed := rock(track("a1-0", "a1"))[0]
	tests := []struct {
		name  string
		in    Input
		n     int
		check func(t *testing.T, picked map[string]int, total int)
	}{
		{name: "close stays with the session's artists over their genre", in: Input{Seeds: seeds, Catalog: genreLib, Profile: Close}, n: 5, check: fromSession(0.8)},
		{name: "balanced stays with the session's artists over their genre", in: Input{Seeds: seeds, Catalog: genreLib, Profile: Balanced}, n: 5, check: fromSession(0.6)},
		{name: "adventurous still leans on the session's artists", in: Input{Seeds: seeds, Catalog: genreLib, Profile: Adventurous}, n: 5, check: fromSession(0.4)},
		{
			name: "a playlist shared with a seed lifts its tracks",
			in: Input{
				Seeds:     []source.Track{seed},
				Catalog:   fakeCatalog{rock(library(1, ten()...)...)},
				Playlists: fakePlaylists{"p": {seed, track("p1", "x"), track("p2", "y")}},
			},
			n: 1,
			check: func(t *testing.T, picked map[string]int, total int) {
				assert.Greater(t, picked["p1"]+picked["p2"], total/2)
			},
		},
		{
			name: "a track of a small shared playlist outweighs one of a large one",
			in: Input{
				Seeds:     []source.Track{seed},
				Catalog:   fakeCatalog{},
				Playlists: fakePlaylists{"small": {seed, track("p1", "x")}, "large": append([]source.Track{seed}, others[:100]...)},
			},
			n: 1,
			check: func(t *testing.T, picked map[string]int, total int) {
				assert.Greater(t, picked["p1"], 5*(total-picked["p1"])/100)
			},
		},
		{
			name: "a seed past the tracks returned still lifts its playlist",
			in: Input{
				Seeds:     []source.Track{seed},
				Catalog:   fakeCatalog{},
				Playlists: fakePlaylists{"p": append(slices.Clone(others[:queryLimit]), seed)},
			},
			n: 1,
			check: func(t *testing.T, picked map[string]int, total int) {
				assert.Equal(t, 200, total)
			},
		},
		{
			name: "a recently played track comes up less",
			in: Input{
				Seeds:   []source.Track{track("a1-0", "a1")},
				Catalog: fakeCatalog{library(3, "a1")},
				Recent:  map[source.Ref]time.Time{{Library: "L", ID: "a1-1"}: time.Unix(1000, 0)},
				Now:     time.Unix(1000+3600, 0),
			},
			n: 1,
			check: func(t *testing.T, picked map[string]int, total int) {
				assert.Greater(t, picked["a1-2"], 2*picked["a1-1"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Rand = rand.New(rand.NewPCG(1, 2))
			picked, total := map[string]int{}, 0
			for range 200 {
				for _, tr := range Pick(context.Background(), tt.in, tt.n) {
					picked[tr.ID]++
					total++
				}
			}
			require.Positive(t, total)
			tt.check(t, picked, total)
		})
	}
}

func TestPickCapCountsTheNewestSeeds(t *testing.T) {
	seeds := firstTracks("a0", "a1", "a2")
	cat := fakeCatalog{library(5, "a0", "a1", "a2")}
	rnd := rand.New(rand.NewPCG(5, 6))
	for range 50 {
		got := Pick(context.Background(), Input{Seeds: seeds, Catalog: cat, Rand: rnd}, 1)
		require.Len(t, got, 1)
		assert.Equal(t, "a0", got[0].Artists[0].ID)
	}
}

func TestPickCloseKeepsTheSeedArtist(t *testing.T) {
	cat := fakeCatalog{rock(append(library(10, "x"), library(1, ten()...)...)...)}
	seeds := cat.tracks[:1]
	rnd := rand.New(rand.NewPCG(7, 8))
	same := 0
	for range 50 {
		same += byArtist(Pick(context.Background(), Input{Seeds: seeds, Catalog: cat, Profile: Close, Rand: rnd}, 5))["x"]
	}
	assert.Positive(t, same)
}

func wantLen(n int) func(t *testing.T, got []source.Track) {
	return func(t *testing.T, got []source.Track) {
		assert.Len(t, got, n)
		assert.Len(t, uniq(got), n)
	}
}

func ids(ts []source.Track) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}

func uniq(ts []source.Track) map[source.Ref]bool {
	m := map[source.Ref]bool{}
	for _, t := range ts {
		m[t.Ref()] = true
	}
	return m
}

func TestTopUp(t *testing.T) {
	// feed hands out tracks "f0", "f1", … from a pool of size, repeating
	// each one dup times.
	feed := func(size, dup int) (func(context.Context, int) ([]source.Track, error), *int) {
		calls, next := 0, 0
		return func(_ context.Context, k int) ([]source.Track, error) {
			calls++
			var out []source.Track
			for range k {
				if next >= size*dup {
					break
				}
				out = append(out, track(fmt.Sprintf("f%d", next/dup)))
				next++
			}
			return out, nil
		}, &calls
	}
	queued := map[source.Ref]bool{{Library: "L", ID: "f0"}: true}
	skip := func(t source.Track) bool { return t.ID == "f1" }

	tests := []struct {
		name      string
		n         int
		size, dup int
		want      int
	}{
		{"exactly 1", 1, 100, 1, 1},
		{"exactly 5", 5, 100, 2, 5},
		{"exactly 25", 25, 100, 3, 25},
		{"fewer when the feed runs dry", 25, 10, 1, 8},
		{"a feed of only duplicates ends", 5, 2, 1_000_000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, calls := feed(tt.size, tt.dup)
			got, err := TopUp(context.Background(), tt.n, next, queued, skip)
			require.NoError(t, err)
			assert.Len(t, got, tt.want)
			assert.Len(t, uniq(got), tt.want)
			assert.False(t, uniq(got)[source.Ref{Library: "L", ID: "f0"}])
			assert.False(t, uniq(got)[source.Ref{Library: "L", ID: "f1"}])
			assert.LessOrEqual(t, *calls, maxRounds)
		})
	}
}
