// Package pendingmarks keeps favorites and ratings set on a server track
// while its server was unreachable, and sends them once it is back.
package pendingmarks

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/atomicfile"
)

const (
	FileName = "pending-marks.json"
	maxItems = 1000
	// maxTries drops a change its server keeps refusing, so one bad item
	// can't hold up the rest of its library forever.
	maxTries = 5
)

var (
	ErrFull = errors.New("too many favorite and rating changes are waiting for their servers")
	// ErrUnreachable from send keeps a change without counting a try.
	ErrUnreachable = errors.New("server unreachable")
)

// Change is the latest favorite and rating of one item; nil fields are
// unchanged.
type Change struct {
	Library  string `json:"library"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Favorite *bool  `json:"favorite,omitempty"`
	Rating   *int   `json:"rating,omitempty"`
	Tries    int    `json:"tries,omitempty"`
	// seq tells a change made while a flush sent the older one.
	seq uint64
}

type key struct{ library, kind, id string }

func (c Change) key() key { return key{c.Library, c.Kind, c.ID} }

type Queue struct {
	path     string
	mu       sync.Mutex
	items    map[key]Change
	seq      uint64
	flushing atomic.Bool
}

// Open loads the queue; a file it can't read is set aside, not overwritten.
func Open(path string) *Queue {
	q := &Queue{path: path, items: map[key]Change{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return q
	}
	var saved struct {
		Version int      `json:"version"`
		Items   []Change `json:"items"`
	}
	if json.Unmarshal(data, &saved) != nil || saved.Version != 1 {
		if err := os.Rename(path, path+".unreadable"); err != nil {
			log.Warnf("%s is unreadable and couldn't be set aside: %v", filepath.Base(path), err)
		} else {
			log.Warnf("%s is unreadable; set aside as %s.unreadable", filepath.Base(path), filepath.Base(path))
		}
		return q
	}
	for _, c := range saved.Items {
		if c.Library == "" || c.ID == "" || len(q.items) >= maxItems {
			continue
		}
		q.items[c.key()] = c
	}
	return q
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Add merges a change into the item's pending one: the latest favorite and
// the latest rating win.
func (q *Queue) Add(c Change) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	cur, ok := q.items[c.key()]
	if !ok && len(q.items) >= maxItems {
		return ErrFull
	}
	if !ok {
		cur = Change{Library: c.Library, Kind: c.Kind, ID: c.ID}
	}
	if c.Favorite != nil {
		cur.Favorite = c.Favorite
	}
	if c.Rating != nil {
		cur.Rating = c.Rating
	}
	q.seq++
	cur.Tries, cur.seq = 0, q.seq
	q.items[c.key()] = cur
	q.saveLocked()
	return nil
}

// Flush sends the changes of the libraries online says are available and
// drops those of libraries that no longer exist. A failure leaves the rest
// of that library for the next flush. send runs without the lock.
func (q *Queue) Flush(ctx context.Context, online map[string]bool, send func(context.Context, Change) error) {
	if !q.flushing.CompareAndSwap(false, true) {
		return
	}
	defer q.flushing.Store(false)
	q.mu.Lock()
	var todo []Change
	for k, c := range q.items {
		available, known := online[c.Library]
		switch {
		case !known:
			delete(q.items, k)
		case available:
			todo = append(todo, c)
		}
	}
	q.mu.Unlock()

	failed := map[string]bool{}
	for _, c := range todo {
		if failed[c.Library] || ctx.Err() != nil {
			continue
		}
		err := send(ctx, c)
		q.mu.Lock()
		cur, ok := q.items[c.key()]
		switch {
		case !ok || cur.seq != c.seq:
			// Changed meanwhile: the newer change goes next time.
		case err == nil:
			delete(q.items, c.key())
		case errors.Is(err, ErrUnreachable):
		case cur.Tries+1 >= maxTries:
			log.Warnf("drop the %s change of %s %s after %d tries: %v", c.Kind, c.Library, c.ID, maxTries, err)
			delete(q.items, c.key())
		default:
			cur.Tries++
			q.items[c.key()] = cur
		}
		q.mu.Unlock()
		if err != nil {
			failed[c.Library] = true
			log.Infof("send the %s change of %s %s: %v", c.Kind, c.Library, c.ID, err)
		}
	}
	q.mu.Lock()
	q.saveLocked()
	q.mu.Unlock()
}

func (q *Queue) saveLocked() {
	if len(q.items) == 0 {
		if err := os.Remove(q.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Warnf("remove the pending marks: %v", err)
		}
		return
	}
	items := make([]Change, 0, len(q.items))
	for _, c := range q.items {
		items = append(items, c)
	}
	data, err := json.Marshal(map[string]any{"version": 1, "items": items})
	if err == nil {
		err = atomicfile.WriteFile(q.path, data, 0o600)
	}
	if err != nil {
		log.Warnf("save the pending marks: %v", err)
	}
}
