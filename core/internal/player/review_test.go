package player

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

// castPlayer runs a player without mpv, playing on a fake remote.
func castPlayer(t *testing.T, cfg Config) (*Player, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cfg.Binary = filepath.Join(t.TempDir(), "no-mpv")
	p := New(cfg)
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()
	// The player saves its state as it stops; let it finish before the
	// test's temp dirs go.
	t.Cleanup(func() {
		cancel()
		<-done
	})
	require.NoError(t, p.SetRemote(ctx, &fakeRemote{}, false))
	return p, ctx
}

func TestPlayAtAnIndexOutOfRangeChangesNothing(t *testing.T) {
	p, ctx := castPlayer(t, Config{})
	require.NoError(t, p.Play(ctx, tracks("a", "b"), 0, Origin{}))
	require.Error(t, p.Play(ctx, tracks("c", "d"), 7, Origin{}))
	q, err := p.Queue(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, q.Index)
	assert.Equal(t, "a", q.Items[0].Track.ID)
}

func TestPreviousPassesBackOverUnavailableTracks(t *testing.T) {
	p, ctx := castPlayer(t, Config{Resolver: skipper{"b": true}})
	require.NoError(t, p.Play(ctx, tracks("a", "b", "c"), 2, Origin{}))
	require.NoError(t, p.Previous(ctx))
	st, err := p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, "a", st.Track.ID)
}

func TestRemovingTheStoppedCurrentTrackStartsTheNextFromZero(t *testing.T) {
	p, ctx := castPlayer(t, Config{})
	require.NoError(t, p.Play(ctx, tracks("a", "b"), 0, Origin{}))
	q, err := p.Queue(ctx)
	require.NoError(t, err)
	require.NoError(t, p.RemoteUpdate(ctx, q.Items[0].UID, 12, false, -1))
	require.NoError(t, p.Stop(ctx))
	require.NoError(t, p.Remove(ctx, 0))
	st, err := p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, "b", st.Track.ID)
	assert.Zero(t, st.Position)
}

// Handing the same track to a cast mid-listen (or restarting mpv for a new
// output) goes on with the listen instead of counting it again.
func TestReloadingTheTrackDoesNotCountItTwice(t *testing.T) {
	played := 0
	p, ctx := castPlayer(t, Config{Publish: func(topic string, _ any) {
		if topic == TopicPlayed {
			played++
		}
	}})
	long := source.Track{Library: "l", ID: "x", Title: "x", Duration: 600}
	require.NoError(t, p.Play(ctx, []source.Track{long}, 0, Origin{}))
	q, err := p.Queue(ctx)
	require.NoError(t, err)
	uid := q.Items[0].UID
	for pos := 1.0; pos <= 600; pos++ {
		if pos == 250 {
			require.NoError(t, p.SetRemote(ctx, &fakeRemote{}, false))
		}
		require.NoError(t, p.RemoteUpdate(ctx, uid, pos, false, -1))
	}
	assert.Equal(t, 1, played)
}

type slowResolver struct{ release chan struct{} }

func (r slowResolver) Stream(ctx context.Context, _ source.Ref) (Stream, error) {
	select {
	case <-r.release:
	case <-ctx.Done():
	}
	return Stream{URL: "/nonexistent"}, nil
}

// A stream URL that takes a server round trip must not hold up the loop.
func TestSlowResolveDoesNotBlockTheLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	res := slowResolver{release: make(chan struct{})}
	t.Cleanup(func() { close(res.release) })
	p := New(Config{Binary: filepath.Join(t.TempDir(), "no-mpv"), Resolver: res})
	go p.Run(ctx)
	go func() { _ = p.Play(ctx, tracks("a"), 0, Origin{}) }()

	answered := make(chan Status, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		st, _ := p.Status(ctx)
		answered <- st
	}()
	select {
	case st := <-answered:
		assert.Equal(t, StatePlaying, st.State, "a loading track shows as playing")
	case <-time.After(2 * time.Second):
		t.Fatal("status waited for the stream lookup")
	}
}

// Real mpv: mute set during a cast holds once playback is local again.
func TestMuteDuringACastHoldsAfterIt(t *testing.T) {
	bin, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not installed")
	}
	dir := t.TempDir()
	res := pathResolver{"long": filepath.Join(dir, "long.wav")}
	writeWAV(t, res["long"], 30)
	p := New(Config{Binary: bin, Socket: filepath.Join(dir, "mpv.sock"), ExtraArgs: []string{"--ao=null"}, Resolver: res})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	require.NoError(t, p.Play(ctx, tracks("long"), 0, Origin{}))
	require.NoError(t, p.SetRemote(ctx, &fakeRemote{}, false))
	require.NoError(t, p.SetMute(ctx, true))
	require.NoError(t, p.SetRemote(ctx, nil, true))
	var muted string
	require.NoError(t, p.do(ctx, func(ctx context.Context) error {
		data, err := p.cmd(ctx, "get_property_string", "mute")
		muted = string(data)
		return err
	}))
	assert.Equal(t, `"yes"`, muted)
}

// Real mpv: with nothing playing, mpv goes after IdleExit; the next play
// brings it back.
func TestIdleMPVStopsAndComesBack(t *testing.T) {
	bin, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not installed")
	}
	dir := t.TempDir()
	res := pathResolver{"a": filepath.Join(dir, "a.wav")}
	writeWAV(t, res["a"], 0.3)
	opts := DefaultOptions()
	opts.IdleExit = 300 * time.Millisecond
	p := New(Config{Binary: bin, Socket: filepath.Join(dir, "mpv.sock"), ExtraArgs: []string{"--ao=null"}, Resolver: res, Options: opts})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	running := func() bool {
		var up bool
		_ = p.do(ctx, func(context.Context) error { up = p.proc != nil; return nil })
		return up
	}
	assert.False(t, running(), "mpv starts with the first play")
	require.NoError(t, p.Play(ctx, tracks("a"), 0, Origin{}))
	require.Eventually(t, func() bool { return !running() }, 5*time.Second, 20*time.Millisecond, "idle mpv was kept")
	require.NoError(t, p.Play(ctx, tracks("a"), 0, Origin{}))
	st, err := p.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, StatePlaying, st.State)
}

func TestMPVVerifiesTLS(t *testing.T) {
	args := baseArgs("s")
	assert.Contains(t, args, "--tls-verify=yes")
	assert.Contains(t, args, "--access-references=no")
}

// A shuffle over whole libraries goes on after a restart: the feed is
// rebuilt from the saved origin.
func TestRestoredFeedQueueKeepsGrowing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, writeState(path, savedState{Version: stateVersion, Feed: true,
		Queue: QueueState{Items: []Item{{UID: 1, Track: source.Track{Library: "l", ID: "a"}}}, Index: 0, Origin: Origin{Kind: "tracks"}}}))
	refeed := func(context.Context, Origin) (Feed, error) {
		return &fakeFeed{batches: [][]source.Track{tracks("b", "c")}}, nil
	}
	p, ctx := castPlayer(t, Config{StatePath: path, Refeed: refeed})
	require.NoError(t, p.Resume(ctx))
	require.Eventually(t, func() bool {
		q, _ := p.Queue(ctx)
		return len(q.Items) == 3
	}, 2*time.Second, 10*time.Millisecond)
}

func TestMarksReachThePlayingTrackAndTheSavedQueue(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	p, ctx := castPlayer(t, Config{StatePath: statePath})
	require.NoError(t, p.Play(ctx, tracks("a", "b", "a"), 0, Origin{}))
	fav, stars := true, 4
	require.NoError(t, p.Mark(ctx, source.Ref{Library: "l", ID: "a"}, &fav, &stars))

	st, err := p.Status(ctx)
	require.NoError(t, err)
	assert.True(t, st.Track.Favorite)
	assert.Equal(t, 4, st.Track.Rating)
	q, err := p.Queue(ctx)
	require.NoError(t, err)
	assert.True(t, q.Items[2].Track.Favorite, "every entry of the track")
	assert.False(t, q.Items[1].Track.Favorite)

	require.NoError(t, p.do(ctx, func(context.Context) error { p.dirty = true; p.save(); return nil }))
	saved, err := loadState(statePath)
	require.NoError(t, err)
	assert.True(t, saved.Queue.Items[0].Track.Favorite, "a restart keeps the heart")
}

// A new track reports its first position at once, never the old track's.
func TestPositionAtTrackStart(t *testing.T) {
	var got []Position
	p := New(Config{Publish: func(topic string, d any) {
		if topic == TopicPosition {
			got = append(got, d.(Position))
		}
	}})
	p.active, p.pos, p.lastPos = true, 200, 200
	// What the gapless handover leaves behind: a new track not loaded yet.
	p.pos, p.lastPos, p.loaded = 0, -1, false
	p.onProperty("time-pos", []byte("201.5"))
	assert.Empty(t, got, "a time-pos before file-loaded is the old track's")
	p.handle(message{Event: "file-loaded"})
	p.onProperty("time-pos", []byte("0.02"))
	require.Len(t, got, 1, "the first time-pos is reported, not the one a second in")
	assert.InDelta(t, 0.02, got[0].Position, 0.001)
	assert.True(t, got[0].Playing)
	assert.Equal(t, 1.0, got[0].Speed)
	assert.InDelta(t, time.Now().UnixMilli(), got[0].At, 1000)
}
