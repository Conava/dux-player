package ipc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A listener gets the published values in order; one that blocks never
// holds up the publisher.
func TestBusListeners(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := NewBus()
	got := make(chan any, 10)
	b.On(ctx, func(_ string, v any) { got <- v }, "a")
	block := make(chan struct{})
	b.On(ctx, func(string, any) { <-block }, "a")

	published := make(chan struct{})
	go func() {
		for i := range 3 {
			b.Publish("a", i)
		}
		b.Publish("other", "ignored")
		close(published)
	}()
	select {
	case <-published:
	case <-time.After(5 * time.Second):
		t.Fatal("a blocked listener held up Publish")
	}
	close(block)
	for i := range 3 {
		select {
		case v := <-got:
			require.Equal(t, i, v)
		case <-time.After(5 * time.Second):
			t.Fatal("event missing")
		}
	}
	assert.Empty(t, got)
}
