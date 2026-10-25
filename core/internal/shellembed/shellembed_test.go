package shellembed

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// qs runs whatever is in the extraction dir, so one that others can write
// to (a planted /tmp dir) is refused.
func TestExtractionDirMustBePrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dplayer-shell")
	require.NoError(t, privateDir(dir))
	require.NoError(t, os.Chmod(dir, 0o755))
	assert.Error(t, privateDir(dir))
}
