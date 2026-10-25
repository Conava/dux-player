package player

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

// With FAKE_MPV_STALL_MS set, the test binary acts as mpv: it answers on
// its --input-ipc-server socket, observe_property at once and everything
// else only after the stall, like an mpv whose core is busy starting.
func TestMain(m *testing.M) {
	if ms := os.Getenv("FAKE_MPV_STALL_MS"); ms != "" {
		fakeMPV(ms)
		return
	}
	// Keeps the tests and their mpv off the host's PipeWire.
	os.Setenv("PIPEWIRE_REMOTE", filepath.Join(os.TempDir(), "dplayer-test-no-pipewire"))
	os.Exit(m.Run())
}

func fakeMPV(stallMS string) {
	n, _ := strconv.Atoi(stallMS)
	ready := time.Now().Add(time.Duration(n) * time.Millisecond)
	var socket string
	for _, a := range os.Args[1:] {
		if v, ok := strings.CutPrefix(a, "--input-ipc-server="); ok {
			socket = v
		}
	}
	l, err := net.Listen("unix", socket)
	if err != nil {
		os.Exit(2)
	}
	c, err := l.Accept()
	if err != nil {
		os.Exit(2)
	}
	enc := json.NewEncoder(c)
	sc := bufio.NewScanner(c)
	entry := 0
	for sc.Scan() {
		var req struct {
			Command   []any `json:"command"`
			RequestID int64 `json:"request_id"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil || len(req.Command) == 0 {
			continue
		}
		reply := map[string]any{"request_id": req.RequestID, "error": "success"}
		switch req.Command[0] {
		case "observe_property":
		case "loadfile":
			time.Sleep(time.Until(ready))
			entry++
			reply["data"] = map[string]int{"playlist_entry_id": entry}
		default:
			time.Sleep(time.Until(ready))
		}
		_ = enc.Encode(reply)
	}
}

// The first play right after mpv starts must wait for a core that is slow
// to come up instead of failing on the per-command timeout.
func TestFirstPlayWaitsForSlowMPVStart(t *testing.T) {
	old := commandTimeout
	commandTimeout = 200 * time.Millisecond
	t.Cleanup(func() { commandTimeout = old })
	t.Setenv("FAKE_MPV_STALL_MS", "2000")

	dir := t.TempDir()
	p := New(Config{
		Binary:   os.Args[0],
		Socket:   filepath.Join(dir, "mpv.sock"),
		Resolver: pathResolver{"a": filepath.Join(dir, "a.wav")},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	require.NoError(t, p.Play(ctx, []source.Track{{Library: "l", ID: "a", Title: "a"}}, 0, Origin{Kind: "track"}))
	st, err := p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, StatePlaying, st.State)
}
