package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A check that finds a hundred changes starts one resync, plus one for what
// came during it; a change is never left without a run after it.
func TestBurstRunsOncePerBurst(t *testing.T) {
	var runs, other atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	b := burst{run: func(_ context.Context, key string) {
		if key == "other" {
			other.Add(1)
			return
		}
		runs.Add(1)
		entered <- struct{}{}
		<-release
	}}
	ctx := context.Background()
	b.trigger(ctx, "lib")
	<-entered
	for range 100 {
		b.trigger(ctx, "lib")
	}
	b.trigger(ctx, "other")
	require.Eventually(t, func() bool { return other.Load() == 1 }, time.Second, time.Millisecond, "keys don't wait for each other")
	close(release)

	idle := func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return len(b.again) == 0
	}
	require.Eventually(t, idle, time.Second, time.Millisecond)
	assert.Equal(t, int32(2), runs.Load())

	b.trigger(ctx, "lib")
	require.Eventually(t, func() bool { return runs.Load() == 3 }, time.Second, time.Millisecond, "a later change runs again")
}
