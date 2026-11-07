package main

import (
	"context"
	"math"
	"path/filepath"
	"sync"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/cache"
	"github.com/Conava/dux-player/core/internal/history"
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/playlists"
	"github.com/Conava/dux-player/core/internal/settings"
	"github.com/Conava/dux-player/core/internal/source"
)

// Caching of server tracks: copies in $XDG_CACHE_HOME/dplayer/tracks play
// before the stream and while the server is unreachable.
func init() {
	var c *cache.Cache
	favorites := burst{run: func(ctx context.Context, lib string) { c.Favorited(ctx, lib) }}
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
				},
				Prefetched: func(ref source.Ref) {
					if err := d.Player.StreamReady(d.Ctx, ref); err != nil {
						log.Debugf("prefetch: %v", err)
					}
				},
				WatchMetered: library.WatchMetered})
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
			prefs := d.Settings.Get()
			c.SetPrefetch(d.Ctx, prefs.PrefetchTracks, prefs.PrefetchOnMetered)
			var last source.Ref
			haveQueue := false
			d.Bus.On(d.Ctx, func(topic string, data any) {
				switch topic {
				case player.TopicStatus:
					st := data.(player.Status)
					// The queue restored at start was published before this
					// listener; it's read once something plays.
					if !haveQueue && st.State != player.StateStopped {
						if q, err := d.Player.Queue(d.Ctx); err == nil {
							haveQueue = true
							c.QueueChanged(d.Ctx, q)
						}
					}
					c.PlayerChanged(d.Ctx, st)
					if st.Track == nil || st.State != player.StatePlaying || st.Track.Ref() == last {
						return
					}
					last = st.Track.Ref()
					go c.Played(d.Ctx, *st.Track)
				case player.TopicQueue:
					haveQueue = true
					c.QueueChanged(d.Ctx, data.(player.QueueSnapshot))
				case ipc.TopicSettings:
					if s, ok := data.(map[string]any)["settings"].(settings.Settings); ok {
						c.SetPrefetch(d.Ctx, s.PrefetchTracks, s.PrefetchOnMetered)
					}
				case ipc.TopicMarks:
					m := data.(map[string]any)
					if _, fav := m["favorite"]; fav {
						favorites.trigger(d.Ctx, m["library"].(string))
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
					ev := data.(map[string]any)
					switch ev["type"] {
					case "changed":
						go c.LibrariesChanged(d.Ctx)
					case "scanDone":
						// A walk takes favorites in without a marks event.
						if lib, _ := ev["library"].(string); d.Libraries.Server(lib) {
							favorites.trigger(d.Ctx, lib)
						}
					}
				}
			}, player.TopicStatus, player.TopicQueue, ipc.TopicSettings, ipc.TopicMarks, library.TopicLibraries,
				playlists.Topic, ipc.TopicPlaylists, history.Topic)
			go c.LibrariesChanged(d.Ctx)
			return nil
		},
	})
}

// burst runs one job per key at a time: triggers during a run add one more
// run, not one each, so a check that finds many changes costs one or two.
type burst struct {
	run func(ctx context.Context, key string)
	mu  sync.Mutex
	// again holds the running keys, true when one more run is due.
	again map[string]bool
}

func (b *burst) trigger(ctx context.Context, key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, running := b.again[key]; running {
		b.again[key] = true
		return
	}
	if b.again == nil {
		b.again = map[string]bool{}
	}
	b.again[key] = false
	go func() {
		for {
			b.run(ctx, key)
			b.mu.Lock()
			if !b.again[key] {
				delete(b.again, key)
				b.mu.Unlock()
				return
			}
			b.again[key] = false
			b.mu.Unlock()
		}
	}()
}
