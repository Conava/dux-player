package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.senan.xyz/taglib"

	"github.com/Conava/dux-player/core/internal/source"
)

var _ source.Detailer = (*Source)(nil)

// musicBrainzTags maps Picard's tag names to the keys browse.details uses.
var musicBrainzTags = map[string]string{
	"MUSICBRAINZ_TRACKID":        "recordingId",
	"MUSICBRAINZ_RELEASETRACKID": "trackId",
	"MUSICBRAINZ_ALBUMID":        "albumId",
	"MUSICBRAINZ_ARTISTID":       "artistId",
	"MUSICBRAINZ_ALBUMARTISTID":  "albumArtistId",
	"MUSICBRAINZ_RELEASEGROUPID": "releaseGroupId",
}

// maxTagValue caps one tag value; an embedded lyrics or cue sheet tag can be
// long and the details view only shows it.
const maxTagValue = 4096

func (s *Source) Details(ctx context.Context, kind, id string) (source.Details, error) {
	switch kind {
	case "track":
		t, err := s.store.Track(ctx, s.lib.ID, id)
		if err != nil {
			return source.Details{}, err
		}
		d, err := fileDetails(t.Path)
		if err != nil {
			return source.Details{}, err
		}
		if d.Size == 0 {
			d.Size = t.Size
		}
		if g, ok := s.LoudnessGain(ctx, id); ok {
			if d.ReplayGain == nil {
				d.ReplayGain = &source.GainDetails{}
			}
			d.ReplayGain.Measured = &g
		}
		return d, nil
	case "album":
		a, err := s.store.Album(ctx, s.lib.ID, id)
		if err != nil {
			return source.Details{}, err
		}
		var d source.Details
		var formats []string
		for i, t := range a.Tracks {
			it, err := s.store.Track(ctx, s.lib.ID, t.ID)
			if err != nil {
				continue
			}
			d.Size += it.Size
			d.DiscCount = max(d.DiscCount, t.DiscNo)
			if f := formatOf(it.Path); f != "" && !slices.Contains(formats, f) {
				formats = append(formats, f)
			}
			if t.Genre != "" && !slices.Contains(d.Genres, t.Genre) {
				d.Genres = append(d.Genres, t.Genre)
			}
			if i == 0 {
				d.Path = filepath.Dir(it.Path)
				if tags, err := taglib.ReadTags(it.Path); err == nil {
					d.MusicBrainz = musicBrainz(tags, "albumId", "albumArtistId", "releaseGroupId")
				}
			}
		}
		d.Format = strings.Join(formats, ", ")
		return d, nil
	}
	return source.Details{}, fmt.Errorf("%q is not track or album", kind)
}

func formatOf(path string) string {
	return strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))
}

func fileDetails(path string) (source.Details, error) {
	props, err := taglib.ReadProperties(path)
	if err != nil {
		return source.Details{}, err
	}
	tags, err := taglib.ReadTags(path)
	if err != nil {
		return source.Details{}, err
	}
	d := source.Details{Format: formatOf(path), Codec: props.InnerCodec, Bitrate: int(props.BitRate),
		SampleRate: int(props.SampleRate), BitDepth: int(props.BitDepth), Channels: int(props.Channels),
		Path: path, Comment: first(tags, taglib.Comment), MusicBrainz: musicBrainz(tags), Tags: map[string][]string{}}
	if info, err := os.Stat(path); err == nil {
		d.Size = info.Size()
	}
	d.DiscCount = leadingInt(afterSlash(first(tags, taglib.DiscNumber)))
	for _, g := range tags[taglib.Genre] {
		if g = source.Clean(g); g != "" {
			d.Genres = append(d.Genres, g)
		}
	}
	rg := source.GainDetails{
		TrackGain: parseGain(first(tags, "REPLAYGAIN_TRACK_GAIN")), TrackPeak: parsePeak(first(tags, "REPLAYGAIN_TRACK_PEAK")),
		AlbumGain: parseGain(first(tags, "REPLAYGAIN_ALBUM_GAIN")), AlbumPeak: parsePeak(first(tags, "REPLAYGAIN_ALBUM_PEAK")),
	}
	if rg != (source.GainDetails{}) {
		d.ReplayGain = &rg
	}
	for k, vs := range tags {
		for _, v := range vs {
			if v = source.Clean(v); v != "" {
				d.Tags[k] = append(d.Tags[k], v[:min(len(v), maxTagValue)])
			}
		}
	}
	return d, nil
}

// musicBrainz picks the MusicBrainz ids; only the given keys when any are.
func musicBrainz(tags map[string][]string, only ...string) map[string]string {
	out := map[string]string{}
	for tag, key := range musicBrainzTags {
		if v := first(tags, tag); v != "" && (len(only) == 0 || slices.Contains(only, key)) {
			out[key] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// afterSlash is "12" of a "3/12" disc or track number.
func afterSlash(s string) string {
	_, total, ok := strings.Cut(s, "/")
	if !ok {
		return ""
	}
	return total
}

func parsePeak(s string) *float64 {
	p, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || p < 0 || p > 10 {
		return nil
	}
	return &p
}
