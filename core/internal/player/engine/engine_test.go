package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// client talks to an engine playing to a recording Null output.
type client struct {
	t   *testing.T
	nc  net.Conn
	out *Null
	enc *json.Encoder

	mu      sync.Mutex
	id      int64
	replies map[int64]chan map[string]any
	events  []map[string]any
	done    chan struct{}
}

func newClient(t *testing.T) *client {
	a, b := net.Pipe()
	c := &client{t: t, nc: b, out: NewNull(true), enc: json.NewEncoder(b), replies: map[int64]chan map[string]any{},
		done: make(chan struct{})}
	go func() {
		defer close(c.done)
		_ = Serve(a, c.out, nil)
	}()
	go func() {
		sc := bufio.NewScanner(b)
		for sc.Scan() {
			var m map[string]any
			if json.Unmarshal(sc.Bytes(), &m) != nil {
				continue
			}
			c.mu.Lock()
			if id, ok := m["request_id"].(float64); ok {
				if ch := c.replies[int64(id)]; ch != nil {
					ch <- m
				}
			} else {
				c.events = append(c.events, m)
			}
			c.mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		b.Close()
		<-c.done
	})
	return c
}

// cmd sends a command and returns its reply's error field.
func (c *client) cmd(args ...any) string {
	c.t.Helper()
	c.mu.Lock()
	c.id++
	id := c.id
	ch := make(chan map[string]any, 1)
	c.replies[id] = ch
	c.mu.Unlock()
	require.NoError(c.t, c.enc.Encode(map[string]any{"command": args, "request_id": id}))
	select {
	case m := <-ch:
		return m["error"].(string)
	case <-time.After(5 * time.Second):
		c.t.Fatalf("no reply to %v", args)
		return ""
	}
}

// waitFor waits for an event and returns it.
func (c *client) waitFor(pred func(map[string]any) bool) map[string]any {
	c.t.Helper()
	var found map[string]any
	require.Eventually(c.t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, e := range c.events {
			if pred(e) {
				found = e
				return true
			}
		}
		return false
	}, 10*time.Second, 10*time.Millisecond)
	return found
}

// files lists the start-file and end-file events: "start 1", "end 1 eof".
func (c *client) files() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, e := range c.events {
		switch e["event"] {
		case "start-file":
			out = append(out, fmt.Sprintf("start %v", e["playlist_entry_id"]))
		case "end-file":
			out = append(out, fmt.Sprintf("end %v %v", e["playlist_entry_id"], e["reason"]))
		}
	}
	return out
}

func ended(id int) func(map[string]any) bool {
	return func(e map[string]any) bool {
		return e["event"] == "end-file" && e["playlist_entry_id"] == float64(id)
	}
}

// Two files join on one stream when their formats match and gapless is on;
// otherwise the first plays out and a new stream opens. No frame is lost.
func TestGapless(t *testing.T) {
	for _, tt := range []struct {
		name          string
		rateA, rateB  int
		gapless       string
		wantStreams   int
		wantFramesOut int
	}{
		{"same format", 44100, 44100, "weak", 1, 2 * 4410},
		{"rate changes", 44100, 48000, "weak", 2, 4410 + 4800},
		{"gapless off", 44100, 44100, "no", 2, 2 * 4410},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			a, b := filepath.Join(dir, "a.wav"), filepath.Join(dir, "b.wav")
			writeWAV(t, a, tt.rateA, 1, 0.1, 0.5)
			writeWAV(t, b, tt.rateB, 1, 0.1, 0.5)
			c := newClient(t)
			require.Equal(t, "success", c.cmd("set_property", "gapless-audio", tt.gapless))
			require.Equal(t, "success", c.cmd("loadfile", a, "replace", "-1"))
			require.Equal(t, "success", c.cmd("loadfile", b, "append", "-1"))
			c.waitFor(ended(2))
			assert.Equal(t, []string{"start 1", "end 1 eof", "start 2", "end 2 eof"}, c.files())
			frames, streams := c.out.Frames()
			assert.Equal(t, tt.wantStreams, streams)
			assert.Len(t, frames, tt.wantFramesOut)
		})
	}
}

// What is heard is ReplayGain times mpv's cubic volume curve; mute is silence.
func TestGain(t *testing.T) {
	for _, tt := range []struct {
		name  string
		props [][2]any
		opts  string
		want  float64
	}{
		{"untouched", nil, "", 1},
		{"replaygain fallback", [][2]any{{"replaygain", "track"}}, "replaygain-fallback=-6.02", 0.5},
		{"fallback without replaygain", nil, "replaygain-fallback=-6.02", 1},
		{"volume 50", [][2]any{{"volume", 50}}, "", 0.125},
		{"muted", [][2]any{{"mute", true}}, "", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "a.wav")
			writeWAV(t, path, 8000, 1, 0.05, 1)
			c := newClient(t)
			for _, p := range tt.props {
				require.Equal(t, "success", c.cmd("set_property", p[0], p[1]))
			}
			require.Equal(t, "success", c.cmd("loadfile", path, "replace", "-1", tt.opts))
			c.waitFor(ended(1))
			frames, _ := c.out.Frames()
			require.NotEmpty(t, frames)
			peak := 0.0
			for _, v := range frames {
				peak = math.Max(peak, math.Abs(float64(v)))
			}
			assert.InDelta(t, tt.want, peak, 0.002)
		})
	}
}

// Broken or foreign files end with an error and the engine keeps serving.
func TestHostileInput(t *testing.T) {
	dir := t.TempDir()
	flacData, err := os.ReadFile(filepath.Join("testdata", "tone.flac"))
	require.NoError(t, err)
	wav := filepath.Join(dir, "long.wav")
	writeWAV(t, wav, 8000, 1, 0.05, 0.5)
	wavData, err := os.ReadFile(wav)
	require.NoError(t, err)
	// data chunk says far more than the file holds
	wavData[40], wavData[41], wavData[42], wavData[43] = 0xff, 0xff, 0xff, 0x7f
	random := make([]byte, 4096)
	for i := range random {
		random[i] = byte(i*131 + i/7)
	}
	for _, tt := range []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{"truncated flac", flacData[:len(flacData)/3], ""},
		{"flac header only", flacData[:100], ""},
		{"oversized wav chunk", wavData, ""},
		{"random bytes", random, NeedsMPV},
		{"empty", nil, errBadFile.Error()},
		{"mp4", []byte("\x00\x00\x00\x20ftypM4A \x00\x00\x00\x00"), NeedsMPV},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name)
			require.NoError(t, os.WriteFile(path, tt.data, 0o644))
			c := newClient(t)
			require.Equal(t, "success", c.cmd("loadfile", path, "replace", "-1"))
			end := c.waitFor(ended(1))
			if tt.wantErr != "" {
				assert.Equal(t, "error", end["reason"])
				assert.Equal(t, tt.wantErr, end["file_error"])
			}
			assert.Equal(t, "success", c.cmd("get_property", "mpv-version"), "engine still answers")
		})
	}
}

// Speed and filters are mpv's; the engine refuses anything but the
// neutral value, which the player logs and survives.
func TestUnsupportedValues(t *testing.T) {
	c := newClient(t)
	assert.Equal(t, "success", c.cmd("set_property", "speed", 1))
	assert.NotEqual(t, "success", c.cmd("set_property", "speed", 1.5))
	assert.Equal(t, "success", c.cmd("set_property", "af", ""))
	assert.NotEqual(t, "success", c.cmd("set_property", "af", "lavfi=[afade=t=out:st=1:d=1]"))
}

// A seek lands where asked, and repeat-one plays the file again without
// ending it.
func TestSeekAndLoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.wav")
	writeWAV(t, path, 8000, 1, 2, 0.5)
	c := newClient(t)
	require.Equal(t, "success", c.cmd("observe_property", 1, "time-pos"))
	require.Equal(t, "success", c.cmd("set_property", "pause", true))
	require.Equal(t, "success", c.cmd("loadfile", path, "replace", "-1"))
	c.waitFor(func(e map[string]any) bool { return e["event"] == "file-loaded" })
	require.Equal(t, "success", c.cmd("seek", 1.5, "absolute"))
	c.waitFor(func(e map[string]any) bool {
		return e["event"] == "property-change" && e["name"] == "time-pos" && e["data"] == 1.5
	})
	require.Equal(t, "success", c.cmd("set_property", "loop-file", "inf"))
	require.Equal(t, "success", c.cmd("set_property", "pause", false))
	time.Sleep(1 * time.Second)
	frames, _ := c.out.Frames()
	assert.Greater(t, len(frames), 4000, "played past the end of the file")
	assert.Equal(t, []string{"start 1"}, c.files(), "a looped file doesn't end")
}
