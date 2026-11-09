package cache

import (
	"context"
	"database/sql"
	"slices"

	"github.com/Conava/dux-player/core/internal/source"
)

// The covers table says which rules hold which tracks of a library, cached
// or not yet. A copy is deleted only when it loses its last row, so copies
// no rule ever claimed (an older version's) stay until the size limit
// evicts them.

// setCover replaces what a rule covers, less the excluded tracks, and
// returns the ids it no longer does.
func (c *Cache) setCover(ctx context.Context, lib, key string, ids []string, static bool) ([]string, error) {
	// One lock for every library's covers writes, which SQLite
	// serialises anyway; per-library locks if that ever shows.
	c.exMu.Lock()
	defer c.exMu.Unlock()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	old, err := column(ctx, tx, "SELECT id FROM covers WHERE library_id = ? AND rule = ?", lib, key)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM covers WHERE library_id = ? AND rule = ?", lib, key); err != nil {
		return nil, err
	}
	now := map[string]bool{}
	excluded := c.excludedIn(lib)
	for _, id := range ids {
		if now[id] || excluded[id] {
			continue
		}
		now[id] = true
		if _, err := tx.ExecContext(ctx, "INSERT INTO covers (library_id, id, rule, static) VALUES (?, ?, ?, ?)",
			lib, id, key, static); err != nil {
			return nil, err
		}
	}
	var lost []string
	for _, id := range old {
		if !now[id] {
			lost = append(lost, id)
		}
	}
	return lost, tx.Commit()
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func column(ctx context.Context, q querier, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// dropStale removes the coverage of a library's rules that are gone. It
// rereads the rules, so a rule added while a sync ran is kept.
func (c *Cache) dropStale(ctx context.Context, lib string) ([]string, error) {
	rules, _, ok := c.rules(ctx, lib)
	c.mu.Lock()
	unread := c.rulesUnread
	c.mu.Unlock()
	local, err := c.readPlaylists()
	if !ok || unread || err != nil {
		// The library went away mid-sync or cache.json is unreadable: the
		// rules are unknown, not gone.
		return nil, nil
	}
	keep := map[string]bool{keptKey: true}
	for _, r := range rules {
		keep[r.key()] = true
	}
	for _, id := range local {
		keep[localPrefix+id] = true
	}
	keys, err := column(ctx, c.db, "SELECT DISTINCT rule FROM covers WHERE library_id = ?", lib)
	if err != nil {
		return nil, err
	}
	var lost []string
	for _, k := range keys {
		if keep[k] {
			continue
		}
		l, err := c.setCover(ctx, lib, k, nil, false)
		if err != nil {
			return lost, err
		}
		lost = append(lost, l...)
	}
	return lost, nil
}

// prune deletes the copies among ids that no rule covers any more.
func (c *Cache) prune(ctx context.Context, lib string, ids []string) error {
	for _, id := range ids {
		if err := c.remove(ctx, `library_id = ? AND id = ? AND NOT EXISTS
			(SELECT 1 FROM covers WHERE covers.library_id = entries.library_id AND covers.id = entries.id)`, lib, id); err != nil {
			return err
		}
	}
	return nil
}

// dynamic recomputes what a library's played and mostPlayed rules hold:
// the most recently played copies, and the top of the play counts, each cut
// to its rule's limit. A rule for all server libraries ranks those of
// servers together and holds this library's share. It returns the ids they
// let go and the mostPlayed tracks to fetch.
func (c *Cache) dynamic(ctx context.Context, lib string, rules []Rule, servers []string) ([]string, []source.Track, error) {
	var lost []string
	var fetch []source.Track
	for _, r := range rules {
		if !r.dynamic() {
			continue
		}
		in := []string{lib}
		if r.Library == "" {
			if !slices.Contains(servers, lib) {
				// lib's rules couldn't be read just now: keep what it holds.
				continue
			}
			in = servers
		}
		var ids []string
		if r.Kind == KindPlayed {
			refs, err := c.recentlyPlayed(ctx, in, *r.Limit)
			if err != nil {
				return lost, nil, err
			}
			for _, ref := range refs {
				if ref.Library == lib {
					ids = append(ids, ref.ID)
				}
			}
		} else {
			top, known := c.mostPlayed(ctx, in, lib, r.Library == "", *r.Limit)
			if !known {
				// Unread play counts are unknown, not empty: keep what it holds.
				continue
			}
			for _, t := range top {
				if t.Library == lib {
					ids, fetch = append(ids, t.ID), append(fetch, t)
				}
			}
		}
		l, err := c.setCover(ctx, lib, r.key(), ids, false)
		if err != nil {
			return lost, nil, err
		}
		lost = append(lost, l...)
	}
	return lost, fetch, nil
}

// recentlyPlayed cuts the played copies of libs, most recent first, to the
// limit.
func (c *Cache) recentlyPlayed(ctx context.Context, libs []string, l Limit) ([]source.Ref, error) {
	rows, err := c.db.QueryContext(ctx, "SELECT library_id, id, size FROM entries WHERE played > 0 ORDER BY played DESC, library_id, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refs []source.Ref
	var sizes []int64
	for rows.Next() {
		var ref source.Ref
		var size int64
		if err := rows.Scan(&ref.Library, &ref.ID, &size); err != nil {
			return nil, err
		}
		if slices.Contains(libs, ref.Library) {
			refs, sizes = append(refs, ref), append(sizes, size)
		}
	}
	return refs[:cut(l, sizes)], rows.Err()
}

// mostPlayed cuts the top tracks of libs, as last read from the play
// history, to the limit: from the list of every server library when
// global, else lib's own; known is false while they aren't read. Tracks
// not cached yet count with a guessed size.
func (c *Cache) mostPlayed(ctx context.Context, libs []string, lib string, global bool, l Limit) (top []source.Track, known bool) {
	if global {
		lib = ""
	}
	c.mu.Lock()
	list, known := c.top[lib]
	if len(c.excluded) > 0 {
		list = slices.DeleteFunc(slices.Clone(list), func(t source.Track) bool { return c.excluded[t.Ref()] })
	}
	c.mu.Unlock()
	var sizes []int64
	for _, t := range list {
		if !slices.Contains(libs, t.Library) {
			continue
		}
		size := int64(guessSize)
		if l.Bytes > 0 {
			_ = c.db.QueryRowContext(ctx, "SELECT size FROM entries WHERE library_id = ? AND id = ?", t.Library, t.ID).Scan(&size)
		}
		top, sizes = append(top, t), append(sizes, size)
	}
	return top[:cut(l, sizes)], known
}

// wantedIDs are the tracks a sync fetches: everything covered except what
// only played holds, which is fetched as it plays.
func (c *Cache) wantedIDs(ctx context.Context, lib string) []source.Track {
	ids, _ := column(ctx, c.db, "SELECT DISTINCT id FROM covers WHERE library_id = ? AND rule NOT IN (?, ?)", lib, KindPlayed, allPrefix+KindPlayed)
	out := make([]source.Track, len(ids))
	for i, id := range ids {
		out[i] = source.Track{Library: lib, ID: id}
	}
	return out
}
