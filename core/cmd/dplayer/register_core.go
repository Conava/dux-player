package main

import "github.com/Conava/dux-player/core/internal/ipc"

func init() {
	addFeature(feature{register: func(r *ipc.Registry, d *daemon) {
		ipc.RegisterCore(r, &d.Deps)
		ipc.RegisterPlayer(r, &d.Deps)
		ipc.RegisterLibraries(r, &d.Deps)
		ipc.RegisterSettings(r, &d.Deps)
		ipc.RegisterUI(r, &d.Deps)
		ipc.RegisterMarks(r, &d.Deps)
	}})
}
