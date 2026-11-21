package player

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

func TestStateSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "player-state.json")
	p := New(Config{StatePath: path})
	p.queue.Replace(tracks("a", "b", "c"), 1, Origin{Kind: "playlist", ID: "p", Name: "Mix"})
	require.NoError(t, p.queue.SetRepeat(RepeatAll))
	p.pos, p.volume = 42.5, 60
	p.markDirty()
	p.save()

	r := New(Config{StatePath: path})
	r.restore()
	st := r.status()
	assert.Equal(t, StateStopped, st.State)
	assert.Equal(t, "b", st.Track.ID)
	assert.Equal(t, 42.5, st.Position, "resume starts here")
	assert.Equal(t, 60.0, st.Volume)
	assert.Equal(t, RepeatAll, st.Repeat)
	assert.Equal(t, "Mix", r.queue.Origin().Name)
}

func TestUnreadableStateIsMovedAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "player-state.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":99,"queue":{}}`), 0o600))
	p := New(Config{StatePath: path})
	p.restore()
	assert.Equal(t, 0, p.queue.Len())
	_, err := os.Stat(path + ".unreadable")
	assert.NoError(t, err, "a newer state file is kept, not overwritten")
}

func TestContinueOffSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "player-state.json")
	opts := DefaultOptions()
	opts.AutoContinue = ContinueSimilar
	cont := func(context.Context, string, []source.Track, map[source.Ref]bool) ([]source.Track, error) {
		return nil, nil
	}
	p := New(Config{StatePath: path, Options: opts, Continue: cont})
	p.queue.Replace(tracks("a"), 0, Origin{})
	p.contMode = ContinueOff
	p.markDirty()
	p.save()

	r := New(Config{StatePath: path, Options: opts, Continue: cont})
	r.restore()
	assert.False(t, r.status().Continuing)

	// A state saved before the override existed follows the setting.
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,"queue":{}}`), 0o600))
	old := New(Config{StatePath: path, Options: opts, Continue: cont})
	old.restore()
	assert.Equal(t, "", old.contMode)
}
