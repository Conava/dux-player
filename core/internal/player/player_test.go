package player

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

// writeWAV writes a silent 16-bit mono WAV of the given length.
func writeWAV(t *testing.T, path string, seconds float64) {
	t.Helper()
	const rate = 8000
	n := int(seconds * rate)
	data := make([]byte, 44+2*n)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(36+2*n))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], rate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(2*n))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

type pathResolver map[string]string

func (r pathResolver) Stream(_ context.Context, ref source.Ref) (Stream, error) {
	return Stream{URL: r[ref.ID]}, nil
}

type recorder struct {
	mu     sync.Mutex
	status []Status
}

func (r *recorder) publish(topic string, data any) {
	if topic != TopicStatus {
		return
	}
	r.mu.Lock()
	r.status = append(r.status, data.(Status))
	r.mu.Unlock()
}

func (r *recorder) saw(pred func(Status) bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.status {
		if pred(s) {
			return true
		}
	}
	return false
}

// Runs real mpv with the null audio output: the daemon's queue must follow
// mpv's own move to the prefetched next track, and a killed mpv must come
// back on the same track.
func TestPlayerFollowsMPV(t *testing.T) {
	bin, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not installed")
	}
	dir := t.TempDir()
	res := pathResolver{}
	var queue []source.Track
	for _, id := range []string{"a", "b", "c"} {
		p := filepath.Join(dir, id+".wav")
		writeWAV(t, p, 0.4)
		res[id] = p
		queue = append(queue, source.Track{Library: "l", ID: id, Title: id})
	}
	long := filepath.Join(dir, "long.wav")
	writeWAV(t, long, 30)
	res["long"] = long

	rec := &recorder{}
	p := New(Config{
		Binary:    bin,
		Socket:    filepath.Join(dir, "mpv.sock"),
		ExtraArgs: []string{"--ao=null"},
		Resolver:  res,
		Publish:   rec.publish,
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	require.NoError(t, p.Play(ctx, queue, 0, Origin{Kind: "adhoc"}))
	require.Eventually(t, func() bool {
		return rec.saw(func(s Status) bool { return s.State == StatePlaying && s.Index == 2 })
	}, 10*time.Second, 20*time.Millisecond, "queue never reached the third track")
	require.Eventually(t, func() bool {
		st, err := p.Status(ctx)
		return err == nil && st.State == StateStopped
	}, 10*time.Second, 20*time.Millisecond, "playback did not stop at the end of the queue")
	st, err := p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, st.Index)

	require.NoError(t, p.Play(ctx, []source.Track{{Library: "l", ID: "long", Title: "long"}}, 0, Origin{Kind: "track"}))
	require.NoError(t, p.Seek(ctx, 12, false))
	var proc *process
	require.NoError(t, p.do(ctx, func(context.Context) error { proc = p.proc; return nil }))
	require.Eventually(t, func() bool {
		st, _ := p.Status(ctx)
		return st.Position >= 12
	}, 5*time.Second, 20*time.Millisecond)
	require.NoError(t, proc.cmd.Process.Kill())

	require.Eventually(t, func() bool {
		var restarted bool
		_ = p.do(ctx, func(context.Context) error { restarted = p.proc != nil && p.proc != proc && p.loaded; return nil })
		return restarted
	}, 10*time.Second, 20*time.Millisecond, "mpv was not restarted")
	st, err = p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, StatePlaying, st.State)
	assert.Equal(t, "long", st.Track.ID)
	assert.GreaterOrEqual(t, st.Position, 12.0, "position was not restored")

	require.NoError(t, p.Stop(ctx))
	st, err = p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, StateStopped, st.State)
	assert.GreaterOrEqual(t, st.Position, 12.0, "stop keeps the position")
	require.NoError(t, p.Resume(ctx))
	require.Eventually(t, func() bool {
		st, _ := p.Status(ctx)
		return st.State == StatePlaying && st.Position >= 12
	}, 5*time.Second, 20*time.Millisecond, "resume after stop did not continue from the position")

	feed := &fakeFeed{batches: [][]source.Track{queue, {{Library: "l", ID: "long", Title: "long"}}, queue}}
	require.NoError(t, p.PlayFeed(ctx, feed, Origin{Kind: "tracks"}))
	require.Eventually(t, func() bool {
		q, _ := p.Queue(ctx)
		return len(q.Items) == 7
	}, 5*time.Second, 20*time.Millisecond, "a feed queue running low was not topped up")
	q, err := p.Queue(ctx)
	require.NoError(t, err)
	assert.False(t, q.Origin.Modified, "refills are not edits")
}

type fakeFeed struct {
	mu      sync.Mutex
	batches [][]source.Track
}

func (f *fakeFeed) Next(context.Context, int) ([]source.Track, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.batches) == 0 {
		return nil, nil
	}
	b := f.batches[0]
	f.batches = f.batches[1:]
	return b, nil
}

func TestVolumeDropsWhenBoostIsTurnedOff(t *testing.T) {
	p := New(Config{Binary: filepath.Join(t.TempDir(), "no-mpv")})
	p.volume = 120
	p.opts.VolumeMax = 100
	p.applyOptions(context.Background())
	assert.Equal(t, 100.0, p.status().Volume)
}

func TestFallbackGainPrefersServerValuesForTheMode(t *testing.T) {
	g := func(v float64) *float64 { return &v }
	measured := g(4)
	tests := []struct {
		name string
		mode string
		rg   *source.ReplayGain
		want *float64
	}{
		{name: "track mode uses track gain", mode: "track", rg: &source.ReplayGain{Track: g(-7), Album: g(-5)}, want: g(-7)},
		{name: "album mode uses album gain", mode: "album", rg: &source.ReplayGain{Track: g(-7), Album: g(-5)}, want: g(-5)},
		{name: "album mode without album gain", mode: "album", rg: &source.ReplayGain{Track: g(-7)}, want: g(-7)},
		{name: "no server values: measured", mode: "track", want: measured},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(Config{})
			p.opts.ReplayGain = tt.mode
			assert.Equal(t, tt.want, p.fallbackGain(source.Track{ReplayGain: tt.rg}, measured))
		})
	}
}

func TestSeekBeforeLoadSurvivesStop(t *testing.T) {
	p := New(Config{Binary: filepath.Join(t.TempDir(), "no-mpv")})
	p.active, p.loaded = true, false
	require.NoError(t, p.seek(context.Background(), 9, false))
	p.onProperty("time-pos", []byte("0"))
	p.stop(context.Background())
	assert.Equal(t, 9.0, p.status().Position)
}

// A play counts once half the track has been heard; a seek past the middle
// is not listening.
func TestPlayCountsAtHalfTheTrack(t *testing.T) {
	tests := []struct {
		name  string
		steps []float64
		want  int
	}{
		{name: "listened to the middle", steps: []float64{1, 2, 3, 50, 51}, want: 0},
		{name: "listened past the middle", steps: stepsTo(52), want: 1},
		{name: "seek to the end", steps: []float64{1, 2, 90, 91, 92}, want: 0},
		{name: "repeat one counts again", steps: append(stepsTo(60), append([]float64{0.5}, stepsTo(60)...)...), want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			played := 0
			p := New(Config{Publish: func(topic string, _ any) {
				if topic == TopicPlayed {
					played++
				}
			}})
			p.queue.Replace([]source.Track{{Library: "l", ID: "x", Duration: 100}}, 0, Origin{})
			p.dur = 100
			p.resetPlay(0)
			for _, pos := range tt.steps {
				p.countPlay(pos)
			}
			assert.Equal(t, tt.want, played)
		})
	}
}

func stepsTo(end float64) []float64 {
	var out []float64
	for pos := 1.0; pos <= end; pos++ {
		out = append(out, pos)
	}
	return out
}

// Real mpv: the sleep timer's end of track stops before the next track
// (which resume then plays), and clearing with keepCurrent keeps playing.
func TestSleepAtEndOfTrackAndClearKeepingCurrent(t *testing.T) {
	bin, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not installed")
	}
	dir := t.TempDir()
	res := pathResolver{}
	for id, secs := range map[string]float64{"a": 0.6, "b": 0.6, "c": 0.6, "long": 30} {
		res[id] = filepath.Join(dir, id+".wav")
		writeWAV(t, res[id], secs)
	}
	p := New(Config{Binary: bin, Socket: filepath.Join(dir, "mpv.sock"), ExtraArgs: []string{"--ao=null"}, Resolver: res})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	require.NoError(t, p.Play(ctx, tracks("a", "b", "c"), 0, Origin{Kind: "adhoc"}))
	require.NoError(t, p.SetSleep(ctx, SleepEndOfTrack, 0))
	require.Eventually(t, func() bool {
		st, _ := p.Status(ctx)
		return st.State == StateStopped
	}, 5*time.Second, 20*time.Millisecond, "playback did not stop at the end of the track")
	st, err := p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, st.Index, "the next track is lined up for resume")
	assert.Equal(t, SleepOff, st.Sleep.Mode)

	require.NoError(t, p.Play(ctx, tracks("long", "a", "b"), 0, Origin{Kind: "adhoc"}))
	require.NoError(t, p.Clear(ctx, true))
	st, err = p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, StatePlaying, st.State)
	assert.Equal(t, 1, st.Length)
	assert.Equal(t, "long", st.Track.ID)
}

// Real mpv: with auto-continue on, the last track of the queue gets more
// tracks behind it and playback goes on.
func TestAutoContinueExtendsTheQueue(t *testing.T) {
	bin, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not installed")
	}
	dir := t.TempDir()
	res := pathResolver{}
	for _, id := range []string{"a", "b"} {
		res[id] = filepath.Join(dir, id+".wav")
		writeWAV(t, res[id], 0.6)
	}
	opts := DefaultOptions()
	opts.AutoContinue = ContinueRandom
	p := New(Config{Binary: bin, Socket: filepath.Join(dir, "mpv.sock"), ExtraArgs: []string{"--ao=null"}, Resolver: res,
		Options: opts, Continue: func(context.Context, string, source.Track, map[source.Ref]bool) ([]source.Track, error) {
			return tracks("b"), nil
		}})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	require.NoError(t, p.Play(ctx, tracks("a"), 0, Origin{Kind: "track"}))
	require.Eventually(t, func() bool {
		st, _ := p.Status(ctx)
		return st.Track != nil && st.Track.ID == "b"
	}, 5*time.Second, 20*time.Millisecond, "the queue did not continue")
}

type probeResolver struct {
	pathResolver
	mu    sync.Mutex
	asked int
}

func (r *probeResolver) Probe(_ context.Context, _ source.Ref, done func(gain float64)) {
	r.mu.Lock()
	r.asked++
	r.mu.Unlock()
	go done(-9)
}

// Which gain a track plays with: the server's ReplayGain value, else the
// measured one, else one probed from its stream; nothing while ReplayGain
// is off.
func TestGainForATrack(t *testing.T) {
	server, measured := -3.0, -5.0
	tests := []struct {
		name       string
		rg         *source.ReplayGain
		measured   *float64
		replayGain string
		want       *float64
		wantProbe  bool
	}{
		{name: "server value", rg: &source.ReplayGain{Track: &server}, replayGain: "track", want: &server},
		{name: "measured", measured: &measured, replayGain: "track", want: &measured},
		{name: "nothing known is probed", replayGain: "track", want: new(-9.0), wantProbe: true},
		{name: "off probes nothing", replayGain: "no"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := &probeResolver{}
			opts := DefaultOptions()
			opts.ReplayGain = tt.replayGain
			p := New(Config{Resolver: res, Options: opts})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go p.Run(ctx)
			var it Item
			require.NoError(t, p.do(ctx, func(context.Context) error {
				p.queue.Replace([]source.Track{{Library: "l", ID: "a", ReplayGain: tt.rg}}, 0, Origin{Kind: "adhoc"})
				it, _ = p.queue.Current()
				p.keepStream(it, Stream{GainDB: tt.measured})
				return nil
			}))
			gain := func() (g *float64) {
				_ = p.do(ctx, func(context.Context) error { g = p.fallbackGain(it.Track, p.streams[it.UID].GainDB); return nil })
				return g
			}
			if tt.want != nil {
				require.Eventually(t, func() bool { return gain() != nil }, time.Second, 5*time.Millisecond)
				assert.Equal(t, *tt.want, *gain())
			} else {
				time.Sleep(20 * time.Millisecond)
				assert.Nil(t, gain())
			}
			res.mu.Lock()
			defer res.mu.Unlock()
			assert.Equal(t, tt.wantProbe, res.asked > 0)
		})
	}
}
