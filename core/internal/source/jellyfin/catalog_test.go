package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

// catalogSource talks to answer for every /Users/{user}/Items request and
// records the queries.
func catalogSource(t *testing.T, answer func(q url.Values) string) (*Source, func() []url.Values) {
	var mu sync.Mutex
	var seen []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Query())
		mu.Unlock()
		_, _ = w.Write([]byte(answer(r.URL.Query())))
	}))
	t.Cleanup(srv.Close)
	secrets := memSecrets{"jf/password": "", "jf/token": "tok", "jf/userId": userID, "jf/sessionFor": srv.URL + "\x00demo"}
	lib := source.Library{ID: "jf", Kind: Kind, Name: "JF", Config: map[string]any{"url": srv.URL, "username": "demo"}}
	src, err := Factory(lib, source.Env{Secrets: secrets, CacheDir: t.TempDir(), Version: "1"})
	require.NoError(t, err)
	return src.(*Source), func() []url.Values {
		mu.Lock()
		defer mu.Unlock()
		return seen
	}
}

// Bitrate and composer come only with Details, which asks for smaller pages,
// so a heavier page stays inside the request timeout.
func TestDetailsPage(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile("testdata/tracks_details.json")
	require.NoError(t, err)
	s, queries := catalogSource(t, func(url.Values) string { return string(data) })

	page, err := s.TrackPage(ctx, source.TrackQuery{Limit: 500, Details: true})
	require.NoError(t, err)
	q := queries()[0]
	assert.Contains(t, q.Get("Fields"), "MediaSources")
	assert.Contains(t, q.Get("Fields"), "People")
	assert.Equal(t, "200", q.Get("Limit"))
	require.Len(t, page.Tracks, 2)
	assert.Equal(t, "Johann Sebastian Bach, Anna Magdalena Bach", page.Tracks[0].Composer)
	assert.Equal(t, 912, page.Tracks[0].Bitrate, "the audio stream's rate")
	assert.Equal(t, 320, page.Tracks[1].Bitrate, "the file's when the stream has none")
	assert.Empty(t, page.Tracks[1].Composer)

	_, err = s.TrackPage(ctx, source.TrackQuery{Limit: 500})
	require.NoError(t, err)
	q = queries()[1]
	assert.NotContains(t, q.Get("Fields"), "MediaSources", "browsing doesn't pay for them")
	assert.Equal(t, "500", q.Get("Limit"))
}

// Jellyfin's empty date would overflow as nanoseconds in the index.
func TestAddedDate(t *testing.T) {
	assert.Zero(t, item{DateCreated: "0001-01-01T00:00:00.0000000Z"}.added())
	assert.Equal(t, int64(1700000000), item{DateCreated: "2023-11-14T22:13:20.0000000Z"}.added())
}

func TestCatalogChanged(t *testing.T) {
	recent := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	for _, tt := range []struct {
		name         string
		stamp        string
		tracks       int
		saved        int
		want         bool
		wantRequests int
	}{
		{"first time", "", 10, 0, true, 1},
		{"unreadable stamp", "nonsense", 10, 0, true, 1},
		{"nothing happened", recent + "|10", 10, 0, false, 2},
		{"a track was removed: no date shows that", recent + "|11", 10, 0, true, 1},
		{"an item was saved", recent + "|10", 10, 1, true, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, queries := catalogSource(t, func(q url.Values) string {
				n := tt.tracks
				if q.Get("minDateLastSaved") != "" {
					n = tt.saved
				}
				return fmt.Sprintf(`{"Items":[],"TotalRecordCount":%d}`, n)
			})
			changed, next, err := s.CatalogChanged(context.Background(), tt.stamp)
			require.NoError(t, err)
			assert.Equal(t, tt.want, changed)
			assert.True(t, strings.HasSuffix(next, "|10"), next)
			assert.Len(t, queries(), tt.wantRequests)
			for _, q := range queries() {
				assert.Equal(t, "0", q.Get("Limit"), "a count reads no items")
			}
		})
	}
}

// The date asked for reaches back past the stamp: the server's clock isn't
// ours.
func TestCatalogChangedAllowsForClockSkew(t *testing.T) {
	at := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	s, queries := catalogSource(t, func(url.Values) string { return `{"Items":[],"TotalRecordCount":3}` })
	_, _, err := s.CatalogChanged(context.Background(), at.Format(time.RFC3339)+"|3")
	require.NoError(t, err)
	assert.Equal(t, "2026-01-02T11:00:00Z", queries()[1].Get("minDateLastSaved"))
}

func TestRecentPlaysIsOneRequest(t *testing.T) {
	s, queries := catalogSource(t, func(url.Values) string {
		data, _ := json.Marshal(itemsReply{Items: []item{{ID: "t", Type: "Audio", UserData: userData{PlayCount: 4}}}, TotalRecordCount: 900})
		return string(data)
	})
	tracks, err := s.RecentPlays(context.Background())
	require.NoError(t, err)
	require.Len(t, tracks, 1)
	assert.Equal(t, 4, tracks[0].PlayCount)
	require.Len(t, queries(), 1)
	assert.Equal(t, "DatePlayed", queries()[0].Get("SortBy"))
	assert.Equal(t, "IsPlayed", queries()[0].Get("Filters"))
}

// A sync sweeps what it didn't list, so a listing may not stop short of a
// large library.
func TestListingReachesPastTenThousand(t *testing.T) {
	const total = 10_500
	s, _ := catalogSource(t, func(q url.Values) string {
		start, _ := strconv.Atoi(q.Get("StartIndex"))
		n := min(pageSize, total-start)
		data, _ := json.Marshal(itemsReply{Items: make([]item, n), TotalRecordCount: total})
		return string(data)
	})
	albums, err := s.Albums(context.Background(), "")
	require.NoError(t, err)
	assert.Len(t, albums, total)
}
