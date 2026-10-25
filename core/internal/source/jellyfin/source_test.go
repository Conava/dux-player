package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

const (
	albumID    = "f7e6d83954e06a0d46724a6068fa21ff"
	playlistID = "d970eee82134b487f1df37d753fab1f9"
	userID     = "4ed1b8b42a7c4ea682f0fe5d08d4e278"
)

type request struct {
	method, path string
	query        url.Values
	auth         string
	body         string
}

// fake answers from testdata recorded from the Jellyfin demo server (playlist
// entry ids changed to differ from item ids, as before Jellyfin 12), accepts
// only the token it last issued, and records requests.
type fake struct {
	*httptest.Server
	mu       sync.Mutex
	token    string
	logins   int
	requests []request
}

func newFake(t *testing.T) *fake {
	f := &fake{}
	routes := map[string]string{
		"/Users/" + userID + "/Items/" + albumID: "album.json",
		"/Playlists/" + playlistID + "/Items":    "playlist_items.json",
		"/Artists/AlbumArtists":                  "album_artists.json",
	}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.requests = append(f.requests, request{r.Method, r.URL.Path, r.URL.Query(), r.Header.Get("Authorization"), string(body)})
		token := f.token
		f.mu.Unlock()
		if r.URL.Path == "/Users/AuthenticateByName" {
			// Slow enough that parallel sign-ins overlap.
			time.Sleep(20 * time.Millisecond)
			f.mu.Lock()
			f.logins++
			f.token = fmt.Sprintf("tok%d", f.logins)
			token = f.token
			f.mu.Unlock()
			_, _ = fmt.Fprintf(w, `{"AccessToken":%q,"User":{"Id":%q}}`, token, userID)
			return
		}
		if !strings.Contains(r.Header.Get("Authorization"), `Token="`+token+`"`) || token == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/Playlists" {
			_, _ = w.Write([]byte(`{"Id":"new1"}`))
			return
		}
		file := routes[r.URL.Path]
		if r.URL.Path == "/Users/"+userID+"/Items" && r.URL.Query().Get("ParentId") == albumID {
			file = "album_tracks.json"
		}
		if file == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		data, err := os.ReadFile(filepath.Join("testdata", file))
		require.NoError(t, err)
		_, _ = w.Write(data)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fake) last(path string) request {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.requests) - 1; i >= 0; i-- {
		if f.requests[i].path == path {
			return f.requests[i]
		}
	}
	return request{}
}

type memSecrets map[string]string

var memMu sync.Mutex

func (m memSecrets) Get(lib, key string) (string, error) {
	memMu.Lock()
	defer memMu.Unlock()
	v, ok := m[lib+"/"+key]
	if !ok {
		return "", errors.New("no " + key)
	}
	return v, nil
}
func (m memSecrets) Set(lib, key, value string) error {
	memMu.Lock()
	defer memMu.Unlock()
	m[lib+"/"+key] = value
	return nil
}
func (m memSecrets) Delete(string) error { return nil }

func newSource(t *testing.T, f *fake, secrets memSecrets) *Source {
	lib := source.Library{ID: "jf", Kind: Kind, Name: "JF", Config: map[string]any{"url": f.URL, "username": "demo"}}
	src, err := Factory(lib, source.Env{Secrets: secrets, CacheDir: t.TempDir(), Version: "1.2.3"})
	require.NoError(t, err)
	s := src.(*Source)
	s.client.http = f.Client()
	return s
}

func TestSignInIsKeptAndRenewedOnce(t *testing.T) {
	f := newFake(t)
	secrets := memSecrets{"jf/password": ""}
	ctx := context.Background()

	_, err := newSource(t, f, secrets).Album(ctx, albumID)
	require.NoError(t, err)
	assert.Equal(t, "tok1", secrets["jf/token"], "the token goes to the keyring")
	auth := f.last("/Users/" + userID + "/Items/" + albumID).auth
	for _, part := range []string{`MediaBrowser Client="Dux Player"`, `DeviceId="dplayer-jf"`, `Version="1.2.3"`, `Token="tok1"`} {
		assert.Contains(t, auth, part)
	}

	// A restart reuses the stored token: no new login.
	_, err = newSource(t, f, secrets).Album(ctx, albumID)
	require.NoError(t, err)
	assert.Equal(t, 1, f.logins)

	// A revoked token means one new login, then the call goes through.
	f.mu.Lock()
	f.token = "revoked"
	f.mu.Unlock()
	_, err = newSource(t, f, secrets).Album(ctx, albumID)
	require.NoError(t, err)
	assert.Equal(t, 2, f.logins)
	assert.Equal(t, "tok2", secrets["jf/token"])
}

// After a restart the stored token may be revoked. The first requests
// arrive together; Jellyfin revokes the previous token of the device on
// every sign-in, so each must not sign in on its own.
func TestStaleTokenIsRenewedOnceForParallelRequests(t *testing.T) {
	f := newFake(t)
	secrets := memSecrets{"jf/password": "", "jf/token": "stale", "jf/userId": userID, "jf/sessionFor": f.URL + "\x00demo"}
	src := newSource(t, f, secrets)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 9)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := src.Album(ctx, albumID)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, 1, f.logins)

	f.mu.Lock()
	f.token = "revoked"
	f.mu.Unlock()
	fresh := newSource(t, f, memSecrets{"jf/password": "", "jf/token": "stale", "jf/userId": userID, "jf/sessionFor": f.URL + "\x00demo"})
	_, err := fresh.StreamURL(ctx, "x")
	require.NoError(t, err)
	assert.Equal(t, 2, f.logins, "a stale token is renewed before mpv gets a stream URL")
}

func TestBrowseRecordedResponses(t *testing.T) {
	f := newFake(t)
	s := newSource(t, f, memSecrets{"jf/password": ""})
	ctx := context.Background()

	album, err := s.Album(ctx, albumID)
	require.NoError(t, err)
	assert.Equal(t, "Nemesis", album.Name)
	assert.Equal(t, "Leap Fidei", album.Artist)
	assert.True(t, album.Favorite)
	require.Len(t, album.Tracks, 1)
	gain := -4.3999996
	assert.Equal(t, source.Track{
		Library: "jf", ID: "90b750a2cec4eb338c0dab3b40b8bb13", Title: "Jellyfin", Artist: "Leap Fidei",
		ArtistID: "78e6763b3d9b58c349a58ce5eb7877c1", Album: "Nemesis", AlbumID: albumID, AlbumArtist: "Leap Fidei",
		Artists: []source.ArtistRef{{ID: "78e6763b3d9b58c349a58ce5eb7877c1", Name: "Leap Fidei"}},
		TrackNo: 10, Year: 2024, Genre: "Rap", Duration: 183.0508539, CoverID: "90b750a2cec4eb338c0dab3b40b8bb13",
		ReplayGain: &source.ReplayGain{Track: &gain}, PlayCount: 8,
	}, album.Tracks[0])

	artists, err := s.Artists(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, artists)
	assert.Equal(t, userID, f.last("/Artists/AlbumArtists").query.Get("UserId"), "the user id is filled in after sign-in")
}

func TestPlaylistEditsUseEntryIDs(t *testing.T) {
	f := newFake(t)
	s := newSource(t, f, memSecrets{"jf/password": ""})
	ctx := context.Background()
	require.NoError(t, s.UpdatePlaylist(ctx, playlistID, source.PlaylistEdit{RemoveIndexes: []int{1}, Add: []string{"x"}}))
	del := f.last("/Playlists/" + playlistID + "/Items")
	assert.Equal(t, http.MethodPost, del.method, "add comes after remove")
	assert.Equal(t, "x", del.query.Get("Ids"))
	var removed request
	for _, r := range f.requests {
		if r.method == http.MethodDelete {
			removed = r
		}
	}
	assert.Equal(t, "entry1", removed.query.Get("EntryIds"))

	require.NoError(t, s.MovePlaylistTrack(ctx, playlistID, 0, 2))
	assert.Equal(t, http.MethodPost, f.last("/Playlists/"+playlistID+"/Items/entry0/Move/2").method)
	assert.Error(t, s.MovePlaylistTrack(ctx, playlistID, 0, 3))
	assert.Error(t, s.UpdatePlaylist(ctx, playlistID, source.PlaylistEdit{RemoveIndexes: []int{5}}))
}

// An empty new playlist sends Ids as [], not null.
func TestCreateEmptyPlaylist(t *testing.T) {
	f := newFake(t)
	s := newSource(t, f, memSecrets{"jf/password": ""})
	pl, err := s.CreatePlaylist(context.Background(), "Mix", nil)
	require.NoError(t, err)
	assert.Equal(t, "new1", pl.ID)
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(f.last("/Playlists").body), &body))
	assert.Equal(t, []any{}, body["Ids"])
	assert.Equal(t, "Mix", body["Name"])
	assert.Equal(t, userID, body["UserId"])
}

func TestCapabilitiesHaveNoRatings(t *testing.T) {
	s := newSource(t, newFake(t), memSecrets{"jf/password": ""})
	caps := source.Capabilities(s)
	assert.NotContains(t, caps, source.CapRatings)
	for _, c := range []string{source.CapFavorites, source.CapPlaylistEdit, source.CapPlaylistReorder, source.CapPlaylistCovers, source.CapScrobble, source.CapRandomTracks} {
		assert.Contains(t, caps, c)
	}
}

func TestStreamURLAndErrorsKeepTheTokenPrivate(t *testing.T) {
	f := newFake(t)
	s := newSource(t, f, memSecrets{"jf/password": ""})
	u, err := s.StreamURL(context.Background(), "a1")
	require.NoError(t, err)
	q, _ := url.Parse(u)
	assert.Equal(t, "tok1", q.Query().Get("ApiKey"))
	assert.Equal(t, "tok1", q.Query().Get("api_key"))

	c, err := NewClient("http://127.0.0.1:1", "demo", "pw", "dev", "1", nil)
	require.NoError(t, err)
	c.setSession("secret-token", userID)
	err = c.get(context.Background(), "/Users/{user}/Items/abc", nil, nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-token")
	assert.NotContains(t, err.Error(), userID)
	assert.NotContains(t, err.Error(), "127.0.0.1:1/")
}

// A stored session belongs to the user it was made for: changing the
// username signs in again instead of reusing the old user's token.
func TestEditedLibrarySignsInAgain(t *testing.T) {
	f := newFake(t)
	secrets := memSecrets{"jf/password": ""}
	ctx := context.Background()
	_, err := newSource(t, f, secrets).Album(ctx, albumID)
	require.NoError(t, err)

	lib := source.Library{ID: "jf", Kind: Kind, Name: "JF", Config: map[string]any{"url": f.URL, "username": "someone"}}
	src, err := Factory(lib, source.Env{Secrets: secrets, CacheDir: t.TempDir()})
	require.NoError(t, err)
	s := src.(*Source)
	s.client.http = f.Client()
	_, err = s.Album(ctx, albumID)
	require.NoError(t, err)
	assert.Equal(t, 2, f.logins)
}
