package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Conava/dux-player/core/internal/source"
)

// FilesLibrary is the built-in library for files played by path outside any
// configured library. A track's id is its absolute path.
const FilesLibrary = "file"

type Files struct{}

func (Files) Library() source.Library {
	return source.Library{ID: FilesLibrary, Kind: Kind, Name: "Files", Enabled: true}
}

func (Files) Close() error { return nil }

func checkPath(id string) error {
	if !filepath.IsAbs(id) || !IsAudio(id) {
		return fmt.Errorf("%q is not an absolute path to an audio file", id)
	}
	info, err := os.Stat(id)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", id)
	}
	return nil
}

func (Files) Track(_ context.Context, id string) (source.Track, error) {
	if err := checkPath(id); err != nil {
		return source.Track{}, err
	}
	t, err := readTrack(id)
	if err != nil {
		return source.Track{}, fmt.Errorf("read tags of %s: %w", id, err)
	}
	t.Library = FilesLibrary
	t.ID = id
	t.AlbumID, t.ArtistID = "", ""
	return t.Track, nil
}

func (Files) StreamURL(_ context.Context, id string) (string, error) {
	if err := checkPath(id); err != nil {
		return "", err
	}
	return id, nil
}
