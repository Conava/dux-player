package scrobble

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/atomicfile"
)

// maxQueued bounds the offline queue; the oldest plays go first.
const maxQueued = 1000

// Listen is one play, with what the services need to identify it.
type Listen struct {
	Library  string    `json:"library"`
	ID       string    `json:"id"`
	Artist   string    `json:"artist"`
	Title    string    `json:"title"`
	Album    string    `json:"album,omitempty"`
	Duration float64   `json:"duration,omitempty"`
	PlayedAt time.Time `json:"playedAt"`
}

// pending is a play that couldn't be sent. Target is a service name, or
// targetLibrary for a play of a server track.
type pending struct {
	Target string `json:"target"`
	Listen
}

const targetLibrary = "library"

// queue is the persisted list of plays waiting to be sent.
type queue struct {
	path  string
	mu    sync.Mutex
	items []pending
}

func openQueue(path string) *queue {
	q := &queue{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		return q
	}
	var saved struct {
		Version int       `json:"version"`
		Items   []pending `json:"items"`
	}
	if json.Unmarshal(data, &saved) != nil || saved.Version != 1 {
		setAside(path)
		return q
	}
	q.items = saved.Items
	return q
}

// setAside keeps a file this version can't read next to it instead of
// overwriting it.
func setAside(path string) {
	if err := os.Rename(path, path+".unreadable"); err != nil {
		log.Warnf("%s is unreadable and couldn't be set aside: %v", filepath.Base(path), err)
		return
	}
	log.Warnf("%s is unreadable; kept as %s.unreadable", filepath.Base(path), filepath.Base(path))
}

func (q *queue) add(p pending) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, p)
	if over := len(q.items) - maxQueued; over > 0 {
		q.items = q.items[over:]
	}
	q.saveLocked()
}

func (q *queue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

func (q *queue) snapshot() []pending {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]pending(nil), q.items...)
}

// remove drops the given entries (compared by value) and saves.
func (q *queue) remove(done []pending) {
	if len(done) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	gone := map[pending]int{}
	for _, d := range done {
		gone[d]++
	}
	kept := q.items[:0]
	for _, it := range q.items {
		if gone[it] > 0 {
			gone[it]--
			continue
		}
		kept = append(kept, it)
	}
	q.items = kept
	q.saveLocked()
}

func (q *queue) saveLocked() {
	if len(q.items) == 0 {
		_ = os.Remove(q.path)
		return
	}
	data, err := json.Marshal(map[string]any{"version": 1, "items": q.items})
	if err == nil {
		err = atomicfile.WriteFile(q.path, data, 0o600)
	}
	if err != nil {
		log.Warnf("save the scrobble queue: %v", err)
	}
}
