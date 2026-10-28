package main

import (
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/library"
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
			go d.Libraries.Recheck(d.Ctx)
			go ipc.RescanStale(d.Deps)
			d.onNetwork = append(d.onNetwork, func() { d.Libraries.Recheck(d.Ctx) })
			return nil
		},
	})
}
