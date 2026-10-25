package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.senan.xyz/taglib"

	"github.com/Conava/dux-player/core/internal/source"
)

var (
	_ source.ArtistDetailer = (*Source)(nil)
	_ source.ArtistImager   = (*Source)(nil)
)

var artistImages = []string{"artist.jpg", "artist.png", "artist.jpeg", "artist.webp"}

func (s *Source) ArtistInfo(ctx context.Context, id string) (source.ArtistInfo, error) {
	info, err := s.store.ArtistInfo(ctx, s.lib.ID, id)
	if err != nil {
		return info, err
	}
	info.MusicBrainzID = s.artistMBID(ctx, id)
	return info, nil
}

// artistMBID reads the artist's MusicBrainz id from one of their files: the
// id at the artist's position among the file's artists or album artists.
func (s *Source) artistMBID(ctx context.Context, id string) string {
	paths, err := s.store.ArtistPaths(ctx, s.lib.ID, id, 1)
	if err != nil || len(paths) == 0 {
		return ""
	}
	tags, err := taglib.ReadTags(paths[0])
	if err != nil {
		return ""
	}
	for _, k := range [][3]string{
		{"ARTISTS", taglib.Artist, "MUSICBRAINZ_ARTISTID"},
		{"ALBUMARTISTS", taglib.AlbumArtist, "MUSICBRAINZ_ALBUMARTISTID"},
	} {
		refs := artistRefs(tags, k[0], k[1], first(tags, k[1]))
		ids := cleanAll(tags[k[2]])
		i := slices.IndexFunc(refs, func(r source.ArtistRef) bool { return r.ID == id })
		if i >= 0 && len(ids) == len(refs) {
			return ids[i]
		}
	}
	return ""
}

// ArtistImage looks for artist.jpg (or .png) in the folders of the artist's
// albums and the folder above them, and for folder.jpg above them when that
// folder is named like the artist (Artist/Album/track layouts).
func (s *Source) ArtistImage(ctx context.Context, id string) (string, error) {
	info, err := s.store.ArtistInfo(ctx, s.lib.ID, id)
	if err != nil {
		return "", err
	}
	paths, err := s.store.ArtistPaths(ctx, s.lib.ID, id, 50)
	if err != nil {
		return "", err
	}
	seen := map[string]bool{}
	for _, p := range paths {
		dir := filepath.Dir(p)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		parent := filepath.Dir(dir)
		// The folder above must still be inside the library: a library
		// folder of albums is not an artist's folder.
		inside := slices.ContainsFunc(Folders(s.lib), func(r string) bool { return strings.HasPrefix(parent, r+"/") })
		var candidates []string
		for _, n := range artistImages {
			candidates = append(candidates, filepath.Join(dir, n))
			if inside {
				candidates = append(candidates, filepath.Join(parent, n))
			}
		}
		if inside && strings.EqualFold(filepath.Base(parent), info.Name) {
			for _, n := range folderCovers {
				if strings.HasPrefix(n, "folder.") {
					candidates = append(candidates, filepath.Join(parent, n))
				}
			}
		}
		for _, c := range candidates {
			if st, err := os.Stat(c); err == nil && st.Mode().IsRegular() && st.Size() <= source.MaxCover {
				return c, nil
			}
		}
	}
	return "", fmt.Errorf("image of artist %s: %w", id, source.ErrNotFound)
}
