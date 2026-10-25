package lyrics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	duxlyrics "github.com/duxlabs-dev/dux-go/lyrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

type localLib struct{ dir string }

func (l localLib) Library() source.Library                             { return source.Library{ID: "loc", Name: "Music"} }
func (l localLib) Track(context.Context, string) (source.Track, error) { return source.Track{}, nil }
func (l localLib) StreamURL(_ context.Context, id string) (string, error) {
	return filepath.Join(l.dir, id), nil
}
func (l localLib) Close() error { return nil }

type serverLib struct{ localLib }

func (serverLib) Library() source.Library                         { return source.Library{ID: "srv", Name: "Navidrome"} }
func (serverLib) Ping(context.Context) (source.ServerInfo, error) { return source.ServerInfo{}, nil }
func (serverLib) StreamURL(context.Context, string) (string, error) {
	return "https://server/stream?t=secret", nil
}
func (serverLib) Lyrics(_ context.Context, id string) (*duxlyrics.Lyrics, error) {
	if id != "song" {
		return &duxlyrics.Lyrics{}, nil
	}
	return &duxlyrics.Lyrics{Synced: []duxlyrics.Line{{Start: 1, Text: "from the server"}}}, nil
}

type libs map[string]source.Source

func (l libs) Get(id string) (source.Source, error) { return l[id], nil }
func (l libs) Tracks(_ context.Context, refs []source.Ref) ([]source.Track, error) {
	return []source.Track{{Library: refs[0].Library, ID: refs[0].ID, Title: "t", Artist: "a"}}, nil
}

// Local tracks find their sidecar file, server tracks their server's
// lyrics; no provider is on, so nothing touches the network.
func TestLookupUsesSidecarsAndTheServer(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "song.flac"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "song.lrc"), []byte("[00:01.00]from the sidecar\n"), 0o644))
	s := New(context.Background(), Config{CacheDir: dir, ConfigDir: dir, Libraries: libs{"loc": localLib{dir}, "srv": serverLib{}},
		Publish: func(string, any) {}, HasSubscriber: func(string) bool { return false }})
	var off []Provider
	for _, p := range defaultProviders() {
		off = append(off, Provider{ID: p.ID, Enabled: p.ID == ServerSource})
	}
	_, err := s.providers.set(off)
	require.NoError(t, err)

	tests := []struct {
		ref    source.Ref
		source duxlyrics.Provider
		text   string
	}{
		{source.Ref{Library: "loc", ID: "song.flac"}, duxlyrics.Sidecar, "from the sidecar"},
		{source.Ref{Library: "srv", ID: "song"}, ServerSource, "from the server"},
		{source.Ref{Library: "srv", ID: "other"}, "", ""},
	}
	for _, tt := range tests {
		res, err := s.Get(context.Background(), tt.ref)
		require.NoError(t, err)
		assert.Equal(t, tt.source, res.Source, tt.ref.ID)
		if tt.text == "" {
			assert.False(t, res.Found)
			continue
		}
		require.NotEmpty(t, res.Synced)
		assert.Equal(t, tt.text, res.Synced[0].Text)
	}
}

func TestProviderSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), providersFile)
	s := openProviders(path)
	assert.Equal(t, defaultProviders(), s.get())

	_, err := s.set([]Provider{{ID: "nope"}})
	assert.Error(t, err)

	cli, err := providerParam([]any{"lrclib", "kugou"})
	require.NoError(t, err)
	_, err = s.set(cli)
	require.NoError(t, err)
	assert.Equal(t, []duxlyrics.Provider{duxlyrics.LRCLIB, duxlyrics.KuGou}, openProviders(path).enabled())

	// A saved file with an unknown id and a provider missing keeps working.
	require.NoError(t, os.WriteFile(path, []byte(`{"providers":[{"id":"gone","enabled":true},{"id":"kugou","enabled":true}]}`), 0o644))
	got := openProviders(path).get()
	assert.Equal(t, ServerSource, got[0].ID, "a file from before the server row gets it first")
	assert.True(t, got[0].Enabled)
	assert.Equal(t, "kugou", got[1].ID)
	assert.Len(t, got, len(defaultProviders()))
}

// Lyrics are looked up only while someone watches, once per track.
func TestCurrentTrackLyricsAreLazy(t *testing.T) {
	dir := t.TempDir()
	events := make(chan Event, 10)
	watching := false
	s := New(context.Background(), Config{CacheDir: dir, ConfigDir: dir, Libraries: libs{"srv": serverLib{}},
		Publish: func(_ string, d any) { events <- d.(Event) }, HasSubscriber: func(string) bool { return watching }})
	_, err := s.providers.set([]Provider{{ID: "lrclib"}, {ID: "betterlyrics"}, {ID: "unison"}, {ID: "lyricsplus"}, {ID: "kugou"}, {ID: "youtubemusic"}})
	require.NoError(t, err)

	s.TrackChanged(&source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"})
	assert.Empty(t, events, "nobody watches")

	watching = true
	s.Subscribed()
	assert.Equal(t, "loading", (<-events).State)
	ready := <-events
	assert.Equal(t, "ready", ready.State)
	assert.Equal(t, "song", ready.ID)

	s.Subscribed()
	assert.Equal(t, "ready", (<-events).State, "a second subscriber gets the result without a new lookup")
	assert.Empty(t, events)
}

// Synced beats plain whoever sends it; between equals the order decides.
func TestPickPrefersSyncOverOrder(t *testing.T) {
	plain := func(src duxlyrics.Provider) answer {
		return answer{res: &duxlyrics.Result{Found: true, Source: src, Lyrics: duxlyrics.Lyrics{Plain: "p"}}}
	}
	synced := func(src duxlyrics.Provider) answer {
		a := plain(src)
		a.res.Synced = []duxlyrics.Line{{Start: 1, Text: "s"}}
		return a
	}
	none := answer{res: &duxlyrics.Result{}}
	failed := answer{err: errors.New("down")}
	serverFirst := []duxlyrics.Provider{ServerSource, duxlyrics.LRCLIB}
	serverLast := []duxlyrics.Provider{duxlyrics.LRCLIB, ServerSource}
	tests := []struct {
		name      string
		order     []duxlyrics.Provider
		srv, prov answer
		want      duxlyrics.Provider
		synced    bool
	}{
		{"server plain loses to provider synced", serverFirst, plain(ServerSource), synced(duxlyrics.LRCLIB), duxlyrics.LRCLIB, true},
		{"server synced beats provider plain", serverLast, synced(ServerSource), plain(duxlyrics.LRCLIB), ServerSource, true},
		{"equal: server first", serverFirst, plain(ServerSource), plain(duxlyrics.LRCLIB), ServerSource, false},
		{"equal: server last", serverLast, synced(ServerSource), synced(duxlyrics.LRCLIB), duxlyrics.LRCLIB, true},
		{"sidecar beats an equal server", serverFirst, synced(ServerSource), synced(duxlyrics.Sidecar), duxlyrics.Sidecar, true},
		{"server plain when providers have nothing", serverFirst, plain(ServerSource), none, ServerSource, false},
		{"server plain when providers fail", serverFirst, plain(ServerSource), failed, ServerSource, false},
		{"provider when the server fails", serverFirst, failed, plain(duxlyrics.LRCLIB), duxlyrics.LRCLIB, false},
	}
	for _, tt := range tests {
		res, err := pick(tt.order, tt.srv, tt.prov)
		require.NoError(t, err, tt.name)
		assert.Equal(t, tt.want, res.Source, tt.name)
		assert.Equal(t, tt.synced, len(res.Synced) > 0, tt.name)
	}
}

// The server row turns off like any provider.
func TestServerLyricsCanBeTurnedOff(t *testing.T) {
	dir := t.TempDir()
	s := New(context.Background(), Config{CacheDir: dir, ConfigDir: dir, Libraries: libs{"srv": serverLib{}},
		Publish: func(string, any) {}, HasSubscriber: func(string) bool { return false }})
	var off []Provider
	for _, p := range defaultProviders() {
		off = append(off, Provider{ID: p.ID})
	}
	_, err := s.providers.set(off)
	require.NoError(t, err)
	res, err := s.Get(context.Background(), source.Ref{Library: "srv", ID: "song"})
	require.NoError(t, err)
	assert.False(t, res.Found)
}
