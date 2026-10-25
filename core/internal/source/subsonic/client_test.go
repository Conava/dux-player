package subsonic

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

const okEnvelope = `{"subsonic-response":{"status":"ok","version":"1.16.1","type":"navidrome","serverVersion":"0.64.2","openSubsonic":true%s}}`

// fakeServer answers /rest/<method> from testdata/<method>.json or from
// inline bodies, checks auth, and records every query.
type fakeServer struct {
	*httptest.Server
	password string
	// accepts are the auth modes the server takes (default: token);
	// rejected ones get rejectCode.
	accepts    []string
	rejectCode int
	// maxVersion, when set, makes requests with a newer v fail with 30.
	maxVersion string
	bodies     map[string]string
	mu         sync.Mutex
	queries    map[string]url.Values
	log        []url.Values
}

func authMode(q url.Values) string {
	switch {
	case q.Get("apiKey") != "":
		return AuthAPIKey
	case q.Get("p") != "":
		return AuthPassword
	}
	return AuthToken
}

func (f *fakeServer) authorized(q url.Values) bool {
	switch authMode(q) {
	case AuthAPIKey:
		return q.Get("u") == "" && q.Get("apiKey") == f.password
	case AuthPassword:
		return q.Get("p") == "enc:"+hex.EncodeToString([]byte(f.password))
	}
	sum := md5.Sum([]byte(f.password + q.Get("s")))
	return q.Get("t") == hex.EncodeToString(sum[:])
}

func newFake(t *testing.T, password string, bodies map[string]string) *fakeServer {
	f := &fakeServer{password: password, bodies: bodies, queries: map[string]url.Values{}, accepts: []string{AuthToken}, rejectCode: 40}
	// TLS: signing in with the password itself needs https.
	f.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/rest/")
		q := r.URL.Query()
		f.mu.Lock()
		f.queries[method] = q
		f.log = append(f.log, q)
		accepts, code := f.accepts, f.rejectCode
		f.mu.Unlock()
		if f.maxVersion != "" && versionLess(f.maxVersion, q.Get("v")) {
			_, _ = fmt.Fprintf(w, `{"subsonic-response":{"status":"failed","version":%q,"error":{"code":30,"message":"Server must upgrade"}}}`, f.maxVersion)
			return
		}
		public := method == "getOpenSubsonicExtensions"
		if !public && !(slices.Contains(accepts, authMode(q)) && f.authorized(q)) {
			if !slices.Contains(accepts, authMode(q)) {
				_, _ = fmt.Fprintf(w, `{"subsonic-response":{"status":"failed","version":"1.16.1","error":{"code":%d,"message":"no"}}}`, code)
				return
			}
			data, _ := os.ReadFile("testdata/auth_failed.json")
			_, _ = w.Write(data)
			return
		}
		if body, ok := f.bodies[method+"?id="+q.Get("id")]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		if body, ok := f.bodies[method]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		data, err := os.ReadFile(filepath.Join("testdata", method+".json"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeServer) query(method string) url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries[method]
}

func newSource(t *testing.T, f *fakeServer, password string) *Source {
	c, err := NewClient(f.URL, "demo", password, AuthAuto, "", f.Client())
	require.NoError(t, err)
	return &Source{lib: source.Library{ID: "nd"}, client: c, coverDir: t.TempDir()}
}

func TestTokenAuth(t *testing.T) {
	f := newFake(t, "demo", map[string]string{"ping": strings.Replace(okEnvelope, "%s", "", 1)})

	_, err := newSource(t, f, "demo").client.Probe(context.Background())
	require.NoError(t, err)
	first := f.query("ping")
	assert.Empty(t, first.Get("p"), "the password itself is never sent")
	assert.Equal(t, "json", first.Get("f"))

	_, err = newSource(t, f, "demo").client.Probe(context.Background())
	require.NoError(t, err)
	assert.NotEqual(t, first.Get("s"), f.query("ping").Get("s"), "every request gets a fresh salt")

	_, err = newSource(t, f, "wrong").client.Probe(context.Background())
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.True(t, apiErr.AuthFailed())
}

func TestDecodeEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "ok", body: `{"subsonic-response":{"status":"ok","version":"1.16.1"}}`},
		{name: "server error", body: `{"subsonic-response":{"status":"failed","error":{"code":70,"message":"not found"}}}`, wantErr: "server error 70: not found"},
		{name: "failed without error", body: `{"subsonic-response":{"status":"failed"}}`, wantErr: `status "failed"`},
		{name: "not subsonic", body: `<html>login</html>`, wantErr: "not an (Open)Subsonic response"},
		{name: "missing wrapper", body: `{"status":"ok"}`, wantErr: "not an (Open)Subsonic response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decode("m", []byte(tt.body), nil)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPingReportsOpenSubsonicExtensions(t *testing.T) {
	f := newFake(t, "demo", map[string]string{
		"ping":                      strings.Replace(okEnvelope, "%s", "", 1),
		"getOpenSubsonicExtensions": strings.Replace(okEnvelope, "%s", `,"openSubsonicExtensions":[{"name":"formPost","versions":[1]},{"name":"songLyrics","versions":[1,2]}]`, 1),
	})
	info, err := newSource(t, f, "demo").Ping(context.Background())
	require.NoError(t, err)
	assert.True(t, info.OpenSubsonic)
	assert.Equal(t, "navidrome", info.Type)
	assert.Equal(t, []string{"formPost", "songLyrics"}, info.Extensions)
}

func TestBrowseRecordedResponses(t *testing.T) {
	f := newFake(t, "demo", nil)
	s := newSource(t, f, "demo")
	ctx := context.Background()

	artists, err := s.Artists(ctx)
	require.NoError(t, err)
	require.Len(t, artists, 3, "artists from every index letter")
	assert.Equal(t, source.Artist{Library: "nd", ID: "33X3V2Xn7Cip7WqqfXNkL8", Name: "2 Mello", AlbumCount: 1, CoverID: "ar-33X3V2Xn7Cip7WqqfXNkL8_ff4f9b91900cbc98", Favorite: true, Rating: 3}, artists[0], "starred and userRating map to favorite and rating")

	album, err := s.Album(ctx, "5mCHAKT6FQSLoJKREdMum8")
	require.NoError(t, err)
	assert.Equal(t, "Brighten", album.Name)
	require.Len(t, album.Tracks, 3)
	assert.Equal(t, source.Track{
		Library: "nd", ID: "3kLPinv71YMdfwl8UVjRSs", Title: "Brighten", Artist: "Back On Earth",
		ArtistID: "04HrSORpypcLGNUdQp37gn", Album: "Brighten", AlbumID: "5mCHAKT6FQSLoJKREdMum8",
		Artists:     []source.ArtistRef{{ID: "04HrSORpypcLGNUdQp37gn", Name: "Back On Earth"}},
		AlbumArtist: "Back On Earth", TrackNo: 1, Year: 2013, Duration: 211,
		CoverID: "mf-3kLPinv71YMdfwl8UVjRSs_b225bb6fbf9f2dd0", Favorite: true, Rating: 1,
		PlayCount: 619, LastPlayed: 1792800507,
	}, album.Tracks[0])
	assert.Equal(t, 1233, album.PlayCount, "the server's own play counts")
}

func TestPlaylistEditsSendRepeatedParams(t *testing.T) {
	f := newFake(t, "demo", map[string]string{
		"createPlaylist": strings.Replace(okEnvelope, "%s", `,"playlist":{"id":"p1","name":"Mix","songCount":2,"owner":"demo"}`, 1),
		"updatePlaylist": strings.Replace(okEnvelope, "%s", "", 1),
	})
	s := newSource(t, f, "demo")
	ctx := context.Background()

	pl, err := s.CreatePlaylist(ctx, "Mix", []string{"a", "b"})
	require.NoError(t, err)
	assert.Equal(t, "p1", pl.ID)
	assert.Equal(t, []string{"a", "b"}, f.query("createPlaylist")["songId"])

	name := "Renamed"
	require.NoError(t, s.UpdatePlaylist(ctx, "p1", source.PlaylistEdit{Name: &name, Add: []string{"c"}, RemoveIndexes: []int{0, 3}}))
	q := f.query("updatePlaylist")
	assert.Equal(t, "p1", q.Get("playlistId"))
	assert.Equal(t, "Renamed", q.Get("name"))
	assert.Equal(t, []string{"c"}, q["songIdToAdd"])
	assert.Equal(t, []string{"0", "3"}, q["songIndexToRemove"])
}

// net/http puts the full request URL, token included, into its errors.
func TestErrorsNeverCarryTheToken(t *testing.T) {
	c, err := NewClient("https://127.0.0.1:1", "demo", "secret", AuthPassword, "", nil)
	require.NoError(t, err)
	_, err = c.Probe(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "p=")
	assert.NotContains(t, err.Error(), hex.EncodeToString([]byte("secret")))
	assert.NotContains(t, err.Error(), "127.0.0.1:1/rest")
	var ue *url.Error
	assert.False(t, errors.As(err, &ue))
}

func TestCoverIsCachedAsLocalFile(t *testing.T) {
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 32)
	f := newFake(t, "demo", map[string]string{"getCoverArt": png})
	s := newSource(t, f, "demo")

	path, err := s.Cover(context.Background(), "al-1", 300)
	require.NoError(t, err)
	assert.Equal(t, ".png", filepath.Ext(path))
	assert.Equal(t, "300", f.query("getCoverArt").Get("size"))

	f.bodies["getCoverArt"] = `{"subsonic-response":{"status":"failed","error":{"code":70,"message":"gone"}}}`
	again, err := s.Cover(context.Background(), "al-1", 300)
	require.NoError(t, err, "a cached cover needs no request")
	assert.Equal(t, path, again)

	_, err = s.Cover(context.Background(), "al-2", 300)
	assert.ErrorContains(t, err, "gone")
}

func TestFavoritesAndRatingsGoToTheServer(t *testing.T) {
	ok := strings.Replace(okEnvelope, "%s", "", 1)
	f := newFake(t, "demo", map[string]string{
		"star": ok, "unstar": ok, "setRating": ok,
		"getStarred2": strings.Replace(okEnvelope, "%s", `,"starred2":{"song":[{"id":"s1","title":"Fav","starred":"2026-01-01T00:00:00Z"}]}`, 1),
	})
	s := newSource(t, f, "demo")
	ctx := context.Background()

	require.NoError(t, s.SetFavorite(ctx, source.ItemAlbum, "al1", true))
	assert.Equal(t, "al1", f.query("star").Get("albumId"), "albums star by albumId")
	require.NoError(t, s.SetFavorite(ctx, source.ItemTrack, "s1", false))
	assert.Equal(t, "s1", f.query("unstar").Get("id"))
	require.NoError(t, s.SetRating(ctx, source.ItemArtist, "ar1", 0))
	assert.Equal(t, "0", f.query("setRating").Get("rating"))
	assert.Error(t, s.SetRating(ctx, source.ItemTrack, "s1", 9))

	fav, err := s.Favorites(ctx)
	require.NoError(t, err)
	require.Len(t, fav.Tracks, 1)
	assert.True(t, fav.Tracks[0].Favorite)
}

func TestTrackCountAndServerReplayGain(t *testing.T) {
	scan := strings.Replace(okEnvelope, "%s", `,"scanStatus":{"scanning":false,"count":501}`, 1)
	f := newFake(t, "demo", map[string]string{
		"getScanStatus":  scan,
		"getRandomSongs": strings.Replace(okEnvelope, "%s", `,"randomSongs":{"song":[{"id":"s1","replayGain":{"trackGain":-7.25,"albumGain":-6}},{"id":"s2"}]}`, 1),
	})
	s := newSource(t, f, "demo")
	ctx := context.Background()
	n, err := s.TrackCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 501, n)

	f.bodies["getScanStatus"] = strings.Replace(okEnvelope, "%s", `,"scanStatus":{"scanning":true,"count":12}`, 1)
	f.bodies["getAlbumList2"] = strings.Replace(okEnvelope, "%s", `,"albumList2":{"album":[{"id":"a","songCount":10},{"id":"b","songCount":7}]}`, 1)
	n, err = s.TrackCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 17, n, "mid-scan counts are partial; the album list is summed instead")

	tracks, err := s.RandomTracks(ctx, 2)
	require.NoError(t, err)
	require.Len(t, tracks, 2)
	require.NotNil(t, tracks[0].ReplayGain)
	assert.Equal(t, -7.25, *tracks[0].ReplayGain.Track)
	assert.Equal(t, -6.0, *tracks[0].ReplayGain.Album)
	assert.Nil(t, tracks[1].ReplayGain)
}

func TestTrackPages(t *testing.T) {
	env := func(s string) string { return strings.Replace(okEnvelope, "%s", ","+s, 1) }
	ctx := context.Background()

	f := newFake(t, "demo", map[string]string{
		"getScanStatus": env(`"scanStatus":{"scanning":false,"count":3}`),
		"search3":       env(`"searchResult3":{"song":[{"id":"s2"},{"id":"s3"}]}`),
	})
	page, err := newSource(t, f, "demo").TrackPage(ctx, source.TrackQuery{Offset: 1, Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, 3, page.Total)
	assert.Equal(t, []string{"s2", "s3"}, ids(page.Tracks))
	assert.Equal(t, "1", f.query("search3").Get("songOffset"))

	// A server that answers the empty search with nothing is paged by album.
	f = newFake(t, "demo", map[string]string{
		"search3":       env(`"searchResult3":{}`),
		"getScanStatus": env(`"scanStatus":{"scanning":false,"count":5}`),
		"getAlbumList2": env(`"albumList2":{"album":[{"id":"a","songCount":2},{"id":"b","songCount":2},{"id":"c","songCount":1}]}`),
		"getAlbum?id=a": env(`"album":{"id":"a","song":[{"id":"a1"},{"id":"a2"}]}`),
		"getAlbum?id=b": env(`"album":{"id":"b","song":[{"id":"b1"},{"id":"b2"}]}`),
		"getAlbum?id=c": env(`"album":{"id":"c","song":[{"id":"c1"}]}`),
	})
	s := newSource(t, f, "demo")
	page, err = s.TrackPage(ctx, source.TrackQuery{Offset: 0, Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"a1", "a2"}, ids(page.Tracks))
	page, err = s.TrackPage(ctx, source.TrackQuery{Offset: 1, Limit: 3})
	require.NoError(t, err)
	assert.Equal(t, 5, page.Total)
	assert.Equal(t, []string{"a2", "b1", "b2"}, ids(page.Tracks))
}

func ids(ts []source.Track) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return out
}

// Shaped like Navidrome 0.64's getSong; its 0 dB gains mean no ReplayGain.
func TestSongDetails(t *testing.T) {
	f := newFake(t, "demo", map[string]string{"getSong": strings.Replace(okEnvelope, "%s", `,"song":{"id":"s1","title":"sad robot",
		"size":9870638,"contentType":"audio/mpeg","suffix":"mp3","bitRate":248,"path":"p/01 - sad robot.mp3","comment":"hi",
		"musicBrainzId":"rec-1","genres":[{"name":"Chiptune"}],"genre":"Chiptune","replayGain":{"trackGain":0,"albumGain":-3.5,"trackPeak":1,"albumPeak":0.9},
		"channelCount":2,"samplingRate":44100,"bitDepth":0}`, 1)})
	s := newSource(t, f, "demo")
	d, err := s.Details(context.Background(), "track", "s1")
	require.NoError(t, err)
	assert.Equal(t, "MP3", d.Format)
	assert.Equal(t, 248, d.Bitrate)
	assert.Equal(t, 44100, d.SampleRate)
	assert.Equal(t, int64(9870638), d.Size)
	assert.Equal(t, "p/01 - sad robot.mp3", d.Path)
	assert.Equal(t, []string{"Chiptune"}, d.Genres)
	assert.Equal(t, map[string]string{"recordingId": "rec-1"}, d.MusicBrainz)
	require.NotNil(t, d.ReplayGain)
	assert.Nil(t, d.ReplayGain.TrackGain)
	assert.Nil(t, d.ReplayGain.TrackPeak)
	assert.InDelta(t, 0.9, *d.ReplayGain.AlbumPeak, 0.001)
	assert.NotContains(t, d.Server, "demo", "no credentials")
}

func TestArtistInfo(t *testing.T) {
	env := func(s string) string { return strings.Replace(okEnvelope, "%s", ","+s, 1) }
	f := newFake(t, "demo", map[string]string{
		"getArtist": env(`"artist":{"id":"ar1","name":"Ann","coverArt":"ar-ar1","starred":"2024-01-01T00:00:00Z","userRating":4,
			"album":[{"id":"al1","songCount":10,"artists":[{"id":"ar1","name":"Ann"},{"id":"ar2","name":"Bob"}]},{"id":"al2","songCount":3}]}`),
		"getArtistInfo2": env(`"artistInfo2":{"biography":"Ann &amp; friends. <a href=\"https://last.fm/ann\">Read more on Last.fm</a>",
			"musicBrainzId":"mb1","similarArtist":[{"id":"ar3","name":"Cy"},{"name":"Not in the library"}]}`),
	})
	s := newSource(t, f, "demo")
	info, err := s.ArtistInfo(context.Background(), "ar1")
	require.NoError(t, err)
	assert.Equal(t, source.Artist{Library: "nd", ID: "ar1", Name: "Ann", AlbumCount: 2, CoverID: "ar-ar1", Favorite: true, Rating: 4}, info.Artist)
	assert.Equal(t, 13, info.TrackCount)
	assert.Equal(t, "Ann & friends. Read more on Last.fm", info.Biography)
	assert.Equal(t, "mb1", info.MusicBrainzID)
	assert.Equal(t, []source.Artist{{Library: "nd", ID: "ar3", Name: "Cy"}}, info.Similar, "only similar artists the library has")

	albums, err := s.Albums(context.Background(), "ar1")
	require.NoError(t, err)
	assert.Equal(t, []source.ArtistRef{{ID: "ar1", Name: "Ann"}, {ID: "ar2", Name: "Bob"}}, albums[0].Artists, "OpenSubsonic album artists")
}

func TestRecentPlaysReadsLastPlayedAlbums(t *testing.T) {
	f := newFake(t, "demo", map[string]string{
		"getAlbumList2": strings.Replace(okEnvelope, "%s", `,"albumList2":{"album":[{"id":"5mCHAKT6FQSLoJKREdMum8"}]}`, 1),
	})
	tracks, err := newSource(t, f, "demo").RecentPlays(context.Background())
	require.NoError(t, err)
	require.Len(t, tracks, 3)
	assert.Equal(t, 619, tracks[0].PlayCount)
	assert.Equal(t, "recent", f.query("getAlbumList2").Get("type"))
}

func TestAPIErrorSignInRefused(t *testing.T) {
	for code, refused := range map[int]bool{40: true, 41: false, 42: false, 44: true, 70: false} {
		err := fmt.Errorf("ping: %w", &APIError{Code: code})
		assert.Equal(t, refused, errors.Is(err, source.ErrSignInRefused), code)
	}
}
