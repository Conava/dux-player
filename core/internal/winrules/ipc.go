package winrules

import (
	"context"

	"github.com/duxlabs-dev/dux-go/ipc/params"

	"github.com/Conava/dux-player/core/internal/ipc"
)

// Register adds winrules.*; svc is nil outside the daemon.
func Register(r *ipc.Registry, svc func() *Service, publish func(Status)) {
	r.Topics(Topic)
	r.Group("winrules", func(ctx context.Context, w *ipc.ConnWriter, req ipc.Request) {
		s := svc()
		if s == nil {
			w.RespondError(req.ID, "window rules are not running")
			return
		}
		var st Status
		var err error
		switch req.Method {
		case "winrules.status":
			st, err = s.Refresh(ctx)
		case "winrules.set":
			st, err = s.Set(ctx, params.BoolLoose(req.Params, "enabled"))
			publish(st)
		}
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		w.Respond(req.ID, st)
	},
		ipc.MethodSpec{Name: "winrules.status", Desc: "Open floating: compositor, shell or manual, whether the rule is on, the rule to add by hand", Read: true},
		ipc.MethodSpec{Name: "winrules.set", Desc: "Turn open floating on or off (needs Dux Shell)", Params: []ipc.ParamSpec{ipc.Req("enabled", "true|false")}},
	)
}
