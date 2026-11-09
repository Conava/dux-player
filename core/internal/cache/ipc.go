package cache

import (
	"context"
	"encoding/json"
	"errors"
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
			if _, set := p["limit"]; set && l == nil {
				w.RespondError(req.ID, "limit must be {songs:N} or {bytes:N}")
				return
			}
			var scope *string
			if v, set := p["scope"]; set {
				s, ok := v.(string)
				if !ok {
					w.RespondError(req.ID, "scope must be a library id, or empty for all servers")
					return
				}
				scope = &s
			}
			if l == nil && scope == nil {
				w.RespondError(req.ID, "give a limit or a scope to change")
				return
			}
			ipc.ReplyOK(w, req.ID, c.UpdateRule(ctx, ipc.Str(p, "library"), ipc.Str(p, "kind"), ipc.Str(p, "id"), l, scope))
		case "cache.removeRule":
			ipc.ReplyOK(w, req.ID, c.RemoveRule(ctx, ipc.Str(p, "library"), Rule{Kind: ipc.Str(p, "kind"), ID: ipc.Str(p, "id")}))
		case "cache.keep", "cache.keepState":
			refs, err := ipc.Refs(p, "tracks")
			if err != nil {
				w.RespondError(req.ID, err.Error())
				return
			}
			if req.Method == "cache.keepState" {
				ipc.Reply(w, req.ID)(c.KeepState(ctx, refs))
				return
			}
			on, ok := ipc.Bool(p, "keep")
			if !ok {
				w.RespondError(req.ID, "keep must be true or false")
				return
			}
			ipc.ReplyOK(w, req.ID, c.Keep(ctx, refs, on))
		case "cache.setLimit":
			mb, err := ipc.Int(p, "mb", 0)
			if err != nil {
				w.RespondError(req.ID, err.Error())
				return
			}
			ipc.ReplyOK(w, req.ID, c.SetLimit(ctx, int64(mb)*1e6))
		case "cache.list":
			ipc.Reply(w, req.ID)(c.List(ctx, ipc.Str(p, "library")))
		case "cache.tracks":
			lib := ipc.Str(p, "library")
			offset, err := ipc.Int(p, "offset", 0)
			limit, lerr := ipc.Int(p, "limit", 0)
			excluded, ok := ipc.Bool(p, "excluded")
			switch {
			case lib == "":
				err = errors.New("library is required")
			case err == nil && lerr != nil:
				err = lerr
			case err == nil && !ok && p["excluded"] != nil:
				err = errors.New("excluded must be true or false")
			}
			if err != nil {
				w.RespondError(req.ID, err.Error())
				return
			}
			ipc.Reply(w, req.ID)(c.Tracks(ctx, lib, offset, limit, excluded))
		case "cache.delete", "cache.include":
			refs, err := ipc.Refs(p, "track")
			if err == nil && len(refs) != 1 {
				err = errors.New("give exactly one track")
			}
			exclude, ok := ipc.Bool(p, "exclude")
			if err == nil && !ok && p["exclude"] != nil {
				err = errors.New("exclude must be true or false")
			}
			if err != nil {
				w.RespondError(req.ID, err.Error())
				return
			}
			if req.Method == "cache.include" {
				ipc.ReplyOK(w, req.ID, c.Include(ctx, refs[0]))
				return
			}
			ipc.ReplyOK(w, req.ID, c.Delete(ctx, refs[0], exclude))
		case "cache.clear":
			ipc.ReplyOK(w, req.ID, c.Clear(ctx, ipc.Str(p, "library")))
		}
	},
		ipc.MethodSpec{Name: "cache.status", Desc: "Size limit, usage, downloads, each server library's usage and one list of every offline rule", Read: true},
		ipc.MethodSpec{Name: "cache.addRule", Desc: "Keep tracks on this device by a rule", Params: []ipc.ParamSpec{ipc.Opt("library", "server library; empty for all servers, or a local playlist with kind playlist"), ipc.Req("kind", strings.Join(kinds, "|")), ipc.Opt("id", "playlist, album or artist id"), ipc.Opt("limit", "played and mostPlayed: {songs:N} or {bytes:N}")}},
		ipc.MethodSpec{Name: "cache.keep", Desc: "Keep single server tracks on this device, or let them go", Params: []ipc.ParamSpec{ipc.Req("tracks", "track refs: library:id strings or {library,id} objects"), ipc.Req("keep", "true|false")}},
		ipc.MethodSpec{Name: "cache.keepState", Desc: "How many of these tracks are kept by hand and how many a pinning rule holds", Params: []ipc.ParamSpec{ipc.Req("tracks", "track refs: library:id strings or {library,id} objects")}, Read: true},
		ipc.MethodSpec{Name: "cache.updateRule", Desc: "Change a played or mostPlayed rule's limit, or move a rule to another library or all servers", Params: []ipc.ParamSpec{ipc.Opt("library", "the rule's library; empty for all servers"), ipc.Req("kind", ""), ipc.Opt("id", ""), ipc.Opt("limit", "played and mostPlayed: {songs:N} or {bytes:N}"), ipc.Opt("scope", "everything, favorites, played and mostPlayed: new library; empty for all servers")}},
		ipc.MethodSpec{Name: "cache.removeRule", Desc: "Remove a rule; deletes the copies no other rule holds", Params: []ipc.ParamSpec{ipc.Opt("library", "the rule's library; empty for all servers, or a local playlist with kind playlist"), ipc.Req("kind", ""), ipc.Opt("id", "")}},
		ipc.MethodSpec{Name: "cache.setLimit", Desc: "Set the size limit; evicts down to it", Params: []ipc.ParamSpec{ipc.Req("mb", "megabytes, at least 100")}},
		ipc.MethodSpec{Name: "cache.list", Desc: "Cached tracks, most recently used first, with the rules holding each", Params: []ipc.ParamSpec{ipc.Opt("library", "only this library")}, Read: true},
		ipc.MethodSpec{Name: "cache.clear", Desc: "Delete cached tracks; exclusions stay", Params: []ipc.ParamSpec{ipc.Opt("library", "only this library; default: all")}},
		ipc.MethodSpec{Name: "cache.tracks", Desc: "One page of a server library's excluded tracks, then its copies most recently used first: {total, items}", Params: []ipc.ParamSpec{ipc.Req("library", "server library"), ipc.Opt("offset", "items to skip"), ipc.Opt("limit", "items per page, default 100, at most 500"), ipc.Opt("excluded", "true: only the excluded tracks")}, Read: true},
		ipc.MethodSpec{Name: "cache.delete", Desc: "Delete one track's copy; with exclude, no rule or prefetch downloads it again", Params: []ipc.ParamSpec{ipc.Req("track", "track ref: library:id string or {library,id} object"), ipc.Opt("exclude", "true|false, default false")}},
		ipc.MethodSpec{Name: "cache.include", Desc: "Lift a track's exclusion, so its rules download it again", Params: []ipc.ParamSpec{ipc.Req("track", "track ref: library:id string or {library,id} object")}},
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
