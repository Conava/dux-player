package main

import (
	"os"
	"strings"

	"github.com/Conava/dux-player/core/internal/artists"
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/playlists"
)

func init() {
	var svc *artists.Service
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) { artists.Register(r, func() *artists.Service { return svc }) },
		start: func(d *daemon) error {
			svc = artists.New(artists.Config{
				Get: d.Libraries.Get,
				Local: func() *playlists.Store {
					if p := localPlaylists.Load(); p != nil {
						return p.Store
					}
					return nil
				},
				Online:   func() bool { return d.Settings.Get().ArtistInfoOnline },
				Secrets:  d.Secrets,
				CacheDir: d.CacheDir,
				Version:  d.Version,
				Lang:     systemLanguage(),
				Publish:  d.Bus.Publish,
			})
			d.Bus.On(d.Ctx, func(_ string, data any) {
				if m, ok := data.(map[string]string); ok {
					svc.Invalidate(m["library"], m["id"])
				}
			}, ipc.TopicPlaylists)
			return nil
		},
	})
}

// systemLanguage is the two-letter language of the locale ("de_DE.UTF-8").
func systemLanguage() string {
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if l := os.Getenv(v); l != "" && l != "C" && l != "POSIX" {
			l, _, _ = strings.Cut(l, "_")
			l, _, _ = strings.Cut(l, ".")
			return strings.ToLower(l)
		}
	}
	return ""
}
