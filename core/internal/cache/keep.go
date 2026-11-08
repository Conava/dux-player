package cache

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Conava/dux-player/core/internal/source"
	"github.com/duxlabs-dev/dux-go/log"
)

// keptKey covers single tracks kept by hand. It sits with the rules' coverage,
// so eviction and removing a rule treat it alike, but it is no rule: it isn't
// listed with them, and clearing the downloads ends it.
const keptKey = "track"

// Keep keeps server tracks on this device or lets them go. A track a rule
// holds too stays until the rule goes.
func (c *Cache) Keep(ctx context.Context, refs []source.Ref, keep bool) error {
	byLibrary := map[string][]string{}
	for _, ref := range refs {
		if _, _, ok := c.rules(ctx, ref.Library); !ok {
			return fmt.Errorf("library %s can't keep tracks on this device", ref.Library)
		}
		byLibrary[ref.Library] = append(byLibrary[ref.Library], ref.ID)
	}
	if keep {
		if _, err := c.cfg.Libraries.Tracks(ctx, refs); errors.Is(err, source.ErrNotFound) {
			return fmt.Errorf("a track to keep: %w", err)
		}
	}
	// room may have been made or the copy may fit now; evict marks it full again
	c.mu.Lock()
	wasFull := c.full
	c.full = false
	c.mu.Unlock()
	for lib, ids := range byLibrary {
		for _, id := range ids {
			query := "DELETE FROM covers WHERE library_id = ? AND id = ? AND rule = ?"
			if keep {
				query = "INSERT OR IGNORE INTO covers (library_id, id, rule, static) VALUES (?, ?, ?, 1)"
			}
			if _, err := c.db.ExecContext(ctx, query, lib, id, keptKey); err != nil {
				return err
			}
		}
		if !keep {
			if err := c.prune(ctx, lib, ids); err != nil {
				return err
			}
		}
		c.refresh(ctx, lib, false)
		if keep {
			c.enqueue(c.missingKept(ctx, lib), true)
		}
	}
	if wasFull && !keep {
		c.Spawn(func() { c.syncAll(c.ctx, all) })
	}
	c.changed()
	return nil
}

// missingKept lists the tracks kept by hand with no copy yet: downloads an
// earlier run didn't finish, or copies that went away.
func (c *Cache) missingKept(ctx context.Context, lib string) []source.Track {
	c.mu.Lock()
	full := c.full
	c.mu.Unlock()
	if full {
		return nil
	}
	ids, err := column(ctx, c.db, `SELECT id FROM covers c WHERE library_id = ? AND rule = ? AND NOT EXISTS
		(SELECT 1 FROM entries e WHERE e.library_id = c.library_id AND e.id = c.id)`, lib, keptKey)
	if err != nil || len(ids) == 0 {
		return nil
	}
	var tracks []source.Track
	for _, id := range ids {
		c.mu.Lock()
		queued := c.queued[source.Ref{Library: lib, ID: id}]
		c.mu.Unlock()
		if queued {
			continue
		}
		// one at a time: a track gone from the server must not hold back the rest
		t, err := c.cfg.Libraries.Tracks(ctx, []source.Ref{{Library: lib, ID: id}})
		if err != nil {
			log.Warnf("cache: track %s kept on this device: %v", id, err)
			continue
		}
		tracks = append(tracks, t...)
	}
	return tracks
}

// KeepState counts tracks kept by hand, and those only a pinning rule holds.
type KeepState struct {
	Kept int `json:"kept"`
	Held int `json:"held"`
}

func (c *Cache) KeepState(ctx context.Context, refs []source.Ref) (KeepState, error) {
	var st KeepState
	for _, ref := range refs {
		keys, err := column(ctx, c.db, "SELECT rule FROM covers WHERE library_id = ? AND id = ? AND static", ref.Library, ref.ID)
		if err != nil {
			return st, err
		}
		switch {
		case slices.Contains(keys, keptKey):
			st.Kept++
		case len(keys) > 0:
			st.Held++
		}
	}
	return st, nil
}
