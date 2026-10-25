package subsonic

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

func TestAuthModeIsProbedAndRemembered(t *testing.T) {
	apiKeyExt := strings.Replace(okEnvelope, "%s", `,"openSubsonicExtensions":[{"name":"apiKeyAuthentication","versions":[1]}]`, 1)
	tests := []struct {
		name       string
		configured string
		accepts    []string
		rejectCode int
		bodies     map[string]string
		password   string
		want       string
		wantErr    bool
	}{
		{name: "token server", configured: AuthAuto, accepts: []string{AuthToken, AuthPassword}, want: AuthToken},
		{name: "no token for this user (41)", configured: AuthAuto, accepts: []string{AuthPassword}, rejectCode: 41, want: AuthPassword},
		{name: "one-way password hash (40)", configured: AuthAuto, accepts: []string{AuthPassword}, rejectCode: 40, want: AuthPassword},
		{name: "api key offered", configured: AuthAuto, accepts: []string{AuthAPIKey}, rejectCode: 42,
			bodies: map[string]string{"getOpenSubsonicExtensions": apiKeyExt}, want: AuthAPIKey},
		{name: "api key not offered is never tried", configured: AuthAuto, accepts: []string{AuthAPIKey}, rejectCode: 42, wantErr: true},
		{name: "explicit token does not fall back", configured: AuthToken, accepts: []string{AuthPassword}, rejectCode: 41, wantErr: true},
		{name: "explicit password", configured: AuthPassword, accepts: []string{AuthPassword}, want: AuthPassword},
		{name: "wrong password", configured: AuthAuto, accepts: []string{AuthToken, AuthPassword}, password: "wrong", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t, "demo", tt.bodies)
			f.accepts = tt.accepts
			if tt.rejectCode != 0 {
				f.rejectCode = tt.rejectCode
			}
			password := tt.password
			if password == "" {
				password = "demo"
			}
			statePath := filepath.Join(t.TempDir(), "state.json")
			c, err := NewClient(f.URL, "demo", password, tt.configured, statePath, f.Client())
			require.NoError(t, err)
			_, err = c.Song(context.Background(), "s1")
			if tt.wantErr {
				require.Error(t, err)
				for _, q := range f.log {
					if tt.configured == AuthToken {
						assert.Empty(t, q.Get("p"), "an explicit token mode never sends the password")
					}
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, authMode(f.query("getSong")))

			// The next start signs in the remembered way on its first request.
			f.mu.Lock()
			f.log = nil
			f.mu.Unlock()
			again, err := NewClient(f.URL, "demo", password, tt.configured, statePath, f.Client())
			require.NoError(t, err)
			_, err = again.Song(context.Background(), "s1")
			require.NoError(t, err)
			assert.Equal(t, tt.want, authMode(f.log[0]))
		})
	}
}

func TestVersionIsNegotiatedDown(t *testing.T) {
	old := `{"subsonic-response":{"status":"ok","version":"1.13.0"%s}}`
	for _, strict := range []bool{false, true} {
		f := newFake(t, "demo", map[string]string{
			"ping":    strings.Replace(old, "%s", "", 1),
			"getSong": strings.Replace(old, "%s", `,"song":{"id":"s1"}`, 1),
		})
		// A strict server refuses any request newer than itself.
		if strict {
			f.maxVersion = "1.13.0"
		}
		_, err := newSource(t, f, "demo").Track(context.Background(), "s1")
		require.NoError(t, err)
		assert.Equal(t, "1.13.0", f.query("getSong").Get("v"))
	}
}

func TestUnsupportedCallsDropTheirCapability(t *testing.T) {
	ok := strings.Replace(okEnvelope, "%s", "", 1)
	f := newFake(t, "demo", map[string]string{
		"star": ok, "unstar": ok, "getStarred2": ok, "getPlaylists": ok,
		// Ampache's way of saying "no such method".
		"scrobble": `{"subsonic-response":{"status":"failed","version":"1.16.1","error":{"code":30,"message":"unsupported"}}}`,
	})
	s := newSource(t, f, "demo")
	ctx := context.Background()
	assert.Contains(t, source.Capabilities(s), source.CapRatings)

	// No setRating body: the fake answers 404, as Funkwhale does.
	err := s.SetRating(ctx, source.ItemTrack, "s1", 3)
	require.ErrorIs(t, err, ErrUnsupported)
	require.ErrorIs(t, s.Scrobble(ctx, "s1", time.Unix(0, 0)), ErrUnsupported)
	caps := source.Capabilities(s)
	assert.NotContains(t, caps, source.CapRatings)
	assert.NotContains(t, caps, source.CapScrobble)
	assert.Contains(t, caps, source.CapFavorites)

	f.bodies["setRating"] = ok
	assert.ErrorIs(t, s.SetRating(ctx, source.ItemTrack, "s1", 3), ErrUnsupported, "not asked again")
}

func TestLenientJSON(t *testing.T) {
	f := newFake(t, "demo", map[string]string{
		// Funkwhale: numeric ids. Old XML-converted servers: one-element
		// lists as objects, numbers and booleans as strings.
		"getAlbum":     strings.Replace(okEnvelope, "%s", `,"album":{"id":12,"name":"A","songCount":"1","song":{"id":34,"title":"T","track":"2","duration":"181","albumId":12}}`, 1),
		"getPlaylists": strings.Replace(okEnvelope, "%s", `,"playlists":{"playlist":{"id":"p","name":"P","readonly":"true","songCount":3}}`, 1),
	})
	s := newSource(t, f, "demo")
	ctx := context.Background()
	a, err := s.Album(ctx, "12")
	require.NoError(t, err)
	assert.Equal(t, "12", a.ID)
	require.Len(t, a.Tracks, 1)
	assert.Equal(t, source.Track{Library: "nd", ID: "34", Title: "T", AlbumID: "12", TrackNo: 2, Duration: 181}, a.Tracks[0])
	pls, err := s.Playlists(ctx)
	require.NoError(t, err)
	require.Len(t, pls, 1)
	assert.True(t, pls[0].ReadOnly)
	assert.Equal(t, 3, pls[0].TrackCount)
}

func TestMovePlaylistTrackRewritesTheOrder(t *testing.T) {
	f := newFake(t, "demo", map[string]string{
		"getPlaylist":    strings.Replace(okEnvelope, "%s", `,"playlist":{"id":"p1","entry":[{"id":"a"},{"id":"b"},{"id":"c"}]}`, 1),
		"createPlaylist": strings.Replace(okEnvelope, "%s", "", 1),
	})
	s := newSource(t, f, "demo")
	require.NoError(t, s.MovePlaylistTrack(context.Background(), "p1", 0, 2))
	q := f.query("createPlaylist")
	assert.Equal(t, "p1", q.Get("playlistId"))
	assert.Empty(t, q.Get("name"))
	assert.Equal(t, []string{"b", "c", "a"}, q["songId"])
	assert.Error(t, s.MovePlaylistTrack(context.Background(), "p1", 0, 3))
}

// A proxy answering 404 while the server restarts must not hide a feature
// for good.
func TestProxyOutageIsNotUnsupported(t *testing.T) {
	ok := strings.Replace(okEnvelope, "%s", "", 1)
	f := newFake(t, "demo", map[string]string{"ping": ok})
	s := newSource(t, f, "demo")
	ctx := context.Background()
	_, err := s.Ping(ctx)
	require.NoError(t, err)

	f.mu.Lock()
	f.bodies["ping"] = "<html>bad gateway</html>"
	f.mu.Unlock()
	err = s.SetRating(ctx, source.ItemTrack, "s1", 3)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrUnsupported)
	assert.Contains(t, source.Capabilities(s), source.CapRatings)

	f.mu.Lock()
	f.bodies["ping"], f.bodies["setRating"] = ok, ok
	f.mu.Unlock()
	assert.NoError(t, s.SetRating(ctx, source.ItemTrack, "s1", 3))
}

func TestServerInputIsChecked(t *testing.T) {
	_, err := NewClient("http://nd.example.com", "demo", "pw", AuthPassword, "", nil)
	assert.ErrorIs(t, err, errPasswordOverHTTP, "the password itself never goes over plain http")
	_, err = NewClient("https://demo:pw@nd.example.com", "demo", "pw", AuthAuto, "", nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "pw", "user info is refused without echoing it")

	f := newFake(t, "demo", map[string]string{
		"getSong": strings.Replace(okEnvelope, "%s", `,"song":{"id":"s1","duration":"NaN"}`, 1),
	})
	tr, err := newSource(t, f, "demo").Track(context.Background(), "s1")
	require.NoError(t, err)
	assert.Zero(t, tr.Duration)
}
