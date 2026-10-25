package main

import (
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/keyring"
	"github.com/Conava/dux-player/core/internal/sysvol"
)

// daemon is what a feature gets: the core services, the daemon's context
// (cancelled at shutdown) and its directories.
type daemon struct {
	ipc.Deps
	ConfigDir, DataDir, StateDir string
	Secrets                      *keyring.Store
	// Mixer drives the system volume; nil without wpctl.
	Mixer *sysvol.Mixer
	// onNetwork runs after the network changed; starts add to it.
	onNetwork []func()
}

// feature is one register_<name>.go file's contribution. register also runs
// in the CLI with an empty daemon (`dplayer ipc list`, completion), so it may
// store d but never read it. start runs once the core services are up;
// requests wait until every start has returned.
type feature struct {
	register func(r *ipc.Registry, d *daemon)
	start    func(d *daemon) error
}

var features []feature

func addFeature(f feature) { features = append(features, f) }

func newRegistry(d *daemon) *ipc.Registry {
	r := ipc.NewRegistry()
	for _, f := range features {
		f.register(r, d)
	}
	return r
}
