package subsonic

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Conava/dux-player/core/internal/source"
)

var _ source.Detailer = (*Source)(nil)

func (s *Source) Details(ctx context.Context, kind, id string) (source.Details, error) {
	d := source.Details{Server: s.client.base.String()}
	switch kind {
	case "track":
		x, err := s.client.Song(ctx, id)
		if err != nil {
			return d, err
		}
		d.Format = strings.ToUpper(x.Suffix)
		d.Codec = x.ContentType
		d.Bitrate, d.SampleRate, d.BitDepth, d.Channels = x.BitRate.int(), x.SamplingRate.int(), x.BitDepth.int(), x.ChannelCount.int()
		d.Size, d.Path, d.Comment = int64(x.Size), source.Clean(x.Path), source.Clean(x.Comment)
		d.Genres = genres(x.Genres, x.Genre)
		if rg := x.ReplayGain.values(); rg != nil {
			d.ReplayGain = &source.GainDetails{TrackGain: rg.Track, AlbumGain: rg.Album}
			if rg.Track != nil {
				d.ReplayGain.TrackPeak = x.ReplayGain.TrackPeak
			}
			if rg.Album != nil {
				d.ReplayGain.AlbumPeak = x.ReplayGain.AlbumPeak
			}
		}
		if mb := source.Clean(x.MusicBrainzID); mb != "" {
			d.MusicBrainz = map[string]string{"recordingId": mb}
		}
		return d, nil
	case "album":
		a, err := s.client.Album(ctx, id)
		if err != nil {
			return d, err
		}
		d.Genres = genres(a.Genres, a.Genre)
		var formats []string
		for _, x := range a.Song {
			d.Size += int64(x.Size)
			d.DiscCount = max(d.DiscCount, x.DiscNumber.int())
			if f := strings.ToUpper(x.Suffix); f != "" && !slices.Contains(formats, f) {
				formats = append(formats, f)
			}
		}
		d.Format = strings.Join(formats, ", ")
		if mb := source.Clean(a.MusicBrainzID); mb != "" {
			d.MusicBrainz = map[string]string{"albumId": mb}
		}
		return d, nil
	}
	return d, fmt.Errorf("%q is not track or album", kind)
}

// genres is OpenSubsonic's list, else the plain genre.
func genres(list []namedEntry, plain string) []string {
	var out []string
	for _, g := range list {
		if n := source.Clean(g.Name); n != "" {
			out = append(out, n)
		}
	}
	if len(out) == 0 && source.Clean(plain) != "" {
		out = []string{source.Clean(plain)}
	}
	return out
}
