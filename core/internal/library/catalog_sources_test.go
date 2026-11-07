package library

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
	"github.com/Conava/dux-player/core/internal/source/jellyfin"
	"github.com/Conava/dux-player/core/internal/source/subsonic"
)

type secretsMap map[string]string

func (s secretsMap) Get(_, key string) (string, error) {
	v, ok := s[key]
	if !ok {
		return "", source.ErrNoSecret
	}
	return v, nil
}
func (s secretsMap) Set(_, key, value string) error { s[key] = value; return nil }
func (s secretsMap) Delete(string) error            { return nil }

func realSourceManager(t *testing.T, kind string, factory source.Factory, config map[string]any, secrets source.Secrets) (*Manager, *Store) {
	t.Helper()
	ctx := context.Background()
	store, err := Open(ctx, dbPath(t))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	require.NoError(t, store.AddLibrary(ctx, source.Library{ID: "srv", Kind: kind, Name: "Server", Enabled: true, Config: config}))
	m := NewManager(store, source.Registry{kind: factory}, source.Env{Secrets: secrets, CacheDir: t.TempDir(), Version: "1"})
	t.Cleanup(m.Close)
	require.NoError(t, m.Load(ctx))
	m.awaitAll(ctx)
	return m, store
}

// jellyfinTrack is an item as /Users/{id}/Items sends it with Details.
func jellyfinTrack(i int) string {
	return fmt.Sprintf(`{"Id":"t%d","Name":"Song %d","Type":"Audio","Album":"Album","AlbumId":"al","AlbumArtist":"Artist",
		"AlbumArtists":[{"Name":"Artist","Id":"ar"}],"Artists":["Artist"],"ArtistItems":[{"Name":"Artist","Id":"ar"}],
		"AlbumPrimaryImageTag":"x","RunTimeTicks":1800000000,"Container":"flac","IndexNumber":%d,"DateCreated":"2024-05-01T10:00:00Z",
		"UserData":{"IsFavorite":%v,"PlayCount":%d},"NormalizationGain":-3.5,
		"MediaSources":[{"Bitrate":900000,"MediaStreams":[{"Type":"Audio","BitRate":800000}]}],
		"People":[{"Name":"Bach","Type":"Composer"}]}`, i, i, i, i == 0, i)
}

// A walk over the real Jellyfin source: every page comes in, in pages of its
// own size, and bitrate and composer end up in the rows.
func TestWalkJellyfin(t *testing.T) {
	const tracks = 450
	const user = "u1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/Artists/AlbumArtists":
			fmt.Fprint(w, `{"Items":[{"Id":"ar","Name":"Artist","Type":"MusicArtist","UserData":{"IsFavorite":true}}],"TotalRecordCount":1}`)
		case q.Get("IncludeItemTypes") == "MusicAlbum":
			fmt.Fprint(w, `{"Items":[{"Id":"al","Name":"Album","Type":"MusicAlbum","AlbumArtist":"Artist","AlbumArtists":[{"Name":"Artist","Id":"ar"}],
				"ImageTags":{"Primary":"x"},"ProductionYear":2001}],"TotalRecordCount":1}`)
		case q.Get("Limit") == "0":
			fmt.Fprintf(w, `{"Items":[],"TotalRecordCount":%d}`, tracks)
		case q.Get("IncludeItemTypes") == "Audio":
			start, _ := strconv.Atoi(q.Get("StartIndex"))
			limit, _ := strconv.Atoi(q.Get("Limit"))
			var items []string
			for i := start; i < min(start+limit, tracks); i++ {
				items = append(items, jellyfinTrack(i))
			}
			fmt.Fprintf(w, `{"Items":[%s],"TotalRecordCount":%d}`, strings.Join(items, ","), tracks)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	secrets := secretsMap{"password": "", "token": "tok", "userId": user, "sessionFor": srv.URL + "\x00demo"}
	m, store := realSourceManager(t, jellyfin.Kind, jellyfin.Factory, map[string]any{"url": srv.URL, "username": "demo"}, secrets)
	ctx := context.Background()

	stats, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	assert.Equal(t, tracks, stats.Seen)
	require.True(t, store.Synced(ctx, "srv"))

	got, err := store.Track(ctx, "srv", "t0")
	require.NoError(t, err)
	assert.Equal(t, "Bach", got.Composer)
	assert.Equal(t, 800, got.Bitrate)
	assert.Equal(t, "FLAC", got.Format)
	assert.True(t, got.Favorite)
	assert.Equal(t, "al", got.CoverID)
	require.NotNil(t, got.ReplayGain)
	assert.Equal(t, -3.5, *got.ReplayGain.Track)
	album, err := store.Album(ctx, "srv", "al")
	require.NoError(t, err)
	assert.Equal(t, tracks, album.TrackCount)
	assert.Equal(t, "al", album.CoverID)
	artists, err := store.Artists(ctx, "srv")
	require.NoError(t, err)
	require.Len(t, artists, 1)
	assert.True(t, artists[0].Favorite)
	assert.Equal(t, 1, artists[0].AlbumCount)
}

// A walk over the real Subsonic source, whose count is only an estimate: a
// server that says too little still gets read to its end, and one that says
// too much still finishes.
func TestWalkSubsonic(t *testing.T) {
	for _, count := range []int{700, 5000} {
		t.Run(strconv.Itoa(count), func(t *testing.T) { walkSubsonic(t, count) })
	}
}

func walkSubsonic(t *testing.T, count int) {
	const tracks = 1100
	ok := func(body string) string {
		return `{"subsonic-response":{"status":"ok","version":"1.16.1","type":"navidrome","openSubsonic":true` + body + `}}`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch strings.TrimPrefix(r.URL.Path, "/rest/") {
		case "getArtists":
			fmt.Fprint(w, ok(`,"artists":{"index":[{"name":"A","artist":[{"id":"ar","name":"Artist","starred":"2024-01-01T00:00:00Z"}]}]}`))
		case "getAlbumList2":
			fmt.Fprint(w, ok(`,"albumList2":{"album":[{"id":"al","name":"Album","artist":"Artist","artistId":"ar","coverArt":"al-c","songCount":1100,"userRating":4}]}`))
		case "getScanStatus":
			fmt.Fprint(w, ok(fmt.Sprintf(`,"scanStatus":{"scanning":false,"count":%d}`, count)))
		case "getIndexes":
			fmt.Fprint(w, ok(`,"indexes":{"lastModified":1700000000000}`))
		case "search3":
			start, _ := strconv.Atoi(q.Get("songOffset"))
			count, _ := strconv.Atoi(q.Get("songCount"))
			var songs []string
			for i := start; i < min(start+count, tracks); i++ {
				songs = append(songs, fmt.Sprintf(`{"id":"t%d","title":"Song","album":"Album","albumId":"al","artist":"Artist","artistId":"ar",
					"coverArt":"t-c","suffix":"opus","bitRate":192,"displayComposer":"Bach","userRating":3,"playCount":2,"created":"2024-05-01T10:00:00Z"}`, i))
			}
			fmt.Fprint(w, ok(`,"searchResult3":{"song":[`+strings.Join(songs, ",")+`]}`))
		default:
			fmt.Fprint(w, ok(""))
		}
	}))
	t.Cleanup(srv.Close)
	m, store := realSourceManager(t, subsonic.Kind, subsonic.Factory, map[string]any{"url": srv.URL, "username": "demo"}, secretsMap{"password": "pw"})
	ctx := context.Background()

	stats, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	assert.Equal(t, tracks, stats.Seen)
	require.True(t, store.Synced(ctx, "srv"))
	stamp, _, err := store.CatalogState(ctx, "srv")
	require.NoError(t, err)
	assert.Equal(t, "1700000000000", stamp)
	got, err := store.Track(ctx, "srv", "t1099")
	require.NoError(t, err)
	assert.Equal(t, 192, got.Bitrate)
	assert.Equal(t, "Bach", got.Composer)
	assert.Equal(t, 3, got.Rating)
	assert.Equal(t, "t-c", got.CoverID)
	assert.Equal(t, 2, got.PlayCount)
	album, err := store.Album(ctx, "srv", "al")
	require.NoError(t, err)
	assert.Equal(t, 4, album.Rating)
	assert.Equal(t, "al-c", album.CoverID)
}
