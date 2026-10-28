package ipc

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	duxipc "github.com/duxlabs-dev/dux-go/ipc"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/source"
	"github.com/Conava/dux-player/core/internal/source/local"
)

type scanned struct {
	lib  source.Library
	done chan struct{}
}

func (s scanned) Library() source.Library                             { return s.lib }
func (s scanned) Track(context.Context, string) (source.Track, error) { return source.Track{}, nil }
func (s scanned) StreamURL(context.Context, string) (string, error)   { return "", nil }
func (s scanned) Close() error                                        { return nil }
func (s scanned) Scan(context.Context, func(int, int)) (source.ScanStats, error) {
	close(s.done)
	return source.ScanStats{}, nil
}

// A library added over IPC or the CLI is scanned without a second request.
func TestAddedLibraryIsScanned(t *testing.T) {
	ctx := context.Background()
	store, err := library.Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	done := make(chan struct{})
	libs := library.NewManager(store, source.Registry{"fake": func(l source.Library, _ source.Env) (source.Source, error) {
		return scanned{lib: l, done: done}, nil
	}}, source.Env{})

	c, peer := net.Pipe()
	t.Cleanup(func() { c.Close(); peer.Close() })
	go func() { _, _ = io.Copy(io.Discard, peer) }()
	deps := Deps{Bus: NewBus(), Libraries: libs, Ctx: ctx}
	handleLibraries(ctx, duxipc.NewConnWriter(c), Request{ID: 1, Method: "libraries.add",
		Params: map[string]any{"kind": "fake", "name": "Music"}}, deps)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the new library wasn't scanned")
	}
}

// A file played by path that a library indexes plays as that library's
// track (cover, now-playing mark); a disabled library or an unknown file
// falls back to the files library.
func TestPlayedPathsMatchLibraryTracks(t *testing.T) {
	ctx := context.Background()
	store, err := library.Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	for _, l := range []source.Library{{ID: "on", Enabled: true}, {ID: "off"}} {
		l.Kind, l.Name = "local", l.ID
		require.NoError(t, store.AddLibrary(ctx, l))
		b, err := store.Begin(ctx, l.ID, 1)
		require.NoError(t, err)
		tr := library.IndexedTrack{Path: "/m/" + l.ID + ".flac"}
		tr.Library, tr.ID, tr.AlbumID, tr.ArtistID = l.ID, "t1", "al", "ar"
		require.NoError(t, b.PutTrack(ctx, tr, ""))
		_, err = b.Finish(ctx)
		require.NoError(t, err)
	}

	tests := []struct {
		path string
		want source.Ref
	}{
		{"/m/on.flac", source.Ref{Library: "on", ID: "t1"}},
		{"/m/off.flac", source.Ref{Library: local.FilesLibrary, ID: "/m/off.flac"}},
		{"/elsewhere/x.flac", source.Ref{Library: local.FilesLibrary, ID: "/elsewhere/x.flac"}},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			require.Equal(t, tt.want, pathRef(ctx, store, tt.path))
		})
	}
}
