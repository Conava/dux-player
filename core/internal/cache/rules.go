package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Conava/dux-player/core/internal/source"
)

// Rule kinds. Static rules pin their tracks; the dynamic ones (played,
// mostPlayed) hold up to their own limit and are what the size limit evicts.
const (
	KindEverything = "everything"
	KindFavorites  = "favorites"
	KindPlaylist   = "playlist"
	KindAlbum      = "album"
	KindArtist     = "artist"
	KindPlayed     = "played"
	KindMostPlayed = "mostPlayed"

	// localPrefix keys the coverage of a local playlist, which can hold
	// tracks of every server library.
	localPrefix = "localPlaylist:"

	maxSongs     = 100_000
	minRuleBytes = 10 << 20
	// guessSize stands in for the size of a track that isn't cached yet
	// when a rule's limit is a size.
	guessSize = 8 << 20
)

var kinds = []string{KindEverything, KindFavorites, KindPlaylist, KindAlbum, KindArtist, KindPlayed, KindMostPlayed}

// Limit bounds a dynamic rule by a song count or a size, exactly one of them.
type Limit struct {
	Songs int   `json:"songs,omitempty"`
	Bytes int64 `json:"bytes,omitempty"`
}

type Rule struct {
	Kind  string `json:"kind"`
	ID    string `json:"id,omitempty"`
	Limit *Limit `json:"limit,omitempty"`
}

func (r Rule) dynamic() bool { return r.Kind == KindPlayed || r.Kind == KindMostPlayed }

func (r Rule) key() string {
	if r.ID == "" {
		return r.Kind
	}
	return r.Kind + ":" + r.ID
}

// RuleRef names a rule that holds a track; Local marks a local playlist.
type RuleRef struct {
	Key   string `json:"key"`
	Kind  string `json:"kind"`
	ID    string `json:"id,omitempty"`
	Local bool   `json:"local,omitempty"`
}

func refOf(key string) RuleRef {
	if id, ok := strings.CutPrefix(key, localPrefix); ok {
		return RuleRef{Key: key, Kind: KindPlaylist, ID: id, Local: true}
	}
	kind, id, _ := strings.Cut(key, ":")
	return RuleRef{Key: key, Kind: kind, ID: id}
}

func validRule(r Rule) (Rule, error) {
	r.ID = source.Clean(r.ID)
	if !slices.Contains(kinds, r.Kind) {
		return r, fmt.Errorf("kind must be one of %s", strings.Join(kinds, ", "))
	}
	needsID := r.Kind == KindPlaylist || r.Kind == KindAlbum || r.Kind == KindArtist
	if needsID != (r.ID != "") {
		if needsID {
			return r, fmt.Errorf("a %s rule needs an id", r.Kind)
		}
		return r, fmt.Errorf("a %s rule takes no id", r.Kind)
	}
	if !r.dynamic() {
		r.Limit = nil
		return r, nil
	}
	return r, validLimit(r.Limit)
}

func validLimit(l *Limit) error {
	switch {
	case l == nil || (l.Songs > 0) == (l.Bytes > 0):
		return errors.New("the limit is either songs or bytes")
	case l.Songs > maxSongs:
		return fmt.Errorf("the limit is at most %d songs", maxSongs)
	case l.Songs < 0 || l.Bytes < 0 || (l.Bytes > 0 && l.Bytes < minRuleBytes):
		return fmt.Errorf("the limit is at least 1 song or %d MB", minRuleBytes>>20)
	}
	return nil
}

// parseRules reads a library's rules from its config; invalid and repeated
// rules are skipped.
func parseRules(raw any) []Rule {
	m, _ := raw.(map[string]any)
	data, _ := json.Marshal(m["rules"])
	var list []Rule
	_ = json.Unmarshal(data, &list)
	out := []Rule{}
	for _, r := range list {
		r, err := validRule(r)
		if err == nil && !slices.ContainsFunc(out, func(x Rule) bool { return x.key() == r.key() }) {
			out = append(out, r)
		}
	}
	return out
}

// migrateConfig turns the old cache config (one mode plus chosen playlists)
// into rules; ok is false when there is nothing to migrate. Old played mode
// had no limit of its own, so its rule gets the size limit.
func migrateConfig(raw any, limit int64) (map[string]any, bool) {
	m, _ := raw.(map[string]any)
	_, hasMode := m["mode"]
	_, hasLists := m["playlists"]
	if _, done := m["rules"]; done || (!hasMode && !hasLists) {
		return nil, false
	}
	rules := []Rule{}
	switch m["mode"] {
	case "played":
		rules = append(rules, Rule{Kind: KindPlayed, Limit: &Limit{Bytes: limit}})
	case "favorites":
		rules = append(rules, Rule{Kind: KindFavorites})
	case "all":
		rules = append(rules, Rule{Kind: KindEverything})
	}
	list, _ := m["playlists"].([]any)
	for _, v := range list {
		if id, _ := v.(string); source.Clean(id) != "" {
			rules = append(rules, Rule{Kind: KindPlaylist, ID: source.Clean(id)})
		}
	}
	out := map[string]any{}
	for k, v := range m {
		if k != "mode" && k != "playlists" {
			out[k] = v
		}
	}
	out["rules"] = rules
	return out, true
}

// migrate rewrites the libraries still on the old cache config.
func (c *Cache) migrate(ctx context.Context) error {
	infos, err := c.cfg.Libraries.List(ctx)
	if err != nil {
		return err
	}
	for _, info := range infos {
		if m, ok := migrateConfig(info.Config[configKey], c.limit); ok {
			if err := c.cfg.Libraries.SetConfig(ctx, info.ID, configKey, m); err != nil {
				return err
			}
		}
	}
	return nil
}

// cut is how many of the tracks, in order, fit a limit.
func cut(l Limit, sizes []int64) int {
	var sum int64
	for i, s := range sizes {
		if l.Songs > 0 && i >= l.Songs {
			return i
		}
		if sum += s; l.Bytes > 0 && sum > l.Bytes {
			return i
		}
	}
	return len(sizes)
}

// scope lists the tracks a static rule covers. Any failure fails the whole
// list, so a flaky server never shrinks a rule and deletes its copies.
func scope(ctx context.Context, src source.Source, r Rule) ([]source.Track, error) {
	b, isBrowser := src.(source.Browser)
	switch r.Kind {
	case KindEverything:
		if !isBrowser {
			return nil, errNoBrowse
		}
		return albumTracks(ctx, b, "")
	case KindArtist:
		if !isBrowser {
			return nil, errNoBrowse
		}
		return albumTracks(ctx, b, r.ID)
	case KindAlbum:
		if !isBrowser {
			return nil, errNoBrowse
		}
		a, err := b.Album(ctx, r.ID)
		return a.Tracks, err
	case KindPlaylist:
		pls, ok := src.(source.Playlists)
		if !ok {
			return nil, errors.New("the server has no playlists")
		}
		p, err := pls.Playlist(ctx, r.ID)
		return p.Tracks, err
	case KindFavorites:
		f, ok := src.(source.Favorites)
		if !ok {
			return nil, errors.New("the server has no favorites")
		}
		res, err := f.Favorites(ctx)
		if err != nil {
			return nil, err
		}
		out := res.Tracks
		for _, a := range res.Albums {
			if !isBrowser {
				break
			}
			full, err := b.Album(ctx, a.ID)
			if err != nil {
				return nil, err
			}
			out = append(out, full.Tracks...)
		}
		return out, nil
	}
	return nil, nil
}

var errNoBrowse = errors.New("the server can't be browsed")

// albumTracks lists every album of an artist, or of the library for "".
func albumTracks(ctx context.Context, b source.Browser, artistID string) ([]source.Track, error) {
	albums, err := b.Albums(ctx, artistID)
	if err != nil {
		return nil, err
	}
	var out []source.Track
	for _, a := range albums {
		full, err := b.Album(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, full.Tracks...)
	}
	return out, nil
}

// supports says whether a server can resolve a rule.
func supports(src source.Source, kind string) bool {
	switch kind {
	case KindEverything, KindAlbum, KindArtist:
		_, ok := src.(source.Browser)
		return ok
	case KindPlaylist:
		_, ok := src.(source.Playlists)
		return ok
	case KindFavorites:
		_, ok := src.(source.Favorites)
		return ok
	}
	return true
}
