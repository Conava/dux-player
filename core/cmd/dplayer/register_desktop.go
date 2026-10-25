package main

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/duxlabs-dev/dux-go/log"
	"github.com/godbus/dbus/v5"

	"github.com/Conava/dux-player/core/internal/cast"
	"github.com/Conava/dux-player/core/internal/devices"
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/mpris"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/settings"
	"github.com/Conava/dux-player/core/internal/source"
	"github.com/Conava/dux-player/core/internal/winrules"
)

func init() {
	addFeature(feature{register: func(*ipc.Registry, *daemon) {}, start: startMpris})

	var dev *devices.Service
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) { devices.Register(r, func() *devices.Service { return dev }) },
		start: func(d *daemon) error {
			var extra devices.Extra
			if castBridge != nil {
				extra = castOutputs{castBridge}
			}
			dev = devices.New(d.Player, d.ConfigDir, filepath.Join(appPaths.SocketDir(), ipc.AppName), eqOf(d.Settings.Get()), extra,
				func(st devices.State) { d.Bus.Publish(devices.Topic, st) })
			casting := false
			d.Bus.On(d.Ctx, func(topic string, data any) {
				switch topic {
				case player.TopicAudioDevices:
					dev.OnDeviceList(data.(json.RawMessage))
				case ipc.TopicSettings:
					// The equalizer runs in the devices' PipeWire path, not in mpv.
					dev.OnEqualizer(eqOf(d.Settings.Get()))
				case cast.Topic:
					// A cast that ends by itself hands the selection back to the
					// local one; every cast event may carry a new device list.
					dev.ExtrasChanged()
					st := data.(cast.Status)
					if casting && !st.Casting {
						dev.ExtraEnded()
					}
					casting = st.Casting
				}
			}, player.TopicAudioDevices, ipc.TopicSettings, cast.Topic)
			go dev.Run(d.Ctx)
			return nil
		},
	})

	var wr *winrules.Service
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) {
			winrules.Register(r, func() *winrules.Service { return wr }, func(st winrules.Status) { d.Bus.Publish(winrules.Topic, st) })
		},
		start: func(d *daemon) error {
			wr = winrules.New(d.ConfigDir)
			go func() {
				st, err := wr.Seed(d.Ctx)
				if err != nil {
					log.Warnf("open floating: %v", err)
				}
				d.Bus.Publish(winrules.Topic, st)
			}()
			return nil
		},
	})
}

func eqOf(s settings.Settings) devices.EQ {
	return devices.EQ{On: s.Equalizer.Enabled, Gains: s.Equalizer.Bands}
}

// startMpris connects in the background: no session bus only costs media
// keys and the shell's media widget.
func startMpris(d *daemon) error {
	go func() {
		conn, err := dbus.ConnectSessionBus(dbus.WithContext(d.Ctx))
		if err != nil {
			log.Warnf("mpris: no session bus: %v", err)
			return
		}
		go func() {
			<-d.Ctx.Done()
			conn.Close()
		}()
		srv, err := mpris.Start(d.Ctx, conn, mpris.Config{
			Player: d.Player,
			Cover:  func(ctx context.Context, t source.Track) (string, error) { return cover(ctx, d, t) },
			Raise:  func() { raiseWindow(d, "mpris raise") },
			Quit:   d.Quit,
		})
		if err != nil {
			log.Warnf("mpris: %v", err)
			return
		}
		d.Bus.On(d.Ctx, func(_ string, data any) {
			switch v := data.(type) {
			case player.Status:
				srv.Update(v)
			case player.Position:
				srv.UpdatePosition(v.Position)
			}
		}, player.TopicStatus, player.TopicPosition)
		if st, err := d.Player.Status(d.Ctx); err == nil {
			srv.Update(st)
		}
	}()
	return nil
}

func cover(ctx context.Context, d *daemon, t source.Track) (string, error) {
	src, err := d.Libraries.Get(t.Library)
	if err != nil {
		return "", err
	}
	c, ok := src.(source.CoverArt)
	if !ok {
		return "", nil
	}
	return c.Cover(ctx, t.CoverID, 512)
}
