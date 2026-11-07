package shuffle

import (
	"context"
	"slices"
	"strings"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/source"
)

// SkipRules keep tracks out of what the daemon picks by itself. They never
// touch a list the user chose.
type SkipRules struct {
	// MinRating skips rated tracks below it; an unrated track is never below.
	MinRating int      `json:"minRating"`
	Genres    []string `json:"genres"`
	Libraries []string `json:"libraries"`
	// ShorterThan is in seconds.
	ShorterThan int `json:"shorterThan"`
}

func (r SkipRules) Skips(t source.Track) bool {
	if t.Rating > 0 && t.Rating < r.MinRating {
		return true
	}
	if t.Duration > 0 && t.Duration < float64(r.ShorterThan) {
		return true
	}
	if slices.Contains(r.Libraries, t.Library) {
		return true
	}
	return slices.ContainsFunc(r.Genres, func(g string) bool { return strings.EqualFold(g, t.Genre) })
}

// Picks is how a shuffle over whole libraries chooses.
type Picks struct {
	// Albums plays whole albums, in random order, instead of single tracks.
	Albums bool
	Skip   SkipRules
}

// Feed hands out tracks until an empty result; Shuffler is one.
type Feed interface {
	Next(ctx context.Context, n int) ([]source.Track, error)
}

// Apply wraps a feed in these picks. album lists a track's album in order.
func (p Picks) Apply(feed Feed, album func(context.Context, source.Track) ([]source.Track, error)) Feed {
	if p.Albums {
		feed = &albumFeed{feed: feed, album: album, seen: map[string]bool{}}
	}
	return &skipFeed{feed: feed, rules: p.Skip}
}

// albumFeed turns each new track into its whole album, so a long album is
// as much more likely as it has more tracks.
type albumFeed struct {
	feed  Feed
	album func(context.Context, source.Track) ([]source.Track, error)
	seen  map[string]bool
}

func (f *albumFeed) Next(ctx context.Context, n int) ([]source.Track, error) {
	var out []source.Track
	for len(out) < n {
		got, err := f.feed.Next(ctx, 1)
		if err != nil {
			if len(out) > 0 {
				return out, nil
			}
			return nil, err
		}
		if len(got) == 0 {
			break
		}
		t := got[0]
		if t.AlbumID == "" || t.Album == source.UnknownAlbum {
			out = append(out, t)
			continue
		}
		key := t.Library + "\x00" + t.AlbumID
		if f.seen[key] {
			continue
		}
		f.seen[key] = true
		tracks, err := f.album(ctx, t)
		if err != nil || len(tracks) == 0 {
			log.Warnf("shuffle: album of %s: %v", t.Title, err)
			tracks = got
		}
		out = append(out, tracks...)
	}
	return out, nil
}

type skipFeed struct {
	feed  Feed
	rules SkipRules
}

// Next ends only when the feed does, however much the rules drop.
func (f *skipFeed) Next(ctx context.Context, n int) ([]source.Track, error) {
	var out []source.Track
	for len(out) < n {
		got, err := f.feed.Next(ctx, n-len(out))
		if err != nil {
			if len(out) > 0 {
				return out, nil
			}
			return nil, err
		}
		if len(got) == 0 {
			break
		}
		out = append(out, slices.DeleteFunc(got, f.rules.Skips)...)
	}
	return out, nil
}
