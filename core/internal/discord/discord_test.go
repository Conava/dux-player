package discord

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDiscord struct {
	handshakes atomic.Int32
	frames     chan map[string]any
}

// startFake serves discord-ipc-0 in a private XDG_RUNTIME_DIR. reject answers every handshake with a close.
func startFake(t *testing.T, reject bool) *fakeDiscord {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("TMPDIR", dir)
	ln, err := net.Listen("unix", filepath.Join(dir, "discord-ipc-0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	f := &fakeDiscord{frames: make(chan map[string]any, 16)}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn, reject)
		}
	}()
	return f
}

func (f *fakeDiscord) serve(conn net.Conn, reject bool) {
	defer conn.Close()
	op, body, err := readFrame(conn)
	if err != nil || op != opHandshake {
		return
	}
	f.handshakes.Add(1)
	var hs map[string]any
	if json.Unmarshal(body, &hs) != nil || hs["client_id"] != AppID {
		return
	}
	if reject {
		_ = writeFrame(conn, opClose, map[string]any{"code": 4000, "message": "Invalid Client ID"})
		return
	}
	_ = writeFrame(conn, opFrame, map[string]any{"cmd": "DISPATCH", "evt": "READY"})
	for {
		_, body, err := readFrame(conn)
		if err != nil {
			return
		}
		var m map[string]any
		if json.Unmarshal(body, &m) != nil {
			return
		}
		f.frames <- m
		_ = writeFrame(conn, opFrame, map[string]any{"cmd": m["cmd"], "nonce": m["nonce"]})
	}
}

func (f *fakeDiscord) next(t *testing.T) map[string]any {
	t.Helper()
	select {
	case m := <-f.frames:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("no frame reached the fake Discord")
		return nil
	}
}

func fastTimers(t *testing.T, bMin, bMax time.Duration) {
	oldMin, oldMax, oldInt := backoffMin, backoffMax, minInterval
	backoffMin, backoffMax, minInterval = bMin, bMax, 0
	t.Cleanup(func() { backoffMin, backoffMax, minInterval = oldMin, oldMax, oldInt })
}

func TestFrameCodec(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeFrame(&buf, opFrame, map[string]int{"v": 1}))
	op, body, err := readFrame(&buf)
	require.NoError(t, err)
	assert.Equal(t, uint32(opFrame), op)
	assert.JSONEq(t, `{"v":1}`, string(body))

	huge := make([]byte, 8+maxFrame+1)
	binary.LittleEndian.PutUint32(huge[4:], maxFrame+1)
	_, _, err = readFrame(bytes.NewReader(huge))
	assert.Error(t, err, "an oversized frame is refused before it is read")
}

func TestActivityReachesDiscord(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	long := strings.Repeat("x", 300)
	cases := []struct {
		name  string
		steps []*Activity // nil is Clear
		want  string      // the activity Discord gets for the last step
	}{
		{"full card", []*Activity{{Title: "Song", Artist: "Band", Album: "Record", Start: start}},
			`{"type":2,"details":"Song","state":"by Band","assets":{"large_image":"logo","large_text":"Record"},"timestamps":{"start":1700000000}}`},
		{"no artist, no album", []*Activity{{Title: "Song"}},
			`{"type":2,"details":"Song","assets":{"large_image":"logo"}}`},
		{"control characters and length", []*Activity{{Title: "So\x00ng\n", Artist: long}},
			`{"type":2,"details":"Song","state":"by ` + strings.Repeat("x", maxFieldLen-3) + `","assets":{"large_image":"logo"}}`},
		{"one-character title and album", []*Activity{{Title: "A", Album: "B"}},
			`{"type":2,"details":"A\u200b","assets":{"large_image":"logo"}}`},
		{"empty title after cleaning", []*Activity{{Title: "\x00\n", Artist: "Band"}},
			`{"type":2,"state":"by Band","assets":{"large_image":"logo"}}`},
		{"clear sends null", []*Activity{{Title: "Song"}, nil}, `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastTimers(t, 10*time.Millisecond, 50*time.Millisecond)
			f := startFake(t, false)
			c := New()
			defer c.Close()
			var got map[string]any
			for _, a := range tc.steps {
				if a == nil {
					c.Clear()
				} else {
					c.Set(*a)
				}
				got = f.next(t)
			}
			assert.Equal(t, int32(1), f.handshakes.Load())
			assert.Equal(t, "SET_ACTIVITY", got["cmd"])
			args := got["args"].(map[string]any)
			assert.NotZero(t, args["pid"])
			raw, err := json.Marshal(args["activity"])
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(raw))
			assert.NotContains(t, string(raw), "://")
		})
	}
}

func TestRejectedHandshakeBacksOffToMax(t *testing.T) {
	fastTimers(t, 10*time.Millisecond, 400*time.Millisecond)
	f := startFake(t, true)
	c := New()
	defer c.Close()
	c.Set(Activity{Title: "Song"})

	require.Eventually(t, func() bool { return f.handshakes.Load() == 1 }, 2*time.Second, 5*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, int32(1), f.handshakes.Load(), "no new attempt before the maximum backoff")
	require.Eventually(t, func() bool { return f.handshakes.Load() == 2 }, 2*time.Second, 5*time.Millisecond)
}

func TestCloseClearsAndReturns(t *testing.T) {
	fastTimers(t, 10*time.Millisecond, 50*time.Millisecond)
	f := startFake(t, false)
	c := New()
	c.Set(Activity{Title: "Song"})
	f.next(t)
	done := make(chan struct{})
	go func() { c.Close(); close(done) }()
	got := f.next(t)
	assert.Nil(t, got["args"].(map[string]any)["activity"])
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close didn't return")
	}
}
