package history

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

func TestPlaysCountAndAge(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	defer s.Close()
	now := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return now }

	a1 := source.Track{Library: "l", ID: "a1", AlbumID: "A", Album: "Alpha", Title: "one"}
	a2 := source.Track{Library: "l", ID: "a2", AlbumID: "A", Album: "Alpha", Title: "two"}
	b1 := source.Track{Library: "l", ID: "b1", AlbumID: "B", Album: "Beta", Title: "three"}
	plays := []struct {
		t   source.Track
		ago time.Duration
	}{
		{a1, 400 * 24 * time.Hour}, // older than a year: pruned
		{b1, 3 * time.Hour}, {a1, 2 * time.Hour}, {a2, time.Hour}, {a1, 30 * time.Minute},
	}
	for _, p := range plays {
		require.NoError(t, s.Add(ctx, p.t, now.Add(-p.ago)))
	}
	require.NoError(t, s.Prune(ctx, 12))

	top, err := s.Top(ctx, source.ItemTrack, 10, false)
	require.NoError(t, err)
	require.Len(t, top, 3)
	assert.Equal(t, "a1", top[0].Track.ID)
	assert.Equal(t, 2, top[0].Count, "the pruned play no longer counts")

	albums, err := s.Top(ctx, source.ItemAlbum, 10, false)
	require.NoError(t, err)
	assert.Equal(t, "A", albums[0].Album.ID)
	assert.Equal(t, 3, albums[0].Count)

	recent, err := s.Top(ctx, source.ItemTrack, 10, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"a1", "a2", "b1"}, []string{recent[0].Track.ID, recent[1].Track.ID, recent[2].Track.ID})

	page, err := s.List(ctx, 2, 0)
	require.NoError(t, err)
	require.Len(t, page, 2)
	rest, err := s.List(ctx, 10, page[1].PlayedAt)
	require.NoError(t, err)
	assert.Equal(t, []string{"a1", "b1"}, []string{rest[0].Track.ID, rest[1].Track.ID}, "paging with before continues after the last item")

	album := source.Album{Library: "l", ID: "A", Tracks: []source.Track{a1, a2}}
	s.Annotate(&album)
	assert.Equal(t, 3, album.PlayCount)
	assert.Equal(t, 2, album.Tracks[0].PlayCount)
	assert.Equal(t, now.Add(-30*time.Minute).Unix(), album.Tracks[0].LastPlayed)

	require.NoError(t, s.Clear(ctx))
	s.Annotate(&album)
	assert.Zero(t, album.PlayCount)
}

func TestAnnotateKeepsServerCounts(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	defer s.Close()
	s.Server = func(lib string) bool { return lib == "srv" }
	local := source.Track{Library: "loc", ID: "a"}
	remote := source.Track{Library: "srv", ID: "b", PlayCount: 7, LastPlayed: 100}
	require.NoError(t, s.Add(ctx, local, time.Unix(50, 0)))
	require.NoError(t, s.Add(ctx, remote, time.Unix(60, 0)))
	got := []source.Track{local, remote}
	s.Annotate(got)
	assert.Equal(t, 1, got[0].PlayCount)
	assert.Equal(t, 7, got[1].PlayCount)
	assert.Equal(t, int64(100), got[1].LastPlayed)
}

// jellyfinLike lists all its played tracks; subsonicLike pages its tracks
// and lists the recently played ones.
type jellyfinLike struct {
	source.Source
	tracks []source.Track
	calls  atomic.Int32
}

func (f *jellyfinLike) Library() source.Library { return source.Library{ID: "jf"} }
func (f *jellyfinLike) PlayStats(context.Context) ([]source.Track, error) {
	f.calls.Add(1)
	return f.tracks, nil
}

type subsonicLike struct {
	source.Source
	mu     sync.Mutex
	all    []source.Track
	recent []source.Track
	pages  int
}

func (f *subsonicLike) Library() source.Library { return source.Library{ID: "nd"} }
func (f *subsonicLike) RecentPlays(context.Context) ([]source.Track, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recent, nil
}
func (f *subsonicLike) TrackPage(_ context.Context, q source.TrackQuery) (source.TrackPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages++
	return source.TrackPage{Total: len(f.all), Tracks: source.Window(f.all, q.Offset, q.Limit)}, nil
}

func TestTopMergesServerCounts(t *testing.T) {
	pagePause = 0
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	defer s.Close()
	now := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return now }
	day := int64(24 * 3600)
	ann := []source.ArtistRef{{ID: "ann", Name: "Ann"}}
	duo := []source.ArtistRef{{ID: "ann", Name: "Ann"}, {ID: "bob", Name: "Bob"}}
	jf := &jellyfinLike{tracks: []source.Track{
		{Library: "jf", ID: "old", AlbumID: "X", Artists: ann, PlayCount: 40, LastPlayed: now.Unix() - 200*day},
		// the server's 5 hold the plays scrobbled from here before the read
		{Library: "jf", ID: "both", AlbumID: "Y", Artists: duo, PlayCount: 5, LastPlayed: now.Unix() - 2*day},
	}}
	// 1200 songs, every 100th played: the full read pages through all of them
	nd := &subsonicLike{}
	for i := range 1200 {
		tr := source.Track{Library: "nd", ID: fmt.Sprint("s", i), AlbumID: "Z"}
		if i%100 == 0 {
			tr.PlayCount = 2
		}
		nd.all = append(nd.all, tr)
	}
	s.Sources = func(context.Context) ([]source.Source, error) { return []source.Source{jf, nd}, nil }
	local := source.Track{Library: "loc", ID: "l1", AlbumID: "L", Artists: []source.ArtistRef{{ID: "cy", Name: "Cy"}}}
	for _, ago := range []int64{3, 2, 1} {
		require.NoError(t, s.Add(ctx, local, time.Unix(now.Unix()-ago*day, 0)))
	}
	require.NoError(t, s.Add(ctx, jf.tracks[1], time.Unix(now.Unix()-3*day, 0)))

	counts := func(kind string) map[string]int {
		top, err := s.Top(ctx, kind, 500, false)
		require.NoError(t, err)
		out := map[string]int{}
		for _, c := range top {
			switch {
			case c.Track != nil:
				out[c.Track.ID] = c.Count
			case c.Album != nil:
				out[c.Album.ID] = c.Count
			default:
				out[c.Artist.ID] = c.Count
			}
		}
		return out
	}
	tracks := counts(source.ItemTrack)
	assert.Equal(t, 40, tracks["old"], "a server count as it is")
	assert.Equal(t, 5, tracks["both"], "the server count already holds the scrobbled local play")
	assert.Equal(t, 3, tracks["l1"])
	require.Eventually(t, func() bool { return len(counts(source.ItemTrack)) == 3+12 }, 5*time.Second, 10*time.Millisecond,
		"the full read stores every played song")
	assert.Equal(t, map[string]int{"ann": 45, "bob": 5, "cy": 3}, counts(source.ItemArtist), "a track counts for each artist")
	assert.Equal(t, map[string]int{"X": 40, "Y": 5, "L": 3, "Z": 24}, counts(source.ItemAlbum))

	require.NoError(t, s.Add(ctx, jf.tracks[1], now))
	assert.Equal(t, 6, counts(source.ItemTrack)["both"], "a play after the read adds to the stored server count")
	assert.Equal(t, int32(1), jf.calls.Load(), "server counts are refreshed at most every 15 minutes")

	nd.mu.Lock()
	pages := nd.pages
	nd.recent = []source.Track{{Library: "nd", ID: "s5", AlbumID: "Z", PlayCount: 1}}
	nd.mu.Unlock()
	now = now.Add(20 * time.Minute)
	assert.Equal(t, 1, counts(source.ItemTrack)["s5"], "the recent refresh adds a newly played song")
	nd.mu.Lock()
	assert.Equal(t, pages, nd.pages, "no second full read the same day")
	nd.mu.Unlock()
}
