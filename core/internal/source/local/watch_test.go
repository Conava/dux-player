package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/library"
)

// Set once: watchers of earlier tests may still be reading them.
func init() { debounce, maxDelay = 50*time.Millisecond, time.Second }

// state is title → unavailable reason for every indexed track.
func state(t *testing.T, s *Source) map[string]string {
	t.Helper()
	out := map[string]string{}
	albums, err := s.Albums(context.Background(), "")
	require.NoError(t, err)
	for _, a := range albums {
		full, err := s.Album(context.Background(), a.ID)
		require.NoError(t, err)
		for _, tr := range full.Tracks {
			out[tr.Title] = tr.Unavailable
		}
	}
	return out
}

func TestWatcherKeepsVanishedFoldersAsMissing(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeWAV(t, filepath.Join(root, "A/a.wav"), tags("A1", "Ann", "Ex", "1"))
	writeWAV(t, filepath.Join(root, "A/b.wav"), tags("A2", "Ann", "Ex", "2"))
	writeWAV(t, filepath.Join(root, "C/c.wav"), tags("C1", "Cid", "Sea", "1"))
	s, _ := newLibrary(t, root, nil)
	_, err := s.Scan(ctx, nil)
	require.NoError(t, err)
	// The watcher adds its watches in the background.
	time.Sleep(200 * time.Millisecond)

	writeWAV(t, filepath.Join(root, "B/new.wav"), tags("B1", "Bo", "Bee", "1"))
	require.NoError(t, os.Remove(filepath.Join(root, "A/b.wav")))
	require.NoError(t, os.RemoveAll(filepath.Join(root, "C")))
	want := map[string]string{"A1": "", "B1": "", "C1": library.TrackMissing.Message}
	require.Eventually(t, func() bool { return assert.ObjectsAreEqual(want, state(t, s)) }, 5*time.Second, 20*time.Millisecond,
		"a file deleted from a folder that stays is dropped, a vanished folder is kept as missing")

	writeWAV(t, filepath.Join(root, "C/c.wav"), tags("C1", "Cid", "Sea", "1"))
	require.Eventually(t, func() bool { return state(t, s)["C1"] == "" }, 5*time.Second, 20*time.Millisecond, "a folder that comes back is available again")
}

func TestScanOfAnEmptiedFolderKeepsItsTracks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeWAV(t, filepath.Join(root, "A/a.wav"), tags("A1", "Ann", "Ex", "1"))
	s, _ := newLibrary(t, root, nil)
	_, err := s.Scan(ctx, nil)
	require.NoError(t, err)

	// What an unmounted drive leaves behind: an empty mount point.
	require.NoError(t, os.RemoveAll(filepath.Join(root, "A")))
	stats, err := s.Scan(ctx, nil)
	require.NoError(t, err)
	assert.Zero(t, stats.Removed)
	assert.Equal(t, map[string]string{"A1": library.TrackMissing.Message}, state(t, s))
	s.checkRoots(ctx)
	assert.NotEmpty(t, s.Unavailable(), "a library whose only folder is gone is unavailable")
}

// A watched change reports which tracks it added, changed and removed.
func TestWatcherReportsWhatChanged(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	a, b := filepath.Join(root, "A/a.wav"), filepath.Join(root, "A/b.wav")
	writeWAV(t, a, tags("A1", "Ann", "Ex", "1"))
	writeWAV(t, b, tags("A2", "Ann", "Ex", "2"))
	s, store := newLibrary(t, root, nil)
	deltas := make(chan *library.Delta, 10)
	store.OnChange(func(_ string, d *library.Delta) { deltas <- d })
	_, err := s.Scan(ctx, nil)
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond)

	c := filepath.Join(root, "A/c.wav")
	writeWAV(t, c, tags("A3", "Ann", "Ex", "3"))
	writeWAV(t, a, tags("A1 (live)", "Ann", "Ex", "1"))
	require.NoError(t, os.Remove(b))
	got := library.Delta{}
	require.Eventually(t, func() bool {
		select {
		case d := <-deltas:
			require.NotNil(t, d)
			got.Added = append(got.Added, d.Added...)
			got.Changed = append(got.Changed, d.Changed...)
			got.Removed = append(got.Removed, d.Removed...)
		default:
		}
		return len(got.Added) == 1 && len(got.Changed) == 1 && len(got.Removed) == 1
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, library.Delta{Added: []string{hashID(c)}, Changed: []string{hashID(a)}, Removed: []string{hashID(b)}}, got)
}
