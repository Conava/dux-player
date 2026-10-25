package ipc

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// maxQueued is how much a client may fall behind before it is dropped.
	// A queue snapshot of a few thousand tracks is a few MiB.
	maxQueued    = 32 << 20
	writeTimeout = 30 * time.Second
)

// newQueuedConn sends replies and events from a goroutine of its own, so a
// client that stops reading never blocks the publisher (the player loop) or
// other clients. One that falls maxQueued behind is disconnected.
func newQueuedConn(c net.Conn) *queuedConn {
	q := &queuedConn{Conn: c, wake: make(chan struct{}, 1)}
	go q.run()
	return q
}

type queuedConn struct {
	net.Conn
	mu     sync.Mutex
	out    [][]byte
	size   int
	closed bool
	wake   chan struct{}
}

var errClientBehind = errors.New("client stopped reading; disconnecting it")

func (q *queuedConn) Write(b []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return 0, net.ErrClosed
	}
	if q.size+len(b) > maxQueued {
		return 0, errClientBehind
	}
	q.out = append(q.out, append([]byte(nil), b...))
	q.size += len(b)
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return len(b), nil
}

// The writer goroutine sets its own deadlines.
func (q *queuedConn) SetWriteDeadline(time.Time) error { return nil }

func (q *queuedConn) Close() error {
	q.mu.Lock()
	if !q.closed {
		q.closed = true
		close(q.wake)
	}
	q.mu.Unlock()
	return q.Conn.Close()
}

// replyConn notes whether a handler wrote its reply: dux-go's ConnWriter
// only logs a reply it can't encode, which would leave the client waiting.
type replyConn struct {
	net.Conn
	wrote atomic.Bool
}

func (r *replyConn) Write(b []byte) (int, error) {
	r.wrote.Store(true)
	return r.Conn.Write(b)
}

func (q *queuedConn) run() {
	for range q.wake {
		q.mu.Lock()
		out := q.out
		q.out, q.size = nil, 0
		q.mu.Unlock()
		for _, b := range out {
			_ = q.Conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if _, err := q.Conn.Write(b); err != nil {
				_ = q.Close()
				return
			}
		}
	}
}
