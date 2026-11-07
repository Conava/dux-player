package cast

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindHelperHonorsCastOff(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "cast-helper"), []byte("#!/bin/sh\n"), 0o755))
	t.Setenv("PATH", dir)
	assert.NotEmpty(t, FindHelper())
	t.Setenv("DPLAYER_CAST", "off")
	assert.Empty(t, FindHelper())
}
