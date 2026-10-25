package ipc

import (
	"context"
	"sync"

	duxipc "github.com/duxlabs-dev/dux-go/ipc"
	"github.com/duxlabs-dev/dux-go/log"
)

// Bus is dux-go's event bus plus listeners inside the daemon, so one
// feature can follow another's events as typed values.
type Bus struct {
	*duxipc.EventBus
	mu sync.RWMutex
	on map[string][]*listener
}

func NewBus() *Bus { return &Bus{EventBus: duxipc.NewEventBus(), on: map[string][]*listener{}} }

func (b *Bus) Publish(topic string, data any) {
	b.EventBus.Publish(topic, data)
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, l := range b.on[topic] {
		l.push(event{topic, data})
	}
}

// On calls fn with every later event on topics, in order, on a goroutine of
// its own: a slow fn delays only its own events, never the publisher. data
// is the published value, shared with every listener; fn must not change
// it. Listening stops when ctx ends.
func (b *Bus) On(ctx context.Context, fn func(topic string, data any), topics ...string) {
	l := &listener{wake: make(chan struct{}, 1)}
	b.mu.Lock()
	for _, t := range topics {
		b.on[t] = append(b.on[t], l)
	}
	b.mu.Unlock()
	go l.run(ctx, fn)
}

type event struct {
	topic string
	data  any
}

// maxPending is how far a listener may fall behind before its events are
// dropped instead of piling up.
const maxPending = 1024

type listener struct {
	mu       sync.Mutex
	pending  []event
	dropping bool
	wake     chan struct{}
}

func (l *listener) push(ev event) {
	l.mu.Lock()
	if len(l.pending) < maxPending {
		l.pending = append(l.pending, ev)
	} else if !l.dropping {
		l.dropping = true
		log.Warnf("an event listener fell %d events behind; dropping %s events until it catches up", maxPending, ev.topic)
	}
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *listener) run(ctx context.Context, fn func(string, any)) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.wake:
		}
		l.mu.Lock()
		evs := l.pending
		l.pending, l.dropping = nil, false
		l.mu.Unlock()
		for _, ev := range evs {
			fn(ev.topic, ev.data)
		}
	}
}
