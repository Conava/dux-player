package source

import (
	"context"
	"html"
	"regexp"
	"strings"
	"unicode"
)

// ArtistRef is one artist of a track or album. ID is empty when the library
// has no artist of that name.
type ArtistRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SingleArtist is the artist list of an item whose source gives only a
// display name: that name as one artist, never split ("A & B" stays one
// unless tagged as two).
func SingleArtist(name, id string) []ArtistRef {
	if name = Clean(name); name == "" {
		return nil
	}
	return []ArtistRef{{ID: id, Name: name}}
}

// HasArtist says whether id is one of the artists, or the main artist id.
func HasArtist(refs []ArtistRef, mainID, id string) bool {
	if id == "" {
		return false
	}
	if mainID == id {
		return true
	}
	for _, r := range refs {
		if r.ID == id {
			return true
		}
	}
	return false
}

// ArtistInfo is what browse.artist shows about one artist.
type ArtistInfo struct {
	Artist
	TrackCount      int    `json:"trackCount"`
	Biography       string `json:"biography,omitempty"`
	BiographySource *Link  `json:"biographySource,omitempty"`
	MusicBrainzID   string `json:"musicBrainzId,omitempty"`
	// Similar are artists of the same library.
	Similar []Artist `json:"similar"`
}

// Link names where a text comes from, for attribution.
type Link struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// ArtistDetailer answers browse.artist.
type ArtistDetailer interface {
	ArtistInfo(ctx context.Context, id string) (ArtistInfo, error)
}

// ArtistImager finds an artist's image among the library's own files.
type ArtistImager interface {
	ArtistImage(ctx context.Context, id string) (string, error)
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// PlainText turns a server's HTML biography into plain text.
func PlainText(s string) string {
	s = html.UnescapeString(htmlTag.ReplaceAllString(s, ""))
	lines := strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, l))
	}
	s = strings.TrimSpace(strings.Join(lines, "\n"))
	if len(s) > maxBiography {
		s = strings.ToValidUTF8(s[:maxBiography], "") + "…"
	}
	return s
}

const maxBiography = 8192
