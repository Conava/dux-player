package shuffle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

type listFeed []source.Track

func (f *listFeed) Next(_ context.Context, n int) ([]source.Track, error) {
	n = min(n, len(*f))
	out := append([]source.Track{}, (*f)[:n]...)
	*f = (*f)[n:]
	return out, nil
}

func TestSkipRules(t *testing.T) {
	rules := SkipRules{MinRating: 2, Genres: []string{"Christmas"}, Libraries: []string{"radio"}, ShorterThan: 30}
	tests := []struct {
		name  string
		track source.Track
		skip  bool
	}{
		{"rated below", source.Track{Rating: 1, Duration: 60}, true},
		{"unrated is not below", source.Track{Rating: 0, Duration: 60}, false},
		{"rated at the limit", source.Track{Rating: 2, Duration: 60}, false},
		{"excluded genre, any case", source.Track{Genre: "christmas", Duration: 60}, true},
		{"short", source.Track{Duration: 29}, true},
		{"length unknown", source.Track{Duration: 0}, false},
		{"excluded library", source.Track{Library: "radio", Duration: 60}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.skip, rules.Skips(tt.track))
		})
	}
}

func TestSkipFeedFillsTheBatchAndEnds(t *testing.T) {
	feed := listFeed{{ID: "a", Rating: 1}, {ID: "b", Rating: 1}, {ID: "c"}, {ID: "d", Rating: 1}, {ID: "e"}}
	f := Picks{Skip: SkipRules{MinRating: 3}}.Apply(&feed, nil)
	ctx := context.Background()
	got, err := f.Next(ctx, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"c", "e"}, ids(got))
	got, err = f.Next(ctx, 2)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAlbumFeedPlaysEachAlbumWholeOnce(t *testing.T) {
	albums := map[string][]source.Track{
		"x": {{ID: "x1", AlbumID: "x"}, {ID: "x2", AlbumID: "x", Rating: 1}, {ID: "x3", AlbumID: "x"}},
		"y": {{ID: "y1", AlbumID: "y"}},
	}
	feed := listFeed{{ID: "x2", AlbumID: "x"}, {ID: "x3", AlbumID: "x"}, {ID: "s"}, {ID: "y1", AlbumID: "y"}, {ID: "x1", AlbumID: "x"}}
	f := Picks{Albums: true, Skip: SkipRules{MinRating: 2}}.Apply(&feed, func(_ context.Context, t source.Track) ([]source.Track, error) {
		return albums[t.AlbumID], nil
	})
	got, err := f.Next(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, []string{"x1", "x3", "s", "y1"}, ids(got), "track order kept, a skipped track dropped from its album")
}

func ids(ts []source.Track) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return out
}
