package engine

import (
	"context"
	"sync"
	"time"
)

// Output is where the engine plays: the PulseAudio server, or Null.
type Output interface {
	// Open starts a stream on a sink ("" is the default). Props are
	// extra stream properties; with wait set the stream is never moved to
	// another sink when its own goes away.
	Open(f format, sink string, props map[string]string, wait bool, paused bool) (Stream, error)
	Sinks() ([]Sink, error)
	// Changed signals a change in the sink list; Lost closes when the
	// server is gone.
	Changed() <-chan struct{}
	Lost() <-chan struct{}
	Close()
}

type Sink struct {
	Name        string
	Description string
}

// Stream takes interleaved float32 frames.
type Stream interface {
	// Write blocks until the server took all of frames or ctx ends, and
	// returns how many values it took.
	Write(ctx context.Context, frames []float32) (int, error)
	// Position is how many frames were written and how many of those were
	// heard so far.
	Position() (written, played int64, err error)
	// Flush drops what was written and not yet played.
	Flush() error
	SetPaused(paused bool) error
	// Drain asks for what was written to be played out, the end of a
	// track; Position reports it all played once it is.
	Drain()
	Close()
}

// Null plays nowhere, at the pace of real time. Tests read what was
// played from Frames.
type Null struct {
	mu      sync.Mutex
	frames  []float32
	keep    bool
	streams int
	// OpenErr, when set, fails every Open.
	OpenErr error
	// writeErr, once set, fails every Write.
	writeErr error
	lost     chan struct{}
	changed  chan struct{}
}

// NewNull returns a Null output; keep records every frame written.
func NewNull(keep bool) *Null {
	return &Null{keep: keep, lost: make(chan struct{}), changed: make(chan struct{}, 1)}
}

// Frames returns the frames written so far and how many streams were opened.
func (n *Null) Frames() ([]float32, int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]float32(nil), n.frames...), n.streams
}

// FailWrites makes every Write fail, as an output removed mid-track does.
func (n *Null) FailWrites(err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.writeErr = err
}

// Lose ends the output as a server that went away would.
func (n *Null) Lose() {
	n.mu.Lock()
	defer n.mu.Unlock()
	select {
	case <-n.lost:
	default:
		close(n.lost)
	}
}

func (n *Null) Open(f format, _ string, _ map[string]string, _ bool, paused bool) (Stream, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.OpenErr != nil {
		return nil, n.OpenErr
	}
	n.streams++
	return &nullStream{out: n, f: f, paused: paused}, nil
}

func (n *Null) Sinks() ([]Sink, error) {
	return []Sink{{Name: "null", Description: "Null output"}}, nil
}

func (n *Null) Changed() <-chan struct{} { return n.changed }
func (n *Null) Lost() <-chan struct{}    { return n.lost }
func (n *Null) Close()                   { n.Lose() }

type nullStream struct {
	out *Null
	f   format
	mu  sync.Mutex
	// written frames; played as of playedAt, advancing in real time while
	// not paused.
	written  int64
	played   int64
	playedAt time.Time
	paused   bool
}

// advance moves played on to now; mu held.
func (s *nullStream) advance() {
	now := time.Now()
	if !s.paused && !s.playedAt.IsZero() {
		s.played = min(s.written, s.played+int64(now.Sub(s.playedAt).Seconds()*float64(s.f.rate)))
	}
	s.playedAt = now
}

// nullBuffer is how far ahead of what's heard a writer may be.
const nullBuffer = 0.1

func (s *nullStream) Write(ctx context.Context, frames []float32) (int, error) {
	s.out.mu.Lock()
	err := s.out.writeErr
	s.out.mu.Unlock()
	if err != nil {
		return 0, err
	}
	for {
		s.mu.Lock()
		s.advance()
		ahead := float64(s.written-s.played) / float64(s.f.rate)
		s.mu.Unlock()
		if ahead < nullBuffer {
			break
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-s.out.lost:
			return 0, errLost
		case <-time.After(10 * time.Millisecond):
		}
	}
	s.mu.Lock()
	s.written += int64(len(frames) / s.f.channels)
	s.mu.Unlock()
	if s.out.keep {
		s.out.mu.Lock()
		s.out.frames = append(s.out.frames, frames...)
		s.out.mu.Unlock()
	}
	return len(frames), nil
}

func (s *nullStream) Position() (int64, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.advance()
	return s.written, s.played, nil
}

func (s *nullStream) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.advance()
	s.written = s.played
	return nil
}

func (s *nullStream) SetPaused(paused bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.advance()
	s.paused = paused
	return nil
}

func (s *nullStream) Drain() {}
func (s *nullStream) Close() {}
