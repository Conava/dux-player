package cache

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Conava/dux-player/core/internal/ipc"
)

// Register adds the cache methods; c is nil outside the daemon and when the
// cache couldn't open.
func Register(r *ipc.Registry, c func() *Cache) {
	r.Topics(Topic)
	r.Group("cache", func(ctx context.Context, w *ipc.ConnWriter, req ipc.Request) {
		c := c()
		if c == nil {
			w.RespondError(req.ID, "the track cache is not available")
			return
		}
		p := req.Params
		switch req.Method {
		case "cache.status":
			ipc.Reply(w, req.ID)(c.Status(ctx))
		case "cache.addRule":
			r := Rule{Kind: ipc.Str(p, "kind"), ID: ipc.Str(p, "id"), Limit: limitParam(p)}
			ipc.ReplyOK(w, req.ID, c.AddRule(ctx, ipc.Str(p, "library"), r))
		case "cache.updateRule":
			l := limitParam(p)
			if l == nil {
				w.RespondError(req.ID, "limit must be {songs:N} or {bytes:N}")
				return
			}
			ipc.ReplyOK(w, req.ID, c.UpdateRule(ctx, ipc.Str(p, "library"), ipc.Str(p, "kind"), *l))
		case "cache.removeRule":
			ipc.ReplyOK(w, req.ID, c.RemoveRule(ctx, ipc.Str(p, "library"), Rule{Kind: ipc.Str(p, "kind"), ID: ipc.Str(p, "id")}))
		case "cache.setLimit":
			mb, err := ipc.Int(p, "mb", 0)
			if err != nil {
				w.RespondError(req.ID, err.Error())
				return
			}
			ipc.ReplyOK(w, req.ID, c.SetLimit(ctx, int64(mb)<<20))
		case "cache.list":
			ipc.Reply(w, req.ID)(c.List(ctx, ipc.Str(p, "library")))
		case "cache.clear":
			ipc.ReplyOK(w, req.ID, c.Clear(ctx, ipc.Str(p, "library")))
		}
	},
		ipc.MethodSpec{Name: "cache.status", Desc: "Size limit, usage, downloads and each server library's offline rules", Read: true},
		ipc.MethodSpec{Name: "cache.addRule", Desc: "Keep a library's tracks on this device by a rule", Params: []ipc.ParamSpec{ipc.Opt("library", "server library; empty for a local playlist"), ipc.Req("kind", strings.Join(kinds, "|")), ipc.Opt("id", "playlist, album or artist id"), ipc.Opt("limit", "played and mostPlayed: {songs:N} or {bytes:N}")}},
		ipc.MethodSpec{Name: "cache.updateRule", Desc: "Change the limit of a played or mostPlayed rule", Params: []ipc.ParamSpec{ipc.Req("library", ""), ipc.Req("kind", "played|mostPlayed"), ipc.Req("limit", "{songs:N} or {bytes:N}")}},
		ipc.MethodSpec{Name: "cache.removeRule", Desc: "Remove a rule; deletes the copies no other rule holds", Params: []ipc.ParamSpec{ipc.Opt("library", "server library; empty for a local playlist"), ipc.Req("kind", ""), ipc.Opt("id", "")}},
		ipc.MethodSpec{Name: "cache.setLimit", Desc: "Set the size limit; evicts down to it", Params: []ipc.ParamSpec{ipc.Req("mb", "megabytes, at least 100")}},
		ipc.MethodSpec{Name: "cache.list", Desc: "Cached tracks, most recently used first, with the rules holding each", Params: []ipc.ParamSpec{ipc.Opt("library", "only this library")}, Read: true},
		ipc.MethodSpec{Name: "cache.clear", Desc: "Delete cached tracks", Params: []ipc.ParamSpec{ipc.Opt("library", "only this library; default: all")}},
	)
}

// limitParam reads {songs:N} or {bytes:N}, also as JSON text (the CLI's
// key=value); nil when absent or malformed.
func limitParam(p map[string]any) *Limit {
	m, ok := p["limit"].(map[string]any)
	if text, isText := p["limit"].(string); isText {
		ok = json.Unmarshal([]byte(text), &m) == nil
	}
	if !ok {
		return nil
	}
	songs, serr := ipc.Int(m, "songs", 0)
	bytes, _, berr := ipc.Num(m, "bytes")
	if serr != nil || berr != nil {
		return nil
	}
	return &Limit{Songs: songs, Bytes: int64(bytes)}
}
