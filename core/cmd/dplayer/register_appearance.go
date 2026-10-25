package main

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
	"github.com/godbus/dbus/v5"

	"github.com/Conava/dux-player/core/internal/ipc"
)

// The desktop's light/dark preference from the settings portal
// (org.freedesktop.appearance color-scheme: 0 no preference, 1 dark,
// 2 light), so the UI needn't ask D-Bus itself.
const (
	topicColorScheme = "colorScheme"
	portalDest       = "org.freedesktop.portal.Desktop"
	portalPath       = "/org/freedesktop/portal/desktop"
	portalSettings   = "org.freedesktop.portal.Settings"
)

func init() {
	// -1 until the portal answered.
	var scheme atomic.Int32
	scheme.Store(-1)
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) {
			r.Topics(topicColorScheme)
			r.Group("appearance", func(_ context.Context, w *ipc.ConnWriter, req ipc.Request) {
				if s := scheme.Load(); s >= 0 {
					w.Respond(req.ID, map[string]int32{"scheme": s})
					return
				}
				w.RespondError(req.ID, "the desktop portal didn't report a color scheme")
			}, ipc.MethodSpec{Name: "appearance.colorScheme", Desc: "Desktop light/dark preference: 0 none, 1 dark, 2 light", Read: true})
		},
		start: func(d *daemon) error {
			go watchColorScheme(d, &scheme)
			return nil
		},
	})
}

func watchColorScheme(d *daemon, scheme *atomic.Int32) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Infof("color scheme: no session bus: %v", err)
		return
	}
	defer conn.Close()
	set := func(v dbus.Variant) {
		// Read (portal v1) wraps the value in one more variant than ReadOne.
		if inner, ok := v.Value().(dbus.Variant); ok {
			v = inner
		}
		s, ok := v.Value().(uint32)
		if !ok || s > 2 || scheme.Swap(int32(s)) == int32(s) {
			return
		}
		d.Bus.Publish(topicColorScheme, map[string]uint32{"scheme": s})
	}
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	if err := conn.AddMatchSignal(dbus.WithMatchObjectPath(portalPath), dbus.WithMatchInterface(portalSettings),
		dbus.WithMatchMember("SettingChanged"), dbus.WithMatchArg(0, "org.freedesktop.appearance")); err != nil {
		log.Infof("color scheme: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(d.Ctx, 5*time.Second)
	var v dbus.Variant
	obj := conn.Object(portalDest, portalPath)
	err = obj.CallWithContext(ctx, portalSettings+".ReadOne", 0, "org.freedesktop.appearance", "color-scheme").Store(&v)
	if err != nil {
		err = obj.CallWithContext(ctx, portalSettings+".Read", 0, "org.freedesktop.appearance", "color-scheme").Store(&v)
	}
	cancel()
	if err != nil {
		log.Infof("color scheme: portal: %v", err)
	} else {
		set(v)
	}
	for {
		select {
		case <-d.Ctx.Done():
			return
		case sig, ok := <-signals:
			if !ok {
				return
			}
			if len(sig.Body) == 3 && sig.Body[0] == "org.freedesktop.appearance" && sig.Body[1] == "color-scheme" {
				if v, ok := sig.Body[2].(dbus.Variant); ok {
					set(v)
				}
			}
		}
	}
}
