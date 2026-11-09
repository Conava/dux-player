package library

import (
	"context"
	"strings"

	"github.com/Conava/dux-player/core/internal/source"
)

// Collaborator is an artist seen with the asked-for artists and the number
// of tracks and albums they share.
type Collaborator struct {
	ID    string
	Count int
}

// maxArtistIDs keeps the query under SQLite's variable limit.
const maxArtistIDs = 200

func capIDs(ids []string) []string { return ids[:min(len(ids), maxArtistIDs)] }

// idArgs is the "(?,?,…)" list for ids and the args: lead, then the ids
// once per copy.
func idArgs(lead []any, ids []string, copies int) (string, []any) {
	args := append([]any{}, lead...)
	for range copies {
		for _, id := range ids {
			args = append(args, id)
		}
	}
	return "(" + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")", args
}

// TracksByArtists returns up to limit playable tracks that any of the
// artists is credited on, as album artist or as one of the track's artists.
func (s *Store) TracksByArtists(ctx context.Context, libID string, artistIDs []string, limit int) ([]source.Track, error) {
	artistIDs = capIDs(artistIDs)
	if len(artistIDs) == 0 || limit <= 0 {
		return []source.Track{}, nil
	}
	in, args := idArgs([]any{libID}, artistIDs, 1)
	args = append(args, libID)
	for _, id := range artistIDs {
		args = append(args, id)
	}
	args = append(args, limit)
	return s.queryTracks(ctx, `t.library_id = ? AND t.missing = 0 AND (t.artist_id IN `+in+` OR t.id IN (SELECT item_id FROM item_artists
		WHERE library_id = ? AND kind = 'track' AND artist_id IN `+in+`)) ORDER BY random() LIMIT ?`, args...)
}

// Collaborators returns the artists that share a track or an album with any
// of the artists, most frequent first, at most limit.
func (s *Store) Collaborators(ctx context.Context, libID string, artistIDs []string, limit int) ([]Collaborator, error) {
	artistIDs = capIDs(artistIDs)
	out := []Collaborator{}
	if len(artistIDs) == 0 || limit <= 0 {
		return out, nil
	}
	in, args := idArgs([]any{libID}, artistIDs, 2)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT other.artist_id, count(DISTINCT other.kind || ':' || other.item_id) AS n
		FROM item_artists me JOIN item_artists other ON other.library_id = me.library_id AND other.kind = me.kind
			AND other.item_id = me.item_id
		WHERE me.library_id = ? AND me.artist_id IN `+in+` AND other.artist_id NOT IN `+in+`
		GROUP BY other.artist_id ORDER BY n DESC, other.artist_id LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c Collaborator
		if err := rows.Scan(&c.ID, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// TracksByGenre returns up to limit playable tracks of a genre, ignoring case.
func (s *Store) TracksByGenre(ctx context.Context, libID, genre string, limit int) ([]source.Track, error) {
	if genre == "" || limit <= 0 {
		return []source.Track{}, nil
	}
	return s.queryTracks(ctx, "t.library_id = ? AND t.missing = 0 AND t.genre = ? COLLATE NOCASE ORDER BY random() LIMIT ?", libID, genre, limit)
}
