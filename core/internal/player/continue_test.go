package player

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

func queueIDs(t *testing.T, ctx context.Context, p *Player) []string {
	q, err := p.Queue(ctx)
	require.NoError(t, err)
	out := []string{}
	for _, it := range q.Items {
		out = append(out, it.Track.ID)
	}
	return out
}

// A queue [a b] plays its last track; continuing offers a, b and c, of which
// only c is new.
func TestQueueContinues(t *testing.T) {
	cases := []struct {
		name string
		auto string
		act  func(context.Context, *Player) error
		want []string
		cur  string
	}{
		{name: "auto-continue appends only tracks not queued", auto: ContinueSimilar, want: []string{"a", "b", "c"}, cur: "b"},
		{name: "without continuing the queue ends", auto: ContinueOff, want: []string{"a", "b"}, cur: "b"},
		{name: "next at the end continues and plays on", auto: ContinueOff, cur: "c", want: []string{"a", "b", "c"},
			act: func(ctx context.Context, p *Player) error { return p.Next(ctx) }},
		{name: "turning auto-continue on applies to the running queue", auto: ContinueOff, want: []string{"a", "b", "c"}, cur: "b",
			act: func(ctx context.Context, p *Player) error {
				o := DefaultOptions()
				o.AutoContinue = ContinueRandom
				return p.Configure(ctx, o)
			}},
		{name: "continue turns it on for this queue", auto: ContinueOff, want: []string{"a", "b", "c"}, cur: "b",
			act: func(ctx context.Context, p *Player) error { return p.Continue(ctx, ContinueSimilar) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := DefaultOptions()
			opts.AutoContinue = tc.auto
			p, ctx := castPlayer(t, Config{Options: opts,
				Continue: func(context.Context, string, source.Track, map[source.Ref]bool) ([]source.Track, error) {
					return tracks("a", "b", "c"), nil
				}})
			require.NoError(t, p.Play(ctx, tracks("a", "b"), 1, Origin{}))
			if tc.act != nil {
				require.NoError(t, tc.act(ctx, p))
			}
			if len(tc.want) > 2 {
				require.Eventually(t, func() bool { return len(queueIDs(t, ctx, p)) >= len(tc.want) }, 2*time.Second, 5*time.Millisecond)
			}
			time.Sleep(50 * time.Millisecond)
			assert.Equal(t, tc.want, queueIDs(t, ctx, p))
			st, err := p.Status(ctx)
			require.NoError(t, err)
			assert.Equal(t, tc.cur, st.Track.ID)
			assert.Equal(t, len(tc.want) > 2, st.Continuing)
		})
	}
}

// Continuing turned on for a queue ends with it.
func TestContinueBelongsToTheQueue(t *testing.T) {
	p, ctx := castPlayer(t, Config{Continue: func(context.Context, string, source.Track, map[source.Ref]bool) ([]source.Track, error) {
		return nil, nil
	}})
	require.NoError(t, p.Play(ctx, tracks("a"), 0, Origin{}))
	require.NoError(t, p.Continue(ctx, ContinueRandom))
	st, _ := p.Status(ctx)
	assert.True(t, st.Continuing)
	require.NoError(t, p.Play(ctx, tracks("b"), 0, Origin{}))
	st, _ = p.Status(ctx)
	assert.False(t, st.Continuing)
}

// Real mpv: Next after the queue played out continues it and plays.
func TestNextAfterTheEndContinues(t *testing.T) {
	bin, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not installed")
	}
	dir := t.TempDir()
	res := pathResolver{}
	for _, id := range []string{"a", "b"} {
		res[id] = filepath.Join(dir, id+".wav")
		writeWAV(t, res[id], 0.4)
	}
	p := New(Config{Binary: bin, Socket: filepath.Join(dir, "mpv.sock"), ExtraArgs: []string{"--ao=null"}, Resolver: res,
		Continue: func(context.Context, string, source.Track, map[source.Ref]bool) ([]source.Track, error) {
			return tracks("b"), nil
		}})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	require.NoError(t, p.Play(ctx, tracks("a"), 0, Origin{Kind: "track"}))
	require.Eventually(t, func() bool {
		st, _ := p.Status(ctx)
		return st.State == StateStopped
	}, 5*time.Second, 20*time.Millisecond, "the queue did not end")
	require.NoError(t, p.Next(ctx))
	st, err := p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, "b", st.Track.ID)
	assert.Equal(t, StatePlaying, st.State)
	assert.True(t, st.Continuing)
}

// A continue that answers after the queue was replaced leaves the new queue
// alone.
func TestLateContinueLeavesANewQueueAlone(t *testing.T) {
	release := make(chan struct{})
	p, ctx := castPlayer(t, Config{Continue: func(context.Context, string, source.Track, map[source.Ref]bool) ([]source.Track, error) {
		<-release
		return tracks("x", "y"), nil
	}})
	require.NoError(t, p.Play(ctx, tracks("a", "b"), 1, Origin{}))
	done := make(chan error, 1)
	go func() { done <- p.Next(ctx) }()
	require.Eventually(t, func() bool {
		st, _ := p.Status(ctx)
		return st.Continuing
	}, 2*time.Second, 5*time.Millisecond)
	require.NoError(t, p.Play(ctx, tracks("c", "d", "e"), 0, Origin{}))
	close(release)
	require.NoError(t, <-done)
	st, err := p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, "c", st.Track.ID)
	assert.Equal(t, []string{"c", "d", "e"}, queueIDs(t, ctx, p))
}

// A continue that fails is asked again on the next queue change; one that
// found nothing new isn't.
func TestFailedContinueIsRetried(t *testing.T) {
	cases := []struct {
		name  string
		first error
		want  []string
	}{
		{"after a failure", errors.New("server down"), []string{"a", "b", "c"}},
		{"after nothing new", nil, []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			opts := DefaultOptions()
			opts.AutoContinue = ContinueSimilar
			p, ctx := castPlayer(t, Config{Options: opts,
				Continue: func(context.Context, string, source.Track, map[source.Ref]bool) ([]source.Track, error) {
					if calls.Add(1) == 1 {
						return nil, tc.first
					}
					return tracks("c"), nil
				}})
			require.NoError(t, p.Play(ctx, tracks("a", "b"), 1, Origin{}))
			require.Eventually(t, func() bool { return calls.Load() >= 1 }, 2*time.Second, 5*time.Millisecond)
			time.Sleep(50 * time.Millisecond)
			require.NoError(t, p.SetRepeat(ctx, RepeatOff))
			time.Sleep(50 * time.Millisecond)
			assert.Equal(t, tc.want, queueIDs(t, ctx, p))
		})
	}
}
