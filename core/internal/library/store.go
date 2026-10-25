// Package library keeps the configured libraries and the index of local
// libraries in SQLite. Every row is keyed by library id and item id.
package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/source"
)

// Each entry upgrades the schema by one version; never edit a shipped entry.
var migrations = []string{
	`CREATE TABLE libraries (
		id      TEXT PRIMARY KEY,
		kind    TEXT NOT NULL,
		name    TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		config  TEXT NOT NULL DEFAULT '{}'
	);
	-- Artists are album artists; a track's artist_id points at its album artist.
	CREATE TABLE artists (
		library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
		id         TEXT NOT NULL,
		name       TEXT NOT NULL,
		PRIMARY KEY (library_id, id)
	);
	CREATE TABLE albums (
		library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
		id         TEXT NOT NULL,
		name       TEXT NOT NULL,
		artist     TEXT NOT NULL,
		artist_id  TEXT NOT NULL,
		year       INTEGER NOT NULL DEFAULT 0,
		cover      TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (library_id, id)
	);
	CREATE TABLE tracks (
		library_id   TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
		id           TEXT NOT NULL,
		path         TEXT NOT NULL,
		title        TEXT NOT NULL,
		artist       TEXT NOT NULL,
		artist_id    TEXT NOT NULL,
		album        TEXT NOT NULL,
		album_id     TEXT NOT NULL,
		album_artist TEXT NOT NULL,
		track_no     INTEGER NOT NULL DEFAULT 0,
		disc_no      INTEGER NOT NULL DEFAULT 0,
		year         INTEGER NOT NULL DEFAULT 0,
		genre        TEXT NOT NULL DEFAULT '',
		duration     REAL NOT NULL DEFAULT 0,
		size         INTEGER NOT NULL,
		mtime        INTEGER NOT NULL,
		scan_gen     INTEGER NOT NULL,
		PRIMARY KEY (library_id, id)
	);
	CREATE INDEX tracks_album ON tracks (library_id, album_id);
	CREATE INDEX tracks_artist ON tracks (library_id, artist_id);`,

	// ReplayGain tag values (NULL: no tag), the ebur128 measurement for
	// files without them, and favorites/ratings of local items. mtime 0
	// makes the next scan reread every file for its ReplayGain tags.
	`ALTER TABLE tracks ADD COLUMN rg_track_gain REAL;
	UPDATE tracks SET mtime = 0;
	ALTER TABLE tracks ADD COLUMN rg_album_gain REAL;
	ALTER TABLE tracks ADD COLUMN loudness_gain REAL;
	ALTER TABLE tracks ADD COLUMN loudness_state TEXT NOT NULL DEFAULT '';
	CREATE TABLE marks (
		library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
		kind       TEXT NOT NULL,
		id         TEXT NOT NULL,
		favorite   INTEGER NOT NULL DEFAULT 0,
		rating     INTEGER NOT NULL DEFAULT 0,
		changed_at INTEGER NOT NULL,
		PRIMARY KEY (library_id, kind, id)
	);`,

	// Tracks whose folder vanished (removed, drive unmounted) stay indexed
	// and are marked missing until the folder is back or a rescan drops them.
	// Albums record when they were added (their oldest file's mtime), and
	// libraries when they were last scanned in full (unix seconds).
	`ALTER TABLE tracks ADD COLUMN missing INTEGER NOT NULL DEFAULT 0;
	CREATE INDEX tracks_path ON tracks (library_id, path);
	ALTER TABLE albums ADD COLUMN added INTEGER NOT NULL DEFAULT 0;
	UPDATE albums SET added = coalesce((SELECT min(mtime) / 1000000000 FROM tracks t
		WHERE t.library_id = albums.library_id AND t.album_id = albums.id), 0);
	ALTER TABLE libraries ADD COLUMN last_scan INTEGER NOT NULL DEFAULT 0;`,

	// Every artist of a track (kind track) and every album artist (kind
	// album), in order; each has a row in artists. mtime 0 rereads every
	// file for its artist tags.
	`CREATE TABLE item_artists (
		library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
		kind       TEXT NOT NULL,
		item_id    TEXT NOT NULL,
		pos        INTEGER NOT NULL,
		artist_id  TEXT NOT NULL,
		PRIMARY KEY (library_id, kind, item_id, pos)
	);
	CREATE INDEX item_artists_artist ON item_artists (library_id, artist_id, kind);
	UPDATE tracks SET mtime = 0;`,

	// Artists come only from tags now, never from splitting a name: reread
	// every file. An "A & B" album artist gets its old id (and favorite)
	// back.
	`UPDATE tracks SET mtime = 0;`,

	// Where a track's music starts and ends, measured with its loudness;
	// NULL is not measured yet, 0 no silence.
	`ALTER TABLE tracks ADD COLUMN silence_start REAL;
	ALTER TABLE tracks ADD COLUMN silence_end REAL;`,
}

// Loudness states of a measured track; "" is waiting for measuring.
const (
	LoudnessMeasured = "measured"
	LoudnessFailed   = "failed"
)

func markCols(kind, table string) string {
	where := "m.library_id = " + table + ".library_id AND m.kind = '" + kind + "' AND m.id = " + table + ".id"
	return "coalesce((SELECT favorite FROM marks m WHERE " + where + "), 0), coalesce((SELECT rating FROM marks m WHERE " + where + "), 0)"
}

type Store struct {
	db       *sql.DB
	onChange atomic.Pointer[func(libID string, d *Delta)]
}

// Delta is which tracks an index change touched, by id.
type Delta struct {
	Added   []string `json:"added"`
	Changed []string `json:"changed"`
	Removed []string `json:"removed"`
}

// OnChange sets what runs after a library's index changed outside a request
// (the folder watcher, a drive coming or going); d is nil when what changed
// isn't known.
func (s *Store) OnChange(f func(libID string, d *Delta)) { s.onChange.Store(&f) }

func (s *Store) changed(libID string, d *Delta) {
	if f := s.onChange.Load(); f != nil {
		(*f)(libID, d)
	}
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := OpenDB(ctx, path, migrations)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// OpenDB opens an SQLite database and brings its schema up to date: each
// migration entry upgrades it by one version.
func OpenDB(ctx context.Context, path string, migrations []string) (*sql.DB, error) {
	// Immediate transactions take the write lock up front, where busy_timeout
	// waits for it; WAL lets the other connections read meanwhile.
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_txlock=immediate"
	// SQLite gives the -wal and -shm files the database's mode.
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600); err == nil {
		f.Close()
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		_ = os.Chmod(p, 0o600)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	db.SetMaxOpenConns(4)
	if err := migrate(ctx, db, migrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return db, nil
}

func migrate(ctx context.Context, db *sql.DB, migrations []string) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("schema %d is newer than this dplayer (%d)", version, len(migrations))
	}
	for v := version; v < len(migrations); v++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate to %d: %w", v+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Libraries(ctx context.Context) ([]source.Library, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, kind, name, enabled, config FROM libraries ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []source.Library{}
	for rows.Next() {
		var lib source.Library
		var cfg string
		if err := rows.Scan(&lib.ID, &lib.Kind, &lib.Name, &lib.Enabled, &cfg); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(cfg), &lib.Config); err != nil {
			return nil, fmt.Errorf("library %s config: %w", lib.ID, err)
		}
		out = append(out, lib)
	}
	return out, rows.Err()
}

func (s *Store) AddLibrary(ctx context.Context, lib source.Library) error {
	cfg, err := json.Marshal(lib.Config)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO libraries (id, kind, name, enabled, config) VALUES (?, ?, ?, ?, ?)",
		lib.ID, lib.Kind, lib.Name, lib.Enabled, string(cfg))
	return err
}

// SetScanned records a finished full scan.
func (s *Store) SetScanned(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE libraries SET last_scan = unixepoch() WHERE id = ?", id)
	return err
}

// LastScans returns library id → unix time of the last full scan.
func (s *Store) LastScans(ctx context.Context) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, last_scan FROM libraries WHERE last_scan > 0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id string
		var t int64
		if err := rows.Scan(&id, &t); err != nil {
			return nil, err
		}
		out[id] = t
	}
	return out, rows.Err()
}

// UpdateLibrary stores a library's name, enabled flag and config.
func (s *Store) UpdateLibrary(ctx context.Context, lib source.Library) error {
	cfg, err := json.Marshal(lib.Config)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE libraries SET name = ?, enabled = ?, config = ? WHERE id = ?",
		lib.Name, lib.Enabled, string(cfg), lib.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("library %s: %w", lib.ID, source.ErrNotFound)
	}
	return nil
}

func (s *Store) SetEnabled(ctx context.Context, id string, enabled bool) error {
	res, err := s.db.ExecContext(ctx, "UPDATE libraries SET enabled = ? WHERE id = ?", enabled, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("library %s: %w", id, source.ErrNotFound)
	}
	return nil
}

// SetConfig changes one key of a library's config, keeping the others.
func (s *Store) SetConfig(ctx context.Context, id, key string, value any) error {
	libs, err := s.Libraries(ctx)
	if err != nil {
		return err
	}
	for _, lib := range libs {
		if lib.ID == id {
			if lib.Config == nil {
				lib.Config = map[string]any{}
			}
			lib.Config[key] = value
			return s.UpdateLibrary(ctx, lib)
		}
	}
	return fmt.Errorf("library %s: %w", id, source.ErrNotFound)
}

func (s *Store) RemoveLibrary(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM libraries WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("library %s: %w", id, source.ErrNotFound)
	}
	return nil
}

// IndexedTrack is a track row with what the scanner needs to skip unchanged
// files.
type IndexedTrack struct {
	source.Track
	Path  string
	Size  int64
	MTime int64
	// ReplayGain tag values in dB; nil when the file has no such tag.
	RGTrack *float64
	RGAlbum *float64
	// AlbumArtists are the album's artists; Track.Artists the track's.
	AlbumArtists []source.ArtistRef
}

// FileStamps returns path → (size, mtime) for a library, to skip unchanged
// files on rescan. Missing tracks get a zero stamp, so they are reread and
// lose the mark once their file is back.
func (s *Store) FileStamps(ctx context.Context, libID string) (map[string][2]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path, CASE missing WHEN 0 THEN size ELSE -1 END, mtime FROM tracks
		WHERE library_id = ?`, libID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][2]int64)
	for rows.Next() {
		var p string
		var size, mtime int64
		if err := rows.Scan(&p, &size, &mtime); err != nil {
			return nil, err
		}
		out[p] = [2]int64{size, mtime}
	}
	return out, rows.Err()
}

// under matches a path that is dir itself or lies below it.
const under = "(path = ?1 OR substr(path, 1, length(?1) + 1) = ?1 || '/')"

// HasTracksUnder says whether the index has tracks at or below dir.
func (s *Store) HasTracksUnder(ctx context.Context, libID, dir string) bool {
	var n int
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM (SELECT 1 FROM tracks WHERE "+under+" AND library_id = ?2 LIMIT 1)",
		dir, libID).Scan(&n)
	return n > 0
}

// MarkMissing flags the tracks at or below dir as missing, keeping them in
// the index. It reports whether any track changed.
func (s *Store) MarkMissing(ctx context.Context, libID, dir string) (bool, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE tracks SET missing = 1 WHERE "+under+" AND library_id = ?2 AND missing = 0", dir, libID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		s.changed(libID, nil)
	}
	return n > 0, nil
}

// Missing reports whether a track's file is known to be gone.
func (s *Store) Missing(ctx context.Context, libID, id string) bool {
	var missing bool
	_ = s.db.QueryRowContext(ctx, "SELECT missing FROM tracks WHERE library_id = ? AND id = ?", libID, id).Scan(&missing)
	return missing
}

// TrackByPath finds the indexed track of a file in any library.
func (s *Store) TrackByPath(ctx context.Context, path string) (source.Ref, bool) {
	var ref source.Ref
	err := s.db.QueryRowContext(ctx, "SELECT library_id, id FROM tracks WHERE path = ? LIMIT 1", path).Scan(&ref.Library, &ref.ID)
	return ref, err == nil
}

// Path returns the file of an indexed track.
func (s *Store) Path(ctx context.Context, libID, id string) (string, bool) {
	var p string
	err := s.db.QueryRowContext(ctx, "SELECT path FROM tracks WHERE library_id = ? AND id = ?", libID, id).Scan(&p)
	return p, err == nil
}

// Batch writes one scan generation. Rows not touched in gen are removed by
// Finish. It commits every batchFor, so a long scan never holds the write
// lock for long; the generation keeps the partial commits consistent.
type Batch struct {
	db     *sql.DB
	tx     *sql.Tx
	opened time.Time
	libID  string
	gen    int64
}

const batchFor = 250 * time.Millisecond

func (s *Store) Begin(_ context.Context, libID string, gen int64) (*Batch, error) {
	return &Batch{db: s.db, libID: libID, gen: gen}, nil
}

func (b *Batch) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	if b.tx != nil && time.Since(b.opened) > batchFor {
		err := b.tx.Commit()
		b.tx = nil
		if err != nil {
			return nil, err
		}
	}
	if b.tx == nil {
		tx, err := b.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		b.tx, b.opened = tx, time.Now()
	}
	return b.tx.ExecContext(ctx, q, args...)
}

func (b *Batch) Touch(ctx context.Context, path string) error {
	_, err := b.exec(ctx, "UPDATE tracks SET scan_gen = ? WHERE library_id = ? AND path = ?", b.gen, b.libID, path)
	return err
}

// KeepMissing keeps the tracks at or below dir through this scan, marked
// missing: their folder is gone, which may only mean an unmounted drive.
func (b *Batch) KeepMissing(ctx context.Context, dir string) error {
	_, err := b.exec(ctx, "UPDATE tracks SET scan_gen = ?3, missing = 1 WHERE "+under+" AND library_id = ?2",
		dir, b.libID, b.gen)
	return err
}

// sameAudio is a reread file whose audio is still the one measured: a tag
// write (ratings, a cover) moves the mtime but keeps the duration and
// changes the size by little. A re-encode to the same length within 64 KiB
// keeps its old measurement too.
const sameAudio = "tracks.duration = excluded.duration AND abs(tracks.size - excluded.size) <= 65536"

func (b *Batch) PutTrack(ctx context.Context, t IndexedTrack, cover string) error {
	_, err := b.exec(ctx, `INSERT INTO tracks (library_id, id, path, title, artist, artist_id, album, album_id,
			album_artist, track_no, disc_no, year, genre, duration, size, mtime, scan_gen, rg_track_gain, rg_album_gain)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (library_id, id) DO UPDATE SET path = excluded.path, title = excluded.title,
			artist = excluded.artist, artist_id = excluded.artist_id, album = excluded.album,
			album_id = excluded.album_id, album_artist = excluded.album_artist, track_no = excluded.track_no,
			disc_no = excluded.disc_no, year = excluded.year, genre = excluded.genre,
			duration = excluded.duration, size = excluded.size, mtime = excluded.mtime, scan_gen = excluded.scan_gen, missing = 0,
			rg_track_gain = excluded.rg_track_gain, rg_album_gain = excluded.rg_album_gain,
			loudness_gain = CASE WHEN `+sameAudio+` THEN tracks.loudness_gain END,
			loudness_state = CASE WHEN `+sameAudio+` THEN tracks.loudness_state ELSE '' END,
			silence_start = CASE WHEN `+sameAudio+` THEN tracks.silence_start END,
			silence_end = CASE WHEN `+sameAudio+` THEN tracks.silence_end END`,
		b.libID, t.ID, t.Path, t.Title, t.Artist, t.ArtistID, t.Album, t.AlbumID, t.AlbumArtist,
		t.TrackNo, t.DiscNo, t.Year, t.Genre, t.Duration, t.Size, t.MTime, b.gen, t.RGTrack, t.RGAlbum)
	if err != nil {
		return fmt.Errorf("index %s: %w", t.Path, err)
	}
	if len(t.AlbumArtists) == 0 {
		if _, err := b.exec(ctx, `INSERT INTO artists (library_id, id, name) VALUES (?, ?, ?)
			ON CONFLICT DO NOTHING`, b.libID, t.ArtistID, t.AlbumArtist); err != nil {
			return err
		}
	}
	if err := b.putArtists(ctx, source.ItemTrack, t.ID, t.Artists); err != nil {
		return err
	}
	if err := b.putArtists(ctx, source.ItemAlbum, t.AlbumID, t.AlbumArtists); err != nil {
		return err
	}
	_, err = b.exec(ctx, `INSERT INTO albums (library_id, id, name, artist, artist_id, year, cover, added)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (library_id, id) DO UPDATE SET year = max(albums.year, excluded.year),
			cover = CASE WHEN albums.cover = '' THEN excluded.cover ELSE albums.cover END,
			added = CASE WHEN albums.added = 0 THEN excluded.added ELSE min(albums.added, excluded.added) END`,
		b.libID, t.AlbumID, t.Album, t.AlbumArtist, t.ArtistID, t.Year, cover, t.MTime/1e9)
	return err
}

func (b *Batch) putArtists(ctx context.Context, kind, id string, refs []source.ArtistRef) error {
	if _, err := b.exec(ctx, "DELETE FROM item_artists WHERE library_id = ? AND kind = ? AND item_id = ?", b.libID, kind, id); err != nil {
		return err
	}
	for i, a := range refs {
		if _, err := b.exec(ctx, "INSERT INTO artists (library_id, id, name) VALUES (?, ?, ?) ON CONFLICT DO NOTHING", b.libID, a.ID, a.Name); err != nil {
			return err
		}
		if _, err := b.exec(ctx, "INSERT INTO item_artists VALUES (?, ?, ?, ?, ?)", b.libID, kind, id, i, a.ID); err != nil {
			return err
		}
	}
	return nil
}

// Stale lists the libraries with files the next scan must reread (after a
// migration).
func (s *Store) Stale(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT library_id FROM tracks WHERE mtime = 0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Finish drops rows the scan didn't see, then albums and artists left
// without tracks, and commits. It returns how many tracks were removed.
// With scope, only rows at or below those paths count as seen or unseen
// (a partial scan); without, the whole library does.
func (b *Batch) Finish(ctx context.Context, scope ...string) (int, error) {
	var removed int64
	del := func(where string, args ...any) error {
		res, err := b.exec(ctx, "DELETE FROM tracks WHERE "+where, args...)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		removed += n
		return nil
	}
	var err error
	if len(scope) == 0 {
		err = del("library_id = ? AND scan_gen <> ?", b.libID, b.gen)
	}
	for _, p := range scope {
		if err = del(under+" AND library_id = ?2 AND scan_gen <> ?3", p, b.libID, b.gen); err != nil {
			break
		}
	}
	if err != nil {
		b.Abort()
		return 0, err
	}
	for _, q := range []string{
		"DELETE FROM albums WHERE library_id = ?1 AND id NOT IN (SELECT album_id FROM tracks WHERE library_id = ?1)",
		"DELETE FROM item_artists WHERE library_id = ?1 AND kind = 'track' AND item_id NOT IN (SELECT id FROM tracks WHERE library_id = ?1)",
		"DELETE FROM item_artists WHERE library_id = ?1 AND kind = 'album' AND item_id NOT IN (SELECT id FROM albums WHERE library_id = ?1)",
		`DELETE FROM artists WHERE library_id = ?1 AND id NOT IN (SELECT artist_id FROM tracks WHERE library_id = ?1)
			AND id NOT IN (SELECT artist_id FROM item_artists WHERE library_id = ?1)`,
	} {
		if _, err := b.exec(ctx, q, b.libID); err != nil {
			b.Abort()
			return 0, err
		}
	}
	return int(removed), b.tx.Commit()
}

// Changed tells listeners that a library's index changed outside a request;
// d is nil when what changed isn't known.
func (s *Store) Changed(libID string, d *Delta) { s.changed(libID, d) }

// Abort drops the writes since the last commit.
func (b *Batch) Abort() {
	if b.tx != nil {
		_ = b.tx.Rollback()
	}
}

// artistsCol is the JSON list of an item's artists in order.
func artistsCol(kind, table string) string {
	return `(SELECT json_group_array(json_object('id', id, 'name', name)) FROM (SELECT ia.artist_id AS id, ar.name AS name
		FROM item_artists ia JOIN artists ar ON ar.library_id = ia.library_id AND ar.id = ia.artist_id
		WHERE ia.library_id = ` + table + `.library_id AND ia.kind = '` + kind + `' AND ia.item_id = ` + table + `.id ORDER BY ia.pos))`
}

// artistRefs reads artistsCol; a row from before artists were indexed gets
// its display name.
func artistRefs(data []byte, name, id string) []source.ArtistRef {
	var refs []source.ArtistRef
	if json.Unmarshal(data, &refs) == nil && len(refs) > 0 {
		return refs
	}
	return source.SingleArtist(name, id)
}

var trackCols = `t.library_id, t.id, t.path, t.title, t.artist, t.artist_id, t.album, t.album_id, t.album_artist,
	t.track_no, t.disc_no, t.year, t.genre, t.duration, a.cover, t.missing, ` + markCols(source.ItemTrack, "t") + `, ` +
	artistsCol(source.ItemTrack, "t")

func scanTrack(rows interface{ Scan(...any) error }) (IndexedTrack, error) {
	var t IndexedTrack
	var cover sql.NullString
	var missing bool
	var artists []byte
	err := rows.Scan(&t.Library, &t.ID, &t.Path, &t.Title, &t.Artist, &t.ArtistID, &t.Album, &t.AlbumID,
		&t.AlbumArtist, &t.TrackNo, &t.DiscNo, &t.Year, &t.Genre, &t.Duration, &cover, &missing, &t.Favorite, &t.Rating, &artists)
	mainID := ""
	if t.Artist == t.AlbumArtist {
		mainID = t.ArtistID
	}
	t.Artists = artistRefs(artists, t.Artist, mainID)
	if cover.String != "" {
		t.CoverID = t.AlbumID
	}
	if missing {
		t.Unavailable, t.UnavailableCode = TrackMissing.Message, TrackMissing.Code
	}
	return t, err
}

func (s *Store) queryTracks(ctx context.Context, where string, args ...any) ([]source.Track, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+trackCols+` FROM tracks t
		LEFT JOIN albums a ON a.library_id = t.library_id AND a.id = t.album_id
		WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []source.Track{}
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t.Track)
	}
	return out, rows.Err()
}

// Track returns one indexed track with its file path.
func (s *Store) Track(ctx context.Context, libID, id string) (IndexedTrack, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+trackCols+` FROM tracks t
		LEFT JOIN albums a ON a.library_id = t.library_id AND a.id = t.album_id
		WHERE t.library_id = ? AND t.id = ?`, libID, id)
	t, err := scanTrack(row)
	if errors.Is(err, sql.ErrNoRows) {
		return IndexedTrack{}, fmt.Errorf("track %s: %w", id, source.ErrNotFound)
	}
	return t, err
}

func (s *Store) Artists(ctx context.Context, libID string) ([]source.Artist, error) {
	return s.queryArtists(ctx, libID, "")
}

// albumCount counts the albums an artist is an album artist of.
const albumCount = `(SELECT count(*) FROM item_artists ia WHERE ia.library_id = ar.library_id AND ia.kind = 'album'
	AND ia.artist_id = ar.id)`

func (s *Store) queryArtists(ctx context.Context, libID, having string, args ...any) ([]source.Artist, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ar.id, ar.name, `+albumCount+`, `+markCols(source.ItemArtist, "ar")+` FROM artists ar
		WHERE ar.library_id = ? `+having+` ORDER BY ar.name COLLATE NOCASE`, append([]any{libID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []source.Artist{}
	for rows.Next() {
		a := source.Artist{Library: libID}
		if err := rows.Scan(&a.ID, &a.Name, &a.AlbumCount, &a.Favorite, &a.Rating); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

var albumCols = `al.id, al.name, al.artist, al.artist_id, al.year, al.cover, al.added,
	(SELECT count(*) FROM tracks t WHERE t.library_id = al.library_id AND t.album_id = al.id),
	(SELECT coalesce(sum(duration), 0) FROM tracks t WHERE t.library_id = al.library_id AND t.album_id = al.id), ` +
	markCols(source.ItemAlbum, "al") + `, ` + artistsCol(source.ItemAlbum, "al")

func (s *Store) queryAlbums(ctx context.Context, libID, where string, limit int, args ...any) ([]source.Album, error) {
	if limit <= 0 {
		limit = -1
	}
	args = append(append([]any{libID}, args...), limit)
	rows, err := s.db.QueryContext(ctx, "SELECT "+albumCols+" FROM albums al WHERE al.library_id = ? "+where+
		" ORDER BY al.artist COLLATE NOCASE, al.year, al.name COLLATE NOCASE LIMIT ?", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []source.Album{}
	for rows.Next() {
		a := source.Album{Library: libID}
		var cover string
		var artists []byte
		if err := rows.Scan(&a.ID, &a.Name, &a.Artist, &a.ArtistID, &a.Year, &cover, &a.Added, &a.TrackCount, &a.Duration, &a.Favorite, &a.Rating, &artists); err != nil {
			return nil, err
		}
		a.Artists = artistRefs(artists, a.Artist, a.ArtistID)
		if cover != "" {
			a.CoverID = a.ID
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Albums(ctx context.Context, libID, artistID string) ([]source.Album, error) {
	if artistID == "" {
		return s.queryAlbums(ctx, libID, "", 0)
	}
	// Albums the artist only appears on (a featured track) count too.
	return s.queryAlbums(ctx, libID, `AND (al.artist_id = ?2 OR al.id IN (SELECT item_id FROM item_artists
		WHERE library_id = ?1 AND kind = 'album' AND artist_id = ?2) OR al.id IN (SELECT t.album_id FROM item_artists ia
		JOIN tracks t ON t.library_id = ia.library_id AND t.id = ia.item_id
		WHERE ia.library_id = ?1 AND ia.kind = 'track' AND ia.artist_id = ?2))`, 0, artistID)
}

func (s *Store) Album(ctx context.Context, libID, id string) (source.Album, error) {
	albums, err := s.queryAlbums(ctx, libID, "AND al.id = ?", 0, id)
	if err != nil {
		return source.Album{}, err
	}
	if len(albums) == 0 {
		return source.Album{}, fmt.Errorf("album %s: %w", id, source.ErrNotFound)
	}
	a := albums[0]
	a.Tracks, err = s.queryTracks(ctx, "t.library_id = ? AND t.album_id = ? ORDER BY t.disc_no, t.track_no, t.title", libID, id)
	return a, err
}

// Cover returns the stored cover file of an album, if any.
func (s *Store) Cover(ctx context.Context, libID, albumID string) (string, error) {
	var cover string
	err := s.db.QueryRowContext(ctx, "SELECT cover FROM albums WHERE library_id = ? AND id = ?", libID, albumID).Scan(&cover)
	if errors.Is(err, sql.ErrNoRows) || err == nil && cover == "" {
		return "", fmt.Errorf("cover %s: %w", albumID, source.ErrNotFound)
	}
	return cover, err
}

func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

// LIKE scan; move to an FTS5 table once libraries reach tens of
// thousands of tracks and search feels slow.
func (s *Store) Search(ctx context.Context, libID, query string, limit int) (source.SearchResult, error) {
	pat := likePattern(query)
	var res source.SearchResult
	var err error
	res.Tracks, err = s.queryTracks(ctx, `t.library_id = ? AND (t.title LIKE ?2 ESCAPE '\' OR t.artist LIKE ?2 ESCAPE '\'
		OR t.album LIKE ?2 ESCAPE '\') ORDER BY t.title COLLATE NOCASE LIMIT ?3`, libID, pat, limit)
	if err != nil {
		return res, err
	}
	res.Albums, err = s.queryAlbums(ctx, libID, `AND (al.name LIKE ? ESCAPE '\' OR al.artist LIKE ? ESCAPE '\')`, limit, pat, pat)
	if err != nil {
		return res, err
	}
	artists, err := s.Artists(ctx, libID)
	if err != nil {
		return res, err
	}
	q := strings.ToLower(query)
	for _, a := range artists {
		if len(res.Artists) < limit && strings.Contains(strings.ToLower(a.Name), q) {
			res.Artists = append(res.Artists, a)
		}
	}
	return res, nil
}

// trackOrder is the ORDER BY for each source.Sort* key; ties fall back to
// album artist, album, disc, track, and the id keeps pages stable.
var trackOrder = map[string]string{
	source.SortDefault: "",
	source.SortTitle:   "t.title COLLATE NOCASE, ",
	source.SortAlbum:   "t.album COLLATE NOCASE, ",
	source.SortYear:    "t.year, ",
	source.SortAdded:   "a.added DESC, ",
}

// TrackPage is one page of a library's tracks in the order q.Sort asks for.
func (s *Store) TrackPage(ctx context.Context, libID string, q source.TrackQuery) (source.TrackPage, error) {
	order, ok := trackOrder[q.Sort]
	if !ok {
		return source.TrackPage{}, fmt.Errorf("sort %q is not title, album, year or added", q.Sort)
	}
	page := source.TrackPage{Sort: q.Sort}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM tracks WHERE library_id = ?", libID).Scan(&page.Total); err != nil {
		return page, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = -1
	}
	var err error
	page.Tracks, err = s.queryTracks(ctx, "t.library_id = ? ORDER BY "+order+`t.album_artist COLLATE NOCASE,
		t.album COLLATE NOCASE, t.disc_no, t.track_no, t.id LIMIT ? OFFSET ?`, libID, limit, max(q.Offset, 0))
	return page, err
}

// RandomTracks returns up to limit tracks in random order.
func (s *Store) RandomTracks(ctx context.Context, libID string, limit int) ([]source.Track, error) {
	return s.queryTracks(ctx, "t.library_id = ? AND t.missing = 0 ORDER BY random() LIMIT ?", libID, limit)
}

func (s *Store) TrackCount(ctx context.Context, libID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM tracks WHERE library_id = ? AND missing = 0", libID).Scan(&n)
	return n, err
}

// PendingMeasure lists tracks waiting for measuring: those without a
// ReplayGain tag and, with silence, those without their silence.
func (s *Store) PendingMeasure(ctx context.Context, libID string, silence bool, limit int) ([]IndexedTrack, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, path FROM tracks WHERE library_id = ? AND missing = 0
		AND ((rg_track_gain IS NULL AND loudness_state = '') OR (? AND silence_start IS NULL)) LIMIT ?`, libID, silence, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IndexedTrack
	for rows.Next() {
		var t IndexedTrack
		if err := rows.Scan(&t.ID, &t.Path); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetMeasured stores a measurement; nil is a failed one, which plays the
// track uncorrected and untrimmed instead of measuring it again.
func (s *Store) SetMeasured(ctx context.Context, libID, id string, r *loudness.Result) error {
	if r == nil {
		_, err := s.db.ExecContext(ctx, `UPDATE tracks SET silence_start = 0, silence_end = 0,
			loudness_state = CASE loudness_state WHEN '' THEN 'failed' ELSE loudness_state END
			WHERE library_id = ? AND id = ?`, libID, id)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE tracks SET loudness_gain = ?, loudness_state = 'measured',
		silence_start = ?, silence_end = ? WHERE library_id = ? AND id = ?`, r.Gain, r.Silence.Start, r.Silence.End, libID, id)
	return err
}

// Silence is a track's measured silence; zero when not measured.
func (s *Store) Silence(ctx context.Context, libID, id string) loudness.Silence {
	var start, end sql.NullFloat64
	_ = s.db.QueryRowContext(ctx, "SELECT silence_start, silence_end FROM tracks WHERE library_id = ? AND id = ?",
		libID, id).Scan(&start, &end)
	return loudness.Silence{Start: start.Float64, End: end.Float64}
}

// LoudnessGain is the measured gain of a track without ReplayGain tags.
func (s *Store) LoudnessGain(ctx context.Context, libID, id string) (float64, bool) {
	var gain sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `SELECT loudness_gain FROM tracks WHERE library_id = ? AND id = ?
		AND rg_track_gain IS NULL AND loudness_state = 'measured'`, libID, id).Scan(&gain)
	return gain.Float64, err == nil && gain.Valid
}

// SetMark changes the favorite flag or the rating of an item; nil leaves a
// value as it is.
func (s *Store) SetMark(ctx context.Context, libID, kind, id string, favorite *bool, rating *int) error {
	if !source.ValidItemKind(kind) {
		return fmt.Errorf("%q is not track, album or artist", kind)
	}
	if rating != nil && (*rating < 0 || *rating > 5) {
		return errors.New("rating must be 0 to 5")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO marks (library_id, kind, id, favorite, rating, changed_at)
		VALUES (?1, ?2, ?3, coalesce(?4, 0), coalesce(?5, 0), unixepoch())
		ON CONFLICT (library_id, kind, id) DO UPDATE SET favorite = coalesce(?4, favorite),
			rating = coalesce(?5, rating), changed_at = unixepoch()`, libID, kind, id, favorite, rating)
	return err
}

func (s *Store) Favorites(ctx context.Context, libID string) (source.SearchResult, error) {
	var res source.SearchResult
	var err error
	fav := func(kind, table string) string {
		return "EXISTS (SELECT 1 FROM marks m WHERE m.library_id = " + table + ".library_id AND m.kind = '" + kind +
			"' AND m.id = " + table + ".id AND m.favorite = 1)"
	}
	if res.Tracks, err = s.queryTracks(ctx, "t.library_id = ? AND "+fav(source.ItemTrack, "t")+" ORDER BY t.title COLLATE NOCASE", libID); err != nil {
		return res, err
	}
	if res.Albums, err = s.queryAlbums(ctx, libID, "AND "+fav(source.ItemAlbum, "al"), 0); err != nil {
		return res, err
	}
	res.Artists, err = s.queryArtists(ctx, libID, "AND "+fav(source.ItemArtist, "ar"))
	return res, err
}

// artistTracks is the WHERE for the tracks of an artist (?1 library, ?2
// artist): as album artist or as one of the track's artists.
const artistTracks = `t.library_id = ?1 AND (t.artist_id = ?2 OR t.id IN (SELECT item_id FROM item_artists
	WHERE library_id = ?1 AND kind = 'track' AND artist_id = ?2))`

// ArtistInfo is an artist's name, marks and counts.
func (s *Store) ArtistInfo(ctx context.Context, libID, id string) (source.ArtistInfo, error) {
	artists, err := s.queryArtists(ctx, libID, "AND ar.id = ?", id)
	if err != nil {
		return source.ArtistInfo{}, err
	}
	if len(artists) == 0 {
		return source.ArtistInfo{}, fmt.Errorf("artist %s: %w", id, source.ErrNotFound)
	}
	info := source.ArtistInfo{Artist: artists[0], Similar: []source.Artist{}}
	err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM tracks t WHERE "+artistTracks, libID, id).Scan(&info.TrackCount)
	return info, err
}

// ArtistPaths lists files of an artist's tracks, album by album.
func (s *Store) ArtistPaths(ctx context.Context, libID, id string, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT t.path FROM tracks t WHERE "+artistTracks+
		" AND t.missing = 0 ORDER BY t.album_id, t.disc_no, t.track_no LIMIT ?3", libID, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
