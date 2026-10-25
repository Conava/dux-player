package main

import (
	"github.com/Conava/dux-player/core/internal/ipc"
)

// Library availability: retried on network changes and on use, status
// changes published on the libraries topic.
func init() {
	addFeature(feature{
		register: func(*ipc.Registry, *daemon) {},
		start: func(d *daemon) error {
			d.Libraries.SetPublish(d.Bus.Publish)
			go d.Libraries.Recheck(d.Ctx)
			go ipc.RescanStale(d.Deps)
			d.onNetwork = append(d.onNetwork, func() { d.Libraries.Recheck(d.Ctx) })
			return nil
		},
	})
}
