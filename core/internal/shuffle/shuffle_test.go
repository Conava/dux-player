package shuffle

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

// fakeLibrary returns random batches capped at batch, with repeats across
// calls, like getRandomSongs.
func fakeLibrary(lib string, total, batch int, rng *rand.Rand) Pool {
	return Pool{Name: lib, Total: total, Fetch: func(_ context.Context, n int) ([]source.Track, error) {
		var out []source.Track
		for range min(n, batch) {
			out = append(out, source.Track{Library: lib, ID: fmt.Sprint(rng.IntN(total))})
		}
		return out, nil
	}}
}

func TestPicksAreUniformOverAllTracks(t *testing.T) {
	tests := []struct {
		name           string
		local, server  int
		wantServerPart float64
	}{
		{name: "equal libraries", local: 1000, server: 1000, wantServerPart: 0.5},
		{name: "server three times larger", local: 1000, server: 3000, wantServerPart: 0.75},
		{name: "small local library", local: 50, server: 5000, wantServerPart: 5000.0 / 5050},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rng := rand.New(rand.NewPCG(1, 2))
			s := New([]Pool{
				fakeLibrary("local", tt.local, 2000, rng),
				fakeLibrary("server", tt.server, 500, rng),
			}, rng)
			got, err := s.Next(context.Background(), 400)
			require.NoError(t, err)
			require.Len(t, got, 400)
			server := 0
			for _, tr := range got {
				if tr.Library == "server" {
					server++
				}
			}
			assert.InDelta(t, tt.wantServerPart, float64(server)/400, 0.06)
		})
	}
}

func TestEveryTrackOnceUntilExhausted(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	s := New([]Pool{fakeLibrary("a", 120, 500, rng), fakeLibrary("b", 80, 50, rng)}, rng)
	seen := map[source.Ref]bool{}
	for {
		batch, err := s.Next(context.Background(), 30)
		require.NoError(t, err)
		if len(batch) == 0 {
			break
		}
		for _, tr := range batch {
			require.False(t, seen[tr.Ref()], "repeated %v", tr.Ref())
			seen[tr.Ref()] = true
		}
	}
	assert.GreaterOrEqual(t, len(seen), 180, "nearly every track came up before the draw ended")
}

func TestFailingLibraryDropsOut(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	broken := Pool{Name: "down", Total: 1000, Fetch: func(context.Context, int) ([]source.Track, error) {
		return nil, fmt.Errorf("server unreachable")
	}}
	s := New([]Pool{broken, fakeLibrary("ok", 100, 500, rng)}, rng)
	got, err := s.Next(context.Background(), 50)
	require.NoError(t, err)
	require.Len(t, got, 50)
	for _, tr := range got {
		assert.Equal(t, "ok", tr.Library)
	}
}

// One failed refill (a server hiccup) doesn't drop a library from the draw.
func TestFetchErrorIsRetried(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	lib := fakeLibrary("srv", 100, 10, rng)
	fail := true
	fetch := lib.Fetch
	lib.Fetch = func(ctx context.Context, n int) ([]source.Track, error) {
		if fail {
			fail = false
			return nil, fmt.Errorf("connection reset")
		}
		return fetch(ctx, n)
	}
	s := New([]Pool{lib}, rng)
	_, err := s.Next(context.Background(), 5)
	require.Error(t, err, "nothing came back: the caller retries later")
	got, err := s.Next(context.Background(), 5)
	require.NoError(t, err)
	assert.Len(t, got, 5)
}
