package ipc

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	duxipc "github.com/duxlabs-dev/dux-go/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
	"github.com/Conava/dux-player/core/internal/source/local"
)

func writeWAV(t *testing.T, path string) {
	t.Helper()
	const rate, n = 8000, 8000
	data := make([]byte, 44+2*n)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], 36+2*n)
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], rate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 2*n)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// silent is a cast receiver that plays nothing, so the test needs no audio.
type silent struct{}

func (silent) Load([]player.Item, int, float64, bool) {}
func (silent) SetPaused(bool)                         {}
func (silent) Seek(float64)                           {}
func (silent) SetVolume(float64)                      {}
func (silent) SetLoop(bool)                           {}
func (silent) Stop()                                  {}

// One file with unreadable tags is left out and counted; the rest of the
// folder plays. A folder with nothing readable or no audio files is refused.
func TestPlayFolderSkipsUnreadableFiles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dir := t.TempDir()
	// Keeps the built-in player off the host's sound server.
	t.Setenv("PULSE_SERVER", filepath.Join(dir, "no-pulse"))
	t.Setenv("PIPEWIRE_REMOTE", filepath.Join(dir, "no-pipewire"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(dir, "run"))
	good := filepath.Join(dir, "good")
	bad := filepath.Join(dir, "bad")
	require.NoError(t, os.MkdirAll(good, 0o755))
	empty := filepath.Join(dir, "empty")
	require.NoError(t, os.MkdirAll(bad, 0o755))
	require.NoError(t, os.MkdirAll(empty, 0o755))
	writeWAV(t, filepath.Join(good, "a.wav"))
	writeWAV(t, filepath.Join(good, "b.wav"))
	require.NoError(t, os.WriteFile(filepath.Join(good, "c.flac"), []byte("not audio"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bad, "x.flac"), []byte("not audio"), 0o644))

	store, err := library.Open(ctx, filepath.Join(dir, "library.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	libs := library.NewManager(store, source.Registry{}, source.Env{}, local.Files{})
	t.Cleanup(func() { libs.Close() })
	pl := player.New(player.Config{Binary: filepath.Join(dir, "no-mpv"), Socket: filepath.Join(dir, "mpv.sock"), Resolver: libs})
	done := make(chan struct{})
	go func() { pl.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	require.NoError(t, pl.SetRemote(ctx, silent{}, false))
	deps := Deps{Player: pl, Libraries: libs, Ctx: ctx}

	play := func(path string) (resp struct {
		Error  string `json:"error"`
		Result struct {
			Length  int `json:"length"`
			Skipped int `json:"skipped"`
		} `json:"result"`
	}) {
		server, client := net.Pipe()
		t.Cleanup(func() { server.Close(); client.Close() })
		go handlePlayer(ctx, duxipc.NewConnWriter(server), Request{ID: 1, Method: "player.play", Params: map[string]any{"paths": []any{path}}}, deps)
		require.NoError(t, json.NewDecoder(bufio.NewReader(client)).Decode(&resp))
		return resp
	}

	r := play(good)
	assert.Empty(t, r.Error)
	assert.Equal(t, 2, r.Result.Length)
	assert.Equal(t, 1, r.Result.Skipped)

	r = play(bad)
	assert.Contains(t, r.Error, "none of these files could be read")

	r = play(empty)
	assert.Contains(t, r.Error, "no audio files in these folders")
}
