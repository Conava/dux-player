package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/source"
)

// Excluded tracks are downloaded by no rule and no prefetch until they are
// included again. Each keeps the snapshot of its last copy, so it can be
// listed offline.

const (
	defaultPage = 100
	maxPage     = 500
)

func (c *Cache) loadExcluded(ctx context.Context) error {
	rows, err := c.db.QueryContext(ctx, "SELECT library_id, id FROM excluded")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ref source.Ref
		if err := rows.Scan(&ref.Library, &ref.ID); err != nil {
			return err
		}
		c.excluded[ref] = true
	}
	return rows.Err()
}

func (c *Cache) isExcluded(ref source.Ref) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.excluded[ref]
}

// excludedIn lists a library's excluded track ids.
func (c *Cache) excludedIn(lib string) map[string]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]bool{}
	for ref := range c.excluded {
		if ref.Library == lib {
			out[ref.ID] = true
		}
	}
	return out
}

// Delete deletes a track's copy and its hand-kept mark and stops its
// download. The rules that cover it fetch it again on their next sync,
// unless exclude keeps every rule from it.
func (c *Cache) Delete(ctx context.Context, ref source.Ref, exclude bool) error {
	c.exMu.Lock()
	file, err := c.deleteRows(ctx, ref, exclude)
	if err == nil && exclude {
		c.mu.Lock()
		c.excluded[ref] = true
		c.mu.Unlock()
	}
	c.exMu.Unlock()
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.queue = slices.DeleteFunc(c.queue, func(j job) bool { return j.track.Ref() == ref })
	delete(c.queued, ref)
	if c.current != nil && c.current.track.Ref() == ref {
		c.cancel()
	}
	c.mu.Unlock()
	if file != "" {
		if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Warnf("cache: can't delete the copy of %s/%s: %v", ref.Library, ref.ID, err)
		}
	}
	if exclude {
		c.refresh(ctx, ref.Library, false)
		c.replan(ctx)
	}
	c.changed()
	return nil
}

// deleteRows deletes the copy's row and its hand-kept mark, or with exclude
// moves its snapshot to excluded and deletes all its covers rows; it returns
// the copy's file, "" for none.
func (c *Cache) deleteRows(ctx context.Context, ref source.Ref, exclude bool) (string, error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var file string
	err = tx.QueryRowContext(ctx, "SELECT file FROM entries WHERE library_id = ? AND id = ?", ref.Library, ref.ID).Scan(&file)
	switch {
	case errors.Is(err, sql.ErrNoRows) && exclude:
		return "", errNotDownloaded
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return "", err
	}
	if exclude {
		res, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO excluded (library_id, id, track) SELECT library_id, id, track FROM entries WHERE library_id = ? AND id = ?",
			ref.Library, ref.ID)
		if err != nil {
			return "", err
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return "", errNotDownloaded
		}
	}
	stmts := []string{"DELETE FROM covers WHERE library_id = ? AND id = ? AND rule = '" + keptKey + "'"}
	if exclude {
		stmts = []string{"DELETE FROM covers WHERE library_id = ? AND id = ?"}
	}
	// The row goes before the file: a crash in between leaves a stray file,
	// which the next start sweeps.
	stmts = append(stmts, "DELETE FROM entries WHERE library_id = ? AND id = ?")
	for _, q := range stmts {
		if _, err := tx.ExecContext(ctx, q, ref.Library, ref.ID); err != nil {
			return "", err
		}
	}
	return file, tx.Commit()
}

var errNotDownloaded = errors.New("only a downloaded track can be excluded")

// Include lifts a track's exclusion and syncs its library, so the rules
// fetch it again.
func (c *Cache) Include(ctx context.Context, ref source.Ref) error {
	lifted, err := c.lift(ctx, []source.Ref{ref})
	if err != nil || !lifted {
		return err
	}
	c.includes.Trigger(c.ctx, ref.Library)
	c.changed()
	return nil
}

// lift lifts the exclusions of refs; lifted says whether there was one.
func (c *Cache) lift(ctx context.Context, refs []source.Ref) (lifted bool, err error) {
	c.exMu.Lock()
	defer c.exMu.Unlock()
	for _, ref := range refs {
		res, err := c.db.ExecContext(ctx, "DELETE FROM excluded WHERE library_id = ? AND id = ?", ref.Library, ref.ID)
		if err != nil {
			return lifted, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			lifted = true
		}
		c.mu.Lock()
		delete(c.excluded, ref)
		c.mu.Unlock()
	}
	return lifted, nil
}

// forgetExclusions drops the exclusions of libraries not in known.
func (c *Cache) forgetExclusions(known map[string]bool) {
	c.mu.Lock()
	var gone []source.Ref
	for ref := range c.excluded {
		if !known[ref.Library] {
			gone = append(gone, ref)
		}
	}
	c.mu.Unlock()
	if len(gone) == 0 {
		return
	}
	if _, err := c.lift(c.ctx, gone); err != nil {
		log.Warnf("cache: drop the exclusions of removed libraries: %v", err)
	}
}

// TrackEntry is a copy, or an excluded track with its last copy's snapshot.
type TrackEntry struct {
	Entry
	Excluded bool `json:"excluded"`
}

type TrackPage struct {
	Total int          `json:"total"`
	Items []TrackEntry `json:"items"`
}

// Tracks pages through a library's excluded tracks, then its copies most
// recently used first; excludedOnly lists only the exclusions.
func (c *Cache) Tracks(ctx context.Context, lib string, offset, limit int, excludedOnly bool) (TrackPage, error) {
	if limit <= 0 {
		limit = defaultPage
	}
	limit, offset = min(limit, maxPage), max(offset, 0)
	const union = `SELECT id, track, 1 AS ex, 0 AS size, 0 AS added, 0 AS used, 0 AS played, rowid AS o FROM excluded WHERE library_id = ?1
		UNION ALL SELECT id, track, 0, size, added, used, played, used FROM entries WHERE library_id = ?1 AND NOT ?2`
	page := TrackPage{Items: []TrackEntry{}}
	if err := c.db.QueryRowContext(ctx, "SELECT count(*) FROM ("+union+")", lib, excludedOnly).Scan(&page.Total); err != nil {
		return page, err
	}
	rows, err := c.db.QueryContext(ctx, "SELECT id, track, ex, size, added, used, played FROM ("+union+") ORDER BY ex DESC, o DESC, id LIMIT ?3 OFFSET ?4",
		lib, excludedOnly, limit, offset)
	if err != nil {
		return page, err
	}
	var ids []string
	for rows.Next() {
		var e TrackEntry
		var id, data string
		if err := rows.Scan(&id, &data, &e.Excluded, &e.Size, &e.Added, &e.Used, &e.Played); err != nil {
			rows.Close()
			return page, err
		}
		_ = json.Unmarshal([]byte(data), &e.Track)
		e.Track.Library, e.Track.ID = lib, id
		ids = append(ids, id)
		page.Items = append(page.Items, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(ids) == 0 {
		return page, nil
	}
	args := []any{lib}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err = c.db.QueryContext(ctx, "SELECT id, rule FROM covers WHERE library_id = ? AND id IN (?"+strings.Repeat(", ?", len(ids)-1)+") ORDER BY rule", args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	held := map[string][]RuleRef{}
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			return page, err
		}
		held[id] = append(held[id], refOf(key))
	}
	for i, id := range ids {
		page.Items[i].Rules = held[id]
		if held[id] == nil {
			page.Items[i].Rules = []RuleRef{}
		}
	}
	return page, rows.Err()
}
