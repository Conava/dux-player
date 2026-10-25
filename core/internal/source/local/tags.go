package local

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"strings"

	"go.senan.xyz/taglib"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/source"
)

var audioExts = map[string]bool{
	".flac": true, ".mp3": true, ".ogg": true, ".oga": true, ".opus": true, ".m4a": true, ".mp4": true,
	".aac": true, ".wav": true, ".aif": true, ".aiff": true, ".wv": true, ".ape": true, ".mpc": true,
}

func IsAudio(path string) bool { return audioExts[strings.ToLower(filepath.Ext(path))] }

func hashID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:8])
}

func first(tags map[string][]string, key string) string {
	if v := tags[key]; len(v) > 0 {
		return source.Clean(v[0])
	}
	return ""
}

// leadingInt reads "3", "3/12" and "2024-05-01" as 3, 3 and 2024.
func leadingInt(s string) int {
	end := strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' })
	if end == 0 {
		return 0
	}
	if end > 0 {
		s = s[:end]
	}
	n, _ := strconv.Atoi(s)
	return n
}

// readTrack reads tags, duration and ReplayGain values. Files without tags
// still index, named after the file.
func readTrack(path string) (library.IndexedTrack, error) {
	tags, err := taglib.ReadTags(path)
	if err != nil {
		return library.IndexedTrack{}, err
	}
	props, err := taglib.ReadProperties(path)
	if err != nil {
		return library.IndexedTrack{}, err
	}
	it := library.IndexedTrack{Path: path}
	t := source.Track{
		Title:       first(tags, taglib.Title),
		Artist:      first(tags, taglib.Artist),
		Album:       first(tags, taglib.Album),
		AlbumArtist: first(tags, taglib.AlbumArtist),
		TrackNo:     leadingInt(first(tags, taglib.TrackNumber)),
		DiscNo:      leadingInt(first(tags, taglib.DiscNumber)),
		Year:        leadingInt(first(tags, taglib.Date)),
		Genre:       first(tags, taglib.Genre),
		Duration:    props.Length.Seconds(),
	}
	it.RGTrack = parseGain(first(tags, "REPLAYGAIN_TRACK_GAIN"))
	it.RGAlbum = parseGain(first(tags, "REPLAYGAIN_ALBUM_GAIN"))
	if t.Title == "" {
		t.Title = source.Clean(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	}
	// A multi-value artist tag shows all its values.
	if v := cleanAll(tags[taglib.Artist]); len(v) > 1 {
		t.Artist = strings.Join(v, ", ")
	}
	if v := cleanAll(tags[taglib.AlbumArtist]); len(v) > 1 {
		t.AlbumArtist = strings.Join(v, ", ")
	}
	if t.Artist == "" {
		t.Artist = strings.Join(cleanAll(tags["ARTISTS"]), ", ")
	}
	if t.Artist == "" {
		t.Artist = "Unknown Artist"
	}
	if t.AlbumArtist == "" {
		t.AlbumArtist = t.Artist
	}
	if t.Album == "" {
		t.Album = source.UnknownAlbum
	}
	t.Artists = artistRefs(tags, "ARTISTS", taglib.Artist, t.Artist)
	it.AlbumArtists = artistRefs(tags, "ALBUMARTISTS", taglib.AlbumArtist, t.AlbumArtist)
	if len(tags[taglib.AlbumArtist]) == 0 && len(tags["ALBUMARTISTS"]) == 0 {
		it.AlbumArtists = t.Artists
	}
	t.ArtistID = it.AlbumArtists[0].ID
	// The album id keeps the display string, so albums keep their ids
	// (history, playlists) whatever the separators are.
	t.AlbumID = hashID(strings.ToLower(t.AlbumArtist), strings.ToLower(t.Album))
	it.Track = t
	return it, nil
}

func artistID(name string) string { return hashID(strings.ToLower(name)) }

// artistRefs reads a list tag (ARTISTS), else the artist tag's values, else
// the display name; a name is never split. Never empty: display is set.
func artistRefs(tags map[string][]string, listKey, key, display string) []source.ArtistRef {
	names := cleanAll(tags[listKey])
	if len(names) == 0 {
		names = cleanAll(tags[key])
	}
	if len(names) == 0 {
		names = []string{display}
	}
	out := make([]source.ArtistRef, len(names))
	for i, n := range names {
		out[i] = source.ArtistRef{ID: artistID(n), Name: n}
	}
	return out
}

func cleanAll(values []string) []string {
	var out []string
	for _, v := range values {
		if v = source.Clean(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// parseGain reads "-6.48 dB"; nil when absent or not a sane gain.
func parseGain(s string) *float64 {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "dB"))
	g, err := strconv.ParseFloat(s, 64)
	if err != nil || g < -60 || g > 60 {
		return nil
	}
	return &g
}
