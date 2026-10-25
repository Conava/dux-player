// Package shuffle draws tracks uniformly over several libraries: each pick
// chooses a library weighted by the tracks it has left, then a random track
// from it, however each library delivers its tracks.
package shuffle

import (
	"context"
	"math/rand/v2"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/source"
)

const (
	// fetchSize is how many tracks one refill asks for; servers return less
	// (Subsonic caps at 500).
	fetchSize = 2000
	// A library whose refills bring nothing new (or fail) this many times in
	// a row counts as exhausted, so a short count can't loop forever.
	maxDryFetches = 3
)

// Pool is one library in the draw.
type Pool struct {
	Name  string
	Total int
	Fetch func(ctx context.Context, n int) ([]source.Track, error)
}

type pool struct {
	Pool
	buf   []source.Track
	taken int
	dry   int
	done  bool
	// failed sits the pool out for the rest of one Next after a fetch error.
	failed bool
}

func (p *pool) left() int {
	if p.done || p.failed {
		return 0
	}
	return max(p.Total-p.taken, 0)
}

// Shuffler hands out each track at most once per session. It is not safe for
// concurrent use.
type Shuffler struct {
	rng   *rand.Rand
	pools []*pool
	seen  map[source.Ref]bool
}

func New(pools []Pool, rng *rand.Rand) *Shuffler {
	s := &Shuffler{rng: rng, seen: map[source.Ref]bool{}}
	for _, p := range pools {
		if p.Total > 0 {
			s.pools = append(s.pools, &pool{Pool: p})
		}
	}
	return s
}

// Next returns up to n tracks; fewer (or none) once every library is used
// up. It fails only when nothing came back because a library couldn't be
// asked; the next call asks it again.
func (s *Shuffler) Next(ctx context.Context, n int) ([]source.Track, error) {
	var out []source.Track
	var fetchErr error
	defer func() {
		for _, p := range s.pools {
			p.failed = false
		}
	}()
	for len(out) < n {
		p := s.pick()
		if p == nil {
			break
		}
		t, ok, err := s.take(ctx, p)
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if err != nil {
			fetchErr = err
		}
		if ok {
			out = append(out, t)
		}
	}
	if len(out) == 0 && fetchErr != nil {
		return nil, fetchErr
	}
	return out, nil
}

func (s *Shuffler) pick() *pool {
	sum := 0
	for _, p := range s.pools {
		sum += p.left()
	}
	if sum == 0 {
		return nil
	}
	r := s.rng.IntN(sum)
	for _, p := range s.pools {
		if r < p.left() {
			return p
		}
		r -= p.left()
	}
	return nil
}

// take returns the pool's next unseen track, refilling its buffer once if
// needed. false means this pick produced nothing; the pool may now be done.
func (s *Shuffler) take(ctx context.Context, p *pool) (source.Track, bool, error) {
	for len(p.buf) > 0 {
		t := p.buf[0]
		p.buf = p.buf[1:]
		if s.seen[t.Ref()] {
			continue
		}
		s.seen[t.Ref()] = true
		p.taken++
		return t, true, nil
	}
	got, err := p.Fetch(ctx, min(p.left(), fetchSize))
	if err != nil {
		log.Warnf("shuffle: %s: %v", p.Name, err)
		p.dry++
		p.done = p.dry >= maxDryFetches
		p.failed = true
		return source.Track{}, false, err
	}
	fresh := 0
	for _, t := range got {
		if !s.seen[t.Ref()] {
			fresh++
		}
	}
	if fresh == 0 {
		p.dry++
		p.done = p.dry >= maxDryFetches
		return source.Track{}, false, nil
	}
	p.dry = 0
	// The library's own order is random already; shuffling again keeps a
	// source that returns sorted batches from biasing the draw.
	s.rng.Shuffle(len(got), func(i, j int) { got[i], got[j] = got[j], got[i] })
	p.buf = got
	return s.take(ctx, p)
}
