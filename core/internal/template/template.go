// Package template is the text template of quickshell/Common/Template.js for
// the CLI: %field%, %a|b% (first non-empty), [ section ] printed only when a
// field inside has a value, 'quoted' literals, a doubled quote for an apostrophe. A name
// missing from fields stays literal so a typo is visible. Both run
// testdata/cases.json.
package template

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Conava/dux-player/core/internal/source"
)

const maxOutput = 512

type state struct {
	in     []rune
	at     int
	fields map[string]string
}

// Format fills template in. fields holds every known name, "" when it has no
// value.
func Format(template string, fields map[string]string) string {
	s := state{in: []rune(template), fields: fields}
	text, _ := s.render(false)
	out := []rune(text)
	return string(out[:min(len(out), maxOutput)])
}

func (s *state) closing(c rune) int {
	if i := slices.Index(s.in[s.at+1:], c); i >= 0 {
		return s.at + 1 + i
	}
	return -1
}

func fieldName(n string) bool {
	return n != "" && strings.IndexFunc(n, func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') }) < 0
}

func (s *state) render(inSection bool) (string, bool) {
	var text strings.Builder
	filled := false
	for s.at < len(s.in) {
		c := s.in[s.at]
		switch {
		case c == ']' && inSection:
			s.at++
			return text.String(), filled
		case c == '[':
			s.at++
			if inner, ok := s.render(true); ok {
				text.WriteString(inner)
				filled = true
			}
			continue
		case c == '\'':
			if end := s.closing(c); end > 0 {
				if end == s.at+1 {
					text.WriteRune(c)
				} else {
					text.WriteString(string(s.in[s.at+1 : end]))
				}
				s.at = end + 1
				continue
			}
		case c == '%':
			if end := s.closing(c); end > 0 {
				if names := strings.Split(string(s.in[s.at+1:end]), "|"); !slices.ContainsFunc(names, func(n string) bool { return !fieldName(n) }) {
					value, known := s.lookup(names)
					if known {
						text.WriteString(value)
					} else {
						text.WriteString(string(s.in[s.at : end+1]))
					}
					filled = filled || value != "" || !known
					s.at = end + 1
					continue
				}
			}
		}
		text.WriteRune(c)
		s.at++
	}
	return text.String(), filled
}

// lookup returns the first non-empty value of names, and whether any is a field.
func (s *state) lookup(names []string) (string, bool) {
	known := false
	for _, n := range names {
		v, ok := s.fields[n]
		known = known || ok
		if v != "" {
			return v, true
		}
	}
	return "", known
}

func count(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func duration(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	total := int(seconds)
	if h := total / 3600; h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, total%3600/60, total%60)
	}
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

// TrackFields are the fields of a track; the caller adds library (its name)
// and what isn't on the track. Zero means unknown for the numbers except the
// play count, where it is a value. Dates are ISO, since scripts read them.
func TrackFields(t source.Track) map[string]string {
	names := make([]string, len(t.Artists))
	for i, a := range t.Artists {
		names[i] = a.Name
	}
	added := ""
	if t.Added > 0 {
		added = time.Unix(t.Added, 0).Format(time.DateOnly)
	}
	rating := ""
	if t.Rating > 0 {
		rating = strings.Repeat("★", t.Rating) + strings.Repeat("☆", max(5-t.Rating, 0))
	}
	return map[string]string{
		"title": t.Title, "artist": t.Artist, "artists": strings.Join(names, ", "), "album": t.Album,
		"albumartist": t.AlbumArtist, "composer": t.Composer, "genre": t.Genre, "year": count(t.Year),
		"tracknumber": count(t.TrackNo), "discnumber": count(t.DiscNo), "duration": duration(t.Duration),
		"rating": rating, "playcount": strconv.Itoa(t.PlayCount), "library": t.Library, "format": t.Format,
		"bitrate": count(t.Bitrate), "added": added, "playingfrom": "",
	}
}
