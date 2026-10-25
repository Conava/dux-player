package jellyfin

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Conava/dux-player/core/internal/source"
)

var _ source.Detailer = (*Source)(nil)

type detailsItem struct {
	Path         string            `json:"Path"`
	Genres       []string          `json:"Genres"`
	Overview     string            `json:"Overview"`
	ProviderIDs  map[string]string `json:"ProviderIds"`
	MediaSources []struct {
		Container    string `json:"Container"`
		Size         int64  `json:"Size"`
		Bitrate      int    `json:"Bitrate"`
		MediaStreams []struct {
			Type       string `json:"Type"`
			Codec      string `json:"Codec"`
			BitRate    int    `json:"BitRate"`
			SampleRate int    `json:"SampleRate"`
			BitDepth   int    `json:"BitDepth"`
			Channels   int    `json:"Channels"`
		} `json:"MediaStreams"`
	} `json:"MediaSources"`
	NormalizationGain *float64 `json:"NormalizationGain"`
}

// Jellyfin's provider id names for the keys browse.details uses.
var musicBrainzIDs = map[string]string{
	"MusicBrainzRecording":    "recordingId",
	"MusicBrainzTrack":        "trackId",
	"MusicBrainzAlbum":        "albumId",
	"MusicBrainzArtist":       "artistId",
	"MusicBrainzAlbumArtist":  "albumArtistId",
	"MusicBrainzReleaseGroup": "releaseGroupId",
}

func (s *Source) Details(ctx context.Context, kind, id string) (source.Details, error) {
	if kind != "track" && kind != "album" {
		return source.Details{}, fmt.Errorf("%q is not track or album", kind)
	}
	var it detailsItem
	q := url.Values{"Fields": {"MediaSources,Path,ProviderIds,Genres,Overview"}}
	if err := s.client.get(ctx, "/Users/{user}/Items/"+url.PathEscape(id), q, &it); err != nil {
		return source.Details{}, err
	}
	d := source.Details{Server: s.client.base.String(), Path: source.Clean(it.Path), Comment: source.Clean(it.Overview)}
	for _, g := range it.Genres {
		if g = source.Clean(g); g != "" {
			d.Genres = append(d.Genres, g)
		}
	}
	for k, v := range it.ProviderIDs {
		if key, ok := musicBrainzIDs[k]; ok && source.Clean(v) != "" {
			if d.MusicBrainz == nil {
				d.MusicBrainz = map[string]string{}
			}
			d.MusicBrainz[key] = source.Clean(v)
		}
	}
	if len(it.MediaSources) > 0 {
		ms := it.MediaSources[0]
		d.Format, d.Size, d.Bitrate = strings.ToUpper(source.Clean(ms.Container)), ms.Size, ms.Bitrate/1000
		for _, st := range ms.MediaStreams {
			if st.Type == "Audio" {
				d.Codec, d.SampleRate, d.BitDepth, d.Channels = source.Clean(st.Codec), st.SampleRate, st.BitDepth, st.Channels
				if st.BitRate > 0 {
					d.Bitrate = st.BitRate / 1000
				}
				break
			}
		}
	}
	if g := it.NormalizationGain; g != nil && *g != 0 {
		d.ReplayGain = &source.GainDetails{TrackGain: g}
	}
	return d, nil
}
