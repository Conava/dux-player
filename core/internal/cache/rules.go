package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
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
	// allPrefix keys the coverage of a rule for all server libraries.
	allPrefix = "all:"

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

// Rule is an offline rule of one server library, or of all of them when
// Library is "".
type Rule struct {
	Kind    string `json:"kind"`
	ID      string `json:"id,omitempty"`
	Library string `json:"library,omitempty"`
	Limit   *Limit `json:"limit,omitempty"`
	// extra holds the saved fields this version doesn't know.
	extra map[string]any
}

// saved is the rule as cache.json stores it, with its unknown fields.
func (r Rule) saved() any {
	if len(r.extra) == 0 {
		return r
	}
	data, _ := json.Marshal(r)
	m := map[string]any{}
	_ = json.Unmarshal(data, &m)
	for k, v := range r.extra {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return m
}

func (r Rule) dynamic() bool { return r.Kind == KindPlayed || r.Kind == KindMostPlayed }

// key names the rule's rows in covers. A rule of one library keeps the key
// older versions gave it, so its copies stay covered.
func (r Rule) key() string {
	switch {
	case r.Library == "":
		return allPrefix + r.Kind
	case r.ID == "":
		return r.Kind
	}
	return r.Kind + ":" + r.ID
}

func (r Rule) same(x Rule) bool { return r.Library == x.Library && r.key() == x.key() }

// RuleRef names a rule that holds a track; Local marks a local playlist,
// Global a rule for all server libraries.
type RuleRef struct {
	Key    string `json:"key"`
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Local  bool   `json:"local,omitempty"`
	Global bool   `json:"global,omitempty"`
}

func refOf(key string) RuleRef {
	if id, ok := strings.CutPrefix(key, localPrefix); ok {
		return RuleRef{Key: key, Kind: KindPlaylist, ID: id, Local: true}
	}
	if kind, ok := strings.CutPrefix(key, allPrefix); ok {
		return RuleRef{Key: key, Kind: kind, Global: true}
	}
	kind, id, _ := strings.Cut(key, ":")
	return RuleRef{Key: key, Kind: kind, ID: id}
}

func validRule(r Rule) (Rule, error) {
	r.ID = source.Clean(r.ID)
	r.Library = source.Clean(r.Library)
	if !slices.Contains(kinds, r.Kind) {
		return r, fmt.Errorf("kind must be one of %s", strings.Join(kinds, ", "))
	}
	needsID := r.Kind == KindPlaylist || r.Kind == KindAlbum || r.Kind == KindArtist
	if needsID && r.Library == "" {
		return r, fmt.Errorf("a %s rule belongs to one library", r.Kind)
	}
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

// parseRules reads a list of saved rules, all of library lib unless it is
// "". Repeated rules are dropped; entries this version can't read come back
// in unknown, scoped to lib, to be saved again as they are.
func parseRules(raw any, lib string) (rules []Rule, unknown []any) {
	var list []any
	data, _ := json.Marshal(raw)
	_ = json.Unmarshal(data, &list)
	rules = []Rule{}
	for _, v := range list {
		m, isMap := v.(map[string]any)
		if isMap && lib != "" {
			m = maps.Clone(m)
			m["library"] = lib
			v = m
		}
		var r Rule
		data, _ := json.Marshal(v)
		if !isMap || json.Unmarshal(data, &r) != nil {
			unknown = append(unknown, v)
			continue
		}
		r, err := validRule(r)
		if err != nil {
			unknown = append(unknown, v)
			continue
		}
		for k, x := range m {
			if !slices.Contains([]string{"kind", "id", "library", "limit"}, k) {
				if r.extra == nil {
					r.extra = map[string]any{}
				}
				r.extra[k] = x
			}
		}
		if !slices.ContainsFunc(rules, r.same) {
			rules = append(rules, r)
		}
	}
	return rules, unknown
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

// migrate moves the rules out of the library configs (older ones as a mode
// plus playlists) into cache.json. cache.json is written first, so a crash
// in between only repeats the move, which skips the rules it already has.
func (c *Cache) migrate(ctx context.Context) error {
	infos, err := c.cfg.Libraries.List(ctx)
	if err != nil {
		return err
	}
	var moved []Rule
	var movedUnknown []any
	rest := map[string]map[string]any{}
	for _, info := range infos {
		m, _ := info.Config[configKey].(map[string]any)
		if mm, ok := migrateConfig(m, c.limit); ok {
			m = mm
		}
		if _, ok := m["rules"]; !ok {
			continue
		}
		rules, unknown := parseRules(m["rules"], info.ID)
		moved, movedUnknown = append(moved, rules...), append(movedUnknown, unknown...)
		rest[info.ID] = map[string]any{}
		for k, v := range m {
			if k != "rules" && k != "mode" && k != "playlists" {
				rest[info.ID][k] = v
			}
		}
	}
	if len(rest) == 0 {
		return nil
	}
	err = c.editAll(func(list []Rule, unknown []any) ([]Rule, []any, error) {
		for _, r := range moved {
			if !slices.ContainsFunc(list, r.same) {
				list = append(list, r)
			}
		}
		for _, v := range movedUnknown {
			if !slices.ContainsFunc(unknown, func(x any) bool { return reflect.DeepEqual(x, v) }) {
				unknown = append(unknown, v)
			}
		}
		return list, unknown, nil
	})
	if err != nil {
		return err
	}
	for id, m := range rest {
		if err := c.cfg.Libraries.SetConfig(ctx, id, configKey, m); err != nil {
			return err
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
func scope(ctx context.Context, src source.Source, r Rule) ([]source.Track, string, error) {
	if r.Kind != KindPlaylist {
		tracks, err := scopeTracks(ctx, src, r)
		return tracks, "", err
	}
	pls, ok := src.(source.Playlists)
	if !ok {
		return nil, "", errors.New("the server has no playlists")
	}
	p, err := pls.Playlist(ctx, r.ID)
	return p.Tracks, source.Clean(p.Name), err
}

func scopeTracks(ctx context.Context, src source.Source, r Rule) ([]source.Track, error) {
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
