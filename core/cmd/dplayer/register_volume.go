package main

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/player"
)

// The system volume target follows changes made elsewhere (shell, keys)
// only while it is the target and a client listens to the player status.
func init() {
	var w *volumeWatch
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) {
			r.OnSubscribe(func() {
				if w != nil {
					w.update()
				}
			})
		},
		start: func(d *daemon) error {
			if d.Mixer == nil {
				return nil
			}
			w = &volumeWatch{d: d}
			d.Bus.On(d.Ctx, func(string, any) { w.update() }, ipc.TopicSettings)
			return nil
		},
	})
}

type volumeWatch struct {
	d      *daemon
	mu     sync.Mutex
	cancel context.CancelFunc
	failed atomic.Bool
}

func (w *volumeWatch) update() {
	want := w.d.Settings.Get().VolumeTarget == player.VolumeSystem && w.d.Bus.HasSubscriber(player.TopicStatus)
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case want && w.cancel == nil:
		ctx, cancel := context.WithCancel(w.d.Ctx)
		w.cancel = cancel
		go func() {
			w.refresh(ctx)
			w.d.Mixer.Watch(ctx, func() { w.refresh(ctx) })
		}()
	case !want && w.cancel != nil:
		w.cancel()
		w.cancel = nil
	}
}

func (w *volumeWatch) refresh(ctx context.Context) {
	v, muted, err := w.d.Mixer.Get(ctx)
	if err != nil {
		// logged on the first failure of a run only: the mixer notifies on every change
		if !w.failed.Swap(true) {
			log.Warnf("system volume: can't read it: %v", err)
		}
		return
	}
	w.failed.Store(false)
	if err := w.d.Player.SystemChanged(ctx, v, muted); err != nil {
		log.Warnf("system volume: player update: %v", err)
	}
}
