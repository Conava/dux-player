package main

import (
	"context"
	"strings"

	"github.com/Conava/dux-player/core/internal/cast"
	"github.com/Conava/dux-player/core/internal/devices"
	"github.com/Conava/dux-player/core/internal/ipc"
)

// castBridge is also the devices feature's extra output. This file's init
// runs before register_desktop.go's (files initialize in name order), so
// the cast feature starts first.
var castBridge *cast.Bridge

func init() {
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) { cast.Register(r, func() *cast.Bridge { return castBridge }) },
		start: func(d *daemon) error {
			castBridge = cast.New(cast.Config{Ctx: d.Ctx, Player: d.Player, Libraries: d.Libraries,
				Publish: d.Bus.Publish, HasSubscriber: d.Bus.HasSubscriber})
			return nil
		},
	})
}

// castOutputs puts the cast devices into the playback device list.
type castOutputs struct{ b *cast.Bridge }

const castPrefix = "cast:"

func (c castOutputs) Devices() []devices.Device {
	var out []devices.Device
	for _, d := range c.b.Devices() {
		out = append(out, devices.Device{ID: castPrefix + d.ID, Name: d.Name, Kind: "cast", Available: true})
	}
	return out
}

// Select casts to a device; "" (a local output picked again) stops the
// cast and plays on here from its position.
func (c castOutputs) Select(ctx context.Context, id string) error {
	if id == "" {
		return c.b.StopCast(ctx, true)
	}
	return c.b.Play(ctx, strings.TrimPrefix(id, castPrefix))
}
