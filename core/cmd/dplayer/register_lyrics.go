package main

import (
	"context"
	"io"

	"github.com/Conava/dux-player/core/internal/cache"
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/lyrics"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

func init() {
	var svc *lyrics.Service
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) { lyrics.Register(r, func() *lyrics.Service { return svc }) },
		start: func(d *daemon) error {
			svc = lyrics.New(d.Ctx, lyrics.Config{CacheDir: d.CacheDir, ConfigDir: d.ConfigDir, DataDir: d.DataDir, Version: d.Version,
				Libraries: d.Libraries, Publish: d.Bus.Publish, HasSubscriber: d.Bus.HasSubscriber,
				Enabled:     func() bool { return d.Settings.Get().Lyrics },
				AudioTiming: func() bool { return d.Settings.Get().LyricsAudioTiming },
				Keep:        func() bool { return d.Settings.Get().LyricsForDownloads },
				HasCopy:     d.Libraries.HasCopy,
				AudioFile:   d.Libraries.AudioFile,
				Decode: func(ctx context.Context, path string, read func(io.Reader) error) error {
					return d.Meter.Decode(ctx, path, lyrics.FitRate, read)
				}})
			var queue *player.QueueSnapshot
			d.Bus.On(d.Ctx, func(topic string, data any) {
				index := 0
				switch topic {
				case player.TopicStatus:
					st := data.(player.Status)
					svc.TrackChanged(st.Track)
					index = st.Index
					// The queue restored at start was published before this listener.
					if queue == nil && st.State != player.StateStopped {
						if q, err := d.Player.Queue(d.Ctx); err == nil {
							queue = &q
						}
					}
				case player.TopicQueue:
					q := data.(player.QueueSnapshot)
					queue, index = &q, q.Index
				}
				if queue == nil {
					return
				}
				var next *source.Track
				if up := queue.Upcoming(index, 1); len(up) > 0 {
					next = &up[0]
				}
				svc.NextChanged(next)
			}, player.TopicStatus, player.TopicQueue)
			// On its own listener, so a download never waits for lyrics.
			d.Bus.On(d.Ctx, func(_ string, data any) {
				svc.Stored(d.Ctx, data.(source.Track), d.Settings.Get().LyricsForDownloads)
			}, cache.TopicStored)
			d.Bus.On(d.Ctx, func(string, any) { svc.SettingsChanged() }, ipc.TopicSettings)
			return nil
		},
		stop: func() { svc.Close() },
	})
}
