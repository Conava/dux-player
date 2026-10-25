package library

import (
	"context"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
	"github.com/godbus/dbus/v5"
)

// A network change usually comes as a burst of signals; act once it settles.
const networkSettle = 3 * time.Second

// WatchNetwork calls onChange after the network changes, as NetworkManager
// or systemd-networkd report it on the system bus. Without a system bus (or
// either service) it does nothing: libraries are then retried on use, and
// queued scrobbles after the next play.
func WatchNetwork(ctx context.Context, onChange func()) {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		log.Infof("no system bus, so network changes go unnoticed: %v", err)
		return
	}
	defer conn.Close()
	for _, opts := range [][]dbus.MatchOption{
		{dbus.WithMatchObjectPath("/org/freedesktop/NetworkManager"), dbus.WithMatchInterface("org.freedesktop.NetworkManager"),
			dbus.WithMatchMember("StateChanged")},
		{dbus.WithMatchObjectPath("/org/freedesktop/network1"), dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
			dbus.WithMatchMember("PropertiesChanged")},
	} {
		if err := conn.AddMatchSignalContext(ctx, opts...); err != nil {
			log.Warnf("watch network: %v", err)
		}
	}
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	watch(ctx, signals, onChange)
}

// watch ends when the bus connection drops: godbus then closes signals.
func watch(ctx context.Context, signals <-chan *dbus.Signal, onChange func()) {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-signals:
			if !ok {
				return
			}
			timer.Reset(networkSettle)
		case <-timer.C:
			onChange()
		}
	}
}
