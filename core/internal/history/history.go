// Package history keeps the plays that counted (half the track or 4
// minutes, the player's TopicPlayed) and answers play counts, the history
// list and the "most played" and "recently played" shelves.
package history

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/source"
)

// The track column is the track as it was played, so the history shows
// even while its library is offline.
var migrations = []string{`
CREATE TABLE plays (
	library   TEXT NOT NULL,
	track_id  TEXT NOT NULL,
	album_id  TEXT NOT NULL,
	played_at INTEGER NOT NULL,
	track     TEXT NOT NULL
);
CREATE INDEX plays_track ON plays(library, track_id);
CREATE INDEX plays_album ON plays(library, album_id);
CREATE INDEX plays_time ON plays(played_at);`,

	// Each server's own play counts (only played tracks), with when they
	// were read (at, unix s) and by which refresh (gen, unix ns).
	`CREATE TABLE server_counts (
	library  TEXT NOT NULL,
	track_id TEXT NOT NULL,
	count    INTEGER NOT NULL,
	last     INTEGER NOT NULL,
	at       INTEGER NOT NULL,
	gen      INTEGER NOT NULL,
	track    TEXT NOT NULL,
	PRIMARY KEY (library, track_id)
);
CREATE TABLE server_refresh (
	library   TEXT PRIMARY KEY,
	full_at   INTEGER NOT NULL DEFAULT 0,
	recent_at INTEGER NOT NULL DEFAULT 0
);`}

// Play is one counted play; PlayedAt is unix seconds.
type Play struct {
	Track    source.Track `json:"track"`
	PlayedAt int64        `json:"playedAt"`
}

// Count is a track or album with how often and when it was last played.
type Count struct {
	Track      *source.Track `json:"track,omitempty"`
	Album      *source.Album `json:"album,omitempty"`
	Artist     *Artist       `json:"artist,omitempty"`
	Count      int           `json:"count"`
	LastPlayed int64         `json:"lastPlayed"`
}

// Artist is an artist of a library; ID is empty when the library doesn't
// know the name as an artist.
type Artist struct {
	Library string `json:"library"`
	ID      string `json:"id"`
	Name    string `json:"name"`
}

type stat struct {
	n    int
	last int64
}

type Store struct {
	db  *sql.DB
	now func() time.Time
	// Server says whether a library is a server; its items keep the play
	// counts the server reports. Set before the first Annotate.
	Server func(lib string) bool
	// Sources are the working libraries; servers among them add their own
	// counts to Top. Set before the first Top.
	Sources func(ctx context.Context) ([]source.Source, error)
	// bg is the store's lifetime, for the background full reads.
	bg     context.Context
	cancel context.CancelFunc

	// Counts per track and album key, so browse replies get their play
	// counts without a query per item.
	mu     sync.Mutex
	tracks map[string]stat
	albums map[string]stat
	// fetching has a channel per running refresh (library and kind).
	fetching map[string]chan struct{}
}

func key(lib, id string) string { return lib + "\x00" + id }

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := library.OpenDB(ctx, path, migrations)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, now: time.Now, fetching: map[string]chan struct{}{}}
	s.bg, s.cancel = context.WithCancel(context.Background())
	return s, s.reload(ctx)
}

func (s *Store) Close() error {
	s.cancel()
	return s.db.Close()
}

func (s *Store) reload(ctx context.Context) error {
	tracks, err := s.counts(ctx, "track_id")
	if err != nil {
		return err
	}
	albums, err := s.counts(ctx, "album_id")
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.tracks, s.albums = tracks, albums
	s.mu.Unlock()
	return nil
}

func (s *Store) counts(ctx context.Context, col string) (map[string]stat, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(
		"SELECT library, %[1]s, COUNT(*), MAX(played_at) FROM plays WHERE %[1]s != '' GROUP BY library, %[1]s", col))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]stat{}
	for rows.Next() {
		var lib, id string
		var st stat
		if err := rows.Scan(&lib, &id, &st.n, &st.last); err != nil {
			return nil, err
		}
		out[key(lib, id)] = st
	}
	return out, rows.Err()
}

func (s *Store) Add(ctx context.Context, t source.Track, at time.Time) error {
	t.PlayCount, t.LastPlayed = 0, 0
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	when := at.Unix()
	if _, err := s.db.ExecContext(ctx, "INSERT INTO plays VALUES (?, ?, ?, ?, ?)", t.Library, t.ID, t.AlbumID, when, data); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bump := func(m map[string]stat, id string) {
		if id == "" {
			return
		}
		st := m[key(t.Library, id)]
		st.n++
		st.last = max(st.last, when)
		m[key(t.Library, id)] = st
	}
	bump(s.tracks, t.ID)
	bump(s.albums, t.AlbumID)
	return nil
}

// Prune drops plays older than months.
func (s *Store) Prune(ctx context.Context, months int) error {
	cut := s.now().AddDate(0, -months, 0).Unix()
	res, err := s.db.ExecContext(ctx, "DELETE FROM plays WHERE played_at < ?", cut)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	return s.reload(ctx)
}

func (s *Store) Clear(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM plays"); err != nil {
		return err
	}
	return s.reload(ctx)
}

// List returns plays newest first, before a unix time (0: now).
func (s *Store) List(ctx context.Context, limit int, before int64) ([]Play, error) {
	if before <= 0 {
		before = 1 << 62
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT track, played_at FROM plays WHERE played_at < ? ORDER BY played_at DESC, rowid DESC LIMIT ?", before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Play{}
	for rows.Next() {
		var data []byte
		var p Play
		if err := rows.Scan(&data, &p.PlayedAt); err != nil {
			return nil, err
		}
		if json.Unmarshal(data, &p.Track) != nil {
			continue
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// played is one track's merged count.
type played struct {
	track source.Track
	n     int
	last  int64
}

// Top lists tracks, albums or artists (kind) of all time by play count, or
// by last play when recent is set: this device's plays merged with the
// servers' own counts (see merge).
func (s *Store) Top(ctx context.Context, kind string, limit int, recent bool) ([]Count, error) {
	local, err := s.localPlays(ctx)
	if err != nil {
		return nil, err
	}
	all, err := s.merge(ctx, local)
	if err != nil {
		return nil, err
	}
	var out []Count
	switch kind {
	case source.ItemAlbum:
		out = group(all, func(t source.Track) []source.ArtistRef {
			if t.AlbumID == "" {
				return nil
			}
			return []source.ArtistRef{{ID: t.AlbumID}}
		}, func(t source.Track, _ source.ArtistRef, n int, last int64) Count {
			return Count{Album: &source.Album{Library: t.Library, ID: t.AlbumID, Name: t.Album, Artist: t.AlbumArtist,
				Year: t.Year, Genre: t.Genre, CoverID: t.CoverID, PlayCount: n, LastPlayed: last}}
		})
	case source.ItemArtist:
		out = group(all, func(t source.Track) []source.ArtistRef {
			if len(t.Artists) > 0 {
				return t.Artists
			}
			return source.SingleArtist(t.Artist, t.ArtistID)
		}, func(t source.Track, a source.ArtistRef, _ int, _ int64) Count {
			return Count{Artist: &Artist{Library: t.Library, ID: a.ID, Name: a.Name}}
		})
	default:
		for _, p := range all {
			t := p.track
			t.PlayCount, t.LastPlayed = p.n, p.last
			out = append(out, Count{Track: &t, Count: p.n, LastPlayed: p.last})
		}
	}
	slices.SortStableFunc(out, func(a, b Count) int {
		if !recent && a.Count != b.Count {
			return cmp.Compare(b.Count, a.Count)
		}
		return cmp.Compare(b.LastPlayed, a.LastPlayed)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []Count{}
	}
	return out, nil
}

// group adds up tracks per key (an album, an artist); keys gives a track's
// keys, an ID or else a Name. The newest track of a key makes its item.
func group(all []played, keys func(source.Track) []source.ArtistRef, item func(source.Track, source.ArtistRef, int, int64) Count) []Count {
	type acc struct {
		newest  played
		ref     source.ArtistRef
		n       int
		last    int64
		ordinal int
	}
	byKey := map[string]*acc{}
	for i, p := range all {
		for _, k := range keys(p.track) {
			id := "id:" + k.ID
			if k.ID == "" {
				id = "name:" + strings.ToLower(k.Name)
			}
			a := byKey[key(p.track.Library, id)]
			if a == nil {
				a = &acc{ordinal: i, ref: k}
				byKey[key(p.track.Library, id)] = a
			}
			a.n += p.n
			if p.last >= a.last {
				a.last, a.newest = p.last, p
			}
		}
	}
	accs := make([]*acc, 0, len(byKey))
	for _, a := range byKey {
		accs = append(accs, a)
	}
	slices.SortFunc(accs, func(a, b *acc) int { return cmp.Compare(a.ordinal, b.ordinal) })
	out := make([]Count, len(accs))
	for i, a := range accs {
		c := item(a.newest.track, a.ref, a.n, a.last)
		c.Count, c.LastPlayed = a.n, a.last
		out[i] = c
	}
	return out
}

// localPlays is this device's plays per track, each with its newest track
// snapshot.
func (s *Store) localPlays(ctx context.Context) ([]played, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT track, n, last FROM (
			SELECT track, played_at AS last, COUNT(*) OVER w AS n,
				ROW_NUMBER() OVER (PARTITION BY library, track_id ORDER BY played_at DESC, rowid DESC) AS r
			FROM plays
			WINDOW w AS (PARTITION BY library, track_id))
		WHERE r = 1 ORDER BY last DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []played
	for rows.Next() {
		var data []byte
		var p played
		if err := rows.Scan(&data, &p.n, &p.last); err != nil {
			return nil, err
		}
		if json.Unmarshal(data, &p.track) != nil {
			continue
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Annotate sets PlayCount and LastPlayed on tracks and albums anywhere in
// a browse reply.
func (s *Store) Annotate(v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.annotate(v)
}

func (s *Store) server(lib string) bool { return s.Server != nil && s.Server(lib) }

func (s *Store) annotate(v any) {
	switch x := v.(type) {
	case *source.Track:
		if s.server(x.Library) {
			return
		}
		st := s.tracks[key(x.Library, x.ID)]
		x.PlayCount, x.LastPlayed = st.n, st.last
	case []source.Track:
		for i := range x {
			s.annotate(&x[i])
		}
	case *source.Album:
		if s.server(x.Library) {
			return
		}
		st := s.albums[key(x.Library, x.ID)]
		x.PlayCount, x.LastPlayed = st.n, st.last
		s.annotate(x.Tracks)
	case []source.Album:
		for i := range x {
			s.annotate(&x[i])
		}
	case *[]source.Track:
		s.annotate(*x)
	case *[]source.Album:
		s.annotate(*x)
	case *source.SearchResult:
		s.annotate(x.Albums)
		s.annotate(x.Tracks)
	case *source.TrackPage:
		s.annotate(x.Tracks)
	case *source.Playlist:
		s.annotate(x.Tracks)
	}
}
