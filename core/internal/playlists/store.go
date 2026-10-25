// Package playlists keeps the player's own playlists. Each item is a
// library+id ref plus a copy of the track as added, so a playlist still lists
// while its libraries are offline.
package playlists

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/source"
)

// Each entry upgrades the schema by one version; never edit a shipped entry.
var migrations = []string{
	`CREATE TABLE playlists (
		id      TEXT PRIMARY KEY,
		name    TEXT NOT NULL,
		cover   TEXT NOT NULL DEFAULT '{"type":"auto"}',
		created INTEGER NOT NULL,
		updated INTEGER NOT NULL
	);
	CREATE TABLE items (
		playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
		pos         INTEGER NOT NULL,
		library_id  TEXT NOT NULL,
		track_id    TEXT NOT NULL,
		duration    REAL NOT NULL DEFAULT 0,
		cover_id    TEXT NOT NULL DEFAULT '',
		track       TEXT NOT NULL,
		PRIMARY KEY (playlist_id, pos)
	);`,
	`CREATE TABLE item_artists (
		playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
		pos         INTEGER NOT NULL,
		library_id  TEXT NOT NULL,
		artist_id   TEXT NOT NULL,
		name        TEXT NOT NULL
	);
	CREATE INDEX item_artists_id ON item_artists (library_id, artist_id);
	CREATE INDEX item_artists_name ON item_artists (name);
	CREATE INDEX item_artists_item ON item_artists (playlist_id, pos);`,
}

const (
	// Kind marks local playlists apart from server ones in the shared shape.
	Kind = "local"

	maxItems = 20000
	mosaic   = 4
)

// Cover is a playlist's picture: "auto" (a mosaic of its first albums),
// "art" (one track's cover) or "shape" (an accent color and a shape name).
type Cover struct {
	Type    string `json:"type"`
	Library string `json:"library,omitempty"`
	ID      string `json:"id,omitempty"`
	Accent  string `json:"accent,omitempty"`
	Shape   string `json:"shape,omitempty"`
}

var (
	colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`)
	shapeRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)
)

func (c Cover) validate() error {
	switch c.Type {
	case "auto":
		if c != (Cover{Type: "auto"}) {
			return errors.New("an auto cover takes no other fields")
		}
	case "art":
		if c.Library == "" || c.ID == "" || c.Accent != "" || c.Shape != "" ||
			source.Clean(c.Library) != c.Library || source.Clean(c.ID) != c.ID {
			return errors.New("an art cover needs the library and id of a track")
		}
	case "shape":
		if !colorRe.MatchString(c.Accent) || !shapeRe.MatchString(c.Shape) || c.Library != "" || c.ID != "" {
			return errors.New("a shape cover needs accent (#rrggbb) and shape")
		}
	default:
		return errors.New("cover type must be auto, art or shape")
	}
	return nil
}

// CoverRef is one tile of an auto cover.
type CoverRef struct {
	Library string `json:"library"`
	CoverID string `json:"coverId"`
}

// Playlist has the fields of a server playlist plus its cover. Library is
// empty: a local playlist belongs to no library.
type Playlist struct {
	Library    string         `json:"library"`
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name"`
	TrackCount int            `json:"trackCount"`
	Duration   float64        `json:"duration"`
	ReadOnly   bool           `json:"readOnly"`
	Cover      Cover          `json:"cover"`
	Mosaic     []CoverRef     `json:"mosaic"`
	Created    int64          `json:"created"`
	Updated    int64          `json:"updated"`
	Tracks     []source.Track `json:"tracks,omitempty"`
}

type Store struct{ db *sql.DB }

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := library.OpenDB(ctx, path, migrations)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.indexArtists(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// indexArtists fills item_artists for the items that have no rows yet,
// which after the migration that added the table is all of them.
func (s *Store) indexArtists(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT playlist_id, pos, library_id, track FROM items i WHERE NOT EXISTS
		(SELECT 1 FROM item_artists a WHERE a.playlist_id = i.playlist_id AND a.pos = i.pos)`)
	if err != nil {
		return err
	}
	type item struct {
		playlist string
		pos      int
		t        source.Track
	}
	var todo []item
	for rows.Next() {
		var it item
		var data string
		if err := rows.Scan(&it.playlist, &it.pos, &it.t.Library, &data); err != nil {
			rows.Close()
			return err
		}
		lib := it.t.Library
		_ = json.Unmarshal([]byte(data), &it.t)
		it.t.Library = lib
		todo = append(todo, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(todo) == 0 {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, it := range todo {
		if err := insertArtists(ctx, tx, it.playlist, it.pos, it.t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// insertArtists indexes one item by every artist it credits. Every item gets
// at least one row, so indexArtists can tell an indexed item from a new one.
func insertArtists(ctx context.Context, tx *sql.Tx, playlist string, pos int, t source.Track) error {
	main := ""
	if len(t.Artists) == 0 {
		main = strings.ToLower(source.Clean(t.Artist))
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO item_artists VALUES (?, ?, ?, ?, ?)", playlist, pos, t.Library, t.ArtistID, main); err != nil {
		return err
	}
	for _, a := range t.Artists {
		if _, err := tx.ExecContext(ctx, "INSERT INTO item_artists VALUES (?, ?, ?, ?, ?)",
			playlist, pos, t.Library, a.ID, strings.ToLower(source.Clean(a.Name))); err != nil {
			return err
		}
	}
	return nil
}

// ArtistMatches counts, per playlist, the tracks of artist id in library lib
// and, with a name, the tracks of any library credited to that name.
func (s *Store) ArtistMatches(ctx context.Context, lib, id, name string) (map[string]int, error) {
	name = strings.ToLower(source.Clean(name))
	rows, err := s.db.QueryContext(ctx, `SELECT playlist_id, count(DISTINCT pos) FROM item_artists
		WHERE (? <> '' AND library_id = ? AND artist_id = ?) OR (? <> '' AND name = ?) GROUP BY playlist_id`,
		id, lib, id, name, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var pl string
		var n int
		if err := rows.Scan(&pl, &n); err != nil {
			return nil, err
		}
		out[pl] = n
	}
	return out, rows.Err()
}

func (s *Store) Close() error { return s.db.Close() }

func cleanName(name string) (string, error) {
	name = source.Clean(name)
	if name == "" {
		return "", errors.New("a playlist needs a name")
	}
	return name, nil
}

func (s *Store) List(ctx context.Context) ([]Playlist, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.id, p.name, p.cover, p.created, p.updated,
		(SELECT count(*) FROM items i WHERE i.playlist_id = p.id),
		(SELECT coalesce(sum(duration), 0) FROM items i WHERE i.playlist_id = p.id)
		FROM playlists p ORDER BY p.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	out := []Playlist{}
	for rows.Next() {
		p := Playlist{Kind: Kind}
		var cover string
		if err := rows.Scan(&p.ID, &p.Name, &cover, &p.Created, &p.Updated, &p.TrackCount, &p.Duration); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal([]byte(cover), &p.Cover)
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Mosaic, err = s.mosaic(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// mosaic is the covers of the first albums in a playlist, for an auto cover.
func (s *Store) mosaic(ctx context.Context, id string) ([]CoverRef, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT library_id, cover_id FROM items WHERE playlist_id = ? AND cover_id <> ''
		GROUP BY library_id, cover_id ORDER BY min(pos) LIMIT ?`, id, mosaic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CoverRef{}
	for rows.Next() {
		var c CoverRef
		if err := rows.Scan(&c.Library, &c.CoverID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns a playlist with its tracks as stored.
func (s *Store) Get(ctx context.Context, id string) (Playlist, error) {
	list, err := s.List(ctx)
	if err != nil {
		return Playlist{}, err
	}
	i := slices.IndexFunc(list, func(p Playlist) bool { return p.ID == id })
	if i < 0 {
		return Playlist{}, fmt.Errorf("playlist %s: %w", id, source.ErrNotFound)
	}
	p := list[i]
	p.Tracks, err = s.tracks(ctx, s.db, id)
	return p, err
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (s *Store) tracks(ctx context.Context, q querier, id string) ([]source.Track, error) {
	rows, err := q.QueryContext(ctx, "SELECT library_id, track_id, track FROM items WHERE playlist_id = ? ORDER BY pos", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []source.Track{}
	for rows.Next() {
		var lib, tid, data string
		if err := rows.Scan(&lib, &tid, &data); err != nil {
			return nil, err
		}
		var t source.Track
		_ = json.Unmarshal([]byte(data), &t)
		// The ref columns are the truth; the copy is only for display.
		t.Library, t.ID = lib, tid
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) Create(ctx context.Context, name string, tracks []source.Track) (Playlist, error) {
	name, err := cleanName(name)
	if err != nil {
		return Playlist{}, err
	}
	id := source.NewID()
	err = s.edit(ctx, id, func(tx *sql.Tx, _ []source.Track) ([]source.Track, error) {
		_, err := tx.ExecContext(ctx, "INSERT INTO playlists (id, name, created, updated) VALUES (?, ?, unixepoch(), unixepoch())", id, name)
		return tracks, err
	})
	if err != nil {
		return Playlist{}, err
	}
	return s.Get(ctx, id)
}

func (s *Store) Rename(ctx context.Context, id, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	return s.update(ctx, id, "name = ?", name)
}

func (s *Store) SetCover(ctx context.Context, id string, c Cover) error {
	if err := c.validate(); err != nil {
		return err
	}
	data, _ := json.Marshal(c)
	return s.update(ctx, id, "cover = ?", string(data))
}

func (s *Store) update(ctx context.Context, id, set string, arg any) error {
	res, err := s.db.ExecContext(ctx, "UPDATE playlists SET "+set+", updated = unixepoch() WHERE id = ?", arg, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("playlist %s: %w", id, source.ErrNotFound)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM playlists WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("playlist %s: %w", id, source.ErrNotFound)
	}
	return nil
}

// Add inserts tracks at position at, or appends them when at is negative
// or past the end.
func (s *Store) Add(ctx context.Context, id string, tracks []source.Track, at int) error {
	return s.edit(ctx, id, func(_ *sql.Tx, cur []source.Track) ([]source.Track, error) {
		if at < 0 || at > len(cur) {
			at = len(cur)
		}
		return slices.Insert(cur, at, tracks...), nil
	})
}

// Remove drops the items at the given positions (before the edit).
func (s *Store) Remove(ctx context.Context, id string, positions []int) error {
	return s.edit(ctx, id, func(_ *sql.Tx, cur []source.Track) ([]source.Track, error) {
		drop := map[int]bool{}
		for _, p := range positions {
			if p < 0 || p >= len(cur) {
				return nil, fmt.Errorf("position %d is not in the playlist", p)
			}
			drop[p] = true
		}
		out := make([]source.Track, 0, len(cur))
		for i, t := range cur {
			if !drop[i] {
				out = append(out, t)
			}
		}
		return out, nil
	})
}

func (s *Store) Move(ctx context.Context, id string, from, to int) error {
	return s.edit(ctx, id, func(_ *sql.Tx, cur []source.Track) ([]source.Track, error) {
		if from < 0 || from >= len(cur) || to < 0 || to >= len(cur) {
			return nil, fmt.Errorf("positions must be 0 to %d", len(cur)-1)
		}
		t := cur[from]
		cur = slices.Delete(cur, from, from+1)
		return slices.Insert(cur, to, t), nil
	})
}

// edit rewrites a playlist's items in one transaction: fn gets the current
// tracks and returns the new list.
func (s *Store) edit(ctx context.Context, id string, fn func(tx *sql.Tx, cur []source.Track) ([]source.Track, error)) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cur, err := s.tracks(ctx, tx, id)
	if err != nil {
		return err
	}
	next, err := fn(tx, cur)
	if err != nil {
		return err
	}
	if len(next) > maxItems {
		return fmt.Errorf("a playlist holds at most %d tracks", maxItems)
	}
	res, err := tx.ExecContext(ctx, "UPDATE playlists SET updated = unixepoch() WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("playlist %s: %w", id, source.ErrNotFound)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM items WHERE playlist_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM item_artists WHERE playlist_id = ?", id); err != nil {
		return err
	}
	for i, t := range next {
		t.Unavailable, t.UnavailableCode = "", ""
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO items (playlist_id, pos, library_id, track_id, duration, cover_id, track)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, id, i, t.Library, t.ID, t.Duration, t.CoverID, string(data)); err != nil {
			return err
		}
		if err := insertArtists(ctx, tx, id, i, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}
