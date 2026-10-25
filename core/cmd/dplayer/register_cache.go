package main

import (
	"context"
	"math"
	"path/filepath"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/cache"
	"github.com/Conava/dux-player/core/internal/history"
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/playlists"
	"github.com/Conava/dux-player/core/internal/source"
)

// Caching of server tracks: copies in $XDG_CACHE_HOME/dplayer/tracks play
// before the stream and while the server is unreachable.
func init() {
	var c *cache.Cache
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) { cache.Register(r, func() *cache.Cache { return c }) },
		start: func(d *daemon) error {
			opened, err := cache.Open(d.Ctx, cache.Config{Dir: filepath.Join(d.CacheDir, "tracks"), ConfigDir: d.ConfigDir,
				Libraries: d.Libraries, Meter: d.Libraries.Meter(), Publish: d.Bus.Publish,
				LocalPlaylist: func(ctx context.Context, id string) ([]source.Track, error) {
					svc := localPlaylists.Load()
					if svc == nil {
						return nil, nil
					}
					p, err := svc.Get(ctx, id)
					return p.Tracks, err
				},
				MostPlayed: func(ctx context.Context) ([]source.Track, error) {
					h, ok := d.Stats.(*history.Store)
					if !ok {
						return nil, nil
					}
					top, err := h.Top(ctx, source.ItemTrack, math.MaxInt, false)
					out := make([]source.Track, 0, len(top))
					for _, c := range top {
						out = append(out, *c.Track)
					}
					return out, err
				}})
			if err != nil {
				log.Warnf("track cache unavailable: %v", err)
				return nil
			}
			c = opened
			d.Libraries.SetOffline(c)
			go func() {
				c.Run(d.Ctx)
				c.Close()
			}()
			var last source.Ref
			d.Bus.On(d.Ctx, func(topic string, data any) {
				switch topic {
				case player.TopicStatus:
					st := data.(player.Status)
					if st.Track == nil || st.State != player.StatePlaying || st.Track.Ref() == last {
						return
					}
					last = st.Track.Ref()
					go c.Played(d.Ctx, *st.Track)
				case ipc.TopicMarks:
					m := data.(map[string]any)
					if _, fav := m["favorite"]; fav {
						go c.Favorited(d.Ctx, m["library"].(string))
					}
				case ipc.TopicPlaylists:
					m := data.(map[string]string)
					go c.PlaylistChanged(d.Ctx, m["library"], m["id"])
				case history.Topic:
					if t, ok := data.(map[string]any)["track"].(source.Track); ok {
						go c.HistoryChanged(d.Ctx, t.Library)
					}
				case playlists.Topic:
					if id, _ := data.(map[string]any)["id"].(string); id != "" {
						go c.LocalPlaylistChanged(d.Ctx, id)
					}
				case library.TopicLibraries:
					if data.(map[string]any)["type"] == "changed" {
						go c.LibrariesChanged(d.Ctx)
					}
				}
			}, player.TopicStatus, ipc.TopicMarks, library.TopicLibraries, playlists.Topic,
				ipc.TopicPlaylists, history.Topic)
			go c.LibrariesChanged(d.Ctx)
			return nil
		},
	})
}
