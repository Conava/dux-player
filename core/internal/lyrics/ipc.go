package lyrics

import (
	"context"
	"errors"
	"slices"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/source"
)

// Register adds the lyrics methods. svc is read per request: the service
// exists only in the daemon.
func Register(r *ipc.Registry, svc func() *Service) {
	r.Topics(TopicProviders)
	r.OnSubscribe(func() { svc().Subscribed() })
	r.Group("lyrics", func(ctx context.Context, w *ipc.ConnWriter, req ipc.Request) {
		s := svc()
		switch req.Method {
		case "lyrics.get":
			lib, id := ipc.Str(req.Params, "library"), ipc.Str(req.Params, "id")
			if lib == "" || id == "" {
				w.RespondError(req.ID, "library and id are required")
				return
			}
			res, err := s.Get(ctx, source.Ref{Library: lib, ID: id})
			if err != nil {
				w.RespondError(req.ID, err.Error())
				return
			}
			w.Respond(req.ID, res)
		case "lyrics.providers":
			w.Respond(req.ID, s.providers.get())
		case "lyrics.setProviders":
			list, err := providerParam(req.Params["providers"])
			if err == nil {
				list, err = s.providers.set(list)
			}
			if err != nil {
				w.RespondError(req.ID, err.Error())
				return
			}
			s.cfg.Publish(TopicProviders, list)
			w.Respond(req.ID, list)
		}
	},
		ipc.MethodSpec{Name: "lyrics.get", Desc: "Lyrics of a track (dux-go lyrics Result)", Params: []ipc.ParamSpec{ipc.Req("library", ""), ipc.Req("id", "")}, Read: true},
		ipc.MethodSpec{Name: "lyrics.providers", Desc: "Lyrics providers in priority order", Read: true},
		ipc.MethodSpec{Name: "lyrics.setProviders", Desc: "Set the provider order and which are on", Params: []ipc.ParamSpec{ipc.Req("providers", "[{id,enabled}] in priority order; from the CLI, the ids to turn on, in order")}},
	)
}

// providerParam reads [{id, enabled}] objects (the UI) or bare ids (the
// CLI: those are on, in that order, the rest off).
func providerParam(v any) ([]Provider, error) {
	raw, ok := v.([]any)
	if !ok {
		if v == nil {
			return nil, errors.New("providers is required")
		}
		raw = []any{v}
	}
	var out []Provider
	bare := false
	for _, x := range raw {
		switch p := x.(type) {
		case string:
			bare = true
			out = append(out, Provider{ID: p, Enabled: true})
		case map[string]any:
			id, _ := p["id"].(string)
			on, _ := p["enabled"].(bool)
			out = append(out, Provider{ID: id, Enabled: on})
		default:
			return nil, errors.New("providers: expected ids or {id, enabled} objects")
		}
	}
	if bare {
		for _, p := range defaultProviders() {
			if !slices.ContainsFunc(out, func(q Provider) bool { return q.ID == p.ID }) {
				out = append(out, Provider{ID: p.ID})
			}
		}
	}
	return out, nil
}
