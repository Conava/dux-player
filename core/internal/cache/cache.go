// Package cache keeps copies of server tracks on disk, so they play while the
// server is unreachable. Each server library has offline rules; a copy stays
// while a rule holds it. The size limit evicts only what the dynamic rules
// (played, mostPlayed) hold, least recently played first.
package cache

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

const (
	Topic = "cache"

	// configKey holds a library's offline rules in its library config.
	configKey = "cache"

	DefaultLimit = 10 << 30
	minLimit     = 100 << 20

	maxFileBytes    = 2 << 30
	downloadTimeout = 30 * time.Minute
	// A download that receives nothing for this long gives up.
	stallTimeout = 30 * time.Second
	// A probe reads about the first minute or two of a lossy file, or 30
	// seconds of FLAC; that is enough for its loudness.
	probeBytes   = 4 << 20
	probeTimeout = time.Minute
	// coverSize matches the size the UI asks browse.cover for, so the
	// prefetched cover is the file it loads offline.
	coverSize = 600
)

var migrations = []string{
	`CREATE TABLE entries (
		library_id TEXT NOT NULL,
		id         TEXT NOT NULL,
		file       TEXT NOT NULL,
		size       INTEGER NOT NULL,
		added      INTEGER NOT NULL,
		used       INTEGER NOT NULL,
		track      TEXT NOT NULL,
		gain       REAL,
		PRIMARY KEY (library_id, id)
	);
	CREATE INDEX entries_used ON entries (used);`,
	// Copies cached before rules count as played when they were last used.
	`ALTER TABLE entries ADD COLUMN played INTEGER NOT NULL DEFAULT 0;
	UPDATE entries SET played = used;
	CREATE TABLE covers (
		library_id TEXT NOT NULL,
		id         TEXT NOT NULL,
		rule       TEXT NOT NULL,
		static     INTEGER NOT NULL,
		PRIMARY KEY (library_id, rule, id)
	);
	CREATE INDEX covers_track ON covers (library_id, id);`,
	// Where a copy's music starts and ends; NULL is not measured yet.
	// probes holds the gain measured from the start of server tracks that
	// aren't cached and have no ReplayGain values.
	`ALTER TABLE entries ADD COLUMN silence_start REAL;
	ALTER TABLE entries ADD COLUMN silence_end REAL;
	CREATE TABLE probes (
		library_id TEXT NOT NULL,
		id         TEXT NOT NULL,
		gain       REAL NOT NULL,
		PRIMARY KEY (library_id, id)
	);`,
}

type Config struct {
	// Dir holds the copies and their index; ConfigDir the size limit.
	Dir       string
	ConfigDir string
	Libraries *library.Manager
	Meter     *loudness.Meter
	Publish   func(topic string, data any)
	// LocalPlaylist returns a local playlist's tracks; the server tracks of
	// one kept on this device are cached.
	LocalPlaylist func(ctx context.Context, id string) ([]source.Track, error)
	// MostPlayed lists the tracks of every library by all-time play count.
	MostPlayed func(ctx context.Context) ([]source.Track, error)
	// Prefetched is told that the next track's copy is ready.
	Prefetched func(ref source.Ref)
	// WatchMetered says whether the connection is metered and calls
	// onChange on every change after; nil is never metered.
	WatchMetered func(ctx context.Context, onChange func(bool)) bool
}

type job struct {
	track source.Track
	// sync jobs come from rules other than played; they never evict each
	// other.
	sync bool
	// from is a prefetched copy that left the buffer.
	from *buffered
}

type Cache struct {
	cfg    Config
	db     *sql.DB
	client *http.Client
	stall  time.Duration

	mu      sync.Mutex
	limit   int64
	queue   []job
	queued  map[source.Ref]bool
	wanted  map[source.Ref]bool
	current *job
	// top is each library's tracks by play count, for mostPlayed rules.
	top map[string][]source.Track
	// cancel stops the current download.
	cancel context.CancelFunc
	full   bool
	// online remembers which libraries were available, so a library that
	// comes back is synced once instead of on every status change.
	online map[string]bool
	wake   chan struct{}
	// probing holds one probe at a time.
	probing chan struct{}
	// fileMu orders the read-modify-writes of cache.json.
	fileMu sync.Mutex
	pre    prefetch
}

func Open(ctx context.Context, cfg Config) (*Cache, error) {
	if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
		return nil, err
	}
	db, err := library.OpenDB(ctx, filepath.Join(cfg.Dir, "index.db"), migrations)
	if err != nil {
		return nil, err
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = 30 * time.Second
	c := &Cache{cfg: cfg, db: db, client: &http.Client{Transport: tr, CheckRedirect: source.SameOrigin}, stall: stallTimeout, limit: loadLimit(cfg.ConfigDir),
		queued: map[source.Ref]bool{}, wanted: map[source.Ref]bool{}, online: map[string]bool{}, top: map[string][]source.Track{},
		wake: make(chan struct{}, 1), probing: make(chan struct{}, 1),
		pre: prefetch{buf: map[source.Ref]*buffered{}, wake: make(chan struct{}, 1), tracks: 1}}
	if err := c.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("move the offline settings to rules: %w", err)
	}
	c.sweep(ctx)
	return c, nil
}

func (c *Cache) Close() error { return c.db.Close() }

func fileName(ref source.Ref) string {
	h := sha256.Sum256([]byte(ref.Library + "\x00" + ref.ID))
	return hex.EncodeToString(h[:12])
}

// sweep removes what an interrupted run left: partial downloads, rows whose
// file is gone and files without a row.
func (c *Cache) sweep(ctx context.Context) {
	files := map[string]bool{}
	rows, err := c.db.QueryContext(ctx, "SELECT library_id, id, file FROM entries")
	if err != nil {
		return
	}
	var gone []source.Ref
	for rows.Next() {
		var ref source.Ref
		var f string
		if rows.Scan(&ref.Library, &ref.ID, &f) != nil {
			continue
		}
		if _, err := os.Stat(f); err != nil {
			gone = append(gone, ref)
			continue
		}
		files[f] = true
	}
	rows.Close()
	for _, ref := range gone {
		_, _ = c.db.ExecContext(ctx, "DELETE FROM entries WHERE library_id = ? AND id = ?", ref.Library, ref.ID)
	}
	matches, _ := filepath.Glob(filepath.Join(c.cfg.Dir, "*", "*"))
	matches = append(matches, filepath.Join(c.cfg.Dir, "probe"), filepath.Join(c.cfg.Dir, "probe.part"))
	for _, m := range matches {
		if !files[m] {
			_ = os.Remove(m)
		}
	}
}

// Cached implements library.Offline.
func (c *Cache) Cached(ctx context.Context, ref source.Ref) (player.Stream, bool) {
	var f string
	var gain, start, end sql.NullFloat64
	err := c.db.QueryRowContext(ctx, "SELECT file, gain, silence_start, silence_end FROM entries WHERE library_id = ? AND id = ?",
		ref.Library, ref.ID).Scan(&f, &gain, &start, &end)
	if err != nil {
		return player.Stream{}, false
	}
	if _, err := os.Stat(f); err != nil {
		_, _ = c.db.ExecContext(ctx, "DELETE FROM entries WHERE library_id = ? AND id = ?", ref.Library, ref.ID)
		return player.Stream{}, false
	}
	_, _ = c.db.ExecContext(ctx, "UPDATE entries SET used = unixepoch() WHERE library_id = ? AND id = ? AND used < unixepoch() - 60",
		ref.Library, ref.ID)
	st := player.Stream{URL: f, Silence: loudness.Silence{Start: start.Float64, End: end.Float64}}
	if gain.Valid {
		st.GainDB = &gain.Float64
	}
	return st, true
}

func (c *Cache) Snapshot(ctx context.Context, ref source.Ref) (source.Track, bool) {
	var data string
	if c.db.QueryRowContext(ctx, "SELECT track FROM entries WHERE library_id = ? AND id = ?", ref.Library, ref.ID).Scan(&data) != nil {
		return source.Track{}, false
	}
	var t source.Track
	return t, json.Unmarshal([]byte(data), &t) == nil
}

// rules reads a library's offline rules; only server libraries cache.
func (c *Cache) rules(ctx context.Context, libID string) ([]Rule, source.Source, bool) {
	lib, err := c.cfg.Libraries.Library(ctx, libID)
	if err != nil || !lib.Enabled {
		return nil, nil, false
	}
	src, err := c.cfg.Libraries.Get(libID)
	if err != nil {
		return nil, nil, false
	}
	if _, remote := src.(source.Pinger); !remote {
		return nil, nil, false
	}
	return parseRules(lib.Config[configKey]), src, true
}

// editRules applies edit to a server library's rules and saves them, keeping
// the config keys this version doesn't know.
func (c *Cache) editRules(ctx context.Context, libID string, edit func([]Rule, source.Source) ([]Rule, error)) error {
	rules, src, ok := c.rules(ctx, libID)
	if !ok {
		return fmt.Errorf("library %s isn't a working server library", libID)
	}
	rules, err := edit(rules, src)
	if err != nil {
		return err
	}
	lib, err := c.cfg.Libraries.Library(ctx, libID)
	if err != nil {
		return err
	}
	m := map[string]any{}
	if old, ok := lib.Config[configKey].(map[string]any); ok {
		maps.Copy(m, old)
	}
	m["rules"] = rules
	if err := c.cfg.Libraries.SetConfig(ctx, libID, configKey, m); err != nil {
		return err
	}
	c.mu.Lock()
	c.full = false
	c.mu.Unlock()
	c.cfg.Libraries.PublishChanges(ctx)
	return nil
}

// AddRule adds an offline rule and syncs it; lib "" with kind playlist keeps
// a local playlist, which can hold tracks of every server library.
func (c *Cache) AddRule(ctx context.Context, libID string, r Rule) error {
	r, err := validRule(r)
	if err != nil {
		return err
	}
	if libID == "" {
		return c.keepLocal(ctx, r, true)
	}
	err = c.editRules(ctx, libID, func(rules []Rule, src source.Source) ([]Rule, error) {
		if !supports(src, r.Kind) {
			return nil, fmt.Errorf("this server can't keep %s offline", r.Kind)
		}
		if slices.ContainsFunc(rules, func(x Rule) bool { return x.key() == r.key() }) {
			return nil, errUnchanged
		}
		return append(rules, r), nil
	})
	if errors.Is(err, errUnchanged) {
		return nil
	}
	if err != nil {
		return err
	}
	go c.Sync(context.WithoutCancel(ctx), libID, func(k string) bool { return k == r.key() })
	c.changed()
	return nil
}

var errUnchanged = errors.New("unchanged")

// UpdateRule changes a dynamic rule's limit.
func (c *Cache) UpdateRule(ctx context.Context, libID, kind string, l Limit) error {
	if err := validLimit(&l); err != nil {
		return err
	}
	err := c.editRules(ctx, libID, func(rules []Rule, _ source.Source) ([]Rule, error) {
		i := slices.IndexFunc(rules, func(x Rule) bool { return x.Kind == kind && x.dynamic() })
		if i < 0 {
			return nil, fmt.Errorf("the library has no %s rule", kind)
		}
		rules[i].Limit = &l
		return rules, nil
	})
	if err != nil {
		return err
	}
	go c.Sync(context.WithoutCancel(ctx), libID, none)
	c.changed()
	return nil
}

// RemoveRule removes a rule and deletes the copies no other rule holds.
func (c *Cache) RemoveRule(ctx context.Context, libID string, r Rule) error {
	r.ID = source.Clean(r.ID)
	if libID == "" {
		return c.keepLocal(ctx, r, false)
	}
	err := c.editRules(ctx, libID, func(rules []Rule, _ source.Source) ([]Rule, error) {
		return slices.DeleteFunc(rules, func(x Rule) bool { return x.key() == r.key() }), nil
	})
	if err != nil {
		return err
	}
	c.Sync(ctx, libID, none)
	c.changed()
	return nil
}

func hasKind(rules []Rule, kind string) bool {
	return slices.ContainsFunc(rules, func(r Rule) bool { return r.Kind == kind })
}

// Played notes a track that started playing and caches it in libraries
// with a played rule.
func (c *Cache) Played(ctx context.Context, t source.Track) {
	ref := t.Ref()
	res, err := c.db.ExecContext(ctx, "UPDATE entries SET played = unixepoch() WHERE library_id = ? AND id = ?", ref.Library, ref.ID)
	rules, _, ok := c.rules(ctx, t.Library)
	if !ok || !hasKind(rules, KindPlayed) {
		return
	}
	if err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			c.refresh(ctx, t.Library, false)
			return
		}
	}
	c.enqueue([]source.Track{t}, false)
}

// HistoryChanged moves a library's mostPlayed rule after a counted play.
func (c *Cache) HistoryChanged(ctx context.Context, libID string) {
	if rules, _, ok := c.rules(ctx, libID); ok && hasKind(rules, KindMostPlayed) {
		c.refresh(ctx, libID, true)
	}
}

// LibrariesChanged syncs the libraries that just became available.
func (c *Cache) LibrariesChanged(ctx context.Context) {
	infos, err := c.cfg.Libraries.List(ctx)
	if err != nil {
		return
	}
	var back []string
	c.mu.Lock()
	for _, info := range infos {
		if info.Available && !c.online[info.ID] {
			back = append(back, info.ID)
		}
		c.online[info.ID] = info.Available
	}
	c.mu.Unlock()
	for _, id := range back {
		c.Sync(ctx, id, all)
	}
}

// Favorited resyncs a library's favorites rule after a favorite changed.
func (c *Cache) Favorited(ctx context.Context, libID string) {
	c.Sync(ctx, libID, func(k string) bool { return k == KindFavorites })
}

// PlaylistChanged resyncs a server playlist's rule after it was edited.
func (c *Cache) PlaylistChanged(ctx context.Context, libID, id string) {
	c.Sync(ctx, libID, func(k string) bool { return k == KindPlaylist+":"+id })
}

func all(string) bool  { return true }
func none(string) bool { return false }

// Sync resolves the rules match picks (by key), then recomputes the dynamic
// rules, deletes the copies that lost their last rule and queues what is
// missing.
func (c *Cache) Sync(ctx context.Context, libID string, match func(key string) bool) {
	rules, src, ok := c.rules(ctx, libID)
	if !ok {
		c.setWanted(libID, nil)
		c.drop(libID, false)
		return
	}
	var lost []string
	var fetch []source.Track
	cover := func(key string, tracks []source.Track, err error) {
		if err != nil {
			log.Warnf("cache: list %s of %s: %v", key, src.Library().Name, err)
			return
		}
		ids := make([]string, 0, len(tracks))
		for _, t := range tracks {
			if t.Library == libID {
				ids = append(ids, t.ID)
				fetch = append(fetch, t)
			}
		}
		l, err := c.setCover(ctx, libID, key, ids, true)
		if err != nil {
			log.Warnf("cache: %v", err)
		}
		lost = append(lost, l...)
	}
	for _, r := range rules {
		if !r.dynamic() && match(r.key()) {
			tracks, err := scope(ctx, src, r)
			cover(r.key(), tracks, err)
		}
	}
	for _, id := range c.localPlaylists() {
		if c.cfg.LocalPlaylist != nil && match(localPrefix+id) {
			tracks, err := c.cfg.LocalPlaylist(ctx, id)
			cover(localPrefix+id, tracks, err)
		}
	}
	l, err := c.dropStale(ctx, libID)
	if err != nil {
		log.Warnf("cache: %v", err)
	}
	if err := c.prune(ctx, libID, append(lost, l...)); err != nil {
		log.Warnf("cache: %v", err)
	}
	c.refresh(ctx, libID, true)
	c.enqueue(fetch, true)
}

// refresh recomputes a library's dynamic rules (reading the play counts
// again when top is set), deletes what they let go and updates the queue.
func (c *Cache) refresh(ctx context.Context, libID string, top bool) {
	rules, _, ok := c.rules(ctx, libID)
	if !ok {
		return
	}
	if top && hasKind(rules, KindMostPlayed) && c.cfg.MostPlayed != nil {
		if list, err := c.cfg.MostPlayed(ctx); err != nil {
			log.Warnf("cache: read play counts: %v", err)
		} else {
			mine := []source.Track{}
			for _, t := range list {
				if t.Library == libID && len(mine) < maxSongs {
					mine = append(mine, t)
				}
			}
			c.mu.Lock()
			c.top[libID] = mine
			c.mu.Unlock()
		}
	}
	lost, fetch, err := c.dynamic(ctx, libID, rules)
	if err != nil {
		log.Warnf("cache: %v", err)
	}
	if err := c.prune(ctx, libID, lost); err != nil {
		log.Warnf("cache: %v", err)
	}
	c.setWanted(libID, c.wantedIDs(ctx, libID))
	c.drop(libID, hasKind(rules, KindPlayed))
	c.enqueue(fetch, true)
}

func (c *Cache) setWanted(libID string, tracks []source.Track) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for ref := range c.wanted {
		if ref.Library == libID {
			delete(c.wanted, ref)
		}
	}
	for _, t := range tracks {
		c.wanted[t.Ref()] = true
	}
}

// drop removes a library's queued downloads no rule wants any more, and
// stops the current one if it is such a download.
func (c *Cache) drop(libID string, played bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	unwanted := func(j job) bool {
		return j.track.Library == libID && !c.wanted[j.track.Ref()] && (j.sync || !played)
	}
	c.queue = slices.DeleteFunc(c.queue, func(j job) bool {
		if unwanted(j) {
			delete(c.queued, j.track.Ref())
			return true
		}
		return false
	})
	if c.current != nil && unwanted(*c.current) {
		c.cancel()
	}
}

func (c *Cache) cachedIDs(libID string) map[string]bool {
	out := map[string]bool{}
	rows, err := c.db.Query("SELECT id FROM entries WHERE library_id = ?", libID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out
}

func (c *Cache) enqueue(tracks []source.Track, sync bool) {
	have := map[string]map[string]bool{}
	for _, t := range tracks {
		if have[t.Library] == nil {
			have[t.Library] = c.cachedIDs(t.Library)
		}
	}
	added := false
	c.mu.Lock()
	for _, t := range tracks {
		ref := t.Ref()
		if c.queued[ref] || (sync && c.full) || have[ref.Library][ref.ID] {
			continue
		}
		c.queued[ref] = true
		c.queue = append(c.queue, job{track: t, sync: sync})
		added = true
	}
	c.mu.Unlock()
	if added {
		select {
		case c.wake <- struct{}{}:
		default:
		}
	}
}

// Run downloads queued tracks one at a time until ctx ends.
func (c *Cache) Run(ctx context.Context) {
	old := &loudness.Loop{Meter: c.cfg.Meter, Pending: c.pendingSilence, Save: c.saveSilence}
	old.Start(ctx)
	go c.runPrefetch(ctx)
	for {
		c.mu.Lock()
		var j job
		ok := len(c.queue) > 0
		jctx, cancel := context.WithCancel(ctx)
		if ok {
			j, c.queue = c.queue[0], c.queue[1:]
			cur := j
			c.current, c.cancel = &cur, cancel
		}
		c.mu.Unlock()
		if !ok {
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-c.wake:
				continue
			}
		}
		if err := c.fetch(jctx, j); err != nil && jctx.Err() == nil {
			log.Warnf("cache %q: %v", j.track.Title, err)
		}
		cancel()
		c.mu.Lock()
		delete(c.queued, j.track.Ref())
		c.current, c.cancel = nil, nil
		c.mu.Unlock()
		c.changed()
		if ctx.Err() != nil {
			return
		}
	}
}

func (c *Cache) fetch(ctx context.Context, j job) error {
	t := j.track
	ref := t.Ref()
	if _, ok := c.Cached(ctx, ref); ok {
		return nil
	}
	src, err := c.cfg.Libraries.Get(ref.Library)
	if err != nil {
		return err
	}
	dir := filepath.Join(c.cfg.Dir, fileName(source.Ref{Library: ref.Library})[:8])
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file := filepath.Join(dir, fileName(ref))
	size, m, ok := c.adopt(j, file)
	if !ok {
		if c.cfg.Libraries.Unavailable(ctx, ref).Code != "" {
			return nil
		}
		if size, err = c.download(ctx, src, ref.ID, file, 0); err != nil {
			return err
		}
		m = c.measure(ctx, t, file)
	}
	data, _ := json.Marshal(t)
	played := "0"
	if !j.sync {
		played = "unixepoch()"
	}
	if _, err := c.db.ExecContext(ctx, `INSERT OR REPLACE INTO entries (library_id, id, file, size, added, used, track, gain, played,
		silence_start, silence_end) VALUES (?, ?, ?, ?, unixepoch(), unixepoch(), ?, ?, `+played+`, ?, ?)`,
		ref.Library, ref.ID, file, size, string(data), m.gain, m.start, m.end); err != nil {
		os.Remove(file)
		return err
	}
	if cov, ok := src.(source.CoverArt); ok && t.CoverID != "" {
		_, _ = cov.Cover(ctx, t.CoverID, coverSize)
	}
	if err := c.evict(ctx, ref, j.sync); err != nil {
		return err
	}
	c.refresh(ctx, ref.Library, false)
	return nil
}

// measured is a measurement for the index; nil fields weren't measured.
type measured struct{ gain, start, end *float64 }

// measure measures a downloaded file when its gain is missing or its
// silence is wanted; the backfill (pendingSilence) catches up on silence
// once it is.
func (c *Cache) measure(ctx context.Context, t source.Track, file string) measured {
	if !c.cfg.Meter.Available() || (hasReplayGain(t) && !c.cfg.Meter.Silence()) {
		return measured{}
	}
	r, err := c.cfg.Meter.Measure(ctx, file)
	if err != nil {
		return measured{}
	}
	return measured{&r.Gain, &r.Silence.Start, &r.Silence.End}
}

// ProbedGain implements library.Offline.
func (c *Cache) ProbedGain(ctx context.Context, ref source.Ref) (float64, bool) {
	var gain float64
	err := c.db.QueryRowContext(ctx, "SELECT gain FROM probes WHERE library_id = ? AND id = ?", ref.Library, ref.ID).Scan(&gain)
	return gain, err == nil
}

// Probe implements library.Offline: it measures the loudness of the first
// probeBytes of a server track and keeps the gain for later plays. Silence
// isn't kept, as the end of the file is missing. A track in the prefetch
// buffer is never probed: its full download brings the gain.
func (c *Cache) Probe(ctx context.Context, ref source.Ref) (float64, error) {
	if !c.cfg.Meter.Available() {
		return 0, errors.New("ffmpeg is not installed")
	}
	if c.cfg.Libraries.Unavailable(ctx, ref).Code != "" {
		return 0, errors.New("the track's library is unavailable")
	}
	src, err := c.cfg.Libraries.Get(ref.Library)
	if err != nil {
		return 0, err
	}
	for {
		if b := c.buffered(ref); b != nil {
			select {
			case <-b.done:
			case <-ctx.Done():
				return 0, ctx.Err()
			}
			if g, ok := c.ProbedGain(ctx, ref); ok {
				return g, nil
			}
			if c.buffered(ref) == b {
				return 0, errors.New("the track's prefetch brought no gain")
			}
			// Dropped or failed; probe after all.
			continue
		}
		gain, again, err := c.probe(ctx, src, ref)
		if !again {
			return gain, err
		}
	}
}

// probe downloads and measures the start of ref's stream unless the
// prefetch holds the track; again says it does, now or since it took over.
func (c *Cache) probe(ctx context.Context, src source.Source, ref source.Ref) (float64, bool, error) {
	select {
	case c.probing <- struct{}{}:
	case <-ctx.Done():
		return 0, false, ctx.Err()
	}
	defer func() { <-c.probing }()
	if g, ok := c.ProbedGain(ctx, ref); ok {
		return g, false, nil
	}
	pctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	c.mu.Lock()
	if b := c.pre.buf[ref]; b != nil && !b.failed {
		c.mu.Unlock()
		return 0, true, nil
	}
	c.pre.probe, c.pre.probeCancel = ref, cancel
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.pre.probe, c.pre.probeCancel = source.Ref{}, nil
		c.mu.Unlock()
	}()
	file := filepath.Join(c.cfg.Dir, "probe")
	defer os.Remove(file)
	if _, err := c.download(pctx, src, ref.ID, file, probeBytes); err != nil {
		return 0, ctx.Err() == nil && c.buffered(ref) != nil, err
	}
	r, err := c.cfg.Meter.Measure(pctx, file)
	if err != nil {
		return 0, ctx.Err() == nil && c.buffered(ref) != nil, err
	}
	_, err = c.db.ExecContext(ctx, "INSERT OR REPLACE INTO probes (library_id, id, gain) VALUES (?, ?, ?)", ref.Library, ref.ID, r.Gain)
	return r.Gain, false, err
}

// pendingSilence lists copies cached before their silence was wanted.
func (c *Cache) pendingSilence(ctx context.Context, silence bool) ([]loudness.Job, error) {
	if !silence {
		return nil, nil
	}
	rows, err := c.db.QueryContext(ctx, "SELECT file FROM entries WHERE silence_start IS NULL LIMIT 50")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []loudness.Job
	for rows.Next() {
		var j loudness.Job
		if err := rows.Scan(&j.Path); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// saveSilence stores a copy's silence; a failed measurement stores none, so
// the copy plays untrimmed instead of being measured again.
func (c *Cache) saveSilence(ctx context.Context, j loudness.Job, r loudness.Result, err error) error {
	if err != nil {
		log.Debugf("silence of %s: %v", j.Path, err)
		r = loudness.Result{}
	}
	_, err = c.db.ExecContext(ctx, "UPDATE entries SET silence_start = ?, silence_end = ? WHERE file = ?",
		r.Silence.Start, r.Silence.End, j.Path)
	return err
}

func hasReplayGain(t source.Track) bool {
	return t.ReplayGain != nil && (t.ReplayGain.Track != nil || t.ReplayGain.Album != nil)
}

var errStalled = errors.New("the server stopped sending the file")

// stallReader moves the stall deadline on with every read that brings data.
type stallReader struct {
	r     io.Reader
	timer *time.Timer
	after time.Duration
}

func (s stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.timer.Reset(s.after)
	}
	return n, err
}

// download writes the original file to path: into a .part file first,
// renamed only once complete. With prefix, only the file's first prefix
// bytes.
func (c *Cache) download(ctx context.Context, src source.Source, id, path string, prefix int64) (int64, error) {
	streamURL, err := src.StreamURL(ctx, id)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	ctx, stalled := context.WithCancelCause(ctx)
	defer stalled(nil)
	timer := time.AfterFunc(c.stall, func() { stalled(errStalled) })
	defer timer.Stop()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return 0, errors.New("the server's stream address is not a URL")
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return 0, errors.New("only streams over http(s) are cached")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		// The URL carries credentials; keep it out of the error.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("server answered %s", resp.Status)
	}
	limit := int64(maxFileBytes)
	if prefix > 0 {
		limit = prefix
	} else if resp.ContentLength > maxFileBytes {
		return 0, errors.New("the file is too large to cache")
	}
	part := path + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, io.LimitReader(stallReader{resp.Body, timer, c.stall}, limit+1))
	if err != nil && errors.Is(context.Cause(ctx), errStalled) {
		err = errStalled
	}
	if prefix > 0 && n > limit {
		n, err = limit, f.Truncate(limit)
	}
	if err == nil && n > maxFileBytes {
		err = errors.New("the file is too large to cache")
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(part, path)
	}
	if err != nil {
		os.Remove(part)
		return 0, err
	}
	return n, nil
}

// evict deletes copies until the cache fits its limit: only copies no
// static rule pins, least recently played first. A sync copy never evicts
// another copy a rule wants; when it doesn't fit, it goes itself and syncing
// stops until the rules or the limit change.
func (c *Cache) evict(ctx context.Context, keep source.Ref, sync bool) error {
	for {
		var used int64
		if err := c.db.QueryRowContext(ctx, "SELECT coalesce(sum(size), 0) FROM entries").Scan(&used); err != nil {
			return err
		}
		c.mu.Lock()
		limit := c.limit
		c.mu.Unlock()
		if used <= limit {
			return nil
		}
		victim, err := c.victim(ctx, keep, sync)
		if err != nil {
			return err
		}
		if victim == keep && !sync && c.pinned(ctx, keep) {
			// A played download a rule holds is a pinned copy now: it stays
			// over the limit like the others.
			return nil
		}
		if victim == keep && sync {
			c.mu.Lock()
			c.full = true
			c.queue = slices.DeleteFunc(c.queue, func(j job) bool {
				if j.sync {
					delete(c.queued, j.track.Ref())
				}
				return j.sync
			})
			c.mu.Unlock()
		}
		if err := c.remove(ctx, "library_id = ? AND id = ?", victim.Library, victim.ID); err != nil {
			return err
		}
		if victim == keep {
			return nil
		}
	}
}

func (c *Cache) pinned(ctx context.Context, ref source.Ref) bool {
	var ok bool
	err := c.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM covers WHERE library_id = ? AND id = ? AND static)",
		ref.Library, ref.ID).Scan(&ok)
	return err == nil && ok
}

// victim is the next copy evict deletes, or keep when there is none.
func (c *Cache) victim(ctx context.Context, keep source.Ref, sync bool) (source.Ref, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT library_id, id FROM entries e WHERE NOT EXISTS
		(SELECT 1 FROM covers WHERE covers.library_id = e.library_id AND covers.id = e.id AND covers.static)
		ORDER BY played, used, added`)
	if err != nil {
		return keep, err
	}
	var refs []source.Ref
	for rows.Next() {
		var ref source.Ref
		if err := rows.Scan(&ref.Library, &ref.ID); err != nil {
			rows.Close()
			return keep, err
		}
		refs = append(refs, ref)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return keep, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ref := range refs {
		if ref != keep && (!sync || !c.wanted[ref]) {
			return ref, nil
		}
	}
	return keep, nil
}

// remove deletes the copies matching where, files first.
func (c *Cache) remove(ctx context.Context, where string, args ...any) error {
	rows, err := c.db.QueryContext(ctx, "SELECT file FROM entries WHERE "+where, args...)
	if err != nil {
		return err
	}
	var files []string
	for rows.Next() {
		var f string
		if rows.Scan(&f) == nil {
			files = append(files, f)
		}
	}
	rows.Close()
	for _, f := range files {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	_, err = c.db.ExecContext(ctx, "DELETE FROM entries WHERE "+where, args...)
	return err
}

// Clear removes every copy, or those of one library, with what the rules
// covered; the rules stay and fetch again on their next sync.
func (c *Cache) Clear(ctx context.Context, libID string) error {
	c.mu.Lock()
	c.queue = slices.DeleteFunc(c.queue, func(j job) bool {
		drop := libID == "" || j.track.Library == libID
		if drop {
			delete(c.queued, j.track.Ref())
		}
		return drop
	})
	if c.current != nil && (libID == "" || c.current.track.Library == libID) {
		c.cancel()
	}
	c.full = false
	for lib := range c.top {
		if libID == "" || lib == libID {
			delete(c.top, lib)
		}
	}
	c.mu.Unlock()
	err := c.remove(ctx, "? = '' OR library_id = ?", libID, libID)
	for _, table := range []string{"covers", "probes"} {
		if err == nil {
			_, err = c.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE ? = '' OR library_id = ?", libID, libID)
		}
	}
	c.changed()
	return err
}

type Entry struct {
	Track source.Track `json:"track"`
	Size  int64        `json:"size"`
	Added int64        `json:"added"`
	Used  int64        `json:"used"`
	// Played is when the track last started playing; 0 for never.
	Played int64 `json:"played"`
	// Rules hold the copy; empty for a copy only the size limit decides on.
	Rules []RuleRef `json:"rules"`
}

// List returns the cached tracks, most recently used first, with the rules
// that hold each.
func (c *Cache) List(ctx context.Context, libID string) ([]Entry, error) {
	held := map[source.Ref][]RuleRef{}
	crows, err := c.db.QueryContext(ctx, `SELECT c.library_id, c.id, c.rule FROM covers c JOIN entries e USING (library_id, id)
		WHERE ? = '' OR c.library_id = ? ORDER BY c.rule`, libID, libID)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var ref source.Ref
		var key string
		if crows.Scan(&ref.Library, &ref.ID, &key) == nil {
			held[ref] = append(held[ref], refOf(key))
		}
	}
	crows.Close()
	rows, err := c.db.QueryContext(ctx, `SELECT library_id, id, track, size, added, used, played FROM entries
		WHERE ? = '' OR library_id = ? ORDER BY used DESC`, libID, libID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		var ref source.Ref
		var data string
		if err := rows.Scan(&ref.Library, &ref.ID, &data, &e.Size, &e.Added, &e.Used, &e.Played); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(data), &e.Track)
		e.Rules = held[ref]
		if e.Rules == nil {
			e.Rules = []RuleRef{}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RuleStatus is a rule with what it covers: Tracks in its scope (for played,
// the copies it holds), Cached of them and their Size.
type RuleStatus struct {
	Rule
	Key    string `json:"key"`
	Local  bool   `json:"local,omitempty"`
	Tracks int    `json:"tracks"`
	Cached int    `json:"cached"`
	Size   int64  `json:"size"`
}

// LibraryStatus is one server library's rules and usage.
type LibraryStatus struct {
	Library string       `json:"library"`
	Rules   []RuleStatus `json:"rules"`
	Used    int64        `json:"used"`
	Count   int          `json:"count"`
}

type Status struct {
	Limit       int64         `json:"limit"`
	Used        int64         `json:"used"`
	Count       int           `json:"count"`
	Queued      int           `json:"queued"`
	Downloading *source.Track `json:"downloading"`
	// Full is set when a pinned copy didn't fit and syncing stopped.
	Full bool `json:"full"`
	// Pinned is the size of the copies static rules hold; PinnedOverLimit
	// says that alone is more than the limit.
	Pinned          int64           `json:"pinned"`
	PinnedOverLimit bool            `json:"pinnedOverLimit"`
	Libraries       []LibraryStatus `json:"libraries"`
	// LocalRules are the local playlists kept on this device.
	LocalRules []RuleStatus `json:"localRules"`
}

type usage struct {
	tracks, cached int
	size           int64
}

// ruleUsage adds up each rule's coverage per library ("" for local
// playlists, summed over libraries).
func (c *Cache) ruleUsage(ctx context.Context) (map[string]map[string]usage, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT c.library_id, c.rule, count(*), count(e.id), coalesce(sum(e.size), 0)
		FROM covers c LEFT JOIN entries e USING (library_id, id) GROUP BY c.library_id, c.rule`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]usage{}
	for rows.Next() {
		var lib, key string
		var u usage
		if err := rows.Scan(&lib, &key, &u.tracks, &u.cached, &u.size); err != nil {
			return nil, err
		}
		if strings.HasPrefix(key, localPrefix) {
			lib = ""
		}
		if out[lib] == nil {
			out[lib] = map[string]usage{}
		}
		sum := out[lib][key]
		out[lib][key] = usage{sum.tracks + u.tracks, sum.cached + u.cached, sum.size + u.size}
	}
	return out, rows.Err()
}

func ruleStatus(r Rule, u usage) RuleStatus {
	rs := RuleStatus{Rule: r, Key: r.key(), Tracks: u.tracks, Cached: u.cached, Size: u.size}
	if r.Kind == KindPlayed {
		rs.Tracks = u.cached
	}
	return rs
}

func (c *Cache) Status(ctx context.Context) (Status, error) {
	st := Status{Libraries: []LibraryStatus{}, LocalRules: []RuleStatus{}}
	usageOf := map[string]LibraryStatus{}
	rows, err := c.db.QueryContext(ctx, "SELECT library_id, sum(size), count(*) FROM entries GROUP BY library_id")
	if err != nil {
		return st, err
	}
	for rows.Next() {
		var u LibraryStatus
		if rows.Scan(&u.Library, &u.Used, &u.Count) == nil {
			usageOf[u.Library] = u
			st.Used += u.Used
			st.Count += u.Count
		}
	}
	rows.Close()
	if err := c.db.QueryRowContext(ctx, `SELECT coalesce(sum(size), 0) FROM entries e WHERE EXISTS
		(SELECT 1 FROM covers WHERE covers.library_id = e.library_id AND covers.id = e.id AND covers.static)`).Scan(&st.Pinned); err != nil {
		return st, err
	}
	rules, err := c.ruleUsage(ctx)
	if err != nil {
		return st, err
	}
	infos, err := c.cfg.Libraries.List(ctx)
	if err != nil {
		return st, err
	}
	for _, info := range infos {
		if !slices.Contains(info.Capabilities, source.CapPing) {
			continue
		}
		u := usageOf[info.ID]
		u.Library, u.Rules = info.ID, []RuleStatus{}
		for _, r := range parseRules(info.Config[configKey]) {
			u.Rules = append(u.Rules, ruleStatus(r, rules[info.ID][r.key()]))
		}
		st.Libraries = append(st.Libraries, u)
	}
	for _, id := range c.localPlaylists() {
		rs := ruleStatus(Rule{Kind: KindPlaylist, ID: id}, rules[""][localPrefix+id])
		rs.Key, rs.Local = localPrefix+id, true
		st.LocalRules = append(st.LocalRules, rs)
	}
	c.mu.Lock()
	st.Limit, st.Queued, st.Full = c.limit, len(c.queue), c.full
	if c.current != nil {
		t := c.current.track
		st.Downloading = &t
	}
	c.mu.Unlock()
	st.PinnedOverLimit = st.Pinned > st.Limit
	return st, nil
}

func (c *Cache) changed() {
	if c.cfg.Publish != nil {
		c.cfg.Publish(Topic, map[string]any{"type": "changed"})
	}
}
