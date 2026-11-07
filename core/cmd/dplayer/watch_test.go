package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A status bar polls these; none of them may bring a quit daemon back.
func TestObservingDoesNotStartDaemon(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("DPLAYER_SOCKET", "")
	started := 0
	old := spawnDaemon
	spawnDaemon = func() (string, error) { started++; return "", errors.New("stub") }
	t.Cleanup(func() { spawnDaemon = old })

	watchTopics = []string{"status"}
	assert.Error(t, watch(context.Background()))
	statusFormat = "%title%"
	t.Cleanup(func() { statusFormat = "" })
	assert.Error(t, statusCmd.RunE(statusCmd, nil))
	assert.Zero(t, started)
}
