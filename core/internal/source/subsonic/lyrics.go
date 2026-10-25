package subsonic

import (
	"context"
	"errors"
	"net/url"
	"strings"

	duxlyrics "github.com/duxlabs-dev/dux-go/lyrics"

	"github.com/Conava/dux-player/core/internal/source"
)

type structuredLyrics struct {
	Synced boolean `json:"synced"`
	Offset num     `json:"offset"`
	Line   list[struct {
		Start num    `json:"start"`
		Value string `json:"value"`
	}] `json:"line"`
}

// Lyrics uses OpenSubsonic's getLyricsBySongId (the songLyrics extension).
// A server without it answers once and is not asked again.
func (s *Source) Lyrics(ctx context.Context, id string) (*duxlyrics.Lyrics, error) {
	var out struct {
		LyricsList struct {
			StructuredLyrics list[structuredLyrics] `json:"structuredLyrics"`
		} `json:"lyricsList"`
	}
	_, err := s.client.call(ctx, "getLyricsBySongId", url.Values{"id": {id}}, &out)
	if errors.Is(err, ErrUnsupported) {
		return &duxlyrics.Lyrics{}, nil
	}
	if err != nil {
		return nil, err
	}
	return pickLyrics(out.LyricsList.StructuredLyrics), nil
}

// pickLyrics prefers a synced set. A positive offset (ms) shows lines sooner.
func pickLyrics(sets []structuredLyrics) *duxlyrics.Lyrics {
	l := &duxlyrics.Lyrics{}
	if len(sets) == 0 {
		return l
	}
	best := sets[0]
	for _, set := range sets {
		if set.Synced {
			best = set
			break
		}
	}
	var plain []string
	for _, line := range best.Line {
		text := source.Clean(line.Value)
		plain = append(plain, text)
		if best.Synced {
			start := max(float64(line.Start)-float64(best.Offset), 0) / 1000
			l.Synced = append(l.Synced, duxlyrics.Line{Start: duxlyrics.Seconds(start), Text: text})
		}
	}
	l.Plain = strings.Join(plain, "\n")
	// Servers that don't parse LRC tags send the timestamps as plain text.
	if !bool(best.Synced) && strings.HasPrefix(strings.TrimSpace(l.Plain), "[") {
		if parsed, err := duxlyrics.ParseLRC([]byte(l.Plain)); err == nil && len(parsed.Synced) > 0 {
			return parsed
		}
	}
	return l
}
