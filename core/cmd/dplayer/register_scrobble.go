package main

import (
	"time"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/scrobble"
)

func init() {
	var svc *scrobble.Service
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) { scrobble.Register(r, func() *scrobble.Service { return svc }) },
		start: func(d *daemon) error {
			svc = scrobble.New(d.Ctx, scrobble.Config{ConfigDir: d.ConfigDir, StateDir: d.StateDir, Version: d.Version,
				Libraries: d.Libraries, Secrets: d.Secrets, Publish: d.Bus.Publish})
			d.onNetwork = append(d.onNetwork, svc.Flush)
			d.Bus.On(d.Ctx, func(_ string, data any) {
				switch v := data.(type) {
				case player.Status:
					svc.PlayerStatus(v)
				case player.Position:
					svc.Position(v.Position)
				case player.Played:
					svc.Played(v.Track, time.Unix(v.PlayedAt, 0))
				}
			}, player.TopicStatus, player.TopicPosition, player.TopicPlayed)
			return nil
		},
	})
}
