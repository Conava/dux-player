package main

import (
	"context"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/source"
)

// Library availability: retried on network changes and on use, status
// changes published on the libraries topic.
func init() {
	addFeature(feature{
		register: func(*ipc.Registry, *daemon) {},
		start: func(d *daemon) error {
			d.Libraries.SetPublish(d.Bus.Publish)
			// A server that is back takes the marks set while it was away.
			d.Bus.On(d.Ctx, func(_ string, data any) {
				if ev, ok := data.(map[string]any); ok && ev["type"] == "changed" {
					ipc.FlushMarks(d.Ctx, d.Deps)
				}
			}, library.TopicLibraries)
			// Before the first check, whose end is what starts a sync.
			d.Libraries.StartCatalogs(d.Ctx, library.CatalogHooks{
				Sync:        func(id string) { ipc.SyncCatalog(d.Deps, id) },
				Flush:       func(ctx context.Context) { ipc.FlushMarks(ctx, d.Deps) },
				Pending:     d.Marks.Has,
				DropPending: d.Marks.DropLibrary,
				TrackMarked: func(ctx context.Context, ref source.Ref, favorite bool) {
					if err := d.Player.Mark(ctx, ref, &favorite, nil); err != nil {
						log.Warnf("queue marks: %v", err)
					}
				},
				WatchMetered: library.WatchMetered,
			})
			go d.Libraries.Recheck(d.Ctx)
			go ipc.RescanStale(d.Deps)
			d.onNetwork = append(d.onNetwork, func() { d.Libraries.RecheckNetwork(d.Ctx) })
			return nil
		},
	})
}
