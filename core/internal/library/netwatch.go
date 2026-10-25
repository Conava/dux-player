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

const (
	nmName  = "org.freedesktop.NetworkManager"
	nmPath  = "/org/freedesktop/NetworkManager"
	nmProps = "org.freedesktop.DBus.Properties"
)

// Metered reads NetworkManager's NMMetered value: yes (1) and guess-yes (3)
// are metered; unknown, no and guess-no aren't.
func Metered(v uint32) bool { return v == 1 || v == 3 }

// WatchMetered says whether NetworkManager calls the connection metered and
// calls onChange on every change after, until ctx ends. Without a system bus
// or NetworkManager the connection is never metered.
func WatchMetered(ctx context.Context, onChange func(metered bool)) bool {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return false
	}
	if err := conn.AddMatchSignalContext(ctx, dbus.WithMatchObjectPath(nmPath), dbus.WithMatchInterface(nmProps),
		dbus.WithMatchMember("PropertiesChanged"), dbus.WithMatchArg(0, nmName)); err != nil {
		conn.Close()
		return false
	}
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	gctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var v dbus.Variant
	if err := conn.Object(nmName, nmPath).CallWithContext(gctx, nmProps+".Get", 0, nmName, "Metered").Store(&v); err != nil {
		log.Debugf("no NetworkManager, so the connection counts as not metered: %v", err)
	}
	n, _ := v.Value().(uint32)
	go func() {
		defer conn.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-signals:
				if !ok {
					return
				}
				if len(sig.Body) < 2 {
					continue
				}
				changed, _ := sig.Body[1].(map[string]dbus.Variant)
				if m, ok := changed["Metered"].Value().(uint32); ok {
					onChange(Metered(m))
				}
			}
		}
	}()
	return Metered(n)
}
