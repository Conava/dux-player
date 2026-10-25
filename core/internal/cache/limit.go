package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/atomicfile"
	"github.com/Conava/dux-player/core/internal/source"
)

// The size limit and the local playlists kept on this device live in
// $XDG_CONFIG_HOME/dplayer/cache.json, so clearing the cache directory
// doesn't reset them. Keys this version doesn't know are kept when it is
// rewritten.

func limitPath(dir string) string { return filepath.Join(dir, "cache.json") }

// readFile reads cache.json; a missing file is empty.
func readFile(dir string) (map[string]any, error) {
	m := map[string]any{}
	data, err := os.ReadFile(limitPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err == nil {
		err = json.Unmarshal(data, &m)
	}
	return m, err
}

func loadLimit(dir string) int64 {
	m, _ := readFile(dir)
	if v, ok := m["limitBytes"].(float64); ok && v >= minLimit {
		return int64(v)
	}
	return DefaultLimit
}

// SetLimit changes the size limit and evicts down to it.
func (c *Cache) SetLimit(ctx context.Context, bytes int64) error {
	if bytes < minLimit {
		return fmt.Errorf("the limit must be at least %d MB", minLimit>>20)
	}
	if err := c.writeKey("limitBytes", bytes); err != nil {
		return err
	}
	c.mu.Lock()
	raised := bytes > c.limit
	c.limit, c.full = bytes, false
	c.mu.Unlock()
	if raised {
		go c.syncAll(context.WithoutCancel(ctx), all)
	}
	err := c.evict(ctx, source.Ref{}, false)
	c.changed()
	return err
}

// syncAll syncs every library.
func (c *Cache) syncAll(ctx context.Context, match func(key string) bool) {
	infos, err := c.cfg.Libraries.List(ctx)
	if err != nil {
		return
	}
	for _, info := range infos {
		c.Sync(ctx, info.ID, match)
	}
}

func (c *Cache) writeKey(key string, v any) error {
	return c.editFile(func(m map[string]any) { m[key] = v })
}

// editFile changes cache.json in one read-modify-write. A file that doesn't
// parse is moved aside to cache.json.bad rather than overwritten.
func (c *Cache) editFile(edit func(m map[string]any)) error {
	c.fileMu.Lock()
	defer c.fileMu.Unlock()
	path := limitPath(c.cfg.ConfigDir)
	m, err := readFile(c.cfg.ConfigDir)
	if err != nil {
		log.Warnf("cache: %s is unreadable, moved aside: %v", path, err)
		if err := os.Rename(path, path+".bad"); err != nil {
			return err
		}
		m = map[string]any{}
	}
	edit(m)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(path, data, 0o600)
}

func (c *Cache) localPlaylists() []string {
	c.fileMu.Lock()
	defer c.fileMu.Unlock()
	m, _ := readFile(c.cfg.ConfigDir)
	return playlistIDs(m)
}

func playlistIDs(m map[string]any) []string {
	out := []string{}
	list, _ := m["localPlaylists"].([]any)
	for _, v := range list {
		if id, ok := v.(string); ok && id != "" {
			out = append(out, id)
		}
	}
	return out
}

// keepLocal keeps a local playlist on this device or lets it go.
func (c *Cache) keepLocal(ctx context.Context, r Rule, keep bool) error {
	if r.Kind != KindPlaylist || r.ID == "" {
		return errors.New("only playlists of this device can be kept without a library")
	}
	err := c.editFile(func(m map[string]any) {
		list := slices.DeleteFunc(playlistIDs(m), func(x string) bool { return x == r.ID })
		if keep {
			list = append(list, r.ID)
		}
		m["localPlaylists"] = list
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.full = false
	c.mu.Unlock()
	key := localPrefix + r.ID
	if keep {
		go c.syncAll(context.WithoutCancel(ctx), func(k string) bool { return k == key })
	} else {
		c.syncAll(ctx, none)
	}
	c.changed()
	return nil
}

// LocalPlaylistChanged resyncs after a kept local playlist was edited.
func (c *Cache) LocalPlaylistChanged(ctx context.Context, id string) {
	if slices.Contains(c.localPlaylists(), id) {
		c.syncAll(ctx, func(k string) bool { return k == localPrefix+id })
	}
}
