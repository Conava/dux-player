package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"math"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Writes on one connection take effect in the order they were sent, a read
// sees the writes sent before it, and a slow read holds up nothing.
func TestRequestsOfOneConnectionKeepTheirOrder(t *testing.T) {
	dir, err := os.MkdirTemp("", "dpo")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)

	var mu sync.Mutex
	var log []string
	r := NewRegistry()
	r.Group("t", func(_ context.Context, w *ConnWriter, req Request) {
		switch req.Method {
		case "t.slowWrite":
			time.Sleep(100 * time.Millisecond)
		case "t.slowRead":
			time.Sleep(400 * time.Millisecond)
		}
		mu.Lock()
		if req.Method == "t.slowWrite" || req.Method == "t.write" {
			log = append(log, req.Method)
		}
		seen := append([]string{}, log...)
		mu.Unlock()
		w.Respond(req.ID, seen)
	},
		MethodSpec{Name: "t.slowWrite"},
		MethodSpec{Name: "t.write"},
		MethodSpec{Name: "t.read", Read: true},
		MethodSpec{Name: "t.slowRead", Read: true},
	)
	srv := NewServer(r, NewBus())
	require.NoError(t, srv.Listen())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(ctx) }()

	c, err := net.Dial("unix", srv.SocketPath())
	require.NoError(t, err)
	defer c.Close()
	rd := bufio.NewScanner(c)
	require.True(t, rd.Scan(), "handshake")
	for i, m := range []string{"t.slowWrite", "t.slowRead", "t.write", "t.read"} {
		line, _ := json.Marshal(Request{ID: i + 1, Method: m})
		_, err := c.Write(append(line, '\n'))
		require.NoError(t, err)
	}
	var order []int
	replies := map[int][]string{}
	for range 4 {
		require.True(t, rd.Scan())
		var resp struct {
			ID     int      `json:"id"`
			Result []string `json:"result"`
		}
		require.NoError(t, json.Unmarshal(rd.Bytes(), &resp))
		order = append(order, resp.ID)
		replies[resp.ID] = resp.Result
	}
	assert.Equal(t, []string{"t.slowWrite", "t.write"}, replies[3], "writes apply in send order")
	assert.Equal(t, []string{"t.slowWrite", "t.write"}, replies[4], "a read sees the writes sent before it")
	assert.Equal(t, 2, order[len(order)-1], "the slow read finishes last; nothing waited for it")
}

// A client that stops reading must not hold up whoever publishes: the
// player loop publishes from its own goroutine.
func TestStalledSubscriberDoesNotBlockPublish(t *testing.T) {
	dir, err := os.MkdirTemp("", "dps")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)

	r := NewRegistry()
	r.Topics("big")
	bus := NewBus()
	srv := NewServer(r, bus)
	require.NoError(t, srv.Listen())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(ctx) }()

	c, err := net.Dial("unix", srv.SocketPath())
	require.NoError(t, err)
	defer c.Close()
	rd := bufio.NewScanner(c)
	require.True(t, rd.Scan(), "handshake")
	_, err = c.Write([]byte(`{"id":1,"method":"subscribe","params":{"topics":"big"}}` + "\n"))
	require.NoError(t, err)
	require.True(t, rd.Scan(), "subscribe reply")

	// The client reads nothing from here on.
	payload := make([]byte, 1<<20)
	for i := range payload {
		payload[i] = 'x'
	}
	done := make(chan struct{})
	go func() {
		for range 8 {
			bus.Publish("big", string(payload))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Publish blocked on a client that doesn't read")
	}
}

// A reply that can't be encoded (NaN from a server) still answers the
// request instead of leaving the client waiting.
func TestUnencodableReplyIsAnError(t *testing.T) {
	dir, err := os.MkdirTemp("", "dpn")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)

	r := NewRegistry()
	r.Group("t", func(_ context.Context, w *ConnWriter, req Request) { w.Respond(req.ID, math.NaN()) },
		MethodSpec{Name: "t.nan"})
	srv := NewServer(r, NewBus())
	require.NoError(t, srv.Listen())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(ctx) }()

	cl, err := Dial(srv.SocketPath())
	require.NoError(t, err)
	defer cl.Close()
	resp, err := cl.Call(Request{ID: 3, Method: "t.nan"})
	require.NoError(t, err)
	assert.Equal(t, 3, resp.ID)
	assert.NotEmpty(t, resp.Error)
}
