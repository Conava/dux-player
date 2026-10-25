package main

import (
	"path/filepath"
	"sync/atomic"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/playlists"
)

// localPlaylists is set as the daemon starts; the cache, started earlier,
// reads kept playlists through it.
var localPlaylists atomic.Pointer[playlists.Service]

func init() {
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) {
			playlists.Register(r, func() *playlists.Service { return localPlaylists.Load() })
		},
		start: func(d *daemon) error {
			store, err := playlists.Open(d.Ctx, filepath.Join(d.DataDir, "playlists.db"))
			if err != nil {
				return err
			}
			go func() {
				<-d.Ctx.Done()
				store.Close()
			}()
			localPlaylists.Store(&playlists.Service{Store: store, Libraries: d.Libraries, Player: d.Player, Publish: d.Bus.Publish})
			return nil
		},
	})
}
