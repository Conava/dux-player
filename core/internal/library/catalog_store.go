package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Conava/dux-player/core/internal/source"
)

// A server's catalog is stored in the local tables, with an empty path. These
// writers exist because PutTrack assumes tags of a local file: it never
// renames an artist or album, and takes its cover from a file.

// validID refuses an id that can't be stored as it came. An id is sent back
// to the server, so it can't be cleaned like text.
func validID(id string) bool { return id != "" && id == source.Clean(id) }

// BeginCatalog starts a sync. Marks changed on this device at or after since
// stay as they are: the server's answer may be older than they are.
func (s *Store) BeginCatalog(libID string, gen, since int64) *Batch {
	return &Batch{db: s.db, libID: libID, gen: gen, since: since}
}

// putMark stores the server's favorite and rating of an item. No row is kept
// for an item without either; changed_at 0 says it came from the server.
func (b *Batch) putMark(ctx context.Context, kind, id string, favorite bool, rating int) error {
	var err error
	if favorite || rating != 0 {
		_, err = b.exec(ctx, `INSERT INTO marks (library_id, kind, id, favorite, rating, changed_at) VALUES (?1, ?2, ?3, ?4, ?5, 0)
			ON CONFLICT (library_id, kind, id) DO UPDATE SET favorite = ?4, rating = ?5 WHERE marks.changed_at < ?6`,
			b.libID, kind, id, favorite, rating, b.since)
		return err
	}
	_, err = b.exec(ctx, `UPDATE marks SET favorite = 0, rating = 0 WHERE library_id = ? AND kind = ? AND id = ?
		AND changed_at < ? AND (favorite <> 0 OR rating <> 0)`, b.libID, kind, id, b.since)
	return err
}

// putServerArtists lists an item's artists. A ref without an id has no row
// of its own: the item's display name stands in for it when read.
func (b *Batch) putServerArtists(ctx context.Context, kind, id string, refs []source.ArtistRef) error {
	if _, err := b.exec(ctx, "DELETE FROM item_artists WHERE library_id = ? AND kind = ? AND item_id = ?", b.libID, kind, id); err != nil {
		return err
	}
	pos := 0
	for _, a := range refs {
		if !validID(a.ID) {
			continue
		}
		if _, err := b.exec(ctx, `INSERT INTO artists (library_id, id, name) VALUES (?, ?, ?)
			ON CONFLICT (library_id, id) DO UPDATE SET name = excluded.name`, b.libID, a.ID, a.Name); err != nil {
			return err
		}
		if _, err := b.exec(ctx, "INSERT INTO item_artists VALUES (?, ?, ?, ?, ?)", b.libID, kind, id, pos, a.ID); err != nil {
			return err
		}
		pos++
	}
	return nil
}

// PutServerArtist stores an artist; keepMark leaves its marks alone.
func (b *Batch) PutServerArtist(ctx context.Context, a source.Artist, keepMark bool) error {
	if !validID(a.ID) {
		return nil
	}
	if _, err := b.exec(ctx, `INSERT INTO artists (library_id, id, name, cover_id) VALUES (?, ?, ?, ?)
		ON CONFLICT (library_id, id) DO UPDATE SET name = excluded.name, cover_id = excluded.cover_id`,
		b.libID, a.ID, a.Name, a.CoverID); err != nil {
		return err
	}
	if keepMark {
		return nil
	}
	return b.putMark(ctx, source.ItemArtist, a.ID, a.Favorite, a.Rating)
}

// PutServerAlbum stores an album; keepMark leaves its marks alone.
func (b *Batch) PutServerAlbum(ctx context.Context, a source.Album, keepMark bool) error {
	if !validID(a.ID) {
		return nil
	}
	if _, err := b.exec(ctx, `INSERT INTO albums (library_id, id, name, artist, artist_id, year, cover_id, added, play_count, last_played)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (library_id, id) DO UPDATE SET name = excluded.name, artist = excluded.artist, artist_id = excluded.artist_id,
			year = excluded.year, cover_id = excluded.cover_id, added = excluded.added,
			play_count = excluded.play_count, last_played = excluded.last_played`,
		b.libID, a.ID, a.Name, a.Artist, a.ArtistID, a.Year, a.CoverID, a.Added, a.PlayCount, a.LastPlayed); err != nil {
		return err
	}
	if err := b.putServerArtists(ctx, source.ItemAlbum, a.ID, a.Artists); err != nil {
		return err
	}
	if keepMark {
		return nil
	}
	return b.putMark(ctx, source.ItemAlbum, a.ID, a.Favorite, a.Rating)
}

// PutServerTrack stores a track; keepMark leaves its marks alone. An album
// that came into the server after the album list gets a row from the track.
func (b *Batch) PutServerTrack(ctx context.Context, t source.Track, keepMark bool) error {
	if !validID(t.ID) {
		return nil
	}
	var rgTrack, rgAlbum *float64
	if t.ReplayGain != nil {
		rgTrack, rgAlbum = t.ReplayGain.Track, t.ReplayGain.Album
	}
	if _, err := b.exec(ctx, `INSERT INTO tracks (library_id, id, path, title, artist, artist_id, album, album_id, album_artist,
			track_no, disc_no, year, genre, duration, size, mtime, scan_gen, rg_track_gain, rg_album_gain,
			composer, format, bitrate, cover_id, play_count, last_played)
		VALUES (?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (library_id, id) DO UPDATE SET title = excluded.title, artist = excluded.artist, artist_id = excluded.artist_id,
			album = excluded.album, album_id = excluded.album_id, album_artist = excluded.album_artist,
			track_no = excluded.track_no, disc_no = excluded.disc_no, year = excluded.year, genre = excluded.genre,
			duration = excluded.duration, mtime = excluded.mtime, scan_gen = excluded.scan_gen,
			rg_track_gain = excluded.rg_track_gain, rg_album_gain = excluded.rg_album_gain, composer = excluded.composer,
			format = excluded.format, bitrate = excluded.bitrate, cover_id = excluded.cover_id,
			play_count = excluded.play_count, last_played = excluded.last_played`,
		b.libID, t.ID, t.Title, t.Artist, t.ArtistID, t.Album, t.AlbumID, t.AlbumArtist, t.TrackNo, t.DiscNo, t.Year, t.Genre,
		t.Duration, t.Added*1e9, b.gen, rgTrack, rgAlbum, t.Composer, t.Format, t.Bitrate, t.CoverID, t.PlayCount, t.LastPlayed); err != nil {
		return fmt.Errorf("index track %s: %w", t.ID, err)
	}
	if err := b.putServerArtists(ctx, source.ItemTrack, t.ID, t.Artists); err != nil {
		return err
	}
	if validID(t.AlbumID) {
		if _, err := b.exec(ctx, `INSERT INTO albums (library_id, id, name, artist, artist_id, year, cover_id, added)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			b.libID, t.AlbumID, t.Album, t.AlbumArtist, t.ArtistID, t.Year, t.CoverID, t.Added); err != nil {
			return err
		}
	}
	if keepMark {
		return nil
	}
	return b.putMark(ctx, source.ItemTrack, t.ID, t.Favorite, t.Rating)
}

// Synced says whether a turned-on library is a server with a complete sync,
// which is what browsing reads from. A local library never has a stamp.
func (s *Store) Synced(ctx context.Context, libID string) bool {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM libraries WHERE id = ? AND enabled = 1 AND last_scan > 0
		AND catalog_stamp <> ''`, libID).Scan(&n)
	return err == nil
}

// SyncedLibraries is Synced for every library.
func (s *Store) SyncedLibraries(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM libraries WHERE enabled = 1 AND last_scan > 0 AND catalog_stamp <> ''")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// CatalogState is the stamp of a library's last sync and when it finished;
// both are empty before the first.
func (s *Store) CatalogState(ctx context.Context, libID string) (stamp string, lastScan int64, err error) {
	err = s.db.QueryRowContext(ctx, "SELECT catalog_stamp, last_scan FROM libraries WHERE id = ?", libID).Scan(&stamp, &lastScan)
	return stamp, lastScan, err
}

// SetCatalogStamp keeps a newer stamp for a catalog that hasn't changed.
func (s *Store) SetCatalogStamp(ctx context.Context, libID, stamp string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE libraries SET catalog_stamp = ? WHERE id = ?", stamp, libID)
	return err
}

// SetCatalogSynced records a complete sync.
func (s *Store) SetCatalogSynced(ctx context.Context, libID, stamp string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE libraries SET catalog_stamp = ?, last_scan = unixepoch() WHERE id = ?", stamp, libID)
	return err
}

// ClearCatalog forgets a library's rows and sync: ids and favorites belong
// to the account they were read with.
func (s *Store) ClearCatalog(ctx context.Context, libID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"tracks", "albums", "artists", "item_artists", "marks"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE library_id = ?", libID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE libraries SET last_scan = 0, catalog_stamp = '' WHERE id = ?", libID); err != nil {
		return err
	}
	return tx.Commit()
}

// MarkChange is one favorite that differs from what the index had.
type MarkChange struct {
	Kind, ID string
	Favorite bool
}

// ApplyFavorites makes the index's favorites what the server listed, except
// for items keep names and marks changed on this device at or after since.
// It returns what changed, and an error for a list too short to believe.
func (s *Store) ApplyFavorites(ctx context.Context, libID string, listed source.SearchResult, since int64, keep func(kind, id string) bool) ([]MarkChange, error) {
	type item struct{ kind, id string }
	want := map[item]bool{}
	for _, a := range listed.Artists {
		want[item{source.ItemArtist, a.ID}] = true
	}
	for _, a := range listed.Albums {
		want[item{source.ItemAlbum, a.ID}] = true
	}
	for _, t := range listed.Tracks {
		want[item{source.ItemTrack, t.ID}] = true
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT kind, id FROM marks WHERE library_id = ? AND favorite = 1", libID)
	if err != nil {
		return nil, err
	}
	have := map[item]bool{}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.kind, &it.id); err != nil {
			rows.Close()
			return nil, err
		}
		have[it] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Acting on a short list deletes offline copies, so it gets the walk's
	// rule: a real change arrives with the next walk, item by item.
	if len(want)*2 < len(have) {
		return nil, fmt.Errorf("the server lists %d favorites, %d are stored; nothing changed", len(want), len(have))
	}
	var changes []MarkChange
	apply := func(it item, on bool) error {
		if !validID(it.id) || keep != nil && keep(it.kind, it.id) {
			return nil
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO marks (library_id, kind, id, favorite, changed_at) VALUES (?1, ?2, ?3, ?4, 0)
			ON CONFLICT (library_id, kind, id) DO UPDATE SET favorite = ?4 WHERE marks.changed_at < ?5`, libID, it.kind, it.id, on, since)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changes = append(changes, MarkChange{it.kind, it.id, on})
		}
		return nil
	}
	for it := range want {
		if !have[it] {
			if err := apply(it, true); err != nil {
				return nil, err
			}
		}
	}
	for it := range have {
		if !want[it] {
			if err := apply(it, false); err != nil {
				return nil, err
			}
		}
	}
	return changes, tx.Commit()
}

// ApplyPlays stores the server's play counts of tracks.
func (s *Store) ApplyPlays(ctx context.Context, libID string, tracks []source.Track) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range tracks {
		if _, err := tx.ExecContext(ctx, "UPDATE tracks SET play_count = ?, last_played = ? WHERE library_id = ? AND id = ?",
			max(t.PlayCount, 0), t.LastPlayed, libID, t.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddPlay counts a play the server accepted, so lists show it before the
// next read of the server's numbers.
func (s *Store) AddPlay(ctx context.Context, libID, id string, at int64) error {
	var album string
	err := s.db.QueryRowContext(ctx, "SELECT album_id FROM tracks WHERE library_id = ? AND id = ?", libID, id).Scan(&album)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []struct{ table, id string }{{"tracks", id}, {"albums", album}} {
		if _, err := tx.ExecContext(ctx, "UPDATE "+q.table+` SET play_count = play_count + 1, last_played = max(last_played, ?)
			WHERE library_id = ? AND id = ?`, at, libID, q.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
