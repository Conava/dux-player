package cast

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
			proto := 3
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
	ranOut  int
	// final is what Load reports: the queue ends at a stop rule.
	final bool
	// decline: RemoteRanOut leaves the cast alone, as the player does when
	// no stop rule ends there. ranUID is the entry it was told about.
	decline bool
	ranUID  uint64
	// current is the entry RemoteUpdate last named; a run-out for another one is declined.
	current uint64
}

func (p *fakePlayer) SetRemote(_ context.Context, r player.Remote, _ bool) error {
	p.mu.Lock()
	p.remote = r
	if r == nil {
		p.detach++
	}
	items, final := p.items, p.final
	p.mu.Unlock()
	if r != nil {
		r.Load(items, 0, 0, final)
	}
	return nil
}

func (p *fakePlayer) RemoteRanOut(_ context.Context, uid uint64) error {
	p.mu.Lock()
	p.ranOut++
	p.ranUID = uid
	r := p.remote
	decline := p.decline || uid != p.current
	if !decline {
		p.remote = nil
	}
	p.mu.Unlock()
	if r != nil && !decline {
		r.Stop()
	}
	return nil
}

func (p *fakePlayer) RemoteUpdate(_ context.Context, uid uint64, _ float64, _ bool, _ float64) error {
	p.mu.Lock()
	p.updates = append(p.updates, uid)
	p.current = uid
	p.mu.Unlock()
	return nil
}

func (p *fakePlayer) seen() (updates []uint64, detach int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]uint64(nil), p.updates...), p.detach
}

// salt makes every server URL new, as token authentication does.
var salt atomic.Int64

type fakeLibs struct{}

func (fakeLibs) Get(string) (source.Source, error) { return nil, errors.New("no covers here") }
func (fakeLibs) Stream(_ context.Context, ref source.Ref) (player.Stream, error) {
	switch ref.Library {
	case "srv":
		return player.Stream{URL: fmt.Sprintf("https://server/rest/stream?id=%s&t=token&s=%d", ref.ID, salt.Add(1))}, nil
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
	b, pl, commands := newBridge(t, "3")
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

func TestCastNeedsProtocol3(t *testing.T) {
	b, pl, _ := newBridge(t, "2")
	assert.ErrorIs(t, b.Play(context.Background(), "tv1"), ErrUpdate)
	assert.True(t, b.Status().UpdateNeeded)
	pl.mu.Lock()
	defer pl.mu.Unlock()
	assert.Nil(t, pl.remote)
}

func TestCastEndsWhenAnotherClientTakesOver(t *testing.T) {
	b, pl, _ := newBridge(t, "3")
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

// "Play next" while casting reaches the receiver at once, as an edit.
func TestCastQueueEdits(t *testing.T) {
	b, pl, commands := newBridge(t, "3")
	require.NoError(t, b.Play(context.Background(), "tv1"))
	require.Eventually(t, func() bool { u, _ := pl.seen(); return len(u) > 0 }, 5*time.Second, 10*time.Millisecond)
	n := len(commands())
	items := append(slices.Clone(pl.items), player.Item{UID: 9, Track: source.Track{Library: "loc", ID: "c"}})
	b.Load(items, 1, -1, false)
	require.Eventually(t, func() bool { return len(commands()) > n && lastCmd(commands) == "edit" }, 5*time.Second, 10*time.Millisecond)
	c := commands()
	assert.Len(t, c[len(c)-1]["items"], 2, "the window from the playing item on")
}

// Stream URLs carry credentials; a plain-http server's never go to the receiver.
func TestCastRefusesPlainHTTP(t *testing.T) {
	b, pl, commands := newBridge(t, "3")
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
	b, pl, commands := newBridge(t, "3")
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
	b, pl, _ := newBridge(t, "3")
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
	b, _, _ := newBridge(t, "3")
	b.cfg.HasSubscriber = func(string) bool { return true }
	b.Subscribed()
	b.mu.Lock()
	p := b.proc
	b.mu.Unlock()
	require.NotNil(t, p)
	p.close()
	require.Eventually(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.proc != nil && b.proc != p }, 2*watchEvery, 50*time.Millisecond)
}

// A queue cut at a stop rule reaches the receiver without looping, and when
// the rule clears it gets the rest back and loops again.
func TestCastQueueCutAtAStopRule(t *testing.T) {
	b, pl, commands := newBridge(t, "3")
	full := append(slices.Clone(pl.items), player.Item{UID: 9, Track: source.Track{Library: "loc", ID: "c"}})
	pl.items = full
	b.SetLoop(true)
	require.NoError(t, b.Play(context.Background(), "tv1"))
	require.Eventually(t, func() bool { u, _ := pl.seen(); return len(u) > 0 }, 5*time.Second, 10*time.Millisecond)
	last := func() map[string]any { c := commands(); return c[len(c)-1] }
	assert.Equal(t, true, last()["loop"])

	n := len(commands())
	b.Load(full[:2], 0, -1, true)
	require.Eventually(t, func() bool { return len(commands()) > n && lastCmd(commands) == "edit" }, 5*time.Second, 10*time.Millisecond)
	reps := cmdsOf(commands, "repeat")
	require.NotEmpty(t, reps)
	assert.Equal(t, "", reps[len(reps)-1]["repeat"], "the receiver stops looping")
	assert.Len(t, last()["items"], 2, "cut after the stop entry")

	n = len(commands())
	b.Load(full, 0, -1, false)
	require.Eventually(t, func() bool {
		items, _ := last()["items"].([]any)
		return len(commands()) > n && len(items) == 3
	}, 5*time.Second, 10*time.Millisecond)
	reps = cmdsOf(commands, "repeat")
	assert.Equal(t, "show", reps[len(reps)-1]["repeat"], "the receiver loops again")
}

func cmdsOf(commands func() []map[string]any, name string) []map[string]any {
	var out []map[string]any
	for _, c := range commands() {
		if c["cmd"] == name {
			out = append(out, c)
		}
	}
	return out
}

// step is one state the helper reports: the receiver's player state, phase
// and queue index, how far into its dur-second item it is, and the status of
// its queue entry. gone is the session ending.
type step struct {
	state, phase string
	index        int
	pos, dur     float64
	status       string
	gone         bool
}

// While a stop rule cuts the queue, the receiver stopping after the stop
// entry ends the cast, but only when the state sequence shows it played out.
func TestCastRunsOutAtTheStopEntry(t *testing.T) {
	play := func(index int, pos float64) step { return step{state: "PLAYING", index: index, pos: pos, dur: 200} }
	idle := func(index int) step { return step{state: "IDLE", index: index, dur: 200} }
	gone := step{gone: true}
	for _, tt := range []struct {
		name string
		// open: the queue is cut at a stop rule; n items, the last one libDur long.
		open    bool
		n       int
		libDur  float64
		steps   []step
		errCode string
		decline bool
		// moved: the queue got another last item since the window was sent.
		moved bool
		// ranOut: RemoteRanOut calls; stopped: the cast ended with a stop sent.
		ranOut    int
		stillCast bool
		stopSent  bool
	}{
		{name: "run-out", open: true, steps: []step{play(2, 199), idle(2)}, ranOut: 1, stopSent: true},
		{name: "mid-track", open: true, steps: []step{play(2, 100), idle(2)}, stillCast: true},
		{name: "no rule", steps: []step{play(2, 199), idle(2)}, stillCast: true},
		{name: "jump, not prepared", open: true, steps: []step{{state: "PLAYING", phase: "converting", index: 2, pos: 199, dur: 200}, idle(2)}, stillCast: true},
		{name: "restart", open: true, steps: []step{play(2, 199), {index: 2, pos: 199, dur: 200}, idle(2)}, stillCast: true},
		{name: "loading", open: true, steps: []step{idle(2)}, stillCast: true},
		{name: "paused", open: true, steps: []step{{state: "PAUSED", index: 2, pos: 100, dur: 200}, idle(2)}, stillCast: true},
		{name: "earlier item", open: true, steps: []step{play(1, 199), idle(1)}, stillCast: true},
		{name: "window", open: true, n: 205, steps: []step{play(199, 199), idle(199)}, stillCast: true},
		{name: "library duration", open: true, libDur: 200, steps: []step{{state: "PLAYING", index: 2, pos: 199}, {state: "IDLE", index: 2}}, ranOut: 1, stopSent: true},
		{name: "bad receiver duration", open: true, libDur: 200, steps: []step{{state: "PLAYING", index: 2, pos: 199, dur: 600}, {state: "IDLE", index: 2, dur: 600}}, ranOut: 1, stopSent: true},
		{name: "no duration", open: true, steps: []step{{state: "PLAYING", index: 2, pos: 199}, {state: "IDLE", index: 2}}, stillCast: true},
		{name: "gone near the end", open: true, steps: []step{play(2, 199), gone}, ranOut: 1},
		{name: "gone mid-track", open: true, steps: []step{play(2, 100), gone}},
		{name: "gone, no rule", steps: []step{play(2, 199), gone}},
		{name: "load error", open: true, steps: []step{play(2, 100)}, errCode: "loadFailed", ranOut: 1, stopSent: true},
		{name: "load error on an earlier item", open: true, steps: []step{play(1, 100)}, errCode: "loadFailed", stillCast: true},
		{name: "load error, no rule", steps: []step{play(2, 100)}, errCode: "loadFailed", stillCast: true},
		{name: "preparation failed", open: true, steps: []step{{state: "IDLE", index: 2, dur: 200, status: "failed"}}, ranOut: 1, stopSent: true},
		{name: "preparation failed, no rule", steps: []step{{state: "IDLE", index: 2, dur: 200, status: "failed"}}, stillCast: true},
		{name: "jump, converting outlasts the old item", open: true, steps: []step{{state: "PLAYING", phase: "converting", index: 2, pos: 199, dur: 200, status: "preparing"},
			{state: "PLAYING", index: 2, pos: 199, dur: 200, status: "preparing"}, idle(2)}, stillCast: true},
		{name: "failed stop entry reached by itself", open: true, steps: []step{play(1, 100), {state: "IDLE", index: 2, dur: 200, status: "failed"}}, ranOut: 1, stopSent: true},
		{name: "failed after a doubtful idle", open: true, steps: []step{{state: "IDLE", phase: "converting", index: 2}, {state: "IDLE", index: 2, status: "failed"}}, ranOut: 1, stopSent: true},
		{name: "stop entry is not the last item", open: true, moved: true, steps: []step{play(2, 199), idle(2)}, stillCast: true},
		{name: "declined", open: true, decline: true, steps: []step{play(2, 199), idle(2)}, ranOut: 1, stillCast: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, pl, commands := newBridge(t, "3")
			pl.final, pl.decline = tt.open, tt.decline
			pl.items = append(pl.items, player.Item{UID: 9, Track: source.Track{Library: "loc", ID: "c", Duration: tt.libDur}})
			for i := 3; i < tt.n; i++ {
				pl.items = append(pl.items, player.Item{UID: uint64(100 + i), Track: source.Track{Library: "loc", ID: fmt.Sprint(i), Duration: tt.libDur}})
			}
			require.NoError(t, b.Play(context.Background(), "tv1"))
			require.Eventually(t, func() bool { u, _ := pl.seen(); return len(u) > 0 }, 5*time.Second, 10*time.Millisecond)
			b.mu.Lock()
			p := b.proc
			if tt.moved {
				b.items = append(slices.Clone(b.items), player.Item{UID: 99, Track: source.Track{Library: "loc", ID: "z"}})
			}
			b.mu.Unlock()
			for _, st := range tt.steps {
				sess := &session{Device: "tv1", Owner: &owner{ID: clientID}, Index: st.index, PlayerState: st.state,
					Phase: cmp.Or(st.phase, "casting"), Duration: st.dur, Position: st.pos, PositionAt: time.Now().UnixMilli()}
				for i := 0; i <= st.index; i++ {
					sess.Queue = append(sess.Queue, queueEntry{Status: "ready"})
				}
				sess.Queue[st.index].Status = cmp.Or(st.status, "ready")
				if st.gone {
					sess = nil
				}
				b.onEvent(p, event{Event: "state", Session: sess})
			}
			if tt.errCode != "" {
				b.onEvent(p, event{Event: "error", Code: tt.errCode})
			}
			pl.mu.Lock()
			assert.Equal(t, tt.ranOut, pl.ranOut)
			pl.mu.Unlock()
			assert.Equal(t, tt.stillCast, b.Status().Casting)
			if tt.stopSent {
				require.Eventually(t, func() bool { return lastCmd(commands) == "stop" }, 5*time.Second, 10*time.Millisecond)
			} else {
				time.Sleep(50 * time.Millisecond)
				assert.Empty(t, cmdsOf(commands, "stop"))
			}
			if tt.decline {
				b.Load(pl.items, 1, 0, true)
				require.Eventually(t, func() bool { return lastCmd(commands) == "jump" }, 5*time.Second, 10*time.Millisecond)
			}
		})
	}
}

// A player stop right after the cast ended must not stop a helper that
// stays attached for the device list.
func TestPlayerStopAfterTheCastEndedSendsNoStop(t *testing.T) {
	b, pl, commands := newBridge(t, "3")
	b.cfg.HasSubscriber = func(string) bool { return true }
	b.Subscribed()
	require.NoError(t, b.Play(context.Background(), "tv1"))
	require.Eventually(t, func() bool { u, _ := pl.seen(); return len(u) > 0 }, 5*time.Second, 10*time.Millisecond)
	b.mu.Lock()
	p := b.proc
	b.mu.Unlock()
	b.onEvent(p, event{Event: "state", Session: nil})
	require.False(t, b.Status().Casting)
	b.Stop()
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, cmdsOf(commands, "stop"))
}

// A queue edit carries the source the helper holds for the playing entry, so
// a server that makes a new URL per lookup doesn't restart the track.
func TestCastEditKeepsThePlayingSource(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         func(url string) bool
	}{
		{"source known", "https://server/rest/stream?id=b&kept", func(u string) bool { return strings.HasSuffix(u, "&kept") }},
		{"no source", "", func(u string) bool { return strings.Contains(u, "&s=") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, pl, commands := newBridge(t, "3")
			require.NoError(t, b.Play(context.Background(), "tv1"))
			require.Eventually(t, func() bool { u, _ := pl.seen(); return len(u) > 0 }, 5*time.Second, 10*time.Millisecond)
			b.mu.Lock()
			p := b.proc
			b.mu.Unlock()
			b.onEvent(p, event{Event: "state", Session: &session{Device: "tv1", Owner: &owner{ID: clientID}, Index: 1,
				PlayerState: "PLAYING", Phase: "casting", Queue: []queueEntry{{Status: "ready"}, {Status: "ready", Source: tt.source}}}})
			n := len(commands())
			items := append(slices.Clone(pl.items), player.Item{UID: 9, Track: source.Track{Library: "srv", ID: "c"}})
			b.Load(items, 1, -1, false)
			require.Eventually(t, func() bool { return len(commands()) > n && lastCmd(commands) == "edit" }, 5*time.Second, 10*time.Millisecond)
			c := commands()
			got := c[len(c)-1]["items"].([]any)
			require.Len(t, got, 2)
			assert.True(t, tt.want(got[0].(map[string]any)["url"].(string)), got[0])
			assert.Contains(t, got[1].(map[string]any)["url"], "&s=", "the other entries resolve again")
		})
	}
}

// The helper's detail can carry a stream URL with the server's token; the UI
// gets it without.
func TestCastErrorDetailLosesTokens(t *testing.T) {
	b, _, _ := newBridge(t, "3")
	require.NoError(t, b.Play(context.Background(), "tv1"))
	b.mu.Lock()
	p := b.proc
	b.mu.Unlock()
	b.onEvent(p, event{Event: "error", Code: "loadFailed", Detail: "cannot open https://srv.example/rest/stream?id=1&t=secret"})
	require.NotNil(t, b.Status().Error)
	assert.NotContains(t, b.Status().Error.Detail, "secret")
}
