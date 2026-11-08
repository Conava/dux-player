package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	duxipc "github.com/duxlabs-dev/dux-go/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/source"
)

// syncServer lists one track and turns unreachable when down is set.
type syncServer struct {
	lib  source.Library
	down bool
	// more lists this many tracks besides the first.
	more int
}

func (s *syncServer) Library() source.Library { return s.lib }
func (s *syncServer) Track(context.Context, string) (source.Track, error) {
	return source.Track{}, errors.New("unreachable")
}
func (s *syncServer) StreamURL(context.Context, string) (string, error) { return "", nil }
func (s *syncServer) Close() error                                      { return nil }
func (s *syncServer) Ping(context.Context) (source.ServerInfo, error) {
	return source.ServerInfo{}, nil
}
func (s *syncServer) Artists(context.Context) ([]source.Artist, error) {
	return []source.Artist{{ID: "ar", Name: "Stored artist"}}, s.err()
}
func (s *syncServer) Albums(context.Context, string) ([]source.Album, error) {
	return []source.Album{{ID: "al", Name: "Stored album"}}, s.err()
}
func (s *syncServer) Album(context.Context, string) (source.Album, error) {
	return source.Album{}, s.err()
}
func (s *syncServer) Search(context.Context, string, int) (source.SearchResult, error) {
	return source.SearchResult{}, s.err()
}
func (s *syncServer) TrackPage(_ context.Context, q source.TrackQuery) (source.TrackPage, error) {
	page := source.TrackPage{Total: 1 + s.more}
	if q.Offset == 0 {
		page.Tracks = []source.Track{{ID: "t", Title: "Stored", AlbumID: "al", Album: "Stored album"}}
		for i := range s.more {
			page.Tracks = append(page.Tracks, source.Track{ID: fmt.Sprintf("t%d", i), AlbumID: "al"})
		}
	}
	return page, s.err()
}
func (s *syncServer) CatalogChanged(context.Context, string) (bool, string, error) {
	return true, "stamp", s.err()
}
func (s *syncServer) CatalogBusy(context.Context) bool { return false }
func (s *syncServer) SetFavorite(context.Context, string, string, bool) error {
	return s.err()
}
func (s *syncServer) Favorites(context.Context) (source.SearchResult, error) {
	return source.SearchResult{}, s.err()
}
func (s *syncServer) err() error {
	if s.down {
		return errors.New("connection refused")
	}
	return nil
}

// dbPath is a database outside t.TempDir: a Manager's goroutines publish
// after a test ends, and a late write would fail the temp dir's removal.
func dbPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "dplayer-ipc-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "library.db")
}

// ask sends one request to handle and returns its reply.
func ask(t *testing.T, handle func(w *ConnWriter, req Request), method string, params map[string]any) json.RawMessage {
	t.Helper()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	go handle(duxipc.NewConnWriter(a), Request{ID: 1, Method: method, Params: params})
	line := make(chan []byte, 1)
	go func() {
		sc := bufio.NewScanner(b)
		sc.Buffer(nil, 1<<20)
		sc.Scan()
		line <- sc.Bytes()
	}()
	select {
	case data := <-line:
		var reply struct {
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		require.NoError(t, json.Unmarshal(data, &reply), string(data))
		require.Empty(t, reply.Error)
		return reply.Result
	case <-time.After(5 * time.Second):
		require.FailNow(t, "no reply to "+method)
		return nil
	}
}

// Only a scan someone asked for may remove most of a server's rows: a walk by
// itself can't tell a deletion from an unmounted disk.
func TestOnlyARescanTakesAMuchSmallerList(t *testing.T) {
	ctx := context.Background()
	store, err := library.Open(ctx, dbPath(t))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	require.NoError(t, store.AddLibrary(ctx, source.Library{ID: "srv", Kind: "fake", Name: "Server", Enabled: true}))
	server := &syncServer{lib: source.Library{ID: "srv", Name: "Server"}, more: 3}
	libs := library.NewManager(store, source.Registry{"fake": func(source.Library, source.Env) (source.Source, error) { return server, nil }}, source.Env{})
	t.Cleanup(libs.Close)
	require.NoError(t, libs.Load(ctx))
	deps := Deps{Bus: NewBus(), Libraries: libs, Ctx: ctx}
	count := func() int {
		n, err := store.TrackCount(ctx, "srv")
		require.NoError(t, err)
		return n
	}
	scan(deps, "srv", false)
	require.Equal(t, 4, count())

	server.more = 0
	SyncCatalog(deps, "srv")
	assert.Equal(t, 4, count())
	scan(deps, "srv", false)
	assert.Equal(t, 1, count())
}

// Over IPC a synced server can be rescanned, is browsed from the index with
// the server down, and a heart set before a walk is there right after it.
func TestSyncedServerOverIPC(t *testing.T) {
	ctx := context.Background()
	store, err := library.Open(ctx, dbPath(t))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	require.NoError(t, store.AddLibrary(ctx, source.Library{ID: "srv", Kind: "fake", Name: "Server", Enabled: true}))
	server := &syncServer{lib: source.Library{ID: "srv", Name: "Server"}}
	libs := library.NewManager(store, source.Registry{"fake": func(source.Library, source.Env) (source.Source, error) { return server, nil }}, source.Env{})
	t.Cleanup(libs.Close)
	require.NoError(t, libs.Load(ctx))
	deps := Deps{Bus: NewBus(), Libraries: libs, Ctx: ctx}
	done := make(chan map[string]any, 4)
	deps.Bus.On(ctx, func(_ string, data any) {
		if ev := data.(map[string]any); ev["type"] == "scanDone" {
			done <- ev
		}
	}, library.TopicLibraries)
	libraries := func(w *ConnWriter, req Request) { handleLibraries(ctx, w, req, deps) }
	browse := func(w *ConnWriter, req Request) { handleBrowse(ctx, w, req, deps) }
	marks := func(w *ConnWriter, req Request) { handleMarks(ctx, w, req, deps) }

	ask(t, libraries, "libraries.scan", map[string]any{"library": "srv"})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "no scanDone")
	}

	ask(t, marks, "marks.setFavorite", map[string]any{"library": "srv", "kind": "album", "id": "al", "favorite": true})
	server.down = true
	data := ask(t, browse, "browse.albums", map[string]any{"library": "srv"})
	var albums []source.Album
	require.NoError(t, json.Unmarshal(data, &albums))
	require.Len(t, albums, 1)
	assert.Equal(t, "Stored album", albums[0].Name)
	assert.True(t, albums[0].Favorite, "written through")
	data = ask(t, browse, "browse.tracks", map[string]any{"library": "srv"})
	var page source.TrackPage
	require.NoError(t, json.Unmarshal(data, &page))
	require.Len(t, page.Tracks, 1)

	// A failed sync says whether anyone asked for it: only then a toast.
	failed := make(chan bool, 2)
	deps.Bus.On(ctx, func(_ string, data any) {
		if ev := data.(map[string]any); ev["type"] == "scanFailed" {
			failed <- ev["background"].(bool)
		}
	}, library.TopicLibraries)
	SyncCatalog(deps, "srv")
	assert.True(t, <-failed)
	scan(deps, "srv", false)
	assert.False(t, <-failed)
}
