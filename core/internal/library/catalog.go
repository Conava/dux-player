package library

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/logtext"
	"github.com/Conava/dux-player/core/internal/source"
)

const (
	// walkPage is what a walk asks for; a source may answer with less.
	walkPage    = 500
	walkTimeout = time.Hour
	// walkGap is the least time between walks that start by themselves, so a
	// gate that always answers changed costs one walk an hour at most.
	walkGap = time.Hour
)

// errCatalogTooLarge is a walk that reached maxCatalog: trying again soon
// can't end differently.
var errCatalogTooLarge = errors.New("the catalog is larger than a sync can hold")

// pagePause keeps a walk gentle on the server, and maxCatalog bounds it, like
// the full read of play counts; tests change both.
var (
	pagePause  = 250 * time.Millisecond
	maxCatalog = 200_000
)

type walkRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// runWalk makes a walk stoppable by stopWalk.
func (m *Manager) runWalk(ctx context.Context, id string, src source.Source, cs source.CatalogSync, manual bool, progress func(done, total int)) (source.ScanStats, error) {
	ctx, cancel := context.WithTimeout(ctx, walkTimeout)
	defer cancel()
	run := walkRun{cancel, make(chan struct{})}
	m.mu.Lock()
	m.walks[id] = run
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.walks, id)
		m.mu.Unlock()
		close(run.done)
	}()
	stats, err := m.walk(ctx, id, src, cs, manual, progress)
	gap := walkGap
	if errors.Is(err, errCatalogTooLarge) {
		gap = walkEvery
	}
	m.mu.Lock()
	m.nextWalk[id] = time.Now().Add(gap)
	m.mu.Unlock()
	if err == nil {
		m.PublishChanges(ctx)
	}
	return stats, err
}

// stopWalk ends a library's walk, if one runs, and waits for it.
func (m *Manager) stopWalk(ctx context.Context, id string) {
	m.mu.RLock()
	run, ok := m.walks[id]
	m.mu.RUnlock()
	if !ok {
		return
	}
	run.cancel()
	select {
	case <-run.done:
	case <-ctx.Done():
	}
}

// walk reads a server's whole catalog into the index: there is no change
// feed, so this is the only write path. Nothing is removed unless the walk is
// known to have seen everything; an error means it doesn't count as a sync,
// and the stamp stays so a later check walks again. manual is a walk the user
// asked for, which may remove over half of what is stored.
func (m *Manager) walk(ctx context.Context, id string, src source.Source, cs source.CatalogSync, manual bool, progress func(done, total int)) (stats source.ScanStats, err error) {
	browser, ok := src.(source.Browser)
	pager, paged := src.(source.TrackPager)
	if !ok || !paged {
		return stats, fmt.Errorf("library %s can't list its catalog", id)
	}
	since := time.Now().Unix()
	old, _, err := m.store.CatalogState(ctx, id)
	if err != nil {
		return stats, err
	}
	// Taken first, so what changes during the walk shows at the next check.
	_, stamp, err := cs.CatalogChanged(ctx, old)
	if err != nil {
		return stats, err
	}
	stored, err := m.store.TrackCount(ctx, id)
	if err != nil {
		return stats, err
	}
	pending := m.pendingMarks(id)
	batch := m.store.BeginCatalog(id, time.Now().UnixNano(), since)
	defer batch.Abort()

	artists, err := browser.Artists(ctx)
	if err != nil {
		return stats, err
	}
	for _, a := range artists {
		if err := batch.PutServerArtist(ctx, a, pending(source.ItemArtist, a.ID)); err != nil {
			return stats, err
		}
	}
	if err := batch.Commit(); err != nil {
		return stats, err
	}
	albums, err := browser.Albums(ctx, "")
	if err != nil {
		return stats, err
	}
	albumTracks := 0
	for _, a := range albums {
		albumTracks += max(a.TrackCount, 0)
		if err := batch.PutServerAlbum(ctx, a, pending(source.ItemAlbum, a.ID)); err != nil {
			return stats, err
		}
	}
	if err := batch.Commit(); err != nil {
		return stats, err
	}

	// Pages can shift during the walk, so tracks count once however often
	// they came. Total is no end marker: a Subsonic server's is an estimate.
	seen := map[string]bool{}
	total, offset := 0, 0
	for offset < maxCatalog {
		page, err := pager.TrackPage(ctx, source.TrackQuery{Offset: offset, Limit: walkPage, Details: true})
		if err != nil {
			return stats, err
		}
		total = max(total, page.Total)
		if page.Estimated && albumTracks > 0 {
			total = albumTracks
		}
		if len(page.Tracks) == 0 {
			break
		}
		for _, t := range page.Tracks {
			seen[t.ID] = true
			if err := batch.PutServerTrack(ctx, t, pending(source.ItemTrack, t.ID)); err != nil {
				return stats, err
			}
		}
		if err := batch.Commit(); err != nil {
			return stats, err
		}
		offset += len(page.Tracks)
		if progress != nil {
			progress(len(seen), total)
		}
		select {
		case <-time.After(pagePause):
		case <-ctx.Done():
			return stats, ctx.Err()
		}
	}
	stats.Seen, stats.Updated = len(seen), len(seen)
	if offset >= maxCatalog {
		return stats, fmt.Errorf("library %s: %w", id, errCatalogTooLarge)
	}
	if len(seen) == 0 || len(seen) < total || cs.CatalogBusy(ctx) {
		return stats, fmt.Errorf("library %s: the walk saw %d of %d tracks or the server is scanning; nothing removed", id, len(seen), total)
	}
	// A server whose disk is unmounted lists little, and answers as if all
	// were well.
	if len(seen)*2 < stored && !manual {
		log.Warnf("library %s: the server lists %d tracks, %d are stored; nothing removed", id, len(seen), stored)
		if err := batch.Commit(); err != nil {
			return stats, err
		}
	} else if stats.Removed, err = batch.Finish(ctx); err != nil {
		return stats, err
	}
	return stats, m.store.SetCatalogSynced(ctx, id, stamp)
}

// pendingMarks says which items have a change queued for the server, which
// outranks what the server says now.
func (m *Manager) pendingMarks(id string) func(kind, itemID string) bool {
	if f := m.pending.Load(); f != nil {
		return func(kind, itemID string) bool { return (*f)(id, kind, itemID) }
	}
	return func(string, string) bool { return false }
}

// storeBrowser reads a synced server's catalog from the index. It is no
// wrapper around the source, which would hide the optional interfaces; ids
// are the server's, so queues and playlists work across the switch.
type storeBrowser struct {
	store *Store
	id    string
}

var (
	_ source.Browser    = storeBrowser{}
	_ source.TrackPager = storeBrowser{}
)

func (b storeBrowser) Artists(ctx context.Context) ([]source.Artist, error) {
	return b.store.Artists(ctx, b.id)
}

func (b storeBrowser) Albums(ctx context.Context, artistID string) ([]source.Album, error) {
	return b.store.Albums(ctx, b.id, artistID)
}

func (b storeBrowser) Album(ctx context.Context, id string) (source.Album, error) {
	return b.store.Album(ctx, b.id, id)
}

func (b storeBrowser) Search(ctx context.Context, query string, limit int) (source.SearchResult, error) {
	return b.store.Search(ctx, b.id, query, limit)
}

func (b storeBrowser) TrackPage(ctx context.Context, q source.TrackQuery) (source.TrackPage, error) {
	return b.store.TrackPage(ctx, b.id, q)
}

// Browser is what lists a library: the index once its server has a complete
// sync, which needs neither the build nor the server, else the source.
func (m *Manager) Browser(ctx context.Context, id string) (source.Browser, error) {
	if m.store.Synced(ctx, id) {
		m.mu.RLock()
		_, failed := m.errs[id]
		retry := failed || m.down[id] || m.unseenRefusalLocked(id, m.sources[id])
		m.mu.RUnlock()
		// Using a broken library is what retries it.
		if retry {
			go func() { _, _ = m.Get(id) }()
		}
		return storeBrowser{m.store, id}, nil
	}
	src, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	b, ok := src.(source.Browser)
	if !ok {
		return nil, fmt.Errorf("library %s does not support browsing", id)
	}
	return b, nil
}

const (
	// checkEvery is how stale favorites, recent plays and new music may get.
	checkEvery = 15 * time.Minute
	// walkEvery brings what no check sees: Subsonic ratings, older play
	// counts, a change the gate missed.
	walkEvery    = 24 * time.Hour
	checkTimeout = 2 * time.Minute
)

// TopicMarks carries favorites and ratings that changed.
const TopicMarks = "marks"

// CatalogHooks is what keeping catalogs current needs from outside.
type CatalogHooks struct {
	// Sync runs a walk (Scan) and publishes how it went.
	Sync func(id string)
	// Flush sends the marks queued for servers that are back.
	Flush func(ctx context.Context)
	// Pending says whether an item has a mark waiting for its server.
	Pending func(lib, kind, id string) bool
	// DropPending forgets the marks waiting for a library's old account.
	DropPending func(lib string)
	// TrackMarked shows a favorite set on another device in the queue.
	TrackMarked func(ctx context.Context, ref source.Ref, favorite bool)
	// WatchMetered says whether the connection is metered and reports
	// changes; nil is never metered.
	WatchMetered func(ctx context.Context, onChange func(bool)) bool
}

type catalogRun struct {
	ctx   context.Context
	hooks CatalogHooks
	// known closes when the connection's meteredness is known.
	known chan struct{}
}

// StartCatalogs keeps server catalogs current until ctx ends: a check after
// each library's ping, and every checkEvery. Nothing here waits on a server.
func (m *Manager) StartCatalogs(ctx context.Context, hooks CatalogHooks) {
	run := &catalogRun{ctx, hooks, make(chan struct{})}
	if hooks.Pending != nil {
		m.pending.Store(&hooks.Pending)
	}
	m.catalog.Store(run)
	go func() {
		defer close(run.known)
		if hooks.WatchMetered != nil {
			m.metered.Store(hooks.WatchMetered(ctx, m.metered.Store))
		}
	}()
	go func() {
		tick := time.NewTicker(checkEvery)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				m.checkCatalogs(ctx)
			}
		}
	}()
}

// checkCatalogs checks every library that is up and pings those that are
// down, whose answer starts their check. A refused one waits for the user.
func (m *Manager) checkCatalogs(ctx context.Context) {
	m.mu.RLock()
	up := map[string]source.Source{}
	for id, src := range m.sources {
		if m.reasonLocked(id).Code == "" {
			up[id] = src
		}
	}
	down := make([]string, 0, len(m.down))
	for id := range m.down {
		down = append(down, id)
	}
	m.mu.RUnlock()
	for id, src := range up {
		go m.catalogCheck(id, src, true)
	}
	for _, id := range down {
		go m.retry(ctx, id)
	}
}

// catalogCheck brings a reachable library up to date as far as that is cheap,
// and starts a walk when it has to. force skips the wait between checks: the
// tick itself, and a server that just came back.
func (m *Manager) catalogCheck(id string, src source.Source, force bool) {
	run := m.catalog.Load()
	cs, ok := src.(source.CatalogSync)
	if run == nil || !ok {
		return
	}
	m.mu.Lock()
	skip := m.checks[id] || m.scanning[id] || !force && time.Since(m.checked[id]) < checkEvery
	if !skip {
		m.checks[id] = true
		m.checked[id] = time.Now()
	}
	m.mu.Unlock()
	if skip {
		return
	}
	defer func() {
		m.mu.Lock()
		delete(m.checks, id)
		m.mu.Unlock()
	}()
	select {
	case <-run.known:
	case <-run.ctx.Done():
		return
	}
	ctx, cancel := context.WithTimeout(run.ctx, checkTimeout)
	defer cancel()
	if m.store.Synced(ctx, id) {
		// First, so a queued change is on the server before its marks are read.
		if run.hooks.Flush != nil {
			run.hooks.Flush(ctx)
		}
		if err := m.refreshMarks(ctx, id, src, run.hooks); errors.Is(err, source.ErrSignInRefused) {
			m.retry(run.ctx, id)
			return
		}
	}
	due, err := m.walkDue(ctx, id, cs)
	if errors.Is(err, source.ErrSignInRefused) {
		m.retry(run.ctx, id)
		return
	}
	if due && run.hooks.Sync != nil {
		run.hooks.Sync(id)
	}
}

// refreshMarks takes other devices' favorites and plays between walks. A call
// the server refused ends it, and is what it returns: the next one would be
// one more failed sign-in.
func (m *Manager) refreshMarks(ctx context.Context, id string, src source.Source, hooks CatalogHooks) error {
	since := time.Now().Unix()
	if f, ok := src.(source.Favorites); ok {
		listed, err := f.Favorites(ctx)
		if errors.Is(err, source.ErrSignInRefused) {
			return err
		}
		if err != nil {
			log.Infof("library %s: favorites: %s", id, logtext.Clean(err.Error()))
		} else {
			changes, err := m.store.ApplyFavorites(ctx, id, listed, since, m.pendingMarks(id))
			if err != nil {
				log.Warnf("library %s: store favorites: %v", id, err)
			}
			for _, c := range changes {
				m.emit(TopicMarks, map[string]any{"library": id, "kind": c.Kind, "id": c.ID, "favorite": c.Favorite})
				if c.Kind == source.ItemTrack && hooks.TrackMarked != nil {
					hooks.TrackMarked(ctx, source.Ref{Library: id, ID: c.ID}, c.Favorite)
				}
			}
		}
	}
	if r, ok := src.(source.RecentPlays); ok {
		tracks, err := r.RecentPlays(ctx)
		if errors.Is(err, source.ErrSignInRefused) {
			return err
		}
		if err != nil {
			log.Infof("library %s: recent plays: %s", id, logtext.Clean(err.Error()))
			return nil
		}
		if err := m.store.ApplyPlays(ctx, id, tracks); err != nil {
			log.Warnf("library %s: store play counts: %v", id, err)
		}
	}
	return nil
}

// walkDue asks the gate and says whether a walk should start by itself. The
// error is the gate's.
func (m *Manager) walkDue(ctx context.Context, id string, cs source.CatalogSync) (bool, error) {
	stamp, last, err := m.store.CatalogState(ctx, id)
	if err != nil {
		log.Warnf("library %s: catalog state: %v", id, err)
		return false, nil
	}
	changed, next, err := cs.CatalogChanged(ctx, stamp)
	if err != nil {
		log.Infof("library %s: catalog check: %s", id, logtext.Clean(err.Error()))
		return false, err
	}
	if !changed && last > 0 && time.Since(time.Unix(last, 0)) < walkEvery {
		if err := m.store.SetCatalogStamp(ctx, id, next); err != nil {
			log.Warnf("library %s: catalog stamp: %v", id, err)
		}
		return false, nil
	}
	m.mu.RLock()
	wait := m.nextWalk[id]
	m.mu.RUnlock()
	return !m.metered.Load() && time.Now().After(wait), nil
}

// startCatalogCheck pings a library that was just added, which leads to its
// first sync, or to none while its server is down.
func (m *Manager) startCatalogCheck(lib source.Library) {
	if run := m.catalog.Load(); run != nil {
		go m.check(run.ctx, lib)
	}
}

// MarkChanged shows a favorite or rating a server took, or will take, in the
// index at once. A local library keeps its own.
func (m *Manager) MarkChanged(ctx context.Context, lib, kind, id string, favorite *bool, rating *int) {
	if !m.Server(lib) {
		return
	}
	if err := m.store.SetMark(ctx, lib, kind, id, favorite, rating); err != nil {
		log.Warnf("library %s: store a mark: %s", lib, logtext.Clean(err.Error()))
	}
}

// PlayCounted shows a play a server took in the index at once.
func (m *Manager) PlayCounted(ctx context.Context, lib, id string, at time.Time) {
	if err := m.store.AddPlay(ctx, lib, id, at.Unix()); err != nil {
		log.Warnf("library %s: store a play: %v", lib, err)
	}
}
