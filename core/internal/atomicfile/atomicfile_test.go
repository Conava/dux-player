package atomicfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteFileReplacesAndLeavesNoTemp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	path := filepath.Join(dir, "state.json")

	require.NoError(t, WriteFile(path, []byte("one"), 0o600))
	require.NoError(t, WriteFile(path, []byte("two"), 0o600))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "two", string(data))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	info, err = os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "state.json", entries[0].Name())
}

// A rename that fails (here: over a directory) leaves no temp file behind.
func TestFailedWriteLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "d"), 0o700))
	assert.Error(t, WriteFile(filepath.Join(dir, "d"), []byte("x"), 0o600))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}
