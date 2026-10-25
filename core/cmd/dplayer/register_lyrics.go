package main

import (
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/lyrics"
	"github.com/Conava/dux-player/core/internal/player"
)

func init() {
	var svc *lyrics.Service
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) { lyrics.Register(r, func() *lyrics.Service { return svc }) },
		start: func(d *daemon) error {
			svc = lyrics.New(d.Ctx, lyrics.Config{CacheDir: d.CacheDir, ConfigDir: d.ConfigDir, Version: d.Version,
				Libraries: d.Libraries, Publish: d.Bus.Publish, HasSubscriber: d.Bus.HasSubscriber})
			d.Bus.On(d.Ctx, func(_ string, data any) { svc.TrackChanged(data.(player.Status).Track) }, player.TopicStatus)
			return nil
		},
	})
}
