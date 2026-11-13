package main

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/window"
)

const notifyTimeout = 3 * time.Second

// notifyStartFailure shows a desktop notification, since a daemon started
// from a launcher or keybind has no terminal to print its error to.
func notifyStartFailure(cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return
	}
	defer conn.Close()
	conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications").CallWithContext(ctx,
		"org.freedesktop.Notifications.Notify", 0,
		ipc.AppName, uint32(0), window.AppID, "Dux Player couldn't start", cause.Error(),
		[]string{}, map[string]dbus.Variant{
			"urgency":       dbus.MakeVariant(byte(2)),
			"desktop-entry": dbus.MakeVariant(window.AppID),
		}, int32(-1))
}
