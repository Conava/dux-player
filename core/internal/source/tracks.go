package source

import (
	"context"
	"math/rand/v2"
	"strings"
)

// Window returns ts[offset:offset+limit], clamped; limit <= 0 is the rest.
func Window[T any](ts []T, offset, limit int) []T {
	offset = min(max(offset, 0), len(ts))
	end := len(ts)
	if limit > 0 {
		end = min(offset+limit, end)
	}
	return ts[offset:end]
}

// Similar picks up to n tracks like last from its library: other tracks by
// the same artist, else random tracks of the same genre.
// Artist then genre; a server's own similar-songs call
// (getSimilarSongs2, Jellyfin InstantMix) would pick better.
func Similar(ctx context.Context, src Source, last Track, n int, skip map[Ref]bool) []Track {
	var out []Track
	keep := func(t Track) {
		if t.ID != last.ID && !skip[t.Ref()] && len(out) < n {
			out = append(out, t)
		}
	}
	if b, ok := src.(Browser); ok && last.ArtistID != "" {
		albums, _ := b.Albums(ctx, last.ArtistID)
		rand.Shuffle(len(albums), func(i, j int) { albums[i], albums[j] = albums[j], albums[i] })
		for _, a := range Window(albums, 0, 5) {
			full, err := b.Album(ctx, a.ID)
			if err != nil {
				continue
			}
			for _, t := range full.Tracks {
				keep(t)
			}
		}
	}
	if r, ok := src.(RandomTracks); ok && len(out) == 0 && last.Genre != "" {
		batch, _ := r.RandomTracks(ctx, 200)
		for _, t := range batch {
			if strings.EqualFold(t.Genre, last.Genre) {
				keep(t)
			}
		}
	}
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}
