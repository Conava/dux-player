package cache

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

func queueOf(lib, repeat string, ids ...string) player.QueueSnapshot {
	q := player.QueueSnapshot{Repeat: repeat}
	for i, id := range ids {
		q.Items = append(q.Items, player.Item{UID: uint64(i + 1), Track: source.Track{Library: lib, ID: id, Title: id}})
	}
	return q
}

func ids(tracks []source.Track) []string {
	out := []string{}
	for _, t := range tracks {
		out = append(out, t.ID)
	}
	return out
}

func TestUpcoming(t *testing.T) {
	tests := []struct {
		name   string
		repeat string
		queue  []string
		index  int
		n      int
		want   []string
	}{
		{"next in order", player.RepeatOff, []string{"a", "b", "c", "d"}, 0, 2, []string{"b", "c"}},
		{"stops at the end", player.RepeatOff, []string{"a", "b", "c"}, 1, 3, []string{"c"}},
		{"last track", player.RepeatOff, []string{"a", "b"}, 1, 1, []string{}},
		{"repeat all wraps", player.RepeatAll, []string{"a", "b", "c"}, 2, 2, []string{"a", "b"}},
		{"repeat all never reaches the current", player.RepeatAll, []string{"a", "b"}, 0, 3, []string{"b"}},
		{"repeat one plays the same file", player.RepeatOne, []string{"a", "b"}, 0, 1, []string{}},
		{"off", player.RepeatOff, []string{"a", "b"}, 0, 0, []string{}},
		{"index past the queue", player.RepeatOff, []string{"a"}, 3, 1, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ids(upcoming(queueOf("L", tt.repeat, tt.queue...), tt.index, tt.n)))
		})
	}
}

// Metered is NetworkManager's NMMetered: 0 unknown, 1 yes, 2 no, 3 guess
// yes, 4 guess no.
func TestPrefetchCountOnMeteredConnections(t *testing.T) {
	tests := []struct {
		tracks    int
		onMetered bool
		nm        uint32
		want      int
	}{
		{1, false, 0, 1},
		{2, false, 1, 0},
		{2, false, 2, 2},
		{3, false, 3, 0},
		{3, false, 4, 3},
		{2, true, 1, 2},
		{2, true, 3, 2},
		{0, true, 2, 0},
		{9, false, 2, maxPrefetch},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, prefetchCount(tt.tracks, tt.onMetered, library.Metered(tt.nm)), "%+v", tt)
	}
}

// pf runs the prefetch worker over a fake server library.
func pf(t *testing.T) (*env, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	e := setup(t, "", nil, nil)
	go e.c.runPrefetch(ctx)
	return e, ctx
}

func (e *env) at(ctx context.Context, state player.State, index int, q player.QueueSnapshot) {
	q.Index = index
	e.c.QueueChanged(ctx, q)
	st := player.Status{State: state, Index: index}
	if index < len(q.Items) {
		st.Track = &q.Items[index].Track
	}
	e.c.PlayerChanged(ctx, st)
}

// buffer waits until the buffer holds complete copies of want and its
// folder no other file.
func (e *env) buffer(t *testing.T, want ...string) {
	t.Helper()
	slices.Sort(want)
	var got []string
	var files []string
	ok := assert.Eventually(t, func() bool {
		e.c.mu.Lock()
		got = []string{}
		for ref, b := range e.c.pre.buf {
			if b.ready {
				got = append(got, ref.ID)
			}
		}
		e.c.mu.Unlock()
		slices.Sort(got)
		files, _ = filepath.Glob(filepath.Join(e.c.cfg.Dir, prefetchDir, "*"))
		return slices.Equal(want, got) && len(files) == len(want)
	}, 10*time.Second, 5*time.Millisecond)
	require.True(t, ok, "buffer %v, files %v, want %v", got, files, want)
}

// ahead is what the worker downloads next, in order.
func (e *env) ahead() []string {
	e.c.mu.Lock()
	defer e.c.mu.Unlock()
	out := []string{}
	for _, ref := range e.c.pre.ahead {
		out = append(out, ref.ID)
	}
	return out
}

func (e *env) hits(id string) int {
	e.cat.mu.Lock()
	defer e.cat.mu.Unlock()
	return e.cat.hits[id]
}

// The buffer follows the queue: the next tracks are downloaded, the current
// one keeps its copy, and whatever leaves the window is deleted.
func TestPrefetchFollowsTheQueue(t *testing.T) {
	e, ctx := pf(t)
	e.c.SetPrefetch(ctx, 2, false)
	q := queueOf(e.lib, player.RepeatOff, "a", "b", "c", "d")

	e.at(ctx, player.StateStopped, 0, q)
	assert.Empty(t, e.ahead(), "nothing is fetched while nothing plays")

	e.at(ctx, player.StatePlaying, 0, q)
	assert.Equal(t, []string{"b", "c"}, e.ahead())
	e.buffer(t, "b", "c")
	st, err := e.m.Stream(ctx, source.Ref{Library: e.lib, ID: "b"})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(st.URL, filepath.Join(e.c.cfg.Dir, prefetchDir)), "the next track plays from its copy")

	e.at(ctx, player.StatePlaying, 1, q)
	e.buffer(t, "b", "c", "d")

	assert.Zero(t, e.hits("a"), "the track playing first streams")

	e.at(ctx, player.StatePlaying, 0, queueOf(e.lib, player.RepeatOff, "d", "c", "a"))
	e.buffer(t, "d", "c", "a")

	e.at(ctx, player.StatePlaying, 0, queueOf(e.lib, player.RepeatOff, "c", "b"))
	e.buffer(t, "c", "b")

	e.at(ctx, player.StateStopped, 0, q)
	e.buffer(t)
	assert.Equal(t, 2, e.hits("b"), "b left the window and came back")
	assert.Equal(t, 1, e.hits("c"))
}

// A download that leaves the window stops, and the next one starts.
func TestPrefetchCancelsWhatLeavesTheWindow(t *testing.T) {
	e, ctx := pf(t)
	e.c.SetPrefetch(ctx, 1, false)
	e.cat.mu.Lock()
	e.cat.hold["slow"] = make(chan struct{})
	e.cat.mu.Unlock()

	e.at(ctx, player.StatePlaying, 0, queueOf(e.lib, player.RepeatOff, "a", "slow", "c"))
	require.Eventually(t, func() bool { return e.hits("slow") == 1 }, 10*time.Second, 5*time.Millisecond)

	e.at(ctx, player.StatePlaying, 0, queueOf(e.lib, player.RepeatOff, "a", "c", "slow"))
	e.buffer(t, "c")
	assert.Nil(t, e.c.buffered(source.Ref{Library: e.lib, ID: "slow"}))
}

// Turning prefetch on while a track plays starts it; the playing track
// itself streams.
func TestPrefetchSettingAppliesLive(t *testing.T) {
	e, ctx := pf(t)
	e.c.SetPrefetch(ctx, 0, false)
	q := queueOf(e.lib, player.RepeatOff, "a", "b")
	e.at(ctx, player.StatePlaying, 0, q)
	assert.Empty(t, e.ahead())
	e.c.SetPrefetch(ctx, 1, false)
	e.buffer(t, "b")
	assert.Zero(t, e.hits("a"))
}

// A metered connection stops prefetching unless the user allowed it.
func TestPrefetchOnMetered(t *testing.T) {
	e, ctx := pf(t)
	var onChange func(bool)
	e.c.cfg.WatchMetered = func(_ context.Context, f func(bool)) bool { onChange = f; return true }
	e.c.SetPrefetch(ctx, 1, false)
	q := queueOf(e.lib, player.RepeatOff, "a", "b")
	e.at(ctx, player.StatePlaying, 0, q)
	assert.Empty(t, e.ahead())
	onChange(false)
	e.buffer(t, "b")
	onChange(true)
	e.buffer(t)
	e.c.SetPrefetch(ctx, 1, true)
	e.buffer(t, "b")
}

// A copy that leaves the buffer while an offline rule wants its track
// becomes a cached copy, without a second download.
func TestPrefetchedCopyGoesToTheCache(t *testing.T) {
	e, ctx := pf(t)
	e.setRules(t, album("A1"))
	e.c.Sync(ctx, e.lib, all)
	e.c.SetPrefetch(ctx, 1, false)
	q := queueOf(e.lib, player.RepeatOff, "x", "a", "y")
	e.at(ctx, player.StatePlaying, 0, q)
	e.buffer(t, "a")
	e.at(ctx, player.StatePlaying, 2, q)
	e.buffer(t)
	require.Eventually(t, func() bool { return assert.ObjectsAreEqual([]string{"a"}, cached(t, e.c)) },
		10*time.Second, 5*time.Millisecond)
	e.drain(t)
	assert.Equal(t, 1, e.hits("a"))
	assert.ElementsMatch(t, []string{"a", "b"}, cached(t, e.c))
	st, err := e.m.Stream(ctx, source.Ref{Library: e.lib, ID: "a"})
	require.NoError(t, err)
	_, err = os.Stat(st.URL)
	assert.NoError(t, err, "the cached copy outlives the buffer's")
}

// A probe of a track the prefetch downloads waits for it and takes its gain
// instead of fetching the track a second time.
func TestProbeWaitsForThePrefetch(t *testing.T) {
	meter := loudness.Find()
	if !meter.Available() {
		t.Skip("ffmpeg not installed")
	}
	e, ctx := pf(t)
	e.c.cfg.Meter = meter
	file := filepath.Join(t.TempDir(), "b.flac")
	require.NoError(t, exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i",
		"sine=f=440:r=8000:d=10", file).Run())
	e.cat.mu.Lock()
	e.cat.audio = map[string]string{"b": file}
	e.cat.hold["b"] = make(chan struct{})
	e.cat.mu.Unlock()
	e.c.SetPrefetch(ctx, 1, false)
	e.at(ctx, player.StatePlaying, 0, queueOf(e.lib, player.RepeatOff, "a", "b"))
	require.Eventually(t, func() bool { return e.hits("b") == 1 }, 10*time.Second, 5*time.Millisecond)

	got := make(chan float64, 1)
	go func() {
		g, err := e.c.Probe(ctx, source.Ref{Library: e.lib, ID: "b"})
		assert.NoError(t, err)
		got <- g
	}()
	time.Sleep(50 * time.Millisecond)
	e.cat.mu.Lock()
	close(e.cat.hold["b"])
	e.cat.mu.Unlock()
	select {
	case <-got:
	case <-ctx.Done():
		t.Fatal("the probe did not finish")
	}
	assert.Equal(t, 1, e.hits("b"))
}

// A track that starts playing before its copy is finished streams; its
// download stops instead of fetching the track a second time.
func TestPrefetchStopsWhenItsTrackStarts(t *testing.T) {
	e, ctx := pf(t)
	e.c.SetPrefetch(ctx, 1, false)
	e.cat.mu.Lock()
	e.cat.hold["b"] = make(chan struct{})
	e.cat.mu.Unlock()
	q := queueOf(e.lib, player.RepeatOff, "a", "b")
	e.at(ctx, player.StatePlaying, 0, q)
	require.Eventually(t, func() bool { return e.hits("b") == 1 }, 10*time.Second, 5*time.Millisecond)

	e.at(ctx, player.StatePlaying, 1, q)
	assert.Nil(t, e.c.buffered(source.Ref{Library: e.lib, ID: "b"}))
	e.buffer(t)
}

// While casting nothing is prefetched; taking playback back resumes it.
func TestNoPrefetchWhileCasting(t *testing.T) {
	e, ctx := pf(t)
	e.c.SetPrefetch(ctx, 1, false)
	q := queueOf(e.lib, player.RepeatOff, "a", "b")
	e.c.QueueChanged(ctx, q)
	st := player.Status{State: player.StatePlaying, Track: &q.Items[0].Track, Casting: true}
	e.c.PlayerChanged(ctx, st)
	assert.Empty(t, e.ahead())
	st.Casting = false
	e.c.PlayerChanged(ctx, st)
	e.buffer(t, "b")
	assert.Zero(t, e.hits("a"))
}

// A failed prefetch leaves the track to the probe.
func TestProbeAfterAFailedPrefetch(t *testing.T) {
	meter := loudness.Find()
	if !meter.Available() {
		t.Skip("ffmpeg not installed")
	}
	e, ctx := pf(t)
	e.c.cfg.Meter = meter
	file := filepath.Join(t.TempDir(), "b.flac")
	require.NoError(t, exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i",
		"sine=f=440:r=8000:d=10", file).Run())
	e.cat.mu.Lock()
	e.cat.audio = map[string]string{"b": file}
	e.cat.broken["b"] = 1
	e.cat.mu.Unlock()
	e.c.SetPrefetch(ctx, 1, false)
	e.at(ctx, player.StatePlaying, 0, queueOf(e.lib, player.RepeatOff, "a", "b"))
	require.Eventually(t, func() bool {
		e.c.mu.Lock()
		defer e.c.mu.Unlock()
		b := e.c.pre.buf[source.Ref{Library: e.lib, ID: "b"}]
		return b != nil && b.failed
	}, 10*time.Second, 5*time.Millisecond)

	_, err := e.c.Probe(ctx, source.Ref{Library: e.lib, ID: "b"})
	require.NoError(t, err)
	assert.Equal(t, 2, e.hits("b"))
}
