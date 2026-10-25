package cache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

const (
	prefetchDir = "prefetch"
	maxPrefetch = 3
	// prefetchCap bounds the buffer; a lower cache limit lowers it.
	prefetchCap = 2 << 30
)

// prefetch is the buffer of upcoming server tracks, apart from the offline
// rules: the current track's copy and the next ones', downloaded in full
// while something plays. Guarded by Cache.mu.
type prefetch struct {
	buf map[source.Ref]*buffered
	// ahead are the next tracks in play order, the ones downloaded.
	ahead     []source.Ref
	tracks    int
	onMetered bool
	metered   bool
	watching  bool
	status    player.Status
	queue     *player.QueueSnapshot
	seq       int
	wake      chan struct{}
	// probe is the track a probe is downloading; probeCancel stops it when
	// the prefetch takes the track over.
	probe       source.Ref
	probeCancel context.CancelFunc
	// planMu orders the plans, so an older one never lands last.
	planMu sync.Mutex
}

// buffered is one track of the buffer. done closes once its download
// finished or failed, or when it is dropped before it started.
type buffered struct {
	track          source.Track
	file           string
	size           int64
	m              measured
	started, ready bool
	// failed is a download that didn't make it; the probe covers the track.
	failed bool
	cancel context.CancelFunc
	done   chan struct{}
}

// upcoming lists the n tracks that play after the one at index, in play
// order.
func upcoming(q player.QueueSnapshot, index, n int) []source.Track {
	if index < 0 || index >= len(q.Items) || q.Repeat == player.RepeatOne {
		return nil
	}
	var out []source.Track
	for i := 1; i <= n; i++ {
		j := index + i
		if j >= len(q.Items) {
			if q.Repeat != player.RepeatAll {
				break
			}
			j %= len(q.Items)
		}
		if j == index {
			break
		}
		out = append(out, q.Items[j].Track)
	}
	return out
}

// prefetchCount is how many tracks to prefetch: the setting, or none on a
// metered connection unless the user allowed it there.
func prefetchCount(tracks int, onMetered, metered bool) int {
	if metered && !onMetered {
		return 0
	}
	return max(0, min(tracks, maxPrefetch))
}

// SetPrefetch applies the prefetch settings.
func (c *Cache) SetPrefetch(ctx context.Context, tracks int, onMetered bool) {
	c.mu.Lock()
	c.pre.tracks, c.pre.onMetered = tracks, onMetered
	c.mu.Unlock()
	c.replan(ctx)
}

// PlayerChanged follows the player's status.
func (c *Cache) PlayerChanged(ctx context.Context, st player.Status) {
	c.mu.Lock()
	c.pre.status = st
	c.mu.Unlock()
	c.replan(ctx)
}

// QueueChanged follows the queue. Its index is newer than the last
// status's; a later status brings the next change of track.
func (c *Cache) QueueChanged(ctx context.Context, q player.QueueSnapshot) {
	c.mu.Lock()
	c.pre.queue = &q
	c.pre.status.Index, c.pre.status.Track = q.Index, nil
	if q.Index >= 0 && q.Index < len(q.Items) {
		c.pre.status.Track = &q.Items[q.Index].Track
	}
	c.mu.Unlock()
	c.replan(ctx)
}

// replan works out the window: nothing while stopped or casting (the cast
// device fetches for itself), else the current server track and the next
// ones.
func (c *Cache) replan(ctx context.Context) {
	c.pre.planMu.Lock()
	defer c.pre.planMu.Unlock()
	c.mu.Lock()
	st, q, n := c.pre.status, c.pre.queue, c.pre.tracks
	c.mu.Unlock()
	var cur *source.Track
	var next []source.Track
	if st.State != player.StateStopped && !st.Casting && st.Track != nil && q != nil {
		if c.cfg.Libraries.Server(st.Track.Library) {
			cur = st.Track
		}
		for _, t := range upcoming(*q, st.Index, min(n, maxPrefetch)) {
			if c.cfg.Libraries.Server(t.Library) {
				next = append(next, t)
			}
		}
	}
	if len(next) > 0 {
		next = next[:min(len(next), c.allowedCount(ctx, n))]
	}
	c.setWindow(ctx, cur, next)
}

// allowedCount applies the metered rule, watching the connection from the
// first time it matters.
func (c *Cache) allowedCount(ctx context.Context, n int) int {
	c.mu.Lock()
	start := !c.pre.watching && c.cfg.WatchMetered != nil
	c.pre.watching = true
	c.mu.Unlock()
	if start {
		m := c.cfg.WatchMetered(ctx, func(m bool) {
			c.mu.Lock()
			c.pre.metered = m
			c.mu.Unlock()
			c.replan(ctx)
		})
		c.mu.Lock()
		c.pre.metered = m
		c.mu.Unlock()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return prefetchCount(n, c.pre.onMetered, c.pre.metered)
}

// setWindow keeps the buffer to cur and next and cancels the rest. The
// current track keeps only a finished copy: one still downloading would be
// fetched twice, as mpv streams the track by now. mpv can't play a file
// that is still growing (it ends the track at the file's end so far).
func (c *Cache) setWindow(ctx context.Context, cur *source.Track, next []source.Track) {
	c.mu.Lock()
	keep := map[source.Ref]bool{}
	ahead := []source.Ref{}
	for _, t := range next {
		ref := t.Ref()
		if keep[ref] {
			continue
		}
		keep[ref] = true
		ahead = append(ahead, ref)
		if c.pre.buf[ref] == nil {
			c.pre.buf[ref] = &buffered{track: t, done: make(chan struct{})}
		}
	}
	var gone []*buffered
	for ref, b := range c.pre.buf {
		if keep[ref] || (cur != nil && ref == cur.Ref() && b.ready) {
			continue
		}
		delete(c.pre.buf, ref)
		if b.cancel != nil {
			b.cancel()
		}
		if !b.started {
			close(b.done)
			continue
		}
		gone = append(gone, b)
	}
	c.pre.ahead = ahead
	c.mu.Unlock()
	for _, b := range gone {
		go c.release(ctx, b)
	}
	select {
	case c.pre.wake <- struct{}{}:
	default:
	}
}

// release deletes a copy that left the window, after an offline rule that
// wants the track has taken it into the cache.
func (c *Cache) release(ctx context.Context, b *buffered) {
	<-b.done
	defer os.Remove(b.file)
	ref := b.track.Ref()
	c.mu.Lock()
	wanted, queued := c.wanted[ref], c.queued[ref]
	c.mu.Unlock()
	if !b.ready || (!wanted && !queued) {
		return
	}
	if err := c.fetch(ctx, job{track: b.track, sync: wanted, from: b}); err != nil {
		log.Warnf("cache %q: %v", b.track.Title, err)
	}
	c.changed()
}

// buffered is ref's entry in the buffer, or nil; a failed download counts
// as none.
func (c *Cache) buffered(ref source.Ref) *buffered {
	c.mu.Lock()
	defer c.mu.Unlock()
	if b := c.pre.buf[ref]; b != nil && !b.failed {
		return b
	}
	return nil
}

// Buffered implements library.Offline: the prefetched copy of ref, once it
// is complete.
func (c *Cache) Buffered(ctx context.Context, ref source.Ref) (player.Stream, bool) {
	c.mu.Lock()
	b := c.pre.buf[ref]
	ok := b != nil && b.ready
	var st player.Stream
	if ok {
		st = player.Stream{URL: b.file, GainDB: b.m.gain}
		if b.m.start != nil {
			st.Silence = loudness.Silence{Start: *b.m.start, End: *b.m.end}
		}
	}
	c.mu.Unlock()
	if ok && st.GainDB == nil {
		if g, found := c.ProbedGain(ctx, ref); found {
			st.GainDB = &g
		}
	}
	return st, ok
}

// adopt links j's prefetched copy to file, so the cache keeps it without a
// second download.
func (c *Cache) adopt(j job, file string) (int64, measured, bool) {
	c.mu.Lock()
	b := j.from
	if b == nil {
		b = c.pre.buf[j.track.Ref()]
	}
	ok := b != nil && b.ready
	var size int64
	var m measured
	var src string
	if ok {
		size, m, src = b.size, b.m, b.file
	}
	c.mu.Unlock()
	if !ok {
		return 0, measured{}, false
	}
	// ErrExist: a concurrent fetch of the same track linked it already.
	if err := os.Link(src, file); err != nil && !errors.Is(err, fs.ErrExist) {
		return 0, measured{}, false
	}
	return size, m, true
}

// runPrefetch downloads the window's next tracks one at a time, in play
// order, until ctx ends.
func (c *Cache) runPrefetch(ctx context.Context) {
	for {
		b, bctx := c.nextPrefetch(ctx)
		if b == nil {
			select {
			case <-ctx.Done():
				return
			case <-c.pre.wake:
				continue
			}
		}
		c.prefetchOne(bctx, b)
	}
}

func (c *Cache) nextPrefetch(ctx context.Context) (*buffered, context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var used int64
	for _, b := range c.pre.buf {
		used += b.size
	}
	if used >= min(prefetchCap, c.limit) {
		return nil, nil
	}
	for _, ref := range c.pre.ahead {
		b := c.pre.buf[ref]
		if b.started {
			continue
		}
		bctx, cancel := context.WithCancel(ctx)
		b.started, b.cancel = true, cancel
		if c.pre.probe == ref && c.pre.probeCancel != nil {
			c.pre.probeCancel()
		}
		c.pre.seq++
		b.file = filepath.Join(c.cfg.Dir, prefetchDir, fmt.Sprintf("%s-%d", fileName(ref), c.pre.seq))
		return b, bctx
	}
	return nil, nil
}

func (c *Cache) prefetchOne(ctx context.Context, b *buffered) {
	defer close(b.done)
	defer b.cancel()
	ref := b.track.Ref()
	if _, ok := c.Cached(ctx, ref); ok || c.cfg.Libraries.Unavailable(ctx, ref).Code != "" {
		return
	}
	src, err := c.cfg.Libraries.Get(ref.Library)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(b.file), 0o700); err != nil {
		log.Warnf("prefetch: %v", err)
		return
	}
	size, err := c.download(ctx, src, ref.ID, b.file, 0)
	if err != nil {
		if ctx.Err() == nil {
			log.Warnf("prefetch %q: %v", b.track.Title, err)
			c.mu.Lock()
			b.failed = true
			c.mu.Unlock()
		}
		return
	}
	m := c.measure(ctx, b.track, b.file)
	if ctx.Err() != nil {
		return
	}
	if m.gain != nil {
		if _, err := c.db.ExecContext(ctx, "INSERT OR REPLACE INTO probes (library_id, id, gain) VALUES (?, ?, ?)",
			ref.Library, ref.ID, *m.gain); err != nil {
			log.Warnf("prefetch: %v", err)
		}
	}
	c.mu.Lock()
	b.size, b.m, b.ready = size, m, true
	c.mu.Unlock()
	if c.cfg.Prefetched != nil {
		c.cfg.Prefetched(ref)
	}
}
