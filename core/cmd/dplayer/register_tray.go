package main

import (
	"github.com/duxlabs-dev/dux-go/log"
	"github.com/godbus/dbus/v5"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/tray"
)

func init() {
	addFeature(feature{register: func(*ipc.Registry, *daemon) {}, start: startTray})
}

// startTray connects in the background; with no session bus or no tray
// host the icon just doesn't show.
func startTray(d *daemon) error {
	go func() {
		conn, err := dbus.ConnectSessionBus(dbus.WithContext(d.Ctx))
		if err != nil {
			log.Debugf("tray: no session bus: %v", err)
			return
		}
		go func() {
			<-d.Ctx.Done()
			conn.Close()
		}()
		t, err := tray.Start(d.Ctx, conn, tray.Config{Player: d.Player, Show: func() { raiseWindow(d, "tray") }, Quit: d.Quit})
		if err != nil {
			log.Warnf("tray: %v", err)
			return
		}
		d.Bus.On(d.Ctx, func(_ string, data any) {
			if st, ok := data.(player.Status); ok {
				t.Update(st)
				return
			}
			t.SetEnabled(d.Settings.Get().ShowTrayIcon)
		}, player.TopicStatus, ipc.TopicSettings)
		if st, err := d.Player.Status(d.Ctx); err == nil {
			t.Update(st)
		}
		t.SetEnabled(d.Settings.Get().ShowTrayIcon)
	}()
	return nil
}

// raiseWindow starts the window, or raises the open one.
func raiseWindow(d *daemon, who string) {
	started, err := d.Window.Start("")
	if err != nil {
		log.Warnf("%s: open window: %v", who, err)
	} else if !started {
		d.Bus.Publish(ipc.TopicUI, map[string]any{"action": "show"})
	}
}
