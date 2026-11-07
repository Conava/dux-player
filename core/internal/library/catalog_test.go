package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/keyring"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

func init() { pagePause = 0 }

// dbPath is a database outside t.TempDir: a Manager's goroutines publish
// after a test ends, and a late write would fail the temp dir's removal.
func dbPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "dplayer-catalog-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "library.db")
}

// catalogServer is a server with a catalog, as far as the Manager can tell.
type catalogServer struct {
	lib source.Library

	mu        sync.Mutex
	tracks    []source.Track
	total     int // what pages report; 0 is len(tracks)
	pageErr   error
	failAt    int // page offset that fails with pageErr
	busy      bool
	changed   bool
	stamp     string
	stamps    []string // what CatalogChanged was asked
	walks     int      // walks started: Artists calls
	live      atomic.Int32
	pingErr   error
	onPage    func()
	pageCtx   context.Context
	favorite  source.SearchResult
	favErr    error
	favCalls  int
	recent    []source.Track
	recentErr error
	ping      chan struct{}
	// estimated marks pages' Total as a guess; albumTracks is what the album
	// list says it holds.
	estimated   bool
	albumTracks int
	// reported is a refusal the source ran into by itself.
	reported atomic.Bool
}

func newCatalogServer(n int) *catalogServer {
	s := &catalogServer{stamp: "s1"}
	for i := range n {
		s.tracks = append(s.tracks, source.Track{ID: fmt.Sprintf("t%d", i), Title: fmt.Sprintf("Track %d", i), AlbumID: "al", Album: "Album", ArtistID: "ar",
			Artist: "Artist", AlbumArtist: "Artist", TrackNo: i, Composer: "Bach", Bitrate: 320, Artists: []source.ArtistRef{{ID: "ar", Name: "Artist"}}})
	}
	return s
}

func (s *catalogServer) Library() source.Library { return s.lib }
func (s *catalogServer) Track(_ context.Context, id string) (source.Track, error) {
	s.live.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pingErr != nil {
		return source.Track{}, s.pingErr
	}
	return source.Track{Library: s.lib.ID, ID: id, Title: "live"}, nil
}
func (s *catalogServer) StreamURL(context.Context, string) (string, error) {
	return "http://srv/x", nil
}
func (s *catalogServer) Close() error { return nil }
func (s *catalogServer) Ping(context.Context) (source.ServerInfo, error) {
	if s.ping != nil {
		<-s.ping
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return source.ServerInfo{}, s.pingErr
}
func (s *catalogServer) Artists(context.Context) ([]source.Artist, error) {
	s.live.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.walks++
	return []source.Artist{{Library: s.lib.ID, ID: "ar", Name: "Artist"}}, nil
}
func (s *catalogServer) Albums(context.Context, string) ([]source.Album, error) {
	s.live.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	return []source.Album{{Library: s.lib.ID, ID: "al", Name: "Album", Artist: "Artist", ArtistID: "ar", TrackCount: s.albumTracks,
		Artists: []source.ArtistRef{{ID: "ar", Name: "Artist"}}}}, nil
}
func (s *catalogServer) Album(context.Context, string) (source.Album, error) {
	s.live.Add(1)
	return source.Album{Library: s.lib.ID, ID: "al", Name: "live"}, nil
}
func (s *catalogServer) Search(context.Context, string, int) (source.SearchResult, error) {
	s.live.Add(1)
	return source.SearchResult{}, nil
}

// holdPage keeps the next page from answering until its walk is cancelled, or
// for long enough to fail the test when nothing cancels it. The channel closes
// once the walk is there.
func (s *catalogServer) holdPage() <-chan struct{} {
	entered := make(chan struct{})
	s.set(func() {
		s.onPage = func() {
			s.onPage = nil
			close(entered)
			ctx := s.pageCtx
			s.mu.Unlock()
			select {
			case <-ctx.Done():
			case <-time.After(10 * time.Second):
			}
			s.mu.Lock()
		}
	})
	return entered
}

func (s *catalogServer) TrackPage(ctx context.Context, q source.TrackQuery) (source.TrackPage, error) {
	s.live.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pageCtx = ctx
	if s.onPage != nil {
		s.onPage()
	}
	if s.pageErr != nil && q.Offset >= s.failAt {
		return source.TrackPage{}, s.pageErr
	}
	total := s.total
	if total == 0 {
		total = len(s.tracks)
	}
	limit := min(q.Limit, 2)
	return source.TrackPage{Total: total, Tracks: source.Window(s.tracks, q.Offset, limit), Estimated: s.estimated}, nil
}
func (s *catalogServer) CatalogChanged(_ context.Context, stamp string) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stamps = append(s.stamps, stamp)
	return s.changed || stamp == "", s.stamp, s.pingErr
}
func (s *catalogServer) CatalogBusy(context.Context) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy
}

func (s *catalogServer) Favorites(context.Context) (source.SearchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.favCalls++
	return s.favorite, s.favErr
}
func (s *catalogServer) SignInRefused() bool                                     { return s.reported.Load() }
func (s *catalogServer) SetFavorite(context.Context, string, string, bool) error { return nil }
func (s *catalogServer) RecentPlays(context.Context) ([]source.Track, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recent, s.recentErr
}

func (s *catalogServer) set(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f()
}

// catalogManager runs one library on server, built at once.
func catalogManager(t *testing.T, server *catalogServer) (*Manager, *Store) {
	t.Helper()
	ctx := context.Background()
	store, err := Open(ctx, dbPath(t))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	lib := source.Library{ID: "srv", Kind: "fake", Name: "Server", Enabled: true, Config: map[string]any{"url": "http://a", "username": "dani"}}
	require.NoError(t, store.AddLibrary(ctx, lib))
	server.lib = lib
	m := NewManager(store, source.Registry{"fake": func(l source.Library, _ source.Env) (source.Source, error) {
		server.lib = l
		return server, nil
	}}, source.Env{Secrets: noSecrets{}})
	require.NoError(t, m.Load(ctx))
	m.awaitAll(ctx)
	return m, store
}

type noSecrets struct{}

func (noSecrets) Get(string, string) (string, error) { return "", source.ErrNoSecret }
func (noSecrets) Set(string, string, string) error   { return nil }
func (noSecrets) Delete(string) error                { return nil }

func stored(t *testing.T, s *Store) int {
	t.Helper()
	n, err := s.TrackCount(context.Background(), "srv")
	require.NoError(t, err)
	return n
}

// A complete walk stores the catalog, with the slower fields, and makes the
// library synced; a later one removes what the server dropped.
func TestWalkFillsAndSweeps(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(5)
	m, store := catalogManager(t, server)
	var done []int
	stats, err := m.Scan(ctx, "srv", func(d, _ int) { done = append(done, d) })
	require.NoError(t, err)
	assert.Equal(t, 5, stats.Seen)
	assert.Equal(t, []int{2, 4, 5}, done)
	assert.True(t, store.Synced(ctx, "srv"))
	got, err := store.Track(ctx, "srv", "t3")
	require.NoError(t, err)
	assert.Equal(t, "Bach", got.Composer)
	assert.Equal(t, 320, got.Bitrate)

	server.set(func() { server.tracks = server.tracks[:4]; server.stamp = "s2" })
	stats, err = m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Removed)
	assert.Equal(t, 4, stored(t, store))
	stamp, _, err := store.CatalogState(ctx, "srv")
	require.NoError(t, err)
	assert.Equal(t, "s2", stamp)
}

// Each way of not knowing the whole catalog keeps every row; the last
// complete sync stays what is read.
func TestWalkDoesNotSweepWithoutProof(t *testing.T) {
	pageFailed := errors.New("HTTP 500")
	for _, tt := range []struct {
		name    string
		breakIt func(s *catalogServer)
		wantErr bool
		synced  bool
	}{
		{"a page failed", func(s *catalogServer) { s.pageErr, s.failAt = pageFailed, 2 }, true, false},
		{"fewer tracks than the server's total", func(s *catalogServer) { s.tracks = s.tracks[:5]; s.total = 6 }, true, false},
		{"the server is scanning", func(s *catalogServer) { s.tracks = s.tracks[:5]; s.busy = true }, true, false},
		{"under half of what is stored", func(s *catalogServer) { s.tracks = s.tracks[:2] }, false, true},
		{"nothing listed", func(s *catalogServer) { s.tracks = nil }, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server := newCatalogServer(6)
			m, store := catalogManager(t, server)
			_, err := m.Scan(ctx, "srv", nil)
			require.NoError(t, err)
			_, before, err := store.CatalogState(ctx, "srv")
			require.NoError(t, err)
			server.set(func() { server.stamp = "s2"; tt.breakIt(server) })

			_, err = m.Sync(ctx, "srv", nil)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, 6, stored(t, store))
			assert.True(t, store.Synced(ctx, "srv"))
			stamp, after, err := store.CatalogState(ctx, "srv")
			require.NoError(t, err)
			if tt.synced {
				assert.Equal(t, "s2", stamp)
			} else {
				assert.Equal(t, "s1", stamp, "the next check walks again")
				assert.Equal(t, before, after)
			}
		})
	}
}

// Rescan is the way out of the guard against an unmounted disk, and of no
// other: a list that can't be trusted to be whole removes nothing by hand
// either.
func TestRescanByHand(t *testing.T) {
	for _, tt := range []struct {
		name    string
		breakIt func(s *catalogServer)
		left    int
	}{
		{"takes a list under half of what is stored", func(s *catalogServer) { s.tracks = s.tracks[:2] }, 2},
		{"not one short of the server's total", func(s *catalogServer) { s.tracks = s.tracks[:2]; s.total = 3 }, 6},
		{"not while the server scans", func(s *catalogServer) { s.tracks = s.tracks[:2]; s.busy = true }, 6},
		{"not an empty list", func(s *catalogServer) { s.tracks = nil }, 6},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server := newCatalogServer(6)
			m, store := catalogManager(t, server)
			_, err := m.Scan(ctx, "srv", nil)
			require.NoError(t, err)
			server.set(func() { tt.breakIt(server) })
			_, _ = m.Scan(ctx, "srv", nil)
			assert.Equal(t, tt.left, stored(t, store))
		})
	}
}

// A Subsonic total is a guess, so the albums' track counts decide when a walk
// saw everything, whichever way the guess is off. An exact total stays.
func TestAlbumsDecideAnEstimatedTotal(t *testing.T) {
	for _, tt := range []struct {
		name        string
		estimated   bool
		total       int
		albumTracks int
		synced      bool
	}{
		{"a guess that is too high", true, 100, 5, true},
		{"a guess that is too low hides a short walk", true, 3, 6, false},
		{"no album counts leave the guess", true, 6, 0, false},
		{"an exact total", false, 5, 9, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newCatalogServer(5)
			server.estimated, server.total, server.albumTracks = tt.estimated, tt.total, tt.albumTracks
			m, store := catalogManager(t, server)
			_, err := m.Sync(context.Background(), "srv", nil)
			assert.Equal(t, tt.synced, err == nil)
			assert.Equal(t, tt.synced, store.Synced(context.Background(), "srv"))
		})
	}
}

// Pages that shift during a walk can repeat a track and so hide one that
// never came: a track counts once.
func TestWalkCountsARepeatedTrackOnce(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(4)
	m, store := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	server.set(func() {
		server.tracks = []source.Track{server.tracks[0], server.tracks[1], server.tracks[1], server.tracks[2]}
		server.stamp = "s2"
	})
	_, err = m.Scan(ctx, "srv", nil)
	require.Error(t, err)
	assert.Equal(t, 4, stored(t, store), "t3 didn't come, but it wasn't proven gone")
}

// A first sync that fails stays unread, and starts over.
func TestInterruptedFirstSyncStaysUnread(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(6)
	server.pageErr, server.failAt = errors.New("timeout"), 2
	m, store := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.Error(t, err)
	assert.False(t, store.Synced(ctx, "srv"))

	server.set(func() { server.pageErr = nil })
	_, err = m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	assert.True(t, store.Synced(ctx, "srv"))
	assert.Equal(t, 6, stored(t, store))
}

// A mark set here while the walk reads older pages keeps its value, and so
// does one queued for the server.
func TestWalkKeepsNewerMarks(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(4)
	server.tracks[1].Favorite = true
	server.tracks[2].Favorite = true
	m, store := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)

	off := false
	queued := func(_, _, id string) bool { return id == "t2" }
	m.pending.Store(&queued)
	server.set(func() {
		server.stamp = "s2"
		server.onPage = func() {
			require.NoError(t, store.SetMark(ctx, "srv", source.ItemTrack, "t1", &off, nil))
			server.onPage = nil
		}
		server.tracks[2].Favorite = false
	})
	_, err = m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	t1, err := store.Track(ctx, "srv", "t1")
	require.NoError(t, err)
	assert.False(t, t1.Favorite, "this device's change is newer than the page")
	t2, err := store.Track(ctx, "srv", "t2")
	require.NoError(t, err)
	assert.True(t, t2.Favorite, "a queued change outranks the server")
}

// An edit to another account stops the walk of the old one and forgets its
// rows; a new password for the same account keeps them.
func TestAccountChangeClearsTheCatalog(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(4)
	m, store := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)

	_, err = m.Update(ctx, "srv", "", nil, map[string]string{"password": "new"})
	require.NoError(t, err)
	assert.True(t, store.Synced(ctx, "srv"))

	entered := server.holdPage()
	walked := make(chan error, 1)
	go func() { _, err := m.Scan(ctx, "srv", nil); walked <- err }()
	<-entered
	updated := make(chan error, 1)
	go func() {
		_, err := m.Update(ctx, "srv", "", map[string]any{"username": "marlon"}, nil)
		updated <- err
	}()
	require.NoError(t, <-updated)
	require.Error(t, <-walked)
	assert.False(t, store.Synced(ctx, "srv"))
	assert.Zero(t, stored(t, store))
}

// What a library was read from decides whether its rows and queued marks
// still belong to it.
func TestChangedAccountForgetsRowsAndQueuedMarks(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config map[string]any
		same   bool
		// downloads belong to the server's ids, not to the account
		cleared bool
	}{
		{"a trailing slash", map[string]any{"url": "http://a/"}, true, false},
		{"https on the same host", map[string]any{"url": "https://a"}, true, false},
		{"the username in other case", map[string]any{"username": "Dani"}, true, false},
		{"another path on the host", map[string]any{"url": "http://a/b"}, false, true},
		{"another host", map[string]any{"url": "http://b"}, false, true},
		{"another user", map[string]any{"username": "marlon"}, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server := newCatalogServer(2)
			m, store := catalogManager(t, server)
			_, err := m.Scan(ctx, "srv", nil)
			require.NoError(t, err)
			off := &clearRecorder{}
			m.SetOffline(off)
			var dropped []string
			m.StartCatalogs(t.Context(), CatalogHooks{DropPending: func(lib string) { dropped = append(dropped, lib) }})

			_, err = m.Update(ctx, "srv", "", tt.config, map[string]string{"password": "pw"})
			require.NoError(t, err)
			assert.Equal(t, tt.same, store.Synced(ctx, "srv"))
			if tt.cleared {
				assert.Equal(t, []string{"srv"}, off.cleared)
			} else {
				assert.Empty(t, off.cleared)
			}
			if tt.same {
				assert.Empty(t, dropped)
			} else {
				assert.Equal(t, []string{"srv"}, dropped)
			}
		})
	}
}

// Removing a library stops its walk before the rows go.
func TestRemoveStopsTheWalk(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(4)
	m, _ := catalogManager(t, server)
	entered := server.holdPage()
	walked := make(chan error, 1)
	go func() { _, err := m.Scan(ctx, "srv", nil); walked <- err }()
	<-entered
	removed := make(chan error, 1)
	go func() { removed <- m.Remove(ctx, "srv") }()
	require.NoError(t, <-removed)
	require.ErrorIs(t, <-walked, context.Canceled)
}

func TestServerWithCatalogListsScan(t *testing.T) {
	server := newCatalogServer(1)
	assert.Contains(t, source.Capabilities(server), source.CapScan)
	m, _ := catalogManager(t, server)
	assert.NoError(t, m.CanScan("srv"))
	assert.Error(t, m.CanScan("nope"))
}

// Browsing is live until a sync is complete, then the index answers without
// asking the server, with the same ids.
func TestBrowsingSwitchesWhenTheSyncIsComplete(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(4)
	server.pageErr, server.failAt = errors.New("timeout"), 2
	m, _ := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.Error(t, err)

	b, err := m.Browser(ctx, "srv")
	require.NoError(t, err)
	assert.Same(t, server, b, "a partial sync stays unread")

	server.set(func() { server.pageErr = nil })
	_, err = m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	server.live.Store(0)
	b, err = m.Browser(ctx, "srv")
	require.NoError(t, err)
	album, err := b.Album(ctx, "al")
	require.NoError(t, err)
	assert.Equal(t, "Album", album.Name)
	require.Len(t, album.Tracks, 4)
	page, err := b.(source.TrackPager).TrackPage(ctx, source.TrackQuery{Limit: 2, Sort: source.SortTitle})
	require.NoError(t, err)
	assert.Equal(t, 4, page.Total)
	assert.Equal(t, source.SortTitle, page.Sort)
	assert.Equal(t, "t0", page.Tracks[0].ID)
	assert.Equal(t, "srv", page.Tracks[0].Library)
	found, err := b.Search(ctx, "track 3", 10)
	require.NoError(t, err)
	require.Len(t, found.Tracks, 1)
	assert.Zero(t, server.live.Load(), "the server wasn't asked")
}

// A library off isn't browsed, whatever is stored for it.
func TestTurnedOffLibraryIsNotBrowsed(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(2)
	m, _ := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	require.NoError(t, m.SetEnabled(ctx, "srv", false))
	_, err = m.Browser(ctx, "srv")
	assert.Error(t, err)
}

// A scanned local library has a last_scan too; it keeps its own source.
func TestLocalLibraryIsNotReadAsASync(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, dbPath(t))
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.AddLibrary(ctx, source.Library{ID: "loc", Kind: "local", Name: "L", Enabled: true}))
	require.NoError(t, store.SetScanned(ctx, "loc"))
	local := newCatalogServer(0)
	m := NewManager(store, source.Registry{"local": func(l source.Library, _ source.Env) (source.Source, error) { return local, nil }}, source.Env{})
	require.NoError(t, m.Load(ctx))
	b, err := m.Browser(ctx, "loc")
	require.NoError(t, err)
	assert.Same(t, local, b)
}

// Favorites of a synced library come from the index, also while its server
// doesn't answer.
func TestFavoritesFromTheIndex(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(2)
	server.tracks[1].Favorite = true
	m, _ := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	server.set(func() { server.pingErr = errors.New("down") })
	res, err := m.Favorites(ctx)
	require.NoError(t, err)
	require.Len(t, res.Tracks, 1)
	assert.Equal(t, "t1", res.Tracks[0].ID)
}

// watched runs a check with hooks that record what happened.
type watched struct {
	t      *testing.T
	m      *Manager
	server *catalogServer
	mu     sync.Mutex
	syncs  []string
	calls  []string
	marks  []map[string]any
}

func watchChecks(t *testing.T, m *Manager, server *catalogServer, hooks CatalogHooks) *watched {
	w := &watched{t: t, m: m, server: server}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	hooks.Sync = func(id string) {
		w.mu.Lock()
		w.syncs = append(w.syncs, id)
		w.mu.Unlock()
		_, _ = m.Sync(ctx, id, nil)
	}
	if hooks.Flush == nil {
		hooks.Flush = func(context.Context) {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.calls = append(w.calls, "flush")
		}
	}
	m.StartCatalogs(ctx, hooks)
	m.SetPublish(func(topic string, data any) {
		if topic == TopicMarks {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.marks = append(w.marks, data.(map[string]any))
		}
	})
	return w
}

func (w *watched) check() { w.m.catalogCheck("srv", w.server, true) }

// gapOver ends the wait after the last walk.
func (w *watched) gapOver() {
	w.m.mu.Lock()
	defer w.m.mu.Unlock()
	delete(w.m.nextWalk, "srv")
}

func (w *watched) walks() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.syncs)
}

// The gate decides: a change walks, no change doesn't, and a library that
// hasn't walked for a day walks anyway. A metered connection walks only by
// hand; a walk that failed doesn't start again at the next check.
func TestWalkStartsWhenDue(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(2)
	m, store := catalogManager(t, server)
	w := watchChecks(t, m, server, CatalogHooks{})

	w.check()
	require.Equal(t, 1, w.walks(), "a first sync")
	require.True(t, store.Synced(ctx, "srv"))

	w.check()
	require.Equal(t, 1, w.walks(), "nothing changed")

	server.set(func() { server.changed = true })
	w.check()
	require.Equal(t, 1, w.walks(), "walkGap after a complete walk")
	w.gapOver()
	w.check()
	require.Equal(t, 2, w.walks())

	server.set(func() { server.changed = false })
	_, err := store.db.ExecContext(ctx, "UPDATE libraries SET last_scan = unixepoch() - 90000 WHERE id = 'srv'")
	require.NoError(t, err)
	w.gapOver()
	w.check()
	require.Equal(t, 3, w.walks(), "at most a day apart")

	m.metered.Store(true)
	server.set(func() { server.changed = true })
	w.gapOver()
	w.check()
	require.Equal(t, 3, w.walks(), "metered")
	_, err = m.Scan(ctx, "srv", nil)
	require.NoError(t, err, "by hand it does")

	m.metered.Store(false)
	server.set(func() { server.pageErr = errors.New("HTTP 500") })
	w.gapOver()
	w.check()
	require.Equal(t, 4, w.walks())
	w.check()
	require.Equal(t, 4, w.walks(), "walkGap after a failure")
}

// A refusal found by any call stops the check and becomes a refused library,
// which the tick then leaves alone: every further try would be one more
// failed sign-in.
func TestRefusalEndsTheChecks(t *testing.T) {
	refused := fmt.Errorf("HTTP 401: %w", source.ErrSignInRefused)
	check := func(w *watched) { w.check() }
	for _, tt := range []struct {
		name   string
		refuse func(s *catalogServer)
		run    func(w *watched)
		gate   int // what still asks the gate after the refusal
	}{
		{"favorites", func(s *catalogServer) { s.favErr = refused }, check, 0},
		{"recent plays", func(s *catalogServer) { s.recentErr = refused }, check, 0},
		{"the gate", func(*catalogServer) {}, check, 1},
		{"a walk", func(s *catalogServer) {
			s.pingErr = nil
			s.onPage = func() { s.onPage, s.pageErr, s.pingErr = nil, refused, refused }
		}, func(w *watched) { _, _ = w.m.Sync(context.Background(), "srv", nil) }, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server := newCatalogServer(2)
			m, _ := catalogManager(t, server)
			_, err := m.Scan(ctx, "srv", nil)
			require.NoError(t, err)
			w := watchChecks(t, m, server, CatalogHooks{})
			server.set(func() { server.pingErr = refused; server.stamps = nil; tt.refuse(server) })

			tt.run(w)
			server.set(func() { assert.Len(t, server.stamps, tt.gate) })
			infos, err := m.List(ctx)
			require.NoError(t, err)
			require.Equal(t, CodeSignInExpired, infos[0].ErrorCode)

			server.set(func() { server.favCalls = 0 })
			m.checkCatalogs(ctx)
			time.Sleep(20 * time.Millisecond)
			server.set(func() { assert.Zero(t, server.favCalls) })
		})
	}
}

// A refusal a source met by itself shows on a synced library too, which
// browses without Get.
func TestBrowsingNoticesARefusal(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(2)
	m, _ := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	server.set(func() { server.pingErr = source.ErrSignInRefused })
	server.reported.Store(true)
	m.mu.Lock()
	m.tried["srv"] = time.Now().Add(-time.Hour)
	m.mu.Unlock()

	_, err = m.Browser(ctx, "srv")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		infos, err := m.List(ctx)
		return err == nil && infos[0].ErrorCode == CodeSignInExpired
	}, 2*time.Second, 5*time.Millisecond)
}

// The tick pings a server that is down, since nothing else would with no
// window open; one that refused the sign-in isn't asked.
func TestTickRetriesDownNotRefused(t *testing.T) {
	for _, tt := range []struct {
		name  string
		err   error
		pings bool
	}{
		{"down", errors.New("no route"), true},
		{"refused", source.ErrSignInRefused, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server := newCatalogServer(2)
			m, _ := catalogManager(t, server)
			server.set(func() { server.pingErr = tt.err })
			_, err := m.Retry(ctx, "srv")
			require.NoError(t, err)
			server.set(func() { server.pingErr = nil })

			m.checkCatalogs(ctx)
			if tt.pings {
				require.Eventually(t, func() bool {
					infos, err := m.List(ctx)
					return err == nil && infos[0].Available
				}, 2*time.Second, 5*time.Millisecond)
				return
			}
			time.Sleep(20 * time.Millisecond)
			infos, err := m.List(ctx)
			require.NoError(t, err)
			assert.False(t, infos[0].Available)
		})
	}
}

// A ping starts the check, at most once per checkEvery unless the server
// just came back; a server that is down starts nothing.
func TestPingStartsTheCheck(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(2)
	server.pingErr = errors.New("connection refused")
	m, store := catalogManager(t, server)
	w := watchChecks(t, m, server, CatalogHooks{})

	m.Recheck(ctx)
	time.Sleep(20 * time.Millisecond)
	assert.Zero(t, w.walks(), "a server that is down starts nothing")

	server.set(func() { server.pingErr = nil })
	m.Recheck(ctx)
	require.Eventually(t, func() bool { return store.Synced(ctx, "srv") }, time.Second, time.Millisecond)

	server.set(func() { server.changed = true })
	m.Recheck(ctx)
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 1, w.walks(), "pings are more frequent than checks")

	server.set(func() { server.pingErr = errors.New("connection refused") })
	m.Recheck(ctx)
	server.set(func() { server.pingErr = nil })
	w.gapOver()
	m.Recheck(ctx)
	require.Eventually(t, func() bool { return w.walks() == 2 }, time.Second, time.Millisecond, "a server that came back")
}

// Between walks a check takes other devices' hearts and plays. Differences
// publish a marks event; a change queued here wins, and is flushed first.
func TestCheckTakesOtherDevicesChanges(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(4)
	server.tracks[0].Favorite = true
	server.tracks[1].Favorite = true
	m, store := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)

	queue := map[string]bool{}
	w := watchChecks(t, m, server, CatalogHooks{
		Pending:     func(_, _, id string) bool { return id == "t1" || id == "t3" },
		TrackMarked: func(_ context.Context, ref source.Ref, favorite bool) { queue[ref.ID] = favorite },
	})
	server.set(func() {
		server.favorite = source.SearchResult{Tracks: []source.Track{{ID: "t2"}, {ID: "t3"}}, Albums: []source.Album{{ID: "al"}}}
		server.recent = []source.Track{{ID: "t2", PlayCount: 9, LastPlayed: 90}}
	})
	w.check()

	fav := func(id string) bool {
		got, err := store.Track(ctx, "srv", id)
		require.NoError(t, err)
		return got.Favorite
	}
	assert.False(t, fav("t0"), "taken away elsewhere")
	assert.True(t, fav("t1"), "queued here")
	assert.True(t, fav("t2"), "set elsewhere")
	assert.False(t, fav("t3"), "queued here")
	album, err := store.Album(ctx, "srv", "al")
	require.NoError(t, err)
	assert.True(t, album.Favorite)
	got, err := store.Track(ctx, "srv", "t2")
	require.NoError(t, err)
	assert.Equal(t, 9, got.PlayCount)
	assert.Equal(t, int64(90), got.LastPlayed)

	w.mu.Lock()
	defer w.mu.Unlock()
	assert.Equal(t, []string{"flush"}, w.calls)
	assert.ElementsMatch(t, []map[string]any{
		{"library": "srv", "kind": "track", "id": "t0", "favorite": false},
		{"library": "srv", "kind": "track", "id": "t2", "favorite": true},
		{"library": "srv", "kind": "album", "id": "al", "favorite": true},
	}, w.marks)
	assert.Equal(t, map[string]bool{"t0": false, "t2": true}, queue, "tracks only")
}

// A server whose disk is unmounted answers with a short favorites list, and
// believing it would delete offline copies. Half of what is stored still
// counts as an answer.
func TestShortFavoritesListChangesNothing(t *testing.T) {
	for _, tt := range []struct {
		name   string
		listed []source.Track
		hearts int
		events int
	}{
		{"empty", nil, 4, 0},
		{"under half", []source.Track{{ID: "t0"}}, 4, 0},
		{"half", []source.Track{{ID: "t0"}, {ID: "t1"}}, 2, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server := newCatalogServer(4)
			for i := range server.tracks {
				server.tracks[i].Favorite = true
			}
			m, store := catalogManager(t, server)
			_, err := m.Scan(ctx, "srv", nil)
			require.NoError(t, err)
			w := watchChecks(t, m, server, CatalogHooks{})
			server.set(func() { server.favorite = source.SearchResult{Tracks: tt.listed} })
			w.check()

			hearts := 0
			for i := range 4 {
				got, err := store.Track(ctx, "srv", fmt.Sprintf("t%d", i))
				require.NoError(t, err)
				if got.Favorite {
					hearts++
				}
			}
			assert.Equal(t, tt.hearts, hearts)
			assert.Len(t, w.marks, tt.events)
		})
	}
}

// A check that finds nothing new publishes nothing, and a failed read
// changes nothing.
func TestCheckIsQuietWithoutDifferences(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(2)
	server.tracks[0].Favorite = true
	m, store := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	w := watchChecks(t, m, server, CatalogHooks{})
	server.set(func() { server.favorite = source.SearchResult{Tracks: []source.Track{{ID: "t0"}}} })
	w.check()
	server.set(func() { server.favorite, server.favErr = source.SearchResult{}, errors.New("HTTP 500") })
	w.check()
	assert.Empty(t, w.marks)
	got, err := store.Track(ctx, "srv", "t0")
	require.NoError(t, err)
	assert.True(t, got.Favorite)
}

// This device's changes show at once, without a sync.
func TestWriteThrough(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(2)
	m, store := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)

	on, stars := true, 4
	m.MarkChanged(ctx, "srv", source.ItemTrack, "t0", &on, &stars)
	m.PlayCounted(ctx, "srv", "t0", time.Unix(500, 0))
	got, err := store.Track(ctx, "srv", "t0")
	require.NoError(t, err)
	assert.True(t, got.Favorite)
	assert.Equal(t, 4, got.Rating)
	assert.Equal(t, 1, got.PlayCount)
	assert.Equal(t, int64(500), got.LastPlayed)
	album, err := store.Album(ctx, "srv", "al")
	require.NoError(t, err)
	assert.Equal(t, 1, album.PlayCount)
}

// A library added while the daemon runs syncs without anyone asking.
func TestAddedServerStartsItsFirstSync(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(2)
	store, err := Open(ctx, dbPath(t))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	m := NewManager(store, source.Registry{"fake": func(l source.Library, _ source.Env) (source.Source, error) {
		server.lib = l
		return server, nil
	}}, source.Env{Secrets: noSecrets{}})
	sctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	m.StartCatalogs(sctx, CatalogHooks{Sync: func(id string) { _, _ = m.Scan(sctx, id, nil) }})
	lib, err := m.Add(ctx, "fake", "Server", map[string]any{}, nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return store.Synced(ctx, lib.ID) }, time.Second, time.Millisecond)
}

// clearRecorder notes which libraries had their downloads cleared.
type clearRecorder struct {
	Offline
	cleared []string
}

func (o *clearRecorder) Clear(_ context.Context, lib string) error {
	o.cleared = append(o.cleared, lib)
	return nil
}

// fakeOffline holds a cached copy of one track.
type fakeOffline struct {
	Offline
	cached string
}

func (o fakeOffline) Cached(_ context.Context, ref source.Ref) (player.Stream, bool) {
	return player.Stream{URL: "/cache/x"}, ref.ID == o.cached
}
func (o fakeOffline) Buffered(context.Context, source.Ref) (player.Stream, bool) {
	return player.Stream{}, false
}
func (o fakeOffline) Snapshot(_ context.Context, ref source.Ref) (source.Track, bool) {
	return source.Track{Library: ref.Library, ID: ref.ID, Title: "snapshot"}, ref.ID == o.cached
}

// With the server down a synced library still lists, a restored queue keeps
// its titles, and only tracks with a cached copy play.
func TestOfflineBrowsing(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(3)
	m, _ := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	m.SetOffline(fakeOffline{cached: "t1"})
	server.set(func() { server.pingErr = errors.New("connection refused") })
	m.Recheck(ctx)

	infos, err := m.List(ctx)
	require.NoError(t, err)
	require.Len(t, infos, 1)
	assert.False(t, infos[0].Available)
	assert.True(t, infos[0].Synced)
	assert.Equal(t, CodeServerUnreachable, infos[0].ErrorCode)

	b, err := m.Browser(ctx, "srv")
	require.NoError(t, err)
	artists, err := b.Artists(ctx)
	require.NoError(t, err)
	assert.Len(t, artists, 1)

	tracks, err := m.Tracks(ctx, []source.Ref{{Library: "srv", ID: "t0"}, {Library: "srv", ID: "t1"}})
	require.NoError(t, err)
	assert.Equal(t, "Track 0", tracks[0].Title, "the index, when no copy is cached")
	assert.Equal(t, "snapshot", tracks[1].Title, "a cached copy's own snapshot first")

	assert.True(t, m.Skip(source.Ref{Library: "srv", ID: "t0"}), "nothing to play it from")
	assert.Equal(t, CodeServerUnreachable, m.Unavailable(ctx, source.Ref{Library: "srv", ID: "t0"}).Code)
	assert.False(t, m.Skip(source.Ref{Library: "srv", ID: "t1"}), "it has a cached copy")
}

// A track a live library can't answer for stays an error without a sync: an
// unread partial walk isn't a catalog.
func TestPartialSyncDoesNotAnswerForTracks(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(4)
	server.pageErr, server.failAt = errors.New("timeout"), 2
	m, _ := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.Error(t, err)
	server.set(func() { server.pingErr = errors.New("connection refused") })
	_, err = m.Tracks(ctx, []source.Ref{{Library: "srv", ID: "t0"}})
	assert.Error(t, err)
}

// A keyring that stays locked leaves no source, but a synced library lists
// what is stored, and still counts as a server, so this device's plays don't
// replace the server's counts.
func TestSyncedLibraryWithoutASource(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(2)
	store, err := Open(ctx, dbPath(t))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	lib := source.Library{ID: "srv", Kind: "fake", Name: "Server", Enabled: true, Config: map[string]any{"url": "http://a"}}
	require.NoError(t, store.AddLibrary(ctx, lib))
	server.lib = lib
	var locked atomic.Bool
	registry := source.Registry{"fake": func(source.Library, source.Env) (source.Source, error) {
		if locked.Load() {
			return nil, keyring.ErrLocked
		}
		return server, nil
	}}
	first := NewManager(store, registry, source.Env{})
	require.NoError(t, first.Load(ctx))
	first.awaitAll(ctx)
	_, err = first.Scan(ctx, "srv", nil)
	require.NoError(t, err)

	locked.Store(true)
	m := NewManager(store, registry, source.Env{})
	require.NoError(t, m.Load(ctx))
	m.awaitAll(ctx)
	infos, err := m.List(ctx)
	require.NoError(t, err)
	assert.Equal(t, CodeKeyringLocked, infos[0].ErrorCode)
	assert.Contains(t, infos[0].Capabilities, source.CapBrowse)
	assert.Contains(t, infos[0].Capabilities, source.CapCover)
	assert.True(t, m.Server("srv"))
	tracks, err := m.Tracks(ctx, []source.Ref{{Library: "srv", ID: "t0"}})
	require.NoError(t, err)
	assert.Equal(t, "Track 0", tracks[0].Title)
}

// Lookups of a library known to be down answer from the row: a server that
// swallows packets would cost a timeout for each. A track without a row still
// asks.
func TestDownServerIsNotAskedForStoredTracks(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(2)
	m, _ := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	server.set(func() { server.pingErr = errors.New("no route") })
	_, err = m.Retry(ctx, "srv")
	require.NoError(t, err)

	asked := server.live.Load()
	tracks, err := m.Tracks(ctx, []source.Ref{{Library: "srv", ID: "t0"}})
	require.NoError(t, err)
	assert.Equal(t, "Track 0", tracks[0].Title)
	assert.Equal(t, asked, server.live.Load())

	_, err = m.Tracks(ctx, []source.Ref{{Library: "srv", ID: "gone"}})
	require.Error(t, err)
	assert.Greater(t, server.live.Load(), asked)
}

// A server library whose source can't be built is still a server, before its
// first sync too: history must not take local counts for it.
func TestServerWithoutASource(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config map[string]any
		want   bool
	}{
		{"has an address", map[string]any{"url": "http://a"}, true},
		{"has folders", map[string]any{"folders": []string{"/m"}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store, err := Open(ctx, dbPath(t))
			require.NoError(t, err)
			t.Cleanup(func() { store.Close() })
			require.NoError(t, store.AddLibrary(ctx, source.Library{ID: "lib", Kind: "fake", Name: "L", Enabled: true, Config: tt.config}))
			m := NewManager(store, source.Registry{"fake": func(source.Library, source.Env) (source.Source, error) {
				return nil, keyring.ErrLocked
			}}, source.Env{})
			require.NoError(t, m.Load(ctx))
			m.awaitAll(ctx)
			assert.Equal(t, tt.want, m.Server("lib"))
		})
	}
}

// The real startup order: open (with the migration), Load in the background,
// then Recheck. A server library upgraded from the old schema stays live, and
// its first sync starts only once its ping answered.
func TestStartupAfterUpgrade(t *testing.T) {
	ctx := context.Background()
	path := dbPath(t)
	old, err := OpenDB(ctx, path, migrations[:7])
	require.NoError(t, err)
	_, err = old.ExecContext(ctx, `INSERT INTO libraries (id, kind, name) VALUES ('srv', 'fake', 'Server')`)
	require.NoError(t, err)
	require.NoError(t, old.Close())

	store, err := Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	server := newCatalogServer(3)
	server.ping = make(chan struct{})
	built := make(chan struct{})
	m := NewManager(store, source.Registry{"fake": func(l source.Library, _ source.Env) (source.Source, error) {
		<-built
		server.lib = l
		return server, nil
	}}, source.Env{})
	require.NoError(t, m.Load(ctx))
	sctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	m.StartCatalogs(sctx, CatalogHooks{Sync: func(id string) { _, _ = m.Scan(sctx, id, nil) }})
	rechecked := make(chan struct{})
	go func() { m.Recheck(sctx); close(rechecked) }()

	infos, err := m.List(ctx)
	require.NoError(t, err, "listing doesn't wait for the build")
	assert.False(t, infos[0].Synced)
	assert.Empty(t, infos[0].Capabilities)

	close(built)
	m.await(ctx, "srv")
	b, err := m.Browser(ctx, "srv")
	require.NoError(t, err)
	assert.Same(t, server, b, "live until a sync is complete")
	time.Sleep(20 * time.Millisecond)
	server.mu.Lock()
	walks := server.walks
	server.mu.Unlock()
	assert.Zero(t, walks, "no walk before the ping answered")

	close(server.ping)
	<-rechecked
	require.Eventually(t, func() bool { return store.Synced(ctx, "srv") }, time.Second, time.Millisecond)
	b, err = m.Browser(ctx, "srv")
	require.NoError(t, err)
	assert.IsType(t, storeBrowser{}, b)
}

// A restart with a synced library: browsing answers from the index while the
// keyring still holds up the build and the server hasn't answered.
func TestRestartBrowsesBeforeTheBuild(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(3)
	m, store := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)

	built := make(chan struct{})
	t.Cleanup(func() { close(built) })
	next := NewManager(store, source.Registry{"fake": func(l source.Library, _ source.Env) (source.Source, error) {
		<-built
		return server, nil
	}}, source.Env{})
	require.NoError(t, next.Load(ctx))
	wait, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	b, err := next.Browser(wait, "srv")
	require.NoError(t, err)
	albums, err := b.Albums(wait, "")
	require.NoError(t, err)
	assert.Len(t, albums, 1)
	res, err := next.Favorites(wait)
	require.NoError(t, err)
	assert.Empty(t, res.Tracks)
	assert.True(t, next.Server("srv"), "a play count must not be replaced while the build runs")
	require.NoError(t, wait.Err())
}

// A library past the bound isn't known in full, whatever a server's estimate
// of its total says.
func TestWalkStopsAtTheBound(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(6)
	m, store := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)

	maxCatalog = 4
	t.Cleanup(func() { maxCatalog = 200_000 })
	server.set(func() { server.total = 4; server.stamp = "s2" })
	_, err = m.Sync(ctx, "srv", nil)
	require.ErrorIs(t, err, errCatalogTooLarge)
	assert.Equal(t, 6, stored(t, store))
	m.mu.RLock()
	defer m.mu.RUnlock()
	assert.Greater(t, time.Until(m.nextWalk["srv"]), walkEvery-time.Minute, "an hour won't change this")
}

// Marks queued here reach the server before its list is read, so the list
// already holds them.
func TestCheckFlushesFirst(t *testing.T) {
	ctx := context.Background()
	server := newCatalogServer(2)
	m, store := catalogManager(t, server)
	_, err := m.Scan(ctx, "srv", nil)
	require.NoError(t, err)
	w := watchChecks(t, m, server, CatalogHooks{Flush: func(context.Context) {
		server.set(func() { server.favorite.Tracks = []source.Track{{ID: "t0"}} })
	}})
	w.check()
	got, err := store.Track(ctx, "srv", "t0")
	require.NoError(t, err)
	assert.True(t, got.Favorite)
}
