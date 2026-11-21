package window

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/duxlabs-dev/dux-go/qtbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A log that passes its limit while the UI runs moves to .1 and the new
// output starts a fresh file.
func TestRotatingLogRotatesWhileRunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui.log")
	l, err := newRotatingLog(path, 10)
	require.NoError(t, err)
	defer l.Close()

	for _, s := range []string{"123456", "789012", "tail"} {
		_, err := l.Write([]byte(s))
		require.NoError(t, err)
	}

	cur, err := os.ReadFile(path)
	require.NoError(t, err)
	old, err := os.ReadFile(path + ".1")
	require.NoError(t, err)
	assert.Equal(t, "789012tail", string(cur))
	assert.Equal(t, "123456", string(old))
}

// The UI starts through the daemon's own binary (the qs subcommand), never a
// bare qs on PATH.
func TestStartRunsTheUIThroughTheQtBaseCommand(t *testing.T) {
	tree := t.TempDir()
	for _, f := range []string{"shell.qml", "DCommon/Widgets/DIcon.qml"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(tree, f)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(tree, f), nil, 0o644))
	}
	l := New(Config{LogPath: filepath.Join(t.TempDir(), "ui.log")})

	started, err := l.Start(tree)

	assert.False(t, started)
	assert.ErrorIs(t, err, qtbase.ErrTestReexec)
}
