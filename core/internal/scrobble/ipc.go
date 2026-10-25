package scrobble

import (
	"context"

	"github.com/duxlabs-dev/dux-go/ipc/params"

	"github.com/Conava/dux-player/core/internal/ipc"
)

// Register adds the scrobbling methods. svc is read per request.
func Register(r *ipc.Registry, svc func() *Service) {
	r.Topics(Topic)
	r.Group("scrobble", func(ctx context.Context, w *ipc.ConnWriter, req ipc.Request) {
		s := svc()
		p := req.Params
		service := ipc.Str(p, "service")
		var err error
		var result any
		switch req.Method {
		case "scrobble.status":
		case "scrobble.connect":
			switch service {
			case ListenBrainz:
				err = s.ConnectListenBrainz(ctx, ipc.Str(p, "token"))
			case LastFM:
				var page string
				if page, err = s.StartLastFM(ctx, ipc.Str(p, "apiKey"), ipc.Str(p, "apiSecret")); err == nil {
					result = map[string]string{"authUrl": page}
				}
			default:
				err = errService
			}
		case "scrobble.finishAuth":
			err = s.FinishLastFM(ctx)
		case "scrobble.disconnect":
			err = s.Disconnect(service)
		case "scrobble.test":
			var user string
			if user, err = s.Test(ctx, service); err == nil {
				result = map[string]any{"ok": true, "user": user}
				go s.Flush()
			}
		case "scrobble.setEnabled":
			err = s.SetEnabled(service, params.BoolLoose(p, "enabled"))
		case "scrobble.setLibrary":
			err = s.SetLibrary(ipc.Str(p, "library"), params.BoolLoose(p, "enabled"))
		}
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		if result == nil {
			result = s.Status()
		}
		w.Respond(req.ID, result)
	},
		ipc.MethodSpec{Name: "scrobble.status", Desc: "Scrobbling accounts, per-library switches and queued plays", Read: true},
		ipc.MethodSpec{Name: "scrobble.connect", Desc: "Set up an account; Last.fm returns the page to approve it on", Params: []ipc.ParamSpec{ipc.Req("service", "listenbrainz|lastfm"), ipc.Opt("token", "ListenBrainz user token"), ipc.Opt("apiKey", "Last.fm API key, required for lastfm"), ipc.Opt("apiSecret", "Last.fm shared secret, required for lastfm")}},
		ipc.MethodSpec{Name: "scrobble.finishAuth", Desc: "Finish the Last.fm sign-in after approving it in the browser"},
		ipc.MethodSpec{Name: "scrobble.disconnect", Desc: "Forget an account", Params: []ipc.ParamSpec{ipc.Req("service", "listenbrainz|lastfm")}},
		ipc.MethodSpec{Name: "scrobble.test", Desc: "Check an account's credentials", Params: []ipc.ParamSpec{ipc.Req("service", "listenbrainz|lastfm")}},
		ipc.MethodSpec{Name: "scrobble.setEnabled", Desc: "Turn scrobbling to an account on or off", Params: []ipc.ParamSpec{ipc.Req("service", "listenbrainz|lastfm"), ipc.Req("enabled", "true|false")}},
		ipc.MethodSpec{Name: "scrobble.setLibrary", Desc: "Scrobble plays from a library or not", Params: []ipc.ParamSpec{ipc.Req("library", ""), ipc.Req("enabled", "true|false")}},
	)
}
