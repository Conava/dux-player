package tagwrite

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.senan.xyz/taglib"
	"golang.org/x/sys/unix"
)

// fixture copies a test file into a temp dir with an extra tag and a cover.
func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, data, 0o640))
	require.NoError(t, taglib.WriteTags(path, map[string][]string{taglib.Comment: {"keep me"}}, 0))
	require.NoError(t, taglib.WriteImage(path, []byte("\xff\xd8\xff\xe0fakejpeg")))
	return path
}

var both = Fields{Favorite: true, Rating: true}

func TestWriteRoundTrip(t *testing.T) {
	for _, name := range []string{"eg.mp3", "eg.flac", "eg.ogg", "eg.m4a"} {
		t.Run(name, func(t *testing.T) {
			path := fixture(t, name)
			before, err := taglib.ReadTags(path)
			require.NoError(t, err)

			changed, err := Write(path, Mark{Favorite: true, Rating: 4}, both)
			require.NoError(t, err)
			assert.True(t, changed)
			got, err := taglib.ReadTags(path)
			require.NoError(t, err)
			assert.Equal(t, []string{"80"}, got[keyRating])
			assert.Equal(t, []string{"0.8"}, got[keyFMPS])
			assert.Equal(t, []string{loved}, got[keyLove])
			for k, v := range before {
				assert.Equal(t, v, got[k], "tag %s is kept", k)
			}
			img, err := taglib.ReadImage(path)
			require.NoError(t, err)
			assert.NotEmpty(t, img, "the cover is kept")
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())

			changed, err = Write(path, Mark{}, both)
			require.NoError(t, err)
			assert.True(t, changed)
			got, err = taglib.ReadTags(path)
			require.NoError(t, err)
			for _, k := range []string{keyRating, keyFMPS, keyLove} {
				assert.NotContains(t, got, k)
			}
			changed, err = Write(path, Mark{}, both)
			require.NoError(t, err)
			assert.False(t, changed, "nothing to change rewrites nothing")
			left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".*"))
			assert.Empty(t, left, "no temp copy is left")
		})
	}
}

func TestWriteKeepsBan(t *testing.T) {
	path := fixture(t, "eg.flac")
	require.NoError(t, taglib.WriteTags(path, map[string][]string{keyLove: {"B"}}, 0))
	_, err := Write(path, Mark{Rating: 2}, both)
	require.NoError(t, err)
	got, err := taglib.ReadTags(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"B"}, got[keyLove])
	assert.Equal(t, []string{"40"}, got[keyRating])
}

func TestWriteSkips(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, path string) string
	}{
		{name: "read-only file", setup: func(t *testing.T, path string) string {
			require.NoError(t, os.Chmod(path, 0o444))
			return path
		}},
		{name: "read-only folder", setup: func(t *testing.T, path string) string {
			dir := filepath.Dir(path)
			require.NoError(t, os.Chmod(dir, 0o555))
			t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
			return path
		}},
		{name: "hard link", setup: func(t *testing.T, path string) string {
			require.NoError(t, os.Link(path, path+".link.flac"))
			return path
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t, fixture(t, "eg.flac"))
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			_, err = Write(path, Mark{Rating: 5}, both)
			var se *SkipError
			require.ErrorAs(t, err, &se)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func TestWriteThroughSymlink(t *testing.T) {
	path := fixture(t, "eg.ogg")
	link := filepath.Join(t.TempDir(), "link.ogg")
	require.NoError(t, os.Symlink(path, link))
	_, err := Write(link, Mark{Rating: 1}, both)
	require.NoError(t, err)
	fi, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, fi.Mode()&os.ModeSymlink, "the link stays a link")
	got, err := taglib.ReadTags(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"20"}, got[keyRating])
}

// The playing file is written only once another track is current; a
// newer mark replaces a queued one; a pass reports when it is done.
func TestWriterQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	var written []Item
	events := make(chan map[string]any, 10)
	w := New(func(_ string, v any) { events <- v.(map[string]any) })
	w.write = func(path string, m Mark, f Fields) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		written = append(written, Item{Path: path, Mark: m, Fields: f})
		if path == "/ro" {
			return false, skip(CodeReadOnlyFile, "Read-only file")
		}
		return true, nil
	}
	got := func() []Item {
		mu.Lock()
		defer mu.Unlock()
		return append([]Item(nil), written...)
	}
	w.SetCurrent("/a")
	w.Enqueue(Item{Path: "/a", Mark: Mark{Rating: 1}})
	w.Enqueue(Item{Path: "/a", Mark: Mark{Rating: 3}})
	require.NoError(t, w.WriteAll("lib", []Item{{Path: "/b", Mark: Mark{Favorite: true}}, {Path: "/ro"}}))
	go w.Run(ctx)

	ev := <-events
	assert.Equal(t, "done", ev["type"])
	assert.Equal(t, Job{Library: "lib", Done: 2, Total: 2, Written: 1, Failed: 1}, ev["job"])
	assert.Equal(t, []Item{{Path: "/b", Mark: Mark{Favorite: true}}, {Path: "/ro"}}, got())
	st := w.Status()
	assert.True(t, st.Waiting)
	assert.Len(t, st.Problems, 1)
	assert.True(t, st.Problems[0].Skipped)
	require.NotNil(t, st.Last, "the finished pass is kept")
	assert.Equal(t, "done", st.Last.State)
	assert.Equal(t, Job{Library: "lib", Done: 2, Total: 2, Written: 1, Failed: 1}, st.Last.Job)
	w.Fail("lib", CodeListFailed, "x")
	<-events
	assert.Equal(t, "failed", w.Status().Last.State, "a pass that couldn't start replaces it")

	w.SetCurrent("/c")
	assert.Eventually(t, func() bool { return len(got()) == 3 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, Item{Path: "/a", Mark: Mark{Rating: 3}}, got()[2])
}

// Only the marks changed in dplayer are written: a rating or love another
// player wrote stays unless the user cleared that mark here.
func TestWriteKeepsOtherPlayersMarks(t *testing.T) {
	other := map[string][]string{keyRating: {"60"}, keyFMPS: {"0.6"}, keyLove: {loved}}
	tests := []struct {
		name   string
		mark   Mark
		fields Fields
		want   map[string][]string
	}{
		{"favorite only", Mark{Favorite: true}, Fields{Favorite: true}, other},
		{"unfavorite keeps the rating", Mark{}, Fields{Favorite: true}, map[string][]string{keyRating: {"60"}, keyFMPS: {"0.6"}}},
		{"rating only", Mark{Rating: 2}, Fields{Rating: true}, map[string][]string{keyRating: {"40"}, keyFMPS: {"0.4"}, keyLove: {loved}}},
		{"rating cleared", Mark{}, Fields{Rating: true}, map[string][]string{keyLove: {loved}}},
		{"nothing changed", Mark{}, Fields{}, other},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fixture(t, "eg.flac")
			require.NoError(t, taglib.WriteTags(path, other, 0))
			_, err := Write(path, tt.mark, tt.fields)
			require.NoError(t, err)
			got, err := taglib.ReadTags(path)
			require.NoError(t, err)
			for _, k := range []string{keyRating, keyFMPS, keyLove} {
				assert.Equal(t, tt.want[k], got[k], k)
			}
		})
	}
}

// A queued mark keeps its fields when a newer one or a pass joins it, and a
// pass's listing doesn't replace a mark the user set since.
func TestWriterMergesQueuedMarks(t *testing.T) {
	tests := []struct {
		name  string
		queue func(w *Writer)
		want  Item
	}{
		{"rating then favorite", func(w *Writer) {
			w.Enqueue(Item{Path: "/a", Mark: Mark{Rating: 3}, Fields: Fields{Rating: true}})
			w.Enqueue(Item{Path: "/a", Mark: Mark{Rating: 3, Favorite: true}, Fields: Fields{Favorite: true}})
		}, Item{Path: "/a", Mark: Mark{Rating: 3, Favorite: true}, Fields: both}},
		{"a pass after a newer mark", func(w *Writer) {
			w.Enqueue(Item{Path: "/a", Mark: Mark{Rating: 5}, Fields: Fields{Rating: true}})
			require.NoError(t, w.WriteAll("lib", []Item{{Path: "/a", Mark: Mark{Rating: 1, Favorite: true}, Fields: both}}))
		}, Item{Path: "/a", Mark: Mark{Rating: 5}, Fields: both}},
		{"a mark after a pass", func(w *Writer) {
			require.NoError(t, w.WriteAll("lib", []Item{{Path: "/a", Mark: Mark{Rating: 1}, Fields: Fields{Rating: true}}}))
			w.Enqueue(Item{Path: "/a", Mark: Mark{Rating: 1, Favorite: true}, Fields: Fields{Favorite: true}})
		}, Item{Path: "/a", Mark: Mark{Rating: 1, Favorite: true}, Fields: both}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := New(nil)
			tt.queue(w)
			e, ok := w.next()
			require.True(t, ok)
			assert.Equal(t, tt.want, e.Item)
			_, ok = w.next()
			assert.False(t, ok)
		})
	}
}

// A copy left by a write that never finished goes with the next write.
func TestWriteSweepsStaleCopies(t *testing.T) {
	path := fixture(t, "eg.flac")
	dir := filepath.Dir(path)
	for _, name := range []string{".eg.flac.4242.flac", ".eg.flac.notes"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}
	_, err := Write(path, Mark{Rating: 1}, both)
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, ".eg.flac.4242.flac"))
	assert.FileExists(t, filepath.Join(dir, ".eg.flac.notes"))
}

func TestWriteKeepsXattrs(t *testing.T) {
	path := fixture(t, "eg.flac")
	if err := unix.Setxattr(path, "user.note", []byte("keep"), 0); err != nil {
		t.Skipf("no user xattrs here: %v", err)
	}
	_, err := Write(path, Mark{Rating: 3}, both)
	require.NoError(t, err)
	buf := make([]byte, 16)
	n, err := unix.Getxattr(path, "user.note", buf)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(buf[:n]))
}
