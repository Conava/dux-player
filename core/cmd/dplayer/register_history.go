package main

import (
	"path/filepath"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/history"
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/player"
)

func init() {
	var store *history.Store
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) {
			history.Register(r, func() *history.Store { return store }, func(t string, v any) { d.Bus.Publish(t, v) })
		},
		start: func(d *daemon) error {
			s, err := history.Open(d.Ctx, filepath.Join(d.DataDir, "history.db"))
			if err != nil {
				log.Warnf("play history unavailable: %v", err)
				return nil
			}
			s.Server = d.Libraries.Server
			s.Sources = d.Libraries.Working
			store = s
			d.Stats = s
			go func() {
				<-d.Ctx.Done()
				s.Close()
			}()
			changed := make(chan struct{}, 1)
			go prune(d, s, changed)
			d.Bus.On(d.Ctx, func(topic string, data any) {
				if topic == ipc.TopicSettings {
					select {
					case changed <- struct{}{}:
					default:
					}
					return
				}
				ev := data.(player.Played)
				if err := s.Add(d.Ctx, ev.Track, time.Unix(ev.PlayedAt, 0)); err != nil {
					log.Warnf("history: %v", err)
					return
				}
				d.Bus.Publish(history.Topic, map[string]any{"type": "played", "track": ev.Track, "playedAt": ev.PlayedAt})
			}, ipc.TopicSettings, player.TopicPlayed)
			return nil
		},
	})
}

// prune keeps the history to the configured months: at start, daily and
// when the setting changes.
func prune(d *daemon, s *history.Store, changed <-chan struct{}) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		months := d.Settings.Get().HistoryMonths
		if err := s.Prune(d.Ctx, months); err != nil {
			log.Warnf("history prune: %v", err)
		}
	wait:
		for {
			select {
			case <-d.Ctx.Done():
				return
			case <-t.C:
				break wait
			case <-changed:
				if d.Settings.Get().HistoryMonths != months {
					break wait
				}
			}
		}
	}
}
