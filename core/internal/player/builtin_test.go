package player

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/player/engine"
	"github.com/Conava/dux-player/core/internal/source"
)

// An output that won't open pauses and says so with its own code, instead of
// looking like a track that stopped by itself.
func TestBuiltinReportsOutputThatWontOpen(t *testing.T) {
	old := builtinOutput
	builtinOutput = func(context.Context) (engine.Output, error) {
		n := engine.NewNull(false)
		n.OpenErr = errors.New("no reply")
		return n, nil
	}
	t.Cleanup(func() { builtinOutput = old })
	dir := t.TempDir()
	res := pathResolver{"a": filepath.Join(dir, "a.wav")}
	writeWAV(t, res["a"], 5)

	errs := make(chan map[string]string, 8)
	p := New(Config{
		Binary:   filepath.Join(dir, "no-mpv"),
		Socket:   filepath.Join(dir, "mpv.sock"),
		Resolver: res,
		Options:  DefaultOptions(),
		Publish: func(topic string, data any) {
			if topic == TopicError {
				errs <- data.(map[string]string)
			}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	require.NoError(t, p.Play(ctx, []source.Track{{Library: "l", ID: "a", Title: "a"}}, 0, Origin{Kind: "adhoc"}))
	select {
	case e := <-errs:
		assert.Equal(t, CodeNoOutput, e["code"])
		assert.Contains(t, e["message"], "no reply")
	case <-time.After(10 * time.Second):
		t.Fatal("no error reached the UI")
	}
}

// An output that fails mid-track pauses and says so too.
func TestBuiltinReportsOutputLostMidTrack(t *testing.T) {
	out := useNull(t)
	dir := t.TempDir()
	res := pathResolver{"a": filepath.Join(dir, "a.wav")}
	writeWAV(t, res["a"], 5)

	errs := make(chan map[string]string, 8)
	p := New(Config{
		Binary:   filepath.Join(dir, "no-mpv"),
		Socket:   filepath.Join(dir, "mpv.sock"),
		Resolver: res,
		Options:  DefaultOptions(),
		Publish: func(topic string, data any) {
			if topic == TopicError {
				errs <- data.(map[string]string)
			}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	require.NoError(t, p.Play(ctx, []source.Track{{Library: "l", ID: "a", Title: "a"}}, 0, Origin{Kind: "adhoc"}))
	require.Eventually(t, func() bool {
		st, _ := p.Status(ctx)
		return st.State == StatePlaying && st.Position > 0.2
	}, 10*time.Second, 20*time.Millisecond)

	out().FailWrites(errors.New("sink removed"))
	select {
	case e := <-errs:
		assert.Equal(t, CodeNoOutput, e["code"])
		assert.Contains(t, e["message"], "sink removed")
	case <-time.After(10 * time.Second):
		t.Fatal("no error reached the UI")
	}
	require.Eventually(t, func() bool {
		st, _ := p.Status(ctx)
		return st.State == StatePaused
	}, 5*time.Second, 20*time.Millisecond, "it should pause")
}

type flakyResolver struct {
	pathResolver
	failed atomic.Bool
}

// Stream fails once for track b, the way a server hiccup fails a prefetch.
func (r *flakyResolver) Stream(ctx context.Context, ref source.Ref) (Stream, error) {
	if ref.ID == "b" && r.failed.CompareAndSwap(false, true) {
		return Stream{}, errors.New("server hiccup")
	}
	return r.pathResolver.Stream(ctx, ref)
}

// A next track that never lined up doesn't end the queue at the end of the
// current one.
func TestQueueGoesOnWhenTheNextTrackDidNotLineUp(t *testing.T) {
	useNull(t)
	dir := t.TempDir()
	res := pathResolver{"a": filepath.Join(dir, "a.wav"), "b": filepath.Join(dir, "b.wav")}
	writeWAV(t, res["a"], 0.5)
	writeWAV(t, res["b"], 5)
	p := New(Config{
		Binary:   filepath.Join(dir, "no-mpv"),
		Socket:   filepath.Join(dir, "mpv.sock"),
		Resolver: &flakyResolver{pathResolver: res},
		Options:  DefaultOptions(),
		Publish:  func(string, any) {},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	queue := []source.Track{{Library: "l", ID: "a", Title: "a"}, {Library: "l", ID: "b", Title: "b"}}
	require.NoError(t, p.Play(ctx, queue, 0, Origin{Kind: "adhoc"}))
	require.Eventually(t, func() bool {
		st, _ := p.Status(ctx)
		return st.Index == 1 && st.State == StatePlaying
	}, 10*time.Second, 20*time.Millisecond, "the queue stopped after the first track")
}

// Without mpv the built-in player plays. The status says what it can't do,
// crossfade never lines up, the speed stays 1, and a file only mpv plays is
// passed over with its own error code.
func TestBuiltinWithoutMPV(t *testing.T) {
	useNull(t)
	dir := t.TempDir()
	res := pathResolver{"m4a": filepath.Join(dir, "a.m4a"), "b": filepath.Join(dir, "b.wav"), "c": filepath.Join(dir, "c.wav")}
	require.NoError(t, os.WriteFile(res["m4a"], []byte("\x00\x00\x00\x20ftypM4A \x00\x00\x00\x00"), 0o644))
	writeWAV(t, res["b"], 5)
	writeWAV(t, res["c"], 5)

	var mu sync.Mutex
	var backends []string
	var codes []string
	var speeds []float64
	opts := DefaultOptions()
	opts.Crossfade, opts.Speed, opts.AlbumGapless = 3, 1.5, false
	p := New(Config{
		Binary:   filepath.Join(dir, "no-mpv"),
		Socket:   filepath.Join(dir, "mpv.sock"),
		Resolver: res,
		Options:  opts,
		OnBackend: func(b string) {
			mu.Lock()
			backends = append(backends, b)
			mu.Unlock()
		},
		Publish: func(topic string, data any) {
			mu.Lock()
			defer mu.Unlock()
			switch topic {
			case TopicError:
				codes = append(codes, data.(map[string]string)["code"])
			case TopicPosition:
				speeds = append(speeds, data.(Position).Speed)
			}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	queue := []source.Track{{Library: "l", ID: "m4a", Title: "m4a"}, {Library: "l", ID: "b", Title: "b", Album: "x"},
		{Library: "l", ID: "c", Title: "c", Album: "y"}}
	require.NoError(t, p.Play(ctx, queue, 0, Origin{Kind: "adhoc"}))
	require.Eventually(t, func() bool {
		st, _ := p.Status(ctx)
		return st.Index == 1 && st.State == StatePlaying && st.Position > 0.3
	}, 10*time.Second, 20*time.Millisecond, "the queue didn't move past the file only mpv plays")

	st, err := p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, BackendBuiltin, st.Backend)
	assert.False(t, st.CanCrossfade)
	assert.False(t, st.CanChangeSpeed)
	assert.True(t, st.Seekable)
	var fades bool
	require.NoError(t, p.do(ctx, func(context.Context) error { fades = p.crossfadeNext(); return nil }))
	assert.False(t, fades, "the built-in player never crossfades")

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{BackendBuiltin}, backends)
	assert.Contains(t, codes, CodeNeedsMPV)
	require.NotEmpty(t, speeds)
	for _, s := range speeds {
		assert.Equal(t, 1.0, s)
	}
}

// Stopping after an entry the built-in player can't play still stops there.
func TestBuiltinStopsAfterAnEntryOnlyMPVPlays(t *testing.T) {
	useNull(t)
	dir := t.TempDir()
	res := pathResolver{"a": filepath.Join(dir, "a.wav"), "m4a": filepath.Join(dir, "b.m4a"), "c": filepath.Join(dir, "c.wav")}
	require.NoError(t, os.WriteFile(res["m4a"], []byte("\x00\x00\x00\x20ftypM4A \x00\x00\x00\x00"), 0o644))
	writeWAV(t, res["a"], 0.3)
	writeWAV(t, res["c"], 10)
	p := New(Config{
		Binary:   filepath.Join(dir, "no-mpv"),
		Socket:   filepath.Join(dir, "mpv.sock"),
		Resolver: res,
		Options:  DefaultOptions(),
		Publish:  func(string, any) {},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	require.NoError(t, p.Play(ctx, tracks("a", "m4a", "c"), 0, Origin{Kind: "adhoc"}))
	require.NoError(t, p.SetSleepAfter(ctx, 1))
	require.Eventually(t, func() bool {
		st, _ := p.Status(ctx)
		return st.State == StateStopped
	}, 3*time.Second, 20*time.Millisecond, "playback went on past the entry")
}

// A feed started with shuffle by album keeps each album together and in
// track order.
func TestPlayFeedShufflesByAlbum(t *testing.T) {
	useNull(t)
	dir := t.TempDir()
	wav := filepath.Join(dir, "a.wav")
	writeWAV(t, wav, 5)
	var batch []source.Track
	res := pathResolver{}
	for _, no := range []int{1, 2, 3} {
		for _, album := range []string{"x", "y", "z"} {
			id := fmt.Sprintf("%s%d", album, no)
			res[id] = wav
			batch = append(batch, source.Track{Library: "l", ID: id, Title: id, AlbumID: album, Album: album, TrackNo: no})
		}
	}
	opts := DefaultOptions()
	opts.ShuffleAlbums = true
	p := New(Config{Binary: filepath.Join(dir, "no-mpv"), Socket: filepath.Join(dir, "mpv.sock"), Resolver: res,
		Options: opts, Publish: func(string, any) {}})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	require.NoError(t, p.PlayFeed(ctx, &fakeFeed{batches: [][]source.Track{batch}}, Origin{Kind: "tracks"}))
	q, err := p.Queue(ctx)
	require.NoError(t, err)
	require.Len(t, q.Items, 9)
	for i := 0; i < 9; i += 3 {
		album := q.Items[i].Track.AlbumID
		for j := 0; j < 3; j++ {
			tr := q.Items[i+j].Track
			assert.Equal(t, album, tr.AlbumID, "album stays together")
			assert.Equal(t, j+1, tr.TrackNo, "in track order")
		}
	}
}
