package cast

import (
	"context"

	"github.com/Conava/dux-player/core/internal/ipc"
)

// Register adds the cast methods. b is read per request. Casting starts
// and stops through the playback device list (devices.select cast:<id>).
func Register(r *ipc.Registry, b func() *Bridge) {
	r.Topics(Topic)
	r.OnSubscribe(func() { b().Subscribed() })
	r.Group("cast", func(_ context.Context, w *ipc.ConnWriter, req ipc.Request) {
		switch req.Method {
		case "cast.status":
			w.Respond(req.ID, b().Status())
		case "cast.devices":
			w.Respond(req.ID, b().Devices())
		}
	},
		ipc.MethodSpec{Name: "cast.status", Desc: "Whether casting is possible and the cast session", Read: true},
		ipc.MethodSpec{Name: "cast.devices", Desc: "Cast devices last seen (discovery runs while castDevices is subscribed)", Read: true},
	)
}
