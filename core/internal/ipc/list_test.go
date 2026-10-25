package ipc

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

// fakeLibrary has n tracks "0".."n-1" over albums of 10, paged like a
// server (at most 500 per page).
type fakeLibrary struct{ n int }

func (f fakeLibrary) Artists(context.Context) ([]source.Artist, error) { return nil, nil }
func (f fakeLibrary) Search(context.Context, string, int) (source.SearchResult, error) {
	return source.SearchResult{}, nil
}
func (f fakeLibrary) Albums(_ context.Context, artist string) ([]source.Album, error) {
	var out []source.Album
	for i := 0; i < f.n; i += 10 {
		out = append(out, source.Album{ID: strconv.Itoa(i), ArtistID: artist})
	}
	return out, nil
}
func (f fakeLibrary) Album(_ context.Context, id string) (source.Album, error) {
	start, _ := strconv.Atoi(id)
	a := source.Album{ID: id}
	for i := start; i < min(start+10, f.n); i++ {
		a.Tracks = append(a.Tracks, source.Track{ID: strconv.Itoa(i)})
	}
	return a, nil
}
func (f fakeLibrary) TrackPage(_ context.Context, q source.TrackQuery) (source.TrackPage, error) {
	page := source.TrackPage{Total: f.n}
	for i := q.Offset; i < min(q.Offset+min(q.Limit, 500), f.n); i++ {
		page.Tracks = append(page.Tracks, source.Track{ID: strconv.Itoa(i)})
	}
	return page, nil
}

// Playing row index of a long list queues a window around it, and the
// row's track is the one that plays.
func TestListWindow(t *testing.T) {
	tests := []struct {
		name         string
		n, index     int
		artist       string
		wantLen      int
		wantFirstRow int
	}{
		{"short list", 30, 7, "", 30, 0},
		{"middle of a long list", 12000, 6000, "", maxListTracks, 3500},
		{"near the end", 12000, 11990, "", maxListTracks, 7000},
		{"artist, near the end", 12000, 11990, "a", maxListTracks, 7000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracks, skip, err := listTracks(context.Background(), fakeLibrary{tt.n}, map[string]any{"artist": tt.artist}, tt.index)
			require.NoError(t, err)
			assert.Len(t, tracks, tt.wantLen)
			assert.Equal(t, tt.wantFirstRow, skip)
			assert.Equal(t, strconv.Itoa(tt.index), tracks[tt.index-skip].ID)
		})
	}
}

// appearsOn is a library where artist "b" has an album of their own and
// features on one track of "a"'s album.
type appearsOn struct{ fakeLibrary }

func (appearsOn) Albums(context.Context, string) ([]source.Album, error) {
	return []source.Album{{ID: "own", ArtistID: "b"}, {ID: "guest", ArtistID: "a"}}, nil
}

func (appearsOn) Album(_ context.Context, id string) (source.Album, error) {
	if id == "own" {
		return source.Album{ID: id, Tracks: []source.Track{{ID: "b1", ArtistID: "b"}, {ID: "b2", ArtistID: "b"}}}, nil
	}
	return source.Album{ID: id, Tracks: []source.Track{
		{ID: "a1", ArtistID: "a"},
		{ID: "a2", ArtistID: "a", Artists: []source.ArtistRef{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}},
	}}, nil
}

func TestArtistTracksTakeOnlyTheirTracksFromAppearances(t *testing.T) {
	tracks, err := artistTracks(context.Background(), appearsOn{}, "b")
	require.NoError(t, err)
	var ids []string
	for _, tr := range tracks {
		ids = append(ids, tr.ID)
	}
	assert.Equal(t, []string{"b1", "b2", "a2"}, ids)
}
