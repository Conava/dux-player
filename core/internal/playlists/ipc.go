package playlists

import (
	"context"
	"errors"

	"github.com/Conava/dux-player/core/internal/ipc"
)

// Register adds the localPlaylists methods (server playlists stay in the
// playlists group); svc is nil outside the daemon.
func Register(r *ipc.Registry, svc func() *Service) {
	id := ipc.Req("id", "local playlist id")
	r.Topics(Topic)
	r.Group("localPlaylists", func(ctx context.Context, w *ipc.ConnWriter, req ipc.Request) {
		s := svc()
		if s == nil {
			w.RespondError(req.ID, "local playlists are not available")
			return
		}
		handle(ctx, w, req, s)
	},
		ipc.MethodSpec{Name: "localPlaylists.list", Desc: "Local playlists, without tracks", Read: true},
		ipc.MethodSpec{Name: "localPlaylists.get", Desc: "A local playlist with its tracks", Params: []ipc.ParamSpec{id}, Read: true},
		ipc.MethodSpec{Name: "localPlaylists.create", Desc: "Create a local playlist", Params: []ipc.ParamSpec{ipc.Req("name", ""), ipc.Opt("tracks", "track refs from any library")}},
		ipc.MethodSpec{Name: "localPlaylists.rename", Desc: "Rename a local playlist", Params: []ipc.ParamSpec{id, ipc.Req("name", "")}},
		ipc.MethodSpec{Name: "localPlaylists.delete", Desc: "Delete a local playlist", Params: []ipc.ParamSpec{id}},
		ipc.MethodSpec{Name: "localPlaylists.duplicate", Desc: "Copy a local or server playlist into a new local one", Params: []ipc.ParamSpec{ipc.Req("id", "playlist id"), ipc.Opt("library", "set for a server playlist"), ipc.Opt("name", "default: the original's")}},
		ipc.MethodSpec{Name: "localPlaylists.add", Desc: "Add tracks to a local playlist", Params: []ipc.ParamSpec{id, ipc.Req("tracks", "track refs from any library"), ipc.Opt("at", "position; default: the end")}},
		ipc.MethodSpec{Name: "localPlaylists.remove", Desc: "Remove tracks from a local playlist", Params: []ipc.ParamSpec{id, ipc.Req("positions", "positions to remove")}},
		ipc.MethodSpec{Name: "localPlaylists.move", Desc: "Move a track within a local playlist", Params: []ipc.ParamSpec{id, ipc.Req("from", "position"), ipc.Req("to", "position")}},
		ipc.MethodSpec{Name: "localPlaylists.setCover", Desc: "Set a local playlist's cover", Params: []ipc.ParamSpec{id, ipc.Req("cover", `{type: auto} | {type: art, library, id} | {type: shape, accent, shape}`)}},
		ipc.MethodSpec{Name: "localPlaylists.play", Desc: "Play a local playlist", Params: []ipc.ParamSpec{id, ipc.Opt("index", "start position"), ipc.Opt("shuffle", "true|false")}},
		ipc.MethodSpec{Name: "localPlaylists.import", Desc: "Import an .m3u, .m3u8 or .pls file as a new local playlist", Params: []ipc.ParamSpec{ipc.Req("path", "absolute path of the file"), ipc.Opt("name", "default: the file name")}},
		ipc.MethodSpec{Name: "localPlaylists.export", Desc: "Write a local playlist as .m3u8 or .pls; tracks without a file are left out", Params: []ipc.ParamSpec{id, ipc.Req("path", "absolute path to write"), ipc.Opt("format", "m3u8|pls; default: by extension")}},
	)
}

func handle(ctx context.Context, w *ipc.ConnWriter, req ipc.Request, svc *Service) {
	p := req.Params
	id := ipc.Str(p, "id")
	ok := func(err error) { ipc.ReplyOK(w, req.ID, err) }
	switch req.Method {
	case "localPlaylists.list":
		ipc.Reply(w, req.ID)(svc.Store.List(ctx))
	case "localPlaylists.get":
		ipc.Reply(w, req.ID)(svc.Get(ctx, id))
	case "localPlaylists.create":
		refs, err := ipc.Refs(p, "tracks")
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		ipc.Reply(w, req.ID)(svc.Create(ctx, ipc.Str(p, "name"), refs))
	case "localPlaylists.rename":
		ok(svc.Rename(ctx, id, ipc.Str(p, "name")))
	case "localPlaylists.delete":
		ok(svc.Delete(ctx, id))
	case "localPlaylists.duplicate":
		ipc.Reply(w, req.ID)(svc.Duplicate(ctx, ipc.Str(p, "library"), id, ipc.Str(p, "name")))
	case "localPlaylists.add":
		refs, err := ipc.Refs(p, "tracks")
		at, aerr := ipc.Int(p, "at", -1)
		if err = errors.Join(err, aerr); err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		ok(svc.Add(ctx, id, refs, at))
	case "localPlaylists.remove":
		pos, err := ipc.Ints(p, "positions")
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		ok(svc.Remove(ctx, id, pos))
	case "localPlaylists.move":
		from, ferr := ipc.Int(p, "from", -1)
		to, terr := ipc.Int(p, "to", -1)
		if err := errors.Join(ferr, terr); err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		ok(svc.Move(ctx, id, from, to))
	case "localPlaylists.setCover":
		raw, _ := p["cover"].(map[string]any)
		c := Cover{Type: ipc.Str(raw, "type"), Library: ipc.Str(raw, "library"), ID: ipc.Str(raw, "id"),
			Accent: ipc.Str(raw, "accent"), Shape: ipc.Str(raw, "shape")}
		if raw == nil {
			// From the CLI: cover=auto, or the fields as flat params.
			c = Cover{Type: ipc.Str(p, "cover"), Library: ipc.Str(p, "library"), ID: ipc.Str(p, "track"),
				Accent: ipc.Str(p, "accent"), Shape: ipc.Str(p, "shape")}
		}
		ok(svc.SetCover(ctx, id, c))
	case "localPlaylists.play":
		index, err := ipc.Int(p, "index", 0)
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		shuffle, _ := ipc.Bool(p, "shuffle")
		ok(svc.Play(ctx, id, index, shuffle))
	case "localPlaylists.import":
		ipc.Reply(w, req.ID)(svc.Import(ctx, ipc.Str(p, "path"), ipc.Str(p, "name")))
	case "localPlaylists.export":
		ipc.Reply(w, req.ID)(svc.Export(ctx, id, ipc.Str(p, "path"), ipc.Str(p, "format")))
	}
}
