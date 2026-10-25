package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	duxlyrics "github.com/duxlabs-dev/dux-go/lyrics"

	"github.com/Conava/dux-player/core/internal/source"
)

type lyricsReply struct {
	Metadata struct {
		IsSynced bool `json:"IsSynced"`
	} `json:"Metadata"`
	Lyrics []struct {
		Text  string `json:"Text"`
		Start *int64 `json:"Start"`
	} `json:"Lyrics"`
}

// Lyrics reads /Audio/{id}/Lyrics (Jellyfin 10.9+); 404 means none.
func (s *Source) Lyrics(ctx context.Context, id string) (*duxlyrics.Lyrics, error) {
	var out lyricsReply
	err := s.client.get(ctx, "/Audio/"+url.PathEscape(id)+"/Lyrics", nil, &out)
	var he *httpError
	if errors.As(err, &he) && he.code == http.StatusNotFound {
		return &duxlyrics.Lyrics{}, nil
	}
	if err != nil {
		return nil, err
	}
	return out.lyrics(), nil
}

func (r lyricsReply) lyrics() *duxlyrics.Lyrics {
	l := &duxlyrics.Lyrics{}
	var plain []string
	for _, line := range r.Lyrics {
		text := source.Clean(line.Text)
		plain = append(plain, text)
		if r.Metadata.IsSynced && line.Start != nil {
			l.Synced = append(l.Synced, duxlyrics.Line{Start: duxlyrics.Seconds(duration(*line.Start)), Text: text})
		}
	}
	l.Plain = strings.Join(plain, "\n")
	return l
}
