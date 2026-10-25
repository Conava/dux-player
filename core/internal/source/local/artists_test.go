package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.senan.xyz/taglib"

	"github.com/Conava/dux-player/core/internal/source"
)

func TestMultipleArtists(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeWAV(t, filepath.Join(root, "duo.wav"), map[string][]string{taglib.Title: {"T1"}, taglib.Artist: {"Ann feat. Bob"}, taglib.Album: {"Duo"}})
	writeWAV(t, filepath.Join(root, "solo.wav"), map[string][]string{taglib.Title: {"T2"}, taglib.Artist: {"Cy & Dee"},
		"ARTISTS": {"Cy", "Dee"}, taglib.AlbumArtist: {"Cy"}, taglib.Album: {"Solo"}})
	s, _ := newLibrary(t, root, nil)
	_, err := s.Scan(ctx, nil)
	require.NoError(t, err)

	artists, err := s.Artists(ctx)
	require.NoError(t, err)
	byName := map[string]source.Artist{}
	for _, a := range artists {
		byName[a.Name] = a
	}
	assert.ElementsMatch(t, []string{"Ann feat. Bob", "Cy", "Dee"}, keys(byName), "list tags make artists, names are never split")
	assert.Equal(t, hashID("ann feat. bob"), byName["Ann feat. Bob"].ID, "an unsplit album artist keeps the id it had before artists were indexed")
	assert.Equal(t, 1, byName["Ann feat. Bob"].AlbumCount)
	assert.Equal(t, 0, byName["Dee"].AlbumCount, "a track artist only")

	albums, err := s.Albums(ctx, byName["Dee"].ID)
	require.NoError(t, err)
	require.Len(t, albums, 1, "albums the artist appears on")
	assert.Equal(t, "Solo", albums[0].Name)
	assert.Equal(t, []source.ArtistRef{{ID: byName["Cy"].ID, Name: "Cy"}}, albums[0].Artists)

	album, err := s.Album(ctx, albums[0].ID)
	require.NoError(t, err)
	require.Len(t, album.Tracks, 1)
	tr := album.Tracks[0]
	assert.Equal(t, "Cy & Dee", tr.Artist, "the display string stays")
	assert.Equal(t, []source.ArtistRef{{ID: byName["Cy"].ID, Name: "Cy"}, {ID: byName["Dee"].ID, Name: "Dee"}}, tr.Artists)
}

func keys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestLocalArtistImageAndInfo(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeWAV(t, filepath.Join(root, "Ann/First/1.wav"), tags("One", "Ann", "First", "1"))
	two := tags("Two", "Ann feat. Bob", "First", "2")
	two[taglib.AlbumArtist] = []string{"Ann"}
	two["ARTISTS"] = []string{"Ann", "Bob"}
	writeWAV(t, filepath.Join(root, "Ann/First/2.wav"), two)
	require.NoError(t, os.WriteFile(filepath.Join(root, "Ann/folder.jpg"), []byte("jpg"), 0o644))
	// an artist straight under the library folder: the folder above is the library's
	writeWAV(t, filepath.Join(root, "Loose/1.wav"), tags("Three", "Cy", "Loose", "1"))
	require.NoError(t, os.WriteFile(filepath.Join(root, "artist.jpg"), []byte("jpg"), 0o644))
	s, _ := newLibrary(t, root, nil)
	_, err := s.Scan(ctx, nil)
	require.NoError(t, err)

	img, err := s.ArtistImage(ctx, artistID("Ann"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "Ann/folder.jpg"), img, "folder.jpg in the folder named like the artist")
	_, err = s.ArtistImage(ctx, artistID("Cy"))
	assert.ErrorIs(t, err, source.ErrNotFound, "never an image from the library folder itself")

	info, err := s.ArtistInfo(ctx, artistID("Bob"))
	require.NoError(t, err)
	assert.Equal(t, "Bob", info.Name)
	assert.Equal(t, 1, info.TrackCount, "a featured artist's tracks")
	info, err = s.ArtistInfo(ctx, artistID("Ann"))
	require.NoError(t, err)
	assert.Equal(t, 2, info.TrackCount)
	assert.Equal(t, 1, info.AlbumCount)
}
