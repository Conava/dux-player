package cache

import (
	"context"
	"sync"
)

// Burst runs one job per key at a time: triggers during a run add one more
// run, not one each, so a check that finds many changes costs one or two.
type Burst struct {
	Run func(ctx context.Context, key string)
	// Spawn starts a run in the background, false when it can't.
	Spawn func(f func()) bool
	mu    sync.Mutex
	// again holds the running keys, true when one more run is due.
	again map[string]bool
}

func (b *Burst) Trigger(ctx context.Context, key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, running := b.again[key]; running {
		b.again[key] = true
		return
	}
	if b.again == nil {
		b.again = map[string]bool{}
	}
	b.again[key] = false
	started := b.Spawn(func() {
		for {
			b.Run(ctx, key)
			b.mu.Lock()
			if !b.again[key] {
				delete(b.again, key)
				b.mu.Unlock()
				return
			}
			b.again[key] = false
			b.mu.Unlock()
		}
	})
	if !started {
		delete(b.again, key)
	}
}
