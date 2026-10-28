package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"

	"github.com/Conava/dux-player/core/internal/cast"
	"github.com/Conava/dux-player/core/internal/devices"
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/sysvol"
)

// The external programs and services the player uses, checked on request
// for the about page.
func init() {
	addFeature(feature{register: func(r *ipc.Registry, d *daemon) {
		r.Group("system", func(ctx context.Context, w *ipc.ConnWriter, req ipc.Request) {
			w.Respond(req.ID, map[string]any{"components": checkComponents(ctx, d)})
		}, ipc.MethodSpec{Name: "system.components", Desc: "Which external programs and services are installed", Read: true})
	}})
}

const (
	componentOK      = "ok"
	componentMissing = "missing"
	componentTooOld  = "tooOld"
	componentFailed  = "failed"
)

type component struct {
	ID         string `json:"id"`
	Required   bool   `json:"required,omitempty"`
	Status     string `json:"status"`
	Version    string `json:"version,omitempty"`
	MinVersion string `json:"minVersion,omitempty"`
	// Detail says in plain English why a component can't be used.
	Detail string `json:"detail,omitempty"`
}

func found(id string, ok bool) component {
	if ok {
		return component{ID: id, Status: componentOK}
	}
	return component{ID: id, Status: componentMissing}
}

// checkComponents runs every check at once; each bounds its own wait.
func checkComponents(ctx context.Context, d *daemon) []component {
	checks := []func() component{
		func() component {
			c := component{ID: "mpv", Required: true, Status: componentOK, MinVersion: fmt.Sprintf("%d.%d", player.MinMPV[0], player.MinMPV[1])}
			version, err := player.CheckMPV(ctx, "mpv")
			c.Version = version
			switch {
			case err == nil:
			case errors.Is(err, exec.ErrNotFound):
				c.Status = componentMissing
			case version != "":
				c.Status, c.Detail = componentTooOld, err.Error()
			default:
				c.Status, c.Detail = componentFailed, err.Error()
			}
			return c
		},
		func() component { return found("ffmpeg", loudness.Find().Available()) },
		func() component { return found("pipewire", devices.PipeWireAvailable()) },
		func() component { return found("wpctl", sysvol.Find() != nil) },
		func() component {
			err := d.Secrets.Available()
			c := found("keyring", err == nil)
			if err != nil {
				c.Detail = err.Error()
			}
			return c
		},
		func() component { return found("cast-helper", cast.FindHelper() != "") },
	}
	out := make([]component, len(checks))
	var wg sync.WaitGroup
	for i, check := range checks {
		wg.Go(func() { out[i] = check() })
	}
	wg.Wait()
	return out
}
