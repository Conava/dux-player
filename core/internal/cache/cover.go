package cache

import (
	"context"
	"database/sql"

	"github.com/Conava/dux-player/core/internal/source"
)

// The covers table says which rules hold which tracks of a library, cached
// or not yet. A copy is deleted only when it loses its last row, so copies
// no rule ever claimed (an older version's) stay until the size limit
// evicts them.

// setCover replaces what a rule covers and returns the ids it no longer does.
func (c *Cache) setCover(ctx context.Context, lib, key string, ids []string, static bool) ([]string, error) {
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
	for _, id := range ids {
		if now[id] {
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
	if !ok {
		// The library went away mid-sync: its rules are unknown, not gone.
		return nil, nil
	}
	keep := map[string]bool{}
	for _, r := range rules {
		keep[r.key()] = true
	}
	for _, id := range c.localPlaylists() {
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
// to its rule's limit. It returns the ids they let go and the mostPlayed
// tracks to fetch.
func (c *Cache) dynamic(ctx context.Context, lib string, rules []Rule) ([]string, []source.Track, error) {
	var lost []string
	var fetch []source.Track
	for _, r := range rules {
		if !r.dynamic() {
			continue
		}
		var ids []string
		var err error
		if r.Kind == KindPlayed {
			ids, err = c.recentlyPlayed(ctx, lib, *r.Limit)
		} else {
			fetch, err = c.mostPlayed(ctx, lib, *r.Limit)
			for _, t := range fetch {
				ids = append(ids, t.ID)
			}
		}
		if err != nil {
			return lost, nil, err
		}
		l, err := c.setCover(ctx, lib, r.key(), ids, false)
		if err != nil {
			return lost, nil, err
		}
		lost = append(lost, l...)
	}
	return lost, fetch, nil
}

func (c *Cache) recentlyPlayed(ctx context.Context, lib string, l Limit) ([]string, error) {
	rows, err := c.db.QueryContext(ctx, "SELECT id, size FROM entries WHERE library_id = ? AND played > 0 ORDER BY played DESC, id", lib)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	var sizes []int64
	for rows.Next() {
		var id string
		var size int64
		if err := rows.Scan(&id, &size); err != nil {
			return nil, err
		}
		ids, sizes = append(ids, id), append(sizes, size)
	}
	return ids[:cut(l, sizes)], rows.Err()
}

// mostPlayed cuts the library's top tracks, as last read from the play
// history, to the limit; tracks not cached yet count with a guessed size.
func (c *Cache) mostPlayed(ctx context.Context, lib string, l Limit) ([]source.Track, error) {
	c.mu.Lock()
	top := c.top[lib]
	c.mu.Unlock()
	sizes := make([]int64, len(top))
	for i, t := range top {
		sizes[i] = guessSize
		if l.Bytes > 0 {
			_ = c.db.QueryRowContext(ctx, "SELECT size FROM entries WHERE library_id = ? AND id = ?", lib, t.ID).Scan(&sizes[i])
		}
	}
	return top[:cut(l, sizes)], nil
}

// wantedIDs are the tracks a sync fetches: everything covered except what
// only played holds, which is fetched as it plays.
func (c *Cache) wantedIDs(ctx context.Context, lib string) []source.Track {
	ids, _ := column(ctx, c.db, "SELECT DISTINCT id FROM covers WHERE library_id = ? AND rule != ?", lib, KindPlayed)
	out := make([]source.Track, len(ids))
	for i, id := range ids {
		out[i] = source.Track{Library: lib, ID: id}
	}
	return out
}
