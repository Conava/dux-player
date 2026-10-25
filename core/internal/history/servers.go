package history

import (
	"context"
	"encoding/json"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/source"
)

// Server counts are refreshed only when Top asks: the cheap refresh (a
// server's PlayStats, or its RecentPlays) at most every recentEvery, a full
// read through TrackPager at most every fullEvery, in the background.
const (
	recentEvery  = 15 * time.Minute
	fullEvery    = 24 * time.Hour
	retryAfter   = time.Minute
	fullRetry    = time.Hour
	fetchTimeout = 30 * time.Second
	fullTimeout  = time.Hour
	// maxServerTracks bounds a full read (500 per page).
	maxServerTracks = 200_000
)

// pagePause keeps a full read gentle on the server; tests set it to 0.
var pagePause = 250 * time.Millisecond

const (
	refreshRecent = "recent_at"
	refreshFull   = "full_at"
)

// merge lays the servers' counts over this device's plays. A server count
// already holds this device's scrobbled plays, so a track counts max(local
// plays, server count + local plays since the server's count was read).
func (s *Store) merge(ctx context.Context, local []played) ([]played, error) {
	index := map[string]int{}
	for i, p := range local {
		index[key(p.track.Library, p.track.ID)] = i
	}
	out := local
	for _, lib := range s.refreshServers(ctx) {
		rows, err := s.db.QueryContext(ctx, `SELECT c.track, c.count, c.last, (SELECT COUNT(*) FROM plays p
			WHERE p.library = c.library AND p.track_id = c.track_id AND p.played_at >= c.at)
			FROM server_counts c WHERE c.library = ?`, lib)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var data []byte
			var t source.Track
			var n, after int
			var last int64
			if err := rows.Scan(&data, &n, &last, &after); err != nil {
				rows.Close()
				return nil, err
			}
			if json.Unmarshal(data, &t) != nil {
				continue
			}
			p := played{track: t, n: n + after, last: last}
			if i, ok := index[key(lib, t.ID)]; ok {
				p.n, p.last = max(p.n, out[i].n), max(p.last, out[i].last)
				out[i] = p
				continue
			}
			index[key(lib, t.ID)] = len(out)
			out = append(out, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// refreshServers refreshes what is due and returns the libraries whose
// stored counts count: the working servers.
func (s *Store) refreshServers(ctx context.Context) []string {
	if s.Sources == nil {
		return nil
	}
	srcs, err := s.Sources(ctx)
	if err != nil {
		return nil
	}
	var libs []string
	for _, src := range srcs {
		lib := src.Library().ID
		switch x := src.(type) {
		case source.PlayStats:
			libs = append(libs, lib)
			s.refresh(ctx, lib, refreshRecent, recentEvery, retryAfter, fetchTimeout, func(ctx context.Context) error {
				tracks, err := x.PlayStats(ctx)
				if err != nil {
					return err
				}
				return s.store(ctx, lib, tracks, true)
			})
		case source.RecentPlays:
			tp, ok := src.(source.TrackPager)
			if !ok {
				continue
			}
			libs = append(libs, lib)
			s.refresh(ctx, lib, refreshRecent, recentEvery, retryAfter, fetchTimeout, func(ctx context.Context) error {
				tracks, err := x.RecentPlays(ctx)
				if err != nil {
					return err
				}
				return s.store(ctx, lib, tracks, false)
			})
			if s.due(ctx, lib, refreshFull, fullEvery) {
				go s.refresh(s.bg, lib, refreshFull, fullEvery, fullRetry, fullTimeout, func(ctx context.Context) error {
					return s.readAll(ctx, lib, tp)
				})
			}
		}
	}
	return libs
}

func (s *Store) due(ctx context.Context, lib, col string, every time.Duration) bool {
	var at int64
	_ = s.db.QueryRowContext(ctx, "SELECT "+col+" FROM server_refresh WHERE library = ?", lib).Scan(&at)
	return s.now().Sub(time.Unix(at, 0)) >= every
}

// refresh runs fn when col is due, one run per library and kind at a time;
// a caller that finds one running waits for it (or for its ctx). A failed
// run is tried again after retry.
func (s *Store) refresh(ctx context.Context, lib, col string, every, retry, timeout time.Duration, fn func(context.Context) error) {
	if !s.due(ctx, lib, col, every) {
		return
	}
	k := key(lib, col)
	s.mu.Lock()
	done, busy := s.fetching[k]
	if !busy {
		done = make(chan struct{})
		s.fetching[k] = done
	}
	s.mu.Unlock()
	if busy {
		select {
		case <-done:
		case <-ctx.Done():
		}
		return
	}
	defer func() {
		s.mu.Lock()
		delete(s.fetching, k)
		close(done)
		s.mu.Unlock()
	}()
	// Not only the request's context: a closed request still fills the store.
	fctx, cancel := context.WithTimeout(s.bg, timeout)
	defer cancel()
	at := s.now()
	if err := fn(fctx); err != nil {
		log.Infof("play counts of library %s: %v", lib, err)
		at = at.Add(retry - every)
	}
	if _, err := s.db.ExecContext(s.bg, `INSERT INTO server_refresh (library, `+col+`) VALUES (?, ?)
		ON CONFLICT (library) DO UPDATE SET `+col+` = excluded.`+col, lib, at.Unix()); err != nil {
		log.Warnf("history: %v", err)
	}
}

// readAll reads every track of a server page by page and keeps the played
// ones; tracks the read didn't see as played are dropped.
func (s *Store) readAll(ctx context.Context, lib string, tp source.TrackPager) error {
	gen := s.now().UnixNano()
	for offset := 0; offset < maxServerTracks; {
		page, err := tp.TrackPage(ctx, source.TrackQuery{Offset: offset, Limit: 500})
		if err != nil {
			return err
		}
		if err := s.write(ctx, lib, page.Tracks, gen); err != nil {
			return err
		}
		offset += len(page.Tracks)
		if len(page.Tracks) == 0 || offset >= page.Total {
			break
		}
		select {
		case <-time.After(pagePause):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM server_counts WHERE library = ? AND gen < ?", lib, gen)
	return err
}

// store writes a refresh; whole replaces the library's counts (the server
// listed all its played tracks).
func (s *Store) store(ctx context.Context, lib string, tracks []source.Track, whole bool) error {
	gen := s.now().UnixNano()
	if err := s.write(ctx, lib, tracks, gen); err != nil || !whole {
		return err
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM server_counts WHERE library = ? AND gen < ?", lib, gen)
	return err
}

func (s *Store) write(ctx context.Context, lib string, tracks []source.Track, gen int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := s.now().Unix()
	for _, t := range tracks {
		if t.PlayCount <= 0 && t.LastPlayed <= 0 {
			continue
		}
		t.Library = lib
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO server_counts VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (library, track_id) DO UPDATE SET count = excluded.count, last = excluded.last,
				at = excluded.at, gen = excluded.gen, track = excluded.track`,
			lib, t.ID, max(t.PlayCount, 0), t.LastPlayed, at, gen, data); err != nil {
			return err
		}
	}
	return tx.Commit()
}
