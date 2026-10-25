package settings

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetValidatesAndKeepsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	require.NoError(t, os.WriteFile(path, []byte(`{"fromTheFuture":{"x":1},"gaplessMode":"yes","replayGainMode":"loud"}`), 0o644))

	s, err := Open(dir)
	require.NoError(t, err)
	assert.Equal(t, "yes", s.Get().GaplessMode)
	assert.Equal(t, "track", s.Get().ReplayGainMode, "a bad value falls back to the default")

	_, err = s.Set(map[string]any{"allowVolumeBoost": "true", "previousThreshold": 99.0})
	require.Error(t, err)
	assert.False(t, s.Get().AllowVolumeBoost, "nothing applies when one value is bad")

	got, err := s.Set(map[string]any{"allowVolumeBoost": "true", "previousThreshold": "8"})
	require.NoError(t, err)
	assert.True(t, got.AllowVolumeBoost)
	assert.Equal(t, 8.0, got.PreviousThreshold)

	var onDisk map[string]any
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &onDisk))
	assert.Equal(t, map[string]any{"x": 1.0}, onDisk["fromTheFuture"])
	assert.Equal(t, true, onDisk["allowVolumeBoost"])

	_, err = s.Set(map[string]any{"volume": 3.0})
	assert.Error(t, err, "unknown keys are rejected on set")

	for _, v := range []any{"NaN", "Inf", math.NaN()} {
		_, err = s.Set(map[string]any{"previousThreshold": v})
		assert.Error(t, err, "%v passes every range check", v)
	}
	assert.Equal(t, 8.0, s.Get().PreviousThreshold)
}

func TestPlayerOptions(t *testing.T) {
	s := Defaults()
	o := s.PlayerOptions()
	assert.Equal(t, 100.0, o.VolumeMax)
	assert.Equal(t, "track", o.ReplayGain)
	assert.Equal(t, 5.0, o.PreviousThreshold)

	s.AllowVolumeBoost, s.ReplayGainMode = true, "off"
	o = s.PlayerOptions()
	assert.Equal(t, 130.0, o.VolumeMax)
	assert.Equal(t, "no", o.ReplayGain, "mpv spells off as no")
}

func TestBrokenFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte(`{"gaplessMode":`), 0o644))
	_, err := Open(dir)
	assert.Error(t, err)
}

func TestPlaybackDSPSettings(t *testing.T) {
	s, err := Open(t.TempDir())
	require.NoError(t, err)
	_, err = s.Set(map[string]any{"speed": "3"})
	assert.Error(t, err, "speed is 0.5 to 2")

	got, err := s.Set(map[string]any{"speed": 1.5, "equalizer": `{"enabled":true,"preset":"bass"}`})
	require.NoError(t, err)
	assert.Equal(t, Presets["bass"], got.Equalizer.Bands, "a preset sets its bands")
	assert.Equal(t, 1.5, got.PlayerOptions().Speed)

	got, err = s.Set(map[string]any{"equalizer": map[string]any{"bands": []any{1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -1.0}}})
	require.NoError(t, err)
	assert.Equal(t, "custom", got.Equalizer.Preset, "own bands make the preset custom")

	_, err = s.Set(map[string]any{"equalizer": map[string]any{"bands": []any{30.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}}})
	assert.Error(t, err, "band gains are capped")

	got, err = s.Set(map[string]any{"equalizer": map[string]any{"enabled": false}})
	require.NoError(t, err)
	assert.False(t, got.Equalizer.Enabled)
}

func TestCrossfadeSettings(t *testing.T) {
	s, err := Open(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, 0.0, s.Get().PlayerOptions().Crossfade, "off by default")
	assert.True(t, s.Get().PlayerOptions().AlbumGapless, "albums stay gapless by default")

	got, err := s.Set(map[string]any{"crossfade": `{"enabled":true}`})
	require.NoError(t, err)
	assert.Equal(t, 5.0, got.PlayerOptions().Crossfade, "a partial object keeps the default length")

	got, err = s.Set(map[string]any{"crossfade": map[string]any{"seconds": 8.0, "albumGapless": false}})
	require.NoError(t, err)
	assert.Equal(t, 8.0, got.PlayerOptions().Crossfade)
	assert.False(t, got.PlayerOptions().AlbumGapless)

	_, err = s.Set(map[string]any{"crossfade": map[string]any{"seconds": 20.0}})
	assert.Error(t, err, "1 to 12 seconds")

	got, err = s.Set(map[string]any{"crossfade": map[string]any{"enabled": false}})
	require.NoError(t, err)
	assert.Equal(t, 0.0, got.PlayerOptions().Crossfade)
	assert.Equal(t, 8.0, got.Crossfade.Seconds, "turning it off keeps the length")
}
