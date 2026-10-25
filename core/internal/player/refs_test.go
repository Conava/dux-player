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

// A long list plays once the played ref and the next few resolve; the rest
// joins the queue in list order as it resolves, minus refs that don't.
func TestPlayRefsStartsBeforeTheListResolves(t *testing.T) {
	tests := []struct {
		name    string
		total   int
		index   int
		missing map[string]bool
		want    int // queue length once everything resolved
	}{
		{"from the start", 30, 0, nil, 30},
		{"from the middle", 30, 12, nil, 30},
		{"some refs gone", 30, 5, map[string]bool{"t02": true, "t20": true}, 28},
		{"the played ref gone", 30, 5, map[string]bool{"t05": true}, 29},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := New(Config{Binary: filepath.Join(t.TempDir(), "no-mpv"), Resolver: skipper{}})
			go p.Run(ctx)
			require.NoError(t, p.SetRemote(ctx, &fakeRemote{}, false))

			refs := make([]source.Ref, tt.total)
			for i := range refs {
				refs[i] = source.Ref{Library: "l", ID: "t" + pad(i)}
			}
			// everything after the first round (the head) waits until released
			release := make(chan struct{})
			var once sync.Once
			var mu sync.Mutex
			calls := 0
			resolve := func(ctx context.Context, rs []source.Ref) ([]source.Track, int) {
				mu.Lock()
				calls++
				first := calls == 1
				mu.Unlock()
				if !first {
					<-release
				}
				var out []source.Track
				for _, r := range rs {
					if !tt.missing[r.ID] {
						out = append(out, source.Track{Library: r.Library, ID: r.ID, Title: r.ID})
					}
				}
				return out, len(rs) - len(out)
			}
			done := make(chan error, 1)
			go func() { done <- p.PlayRefs(ctx, refs, tt.index, Origin{Kind: "adhoc"}, false, resolve) }()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(2 * time.Second):
				once.Do(func() { close(release) })
				t.Fatal("play waited for the whole list")
			}
			q, err := p.Queue(ctx)
			require.NoError(t, err)
			assert.Less(t, len(q.Items), tt.want)
			once.Do(func() { close(release) })
			assert.Eventually(t, func() bool {
				q, err := p.Queue(ctx)
				return err == nil && len(q.Items) == tt.want
			}, 2*time.Second, 5*time.Millisecond)
			q, _ = p.Queue(ctx)
			ids := make([]string, len(q.Items))
			for i, it := range q.Items {
				ids[i] = it.Track.ID
				assert.False(t, it.Added)
			}
			assert.IsIncreasing(t, ids)
			st, err := p.Status(ctx)
			require.NoError(t, err)
			wantCur := refs[tt.index].ID
			if tt.missing[wantCur] {
				wantCur = refs[tt.index+1].ID
			}
			assert.Equal(t, wantCur, st.Track.ID)
		})
	}
}

func pad(i int) string {
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
