package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenTargets(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.flac", "b.mp3", "Sub/c.opus", "notes.txt"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), nil, 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mix.m3u8"), []byte("#EXTM3U\nb.mp3\nSub/c.opus\nhttp://radio/stream\ngone.flac\n"), 0o644))
	paths, skipped := openTargets([]string{
		"file://" + filepath.Join(dir, "a%2Eflac"), filepath.Join(dir, "mix.m3u8"), filepath.Join(dir, "Sub"),
		filepath.Join(dir, "notes.txt"), "https://example.com/x.mp3",
	})
	assert.Equal(t, []string{filepath.Join(dir, "a.flac"), filepath.Join(dir, "b.mp3"), filepath.Join(dir, "Sub/c.opus"), filepath.Join(dir, "Sub")}, paths)
	assert.Equal(t, 4, skipped, "the stream and missing entry of the playlist, the text file, the https URI")
}
