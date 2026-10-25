package player

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

type skipper map[string]bool

func (s skipper) Stream(_ context.Context, _ source.Ref) (Stream, error) { return Stream{}, nil }
func (s skipper) Skip(ref source.Ref) bool                               { return s[ref.ID] }
func (s skipper) Failed(source.Ref, func())                              {}

func TestQueueSkipsUnavailable(t *testing.T) {
	tests := []struct {
		name   string
		skip   skipper
		repeat string
		want   []string // current after each Advance, until it fails
	}{
		{"passes over", skipper{"b": true, "c": true}, RepeatOff, []string{"d"}},
		{"end", skipper{"b": true, "c": true, "d": true}, RepeatOff, nil},
		{"wraps with repeat all", skipper{"c": true, "d": true}, RepeatAll, []string{"b", "a", "b"}},
		{"nothing playable", skipper{"a": true, "b": true, "c": true, "d": true}, RepeatAll, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newQueue(tt.skip)
			q.Replace(tracks("a", "b", "c", "d"), 0, Origin{})
			_ = q.SetRepeat(tt.repeat)
			var got []string
			for len(got) < len(tt.want) && q.Advance() {
				got = append(got, current(q))
			}
			assert.Equal(t, tt.want, got)
			if len(tt.want) == 0 {
				assert.False(t, q.Advance())
			}
		})
	}
}

// failer is a library whose server turns out to be down once a track fails.
type failer struct {
	mu   sync.Mutex
	down bool
}

func (f *failer) Stream(context.Context, source.Ref) (Stream, error) { return Stream{}, nil }
func (f *failer) Skip(ref source.Ref) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.down && ref.ID != "c"
}
func (f *failer) Failed(_ source.Ref, recheck func()) {
	go func() {
		f.mu.Lock()
		f.down = true
		f.mu.Unlock()
		recheck()
	}()
}

// A track that fails to load has its library checked; once that says the
// library is down, the queue moves past its tracks instead of trying each.
func TestFailedTrackRechecksTheQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &failer{}
	p := New(Config{Binary: filepath.Join(t.TempDir(), "no-mpv"), Resolver: f})
	go p.Run(ctx)
	r := &fakeRemote{}
	require.NoError(t, p.SetRemote(ctx, r, false))
	require.NoError(t, p.Play(ctx, tracks("a", "b", "c"), 0, Origin{}))
	require.NoError(t, p.do(ctx, func(context.Context) error {
		// mpv reports the current entry failed and moves on to the next.
		p.curEntry, p.nextEntry = 7, 8
		p.onEndFile(message{Event: "end-file", EntryID: 7, Reason: "error"})
		return nil
	}))
	assert.Eventually(t, func() bool {
		st, err := p.Status(ctx)
		return err == nil && st.Track != nil && st.Track.ID == "c"
	}, 2*time.Second, 5*time.Millisecond)
}

// Plain play puts the list in order and turns shuffle off; PlayShuffled turns it on.
func TestPlayTurnsShuffleOff(t *testing.T) {
	tests := []struct {
		name     string
		before   bool
		shuffled bool
		want     bool
	}{
		{"play after shuffle", true, false, false},
		{"play without shuffle", false, false, false},
		{"shuffled play", false, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := New(Config{Binary: filepath.Join(t.TempDir(), "no-mpv"), Resolver: skipper{}})
			go p.Run(ctx)
			require.NoError(t, p.SetRemote(ctx, &fakeRemote{}, false))
			require.NoError(t, p.SetShuffle(ctx, tt.before))
			if tt.shuffled {
				require.NoError(t, p.PlayShuffled(ctx, tracks("a", "b", "c"), Origin{}))
			} else {
				require.NoError(t, p.Play(ctx, tracks("a", "b", "c"), 1, Origin{}))
			}
			st, err := p.Status(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.want, st.Shuffle)
			if !tt.shuffled {
				q, err := p.Queue(ctx)
				require.NoError(t, err)
				assert.Equal(t, "b", st.Track.ID)
				assert.Len(t, q.Items, 3)
			}
		})
	}
}
