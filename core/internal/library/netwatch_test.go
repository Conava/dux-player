package library

import (
	"context"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// godbus closes the signal channel when the bus goes away; the watcher must
// end instead of spinning on it.
func TestNetworkWatchEndsWithTheBus(t *testing.T) {
	signals := make(chan *dbus.Signal)
	close(signals)
	done := make(chan struct{})
	go func() {
		watch(context.Background(), signals, func() {})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the watcher kept running on a closed channel")
	}
}
