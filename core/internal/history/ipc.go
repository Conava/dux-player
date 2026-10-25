package history

import (
	"context"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/source"
)

const Topic = "history"

// Register adds the history methods; store is read per request because it
// exists only in the daemon.
func Register(r *ipc.Registry, store func() *Store, publish func(topic string, data any)) {
	r.Topics(Topic)
	kindDesc := "track|album|artist, default track"
	r.Group("history", func(ctx context.Context, w *ipc.ConnWriter, req ipc.Request) {
		s := store()
		if s == nil {
			w.RespondError(req.ID, "history is not available")
			return
		}
		p := req.Params
		limit, _ := ipc.Int(p, "limit", 50)
		limit = min(max(limit, 1), 500)
		kind := source.ItemTrack
		if k, _ := p["kind"].(string); k == source.ItemAlbum || k == source.ItemArtist {
			kind = k
		}
		var v any
		var err error
		switch req.Method {
		case "history.list":
			before, _ := ipc.Int(p, "before", 0)
			v, err = s.List(ctx, limit, int64(before))
		case "history.top":
			v, err = s.Top(ctx, kind, limit, false)
		case "history.recent":
			v, err = s.Top(ctx, kind, limit, true)
		case "history.clear":
			if err = s.Clear(ctx); err == nil {
				publish(Topic, map[string]any{"type": "cleared"})
				v = map[string]bool{"ok": true}
			}
		}
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		w.Respond(req.ID, v)
	},
		ipc.MethodSpec{Name: "history.list", Desc: "Counted plays, newest first", Params: []ipc.ParamSpec{ipc.Opt("limit", "1-500, default 50"), ipc.Opt("before", "unix seconds: plays before this, for paging")}, Read: true},
		ipc.MethodSpec{Name: "history.top", Desc: "Most played tracks, albums or artists of every library: this device's plays and the servers' counts", Params: []ipc.ParamSpec{ipc.Opt("kind", kindDesc), ipc.Opt("limit", "1-500, default 50")}, Read: true},
		ipc.MethodSpec{Name: "history.recent", Desc: "Recently played tracks, albums or artists, each once", Params: []ipc.ParamSpec{ipc.Opt("kind", kindDesc), ipc.Opt("limit", "1-500, default 50")}, Read: true},
		ipc.MethodSpec{Name: "history.clear", Desc: "Delete the whole play history"},
	)
}
