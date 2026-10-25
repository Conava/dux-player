package cast

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

// The test binary doubles as a fake cast-helper: with DPLAYER_FAKE_CAST
// set it speaks the protocol on stdin/stdout and logs each command to that
// file. DPLAYER_FAKE_CAST_PROTOCOL sets what check reports.
func TestMain(m *testing.M) {
	if log := os.Getenv("DPLAYER_FAKE_CAST"); log != "" {
		fakeHelper(log)
		return
	}
	os.Exit(m.Run())
}

func fakeHelper(logPath string) {
	logf, _ := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	defer logf.Close()
	out := json.NewEncoder(os.Stdout)
	devices := []map[string]any{{"id": "tv1", "name": "Living room"}}
	state := func(sess any) { _ = out.Encode(map[string]any{"event": "state", "devices": devices, "session": sess}) }
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var cmd map[string]any
		if json.Unmarshal(sc.Bytes(), &cmd) != nil {
			continue
		}
		fmt.Fprintln(logf, sc.Text())
		switch cmd["cmd"] {
		case "check":
			proto := 2
			fmt.Sscan(os.Getenv("DPLAYER_FAKE_CAST_PROTOCOL"), &proto)
			_ = out.Encode(map[string]any{"event": "check", "protocol": proto, "version": "0.1.0"})
		case "discover":
			state(nil)
		case "queue":
			state(map[string]any{"kind": "media", "device": cmd["device"], "phase": "casting", "playerState": "PLAYING",
				"position": 3, "positionAt": time.Now().UnixMilli(), "duration": 200, "index": 1, "volume": 0.5, "canVolume": true,
				"owner": map[string]any{"id": "dplayer", "name": "Dux Player"}})
		case "stop":
			state(nil)
		}
	}
}

type fakePlayer struct {
	mu      sync.Mutex
	remote  player.Remote
	items   []player.Item
	updates []uint64
	detach  int
}

func (p *fakePlayer) SetRemote(_ context.Context, r player.Remote, _ bool) error {
	p.mu.Lock()
	p.remote = r
	if r == nil {
		p.detach++
	}
	items := p.items
	p.mu.Unlock()
	if r != nil {
		r.Load(items, 0, 0)
	}
	return nil
}

func (p *fakePlayer) RemoteUpdate(_ context.Context, uid uint64, _ float64, _ bool, _ float64) error {
	p.mu.Lock()
	p.updates = append(p.updates, uid)
	p.mu.Unlock()
	return nil
}

func (p *fakePlayer) seen() (updates []uint64, detach int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]uint64(nil), p.updates...), p.detach
}

type fakeLibs struct{}

func (fakeLibs) Get(string) (source.Source, error) { return nil, errors.New("no covers here") }
func (fakeLibs) Stream(_ context.Context, ref source.Ref) (player.Stream, error) {
	switch ref.Library {
	case "srv":
		return player.Stream{URL: "https://server/rest/stream?id=" + ref.ID + "&t=token"}, nil
	case "plain":
		return player.Stream{URL: "http://server.example.com/rest/stream?id=" + ref.ID + "&t=token"}, nil
	case "lan":
		return player.Stream{URL: "http://192.168.1.5:4533/rest/stream?id=" + ref.ID + "&t=token"}, nil
	}
	return player.Stream{URL: "/music/" + ref.ID + ".flac"}, nil
}

func newBridge(t *testing.T, protocol string) (*Bridge, *fakePlayer, func() []map[string]any) {
	logPath := t.TempDir() + "/commands"
	t.Setenv("DPLAYER_FAKE_CAST", logPath)
	t.Setenv("DPLAYER_FAKE_CAST_PROTOCOL", protocol)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pl := &fakePlayer{items: []player.Item{
		{UID: 7, Track: source.Track{Library: "loc", ID: "a", Title: "A", Artist: "X", Album: "Y"}},
		{UID: 8, Track: source.Track{Library: "srv", ID: "b", Title: "B"}},
	}}
	exe, err := os.Executable()
	require.NoError(t, err)
	b := New(Config{Ctx: ctx, Player: pl, Libraries: fakeLibs{}, Publish: func(string, any) {},
		HasSubscriber: func(string) bool { return false }, Find: func() string { return exe }})
	commands := func() []map[string]any {
		data, _ := os.ReadFile(logPath)
		var out []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			var c map[string]any
			if json.Unmarshal([]byte(line), &c) == nil {
				out = append(out, c)
			}
		}
		return out
	}
	return b, pl, commands
}

func TestCastPlaysTheQueueAndFollowsTheSession(t *testing.T) {
	b, pl, commands := newBridge(t, "2")
	require.NoError(t, b.Play(context.Background(), "tv1"))

	// The session reports index 1: the player follows to the second item.
	require.Eventually(t, func() bool { u, _ := pl.seen(); return len(u) > 0 && u[len(u)-1] == 8 }, 5*time.Second, 10*time.Millisecond)
	cmds := commands()
	require.GreaterOrEqual(t, len(cmds), 3)
	assert.Equal(t, map[string]any{"cmd": "hello", "client": "dplayer", "name": "Dux Player"}, cmds[0])
	assert.Equal(t, "check", cmds[1]["cmd"])
	q := cmds[2]
	assert.Equal(t, "queue", q["cmd"])
	assert.Equal(t, "tv1", q["device"])
	items := q["items"].([]any)
	assert.Equal(t, map[string]any{"path": "/music/a.flac", "title": "A", "artist": "X", "album": "Y"}, items[0])
	assert.Contains(t, items[1].(map[string]any)["url"], "https://server/")
	assert.True(t, b.Status().Casting)

	b.SetPaused(true)
	b.Seek(42)
	require.NoError(t, b.StopCast(context.Background(), true))
	_, detach := pl.seen()
	assert.Equal(t, 1, detach, "the player is local again")
	require.Eventually(t, func() bool {
		c := commands()
		return len(c) >= 6 && c[len(c)-1]["cmd"] == "stop"
	}, 5*time.Second, 10*time.Millisecond)
	c := commands()
	assert.Equal(t, []any{"pause", "seek", "stop"}, []any{c[3]["cmd"], c[4]["cmd"], c[5]["cmd"]})

	// Nothing needs the helper any more: no idle child.
	b.mu.Lock()
	defer b.mu.Unlock()
	assert.Nil(t, b.proc)
}

func TestCastNeedsProtocol2(t *testing.T) {
	b, pl, _ := newBridge(t, "1")
	assert.ErrorIs(t, b.Play(context.Background(), "tv1"), ErrUpdate)
	assert.True(t, b.Status().UpdateNeeded)
	pl.mu.Lock()
	defer pl.mu.Unlock()
	assert.Nil(t, pl.remote)
}

func TestCastEndsWhenAnotherClientTakesOver(t *testing.T) {
	b, pl, _ := newBridge(t, "2")
	require.NoError(t, b.Play(context.Background(), "tv1"))
	require.Eventually(t, func() bool { u, _ := pl.seen(); return len(u) > 0 }, 5*time.Second, 10*time.Millisecond)
	b.mu.Lock()
	p := b.proc
	b.mu.Unlock()
	b.onEvent(p, event{Event: "state", Session: &session{Device: "tv1", Owner: &owner{ID: "dms", Name: "Dux Shell"}}})
	_, detach := pl.seen()
	assert.Equal(t, 1, detach)
	assert.False(t, b.Status().Casting)
}

func lastCmd(commands func() []map[string]any) string {
	c := commands()
	if len(c) == 0 {
		return ""
	}
	return fmt.Sprint(c[len(c)-1]["cmd"])
}

// "Play next" while casting reaches the receiver: at once with edit, at its
// next track change without.
func TestCastQueueEdits(t *testing.T) {
	for _, tc := range []struct{ protocol, want string }{{"3", "edit"}, {"2", "queue"}} {
		b, pl, commands := newBridge(t, tc.protocol)
		require.NoError(t, b.Play(context.Background(), "tv1"))
		require.Eventually(t, func() bool { u, _ := pl.seen(); return len(u) > 0 }, 5*time.Second, 10*time.Millisecond)
		n := len(commands())
		items := append(slices.Clone(pl.items), player.Item{UID: 9, Track: source.Track{Library: "loc", ID: "c"}})
		b.Load(items, 1, -1)
		if tc.protocol == "2" {
			time.Sleep(50 * time.Millisecond)
			assert.Len(t, commands(), n, "without edit the change waits for the next track")
			b.mu.Lock()
			p := b.proc
			b.mu.Unlock()
			b.onEvent(p, event{Event: "state", Session: &session{Device: "tv1", Owner: &owner{ID: clientID}, Index: 0}})
		}
		require.Eventually(t, func() bool { return lastCmd(commands) == tc.want }, 5*time.Second, 10*time.Millisecond, tc.protocol)
		c := commands()
		got := c[len(c)-1]["items"].([]any)
		assert.Len(t, got, 2, "the window from the playing item on")
	}
}

// Stream URLs carry credentials; a plain-http server's never go to the receiver.
func TestCastRefusesPlainHTTP(t *testing.T) {
	b, pl, commands := newBridge(t, "2")
	pl.items = []player.Item{{UID: 1, Track: source.Track{Library: "plain", ID: "x"}}}
	require.NoError(t, b.Play(context.Background(), "tv1"))
	require.Eventually(t, func() bool { return !b.Status().Casting }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, errInsecure, b.Status().Error.Code)
	for _, c := range commands() {
		assert.NotEqual(t, "queue", c["cmd"])
	}
}

func TestHomeNetwork(t *testing.T) {
	for raw, want := range map[string]bool{
		"http://192.168.1.5:4533/x": true, "http://10.0.0.2/x": true, "http://172.16.3.1/x": true,
		"http://169.254.1.1/x": true, "http://127.0.0.1/x": true, "http://[fd00::1]/x": true, "http://[fe80::1]/x": true,
		"http://nas.local:4533/x": true, "http://localhost/x": true,
		"http://93.184.216.34/x": false, "http://music.example.com/x": false, "http://nas/x": false, "https://192.168.1.5/x": false,
	} {
		assert.Equal(t, want, homeNetwork(raw), raw)
	}
}

// A plain-http server on the home network is cast; repeat all sends the
// whole queue with loop, starting at the playing item.
func TestCastHomeServerWithRepeatAll(t *testing.T) {
	b, pl, commands := newBridge(t, "2")
	pl.items = []player.Item{{UID: 1, Track: source.Track{Library: "lan", ID: "x"}}, {UID: 2, Track: source.Track{Library: "lan", ID: "y"}}}
	b.SetLoop(true)
	require.NoError(t, b.Play(context.Background(), "tv1"))
	require.Eventually(t, func() bool { return lastCmd(commands) == "queue" }, 5*time.Second, 10*time.Millisecond)
	c := commands()
	q := c[len(c)-1]
	assert.Equal(t, true, q["loop"])
	assert.Len(t, q["items"], 2)
	assert.Nil(t, b.Status().Error)
}

// One position ticker, and only while the cast plays.
func TestCastTicksOnlyWhilePlaying(t *testing.T) {
	b, pl, _ := newBridge(t, "2")
	require.NoError(t, b.Play(context.Background(), "tv1"))
	require.Eventually(t, func() bool { u, _ := pl.seen(); return len(u) > 0 }, 5*time.Second, 10*time.Millisecond)
	ticking := func() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.tickStop != nil }
	assert.True(t, ticking())
	b.mu.Lock()
	p := b.proc
	b.mu.Unlock()
	b.onEvent(p, event{Event: "state", Session: &session{Device: "tv1", Owner: &owner{ID: clientID}, Index: 1, PlayerState: "PAUSED", Paused: true}})
	assert.False(t, ticking())
	require.NoError(t, b.StopCast(context.Background(), false))
	assert.False(t, ticking())
}

// A helper that dies while the device list shows comes back.
func TestDiscoveryRestartsTheHelper(t *testing.T) {
	b, _, _ := newBridge(t, "2")
	b.cfg.HasSubscriber = func(string) bool { return true }
	b.Subscribed()
	b.mu.Lock()
	p := b.proc
	b.mu.Unlock()
	require.NotNil(t, p)
	p.close()
	require.Eventually(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.proc != nil && b.proc != p }, 2*watchEvery, 50*time.Millisecond)
}
