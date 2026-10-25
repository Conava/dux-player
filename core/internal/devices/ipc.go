package devices

import (
	"context"
	"errors"
	"fmt"

	"github.com/duxlabs-dev/dux-go/ipc/params"

	"github.com/Conava/dux-player/core/internal/ipc"
)

// Register adds the devices.* methods; svc is nil outside the daemon.
func Register(r *ipc.Registry, svc func() *Service) {
	r.Topics(Topic)
	r.OnSubscribe(func() {
		if s := svc(); s != nil {
			s.publish(s.State())
		}
	})
	r.Group("devices", func(ctx context.Context, w *ipc.ConnWriter, req ipc.Request) {
		s := svc()
		if s == nil {
			w.RespondError(req.ID, "playback devices are not running")
			return
		}
		var err error
		switch req.Method {
		case "devices.list":
		case "devices.select":
			err = s.Select(ctx, ipc.Strs(req.Params, "outputs"))
		case "devices.setDelay":
			ms, ok, nerr := ipc.Num(req.Params, "ms")
			switch {
			case nerr != nil:
				err = nerr
			case !ok:
				err = errors.New("ms must be a number")
			default:
				err = s.SetDelay(ctx, ipc.Str(req.Params, "id"), int(ms))
			}
		case "devices.setOnDisconnect":
			err = s.SetOnDisconnect(ctx, ipc.Str(req.Params, "id"), ipc.Str(req.Params, "mode"))
		case "devices.setPauseOnDisconnect":
			err = s.SetPauseOnDisconnect(ctx, params.BoolLoose(req.Params, "enabled"))
		case "devices.setResetOnStart":
			err = s.SetResetOnStart(ctx, params.BoolLoose(req.Params, "enabled"))
		case "devices.testTone":
			err = s.TestTone(ctx)
		}
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		w.Respond(req.ID, s.State())
	},
		ipc.MethodSpec{Name: "devices.list", Desc: "Playback devices, the chosen one(s) and per-output delays", Read: true},
		ipc.MethodSpec{Name: "devices.select", Desc: "Play on one device, or on several local outputs at once", Params: []ipc.ParamSpec{ipc.Req("outputs", "device ids; give several for several outputs")}},
		ipc.MethodSpec{Name: "devices.setDelay", Desc: "Delay one output while playing on several", Params: []ipc.ParamSpec{ipc.Req("id", "output id"), ipc.Req("ms", fmt.Sprintf("0-%d", MaxDelayMs))}},
		ipc.MethodSpec{Name: "devices.setOnDisconnect", Desc: "What one of several outputs going away does", Params: []ipc.ParamSpec{ipc.Req("id", "output id"), ipc.Req("mode", "pause (default) | continue: the other outputs keep playing")}},
		ipc.MethodSpec{Name: "devices.setPauseOnDisconnect", Desc: "Pause when the one output played on goes away; off, the default output takes over", Params: []ipc.ParamSpec{ipc.Req("enabled", "true|false")}},
		ipc.MethodSpec{Name: "devices.setResetOnStart", Desc: "Return to the default output when the player starts", Params: []ipc.ParamSpec{ipc.Req("enabled", "true|false")}},
		ipc.MethodSpec{Name: "devices.testTone", Desc: "Play clicks on all outputs to match their delays"},
	)
}
