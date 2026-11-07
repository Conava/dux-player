package player

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

type fakeRemote struct {
	mu  sync.Mutex
	log []string
	// loaded is the track ids of the last Load; final its flag.
	loaded []string
	final  bool
	index  int
}

func (r *fakeRemote) add(s string) {
	r.mu.Lock()
	r.log = append(r.log, s)
	r.mu.Unlock()
}

func (r *fakeRemote) Load(items []Item, index int, start float64, final bool) {
	r.mu.Lock()
	r.loaded, r.final, r.index = nil, final, index
	for _, it := range items {
		r.loaded = append(r.loaded, it.Track.ID)
	}
	r.mu.Unlock()
	r.add(fmt.Sprintf("load %s at %.0f", items[index].Track.ID, start))
}
func (r *fakeRemote) SetPaused(p bool)    { r.add(fmt.Sprintf("paused %t", p)) }
func (r *fakeRemote) Seek(pos float64)    { r.add(fmt.Sprintf("seek %.0f", pos)) }
func (r *fakeRemote) SetVolume(v float64) { r.add(fmt.Sprintf("volume %.0f", v)) }
func (r *fakeRemote) Stop()               { r.add("stop") }

// SetLoop records only turning it on; every load sends the current value.
func (r *fakeRemote) SetLoop(on bool) {
	if on {
		r.add("loop")
	}
}

func (r *fakeRemote) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.log
	r.log = nil
	return out
}

// While a remote plays, every transport command reaches it and none needs
// mpv (there is none here); taking playback back keeps the remote's spot.
func TestRemoteGetsTransportCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := New(Config{Binary: filepath.Join(t.TempDir(), "no-mpv")})
	run(t, ctx, p)
	r := &fakeRemote{}

	require.NoError(t, p.SetRemote(ctx, r, false))
	require.NoError(t, p.Play(ctx, tracks("a", "b", "c"), 0, Origin{}))
	require.NoError(t, p.Pause(ctx))
	require.NoError(t, p.Toggle(ctx))
	require.NoError(t, p.Seek(ctx, 30, false))
	require.NoError(t, p.Seek(ctx, -10, true))
	require.NoError(t, p.SetVolume(ctx, 40, false))
	require.NoError(t, p.Next(ctx))
	assert.Equal(t, []string{"load a at 0", "paused false", "paused true", "paused false", "seek 30", "seek 20", "volume 40", "load b at 0"}, r.take())

	q, err := p.Queue(ctx)
	require.NoError(t, err)
	require.NoError(t, p.RemoteUpdate(ctx, q.Items[2].UID, 42, true, 55))
	st, err := p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, st.Index)
	assert.Equal(t, StatePaused, st.State)
	assert.Equal(t, 42.0, st.Position)
	assert.True(t, st.Casting)

	require.NoError(t, p.SetRemote(ctx, nil, false))
	st, err = p.Status(ctx)
	require.NoError(t, err)
	assert.False(t, st.Casting)
	assert.Equal(t, StateStopped, st.State)
	assert.Equal(t, 42.0, st.Position, "resume continues where the remote was")
	assert.Equal(t, 2, st.Index)

	require.NoError(t, p.SetRemote(ctx, r, false))
	require.NoError(t, p.SetRepeat(ctx, RepeatAll))
	require.NoError(t, p.Stop(ctx))
	assert.Equal(t, []string{"load c at 42", "loop", "load c at -1", "stop"}, r.take(), "repeat all loops the remote's queue")
}

// Plays on a remote count like local ones: history and scrobbling follow
// the same played event.
func TestRemotePlaysCount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	played := 0
	p := New(Config{Binary: filepath.Join(t.TempDir(), "no-mpv"), Publish: func(topic string, _ any) {
		if topic == TopicPlayed {
			mu.Lock()
			played++
			mu.Unlock()
		}
	}})
	run(t, ctx, p)
	require.NoError(t, p.SetRemote(ctx, &fakeRemote{}, false))
	require.NoError(t, p.Play(ctx, []source.Track{{Library: "l", ID: "a", Duration: 100}}, 0, Origin{}))
	for pos := 1.0; pos <= 51; pos++ {
		require.NoError(t, p.RemoteUpdate(ctx, 0, pos, false, -1))
	}
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, played)
}

// Runs real mpv (null output). A crossfade hands the queue to a second mpv
// before the first track ends; the old one goes away after the fade, and a
// skip during the fade ends it at once. Tracks of one album stay gapless.
func TestCrossfade(t *testing.T) {
	bin, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not installed")
	}
	dir := t.TempDir()
	res := pathResolver{}
	for _, id := range []string{"a", "b", "c"} {
		res[id] = filepath.Join(dir, id+".wav")
		writeWAV(t, res[id], 3)
	}
	fading := func(p *Player) (in, out bool) {
		_ = p.do(context.Background(), func(context.Context) error {
			in, out = p.xf != nil && p.xf.in != nil, p.xf != nil && p.xf.out != nil
			return nil
		})
		return in, out
	}
	for _, tc := range []struct {
		name    string
		albums  [3]string
		skip    bool
		wantXF  bool
		wantEnd int
	}{
		{name: "different albums fade", albums: [3]string{"x", "y", "z"}, wantXF: true, wantEnd: 2},
		{name: "one album stays gapless", albums: [3]string{"x", "x", "x"}, wantEnd: 2},
		{name: "skip ends the fade", albums: [3]string{"x", "y", "z"}, skip: true, wantXF: true, wantEnd: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := DefaultOptions()
			opts.Crossfade = 1
			p := New(Config{Binary: bin, Socket: filepath.Join(dir, "mpv.sock"), ExtraArgs: []string{"--ao=null"}, Resolver: res, Options: opts})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			run(t, ctx, p)
			var q []source.Track
			for i, id := range []string{"a", "b", "c"} {
				q = append(q, source.Track{Library: "l", ID: id, Title: id, Album: tc.albums[i], Duration: 3})
			}
			require.NoError(t, p.Play(ctx, q, 0, Origin{Kind: "adhoc"}))

			sawFade := false
			deadline := time.Now().Add(15 * time.Second)
			for time.Now().Before(deadline) {
				if _, out := fading(p); out {
					sawFade = true
					st, err := p.Status(ctx)
					require.NoError(t, err)
					assert.Equal(t, StatePlaying, st.State)
					assert.Equal(t, 1, st.Index, "the next track is current from the fade's start")
					break
				}
				st, _ := p.Status(ctx)
				if st.Index == 1 && st.Position > 1.5 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			assert.Equal(t, tc.wantXF, sawFade)
			if tc.skip {
				require.NoError(t, p.Next(ctx))
				in, out := fading(p)
				assert.False(t, in || out, "a skip ends the fade")
			}
			require.Eventually(t, func() bool {
				st, _ := p.Status(ctx)
				return st.State == StateStopped
			}, 20*time.Second, 20*time.Millisecond, "the queue did not play out")
			st, err := p.Status(ctx)
			require.NoError(t, err)
			assert.Equal(t, tc.wantEnd, st.Index)
			in, out := fading(p)
			assert.False(t, in || out, "no second mpv is left over")
		})
	}
}

func (r *fakeRemote) queue() (ids []string, final bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.loaded...), r.final
}

// A stop rule ahead cuts the queue the receiver gets after its entry, so it
// ends there by itself; clearing the rule gives the rest back.
func TestCastQueueEndsAtTheStopRule(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := New(Config{Binary: filepath.Join(t.TempDir(), "no-mpv")})
	run(t, ctx, p)
	r := &fakeRemote{}
	require.NoError(t, p.SetRemote(ctx, r, false))
	require.NoError(t, p.Play(ctx, tracks("a", "b", "c", "d"), 0, Origin{}))

	require.NoError(t, p.SetSleepAfter(ctx, 2))
	got, final := r.queue()
	assert.Equal(t, []string{"a", "b", "c"}, got)
	assert.True(t, final)

	require.NoError(t, p.SetSleep(ctx, SleepEndOfTrack, 0))
	got, final = r.queue()
	assert.Equal(t, []string{"a"}, got, "the end of the track cuts after the current one")
	assert.True(t, final)

	require.NoError(t, p.SetSleep(ctx, SleepOff, 0))
	got, final = r.queue()
	assert.Equal(t, []string{"a", "b", "c", "d"}, got)
	assert.False(t, final)

	require.NoError(t, p.SetSleepAfter(ctx, 1))
	require.NoError(t, p.Remove(ctx, 1))
	got, final = r.queue()
	assert.Equal(t, []string{"a", "c", "d"}, got, "a rule whose entry left the queue lets the queue grow again")
	assert.False(t, final)
}

// The receiver ran out at the stop rule: stopped with the next track lined
// up, the rule cleared, the receiver told to stop. Anything else leaves the
// cast alone.
func TestCastRunsOutAtTheStopRule(t *testing.T) {
	for _, tt := range []struct {
		name string
		// rule: "after 1" is set; clear: it is cleared before the run-out;
		// wrongUID reports the entry before the current one.
		rule, clear, wrongUID, elsewhere bool
		wantCast                         bool
		wantIndex                        int
		wantRule                         string
		wantStops                        int
	}{
		{"rule due", true, false, false, false, false, 2, SleepOff, 1},
		{"no rule", false, false, false, false, true, 1, SleepOff, 0},
		{"rule cleared first", true, true, false, false, true, 1, SleepOff, 0},
		{"not the current entry", true, false, true, false, true, 1, SleepAfterEntry, 0},
		{"ended elsewhere", true, false, false, true, false, 1, SleepAfterEntry, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := New(Config{Binary: filepath.Join(t.TempDir(), "no-mpv")})
			run(t, ctx, p)
			r := &fakeRemote{}
			require.NoError(t, p.SetRemote(ctx, r, false))
			require.NoError(t, p.Play(ctx, tracks("a", "b", "c"), 0, Origin{}))
			if tt.rule {
				require.NoError(t, p.SetSleepAfter(ctx, 1))
			}
			q, err := p.Queue(ctx)
			require.NoError(t, err)
			require.NoError(t, p.RemoteUpdate(ctx, q.Items[1].UID, 0, false, -1))
			if tt.clear {
				require.NoError(t, p.SetSleep(ctx, SleepOff, 0))
			}
			r.take()

			switch {
			case tt.elsewhere:
				require.NoError(t, p.SetRemote(ctx, nil, false))
			case tt.wrongUID:
				require.NoError(t, p.RemoteRanOut(ctx, q.Items[0].UID))
			default:
				require.NoError(t, p.RemoteRanOut(ctx, q.Items[1].UID))
			}
			st, err := p.Status(ctx)
			require.NoError(t, err)
			if tt.wantCast {
				assert.Equal(t, StatePlaying, st.State)
			} else {
				assert.Equal(t, StateStopped, st.State)
			}
			assert.Equal(t, tt.wantIndex, st.Index)
			assert.Equal(t, tt.wantRule, st.Sleep.Mode)
			stops := 0
			for _, l := range r.take() {
				if l == "stop" {
					stops++
				}
			}
			assert.Equal(t, tt.wantStops, stops)
		})
	}
}

// With repeat all a rule behind the current track is reached again: the
// receiver gets the queue from the current track on, wrapped round to the entry.
func TestCastQueueWrapsToARuleBehindWithRepeatAll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := New(Config{Binary: filepath.Join(t.TempDir(), "no-mpv")})
	run(t, ctx, p)
	r := &fakeRemote{}
	require.NoError(t, p.SetRemote(ctx, r, false))
	require.NoError(t, p.Play(ctx, tracks("a", "b", "c", "d"), 0, Origin{}))
	require.NoError(t, p.SetRepeat(ctx, RepeatAll))
	require.NoError(t, p.SetSleepAfter(ctx, 1))
	require.NoError(t, p.Jump(ctx, 2))
	got, final := r.queue()
	assert.Equal(t, []string{"c", "d", "a", "b"}, got)
	assert.True(t, final)
	assert.Equal(t, 0, r.index)
}

// A rule whose entry lies behind the current track can't fire: it is
// cleared, unless repeat all comes back to it.
func TestStopRuleBehindTheCurrentTrack(t *testing.T) {
	for _, tt := range []struct {
		repeat string
		want   string
	}{{RepeatOff, SleepOff}, {RepeatAll, SleepAfterEntry}} {
		t.Run(tt.repeat, func(t *testing.T) {
			p, ctx := castPlayer(t, Config{})
			require.NoError(t, p.Play(ctx, tracks("a", "b", "c", "d"), 0, Origin{}))
			require.NoError(t, p.SetRepeat(ctx, tt.repeat))
			require.NoError(t, p.SetSleepAfter(ctx, 1))
			require.NoError(t, p.Jump(ctx, 3))
			st, err := p.Status(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.want, st.Sleep.Mode)
		})
	}
}
