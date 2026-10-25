package artists

import (
	"context"
	"path/filepath"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/source"
)

// Register adds the artist page methods; svc is read per request because
// the service exists only in the daemon.
func Register(r *ipc.Registry, svc func() *Service) {
	r.Topics(TopicPlaylists)
	lib, id := ipc.Req("library", ""), ipc.Req("id", "artist id")
	handler := func(ctx context.Context, w *ipc.ConnWriter, req ipc.Request) {
		s := svc()
		if s == nil {
			w.RespondError(req.ID, "artist info is not available")
			return
		}
		p := req.Params
		switch req.Method {
		case "browse.artist":
			ipc.Reply(w, req.ID)(s.Info(ctx, ipc.Str(p, "library"), ipc.Str(p, "id")))
		case "browse.artistImage":
			size, _ := ipc.Int(p, "size", 0)
			path, from, err := s.Image(ctx, ipc.Str(p, "library"), ipc.Str(p, "id"), ipc.Str(p, "name"), ipc.Str(p, "coverId"), size)
			if err != nil {
				w.RespondError(req.ID, err.Error())
				return
			}
			w.Respond(req.ID, map[string]string{"path": path, "source": from,
				"color": source.CoverColor(filepath.Join(s.cfg.CacheDir, "colors"), path)})
		case "browse.artistPlaylists":
			ipc.Reply(w, req.ID)(s.Playlists(ctx, ipc.Str(p, "library"), ipc.Str(p, "id"), ipc.Str(p, "name")))
		case "artistInfo.status":
			w.Respond(req.ID, s.Status())
		case "artistInfo.setKey":
			if err := s.SetKey(ipc.Str(p, "provider"), ipc.Str(p, "key")); err != nil {
				w.RespondError(req.ID, err.Error())
				return
			}
			w.Respond(req.ID, s.Status())
		}
	}
	r.Group("browse", handler,
		ipc.MethodSpec{Name: "browse.artist", Desc: "An artist's info: image id, biography, similar artists, favorite, counts", Params: []ipc.ParamSpec{lib, id}, Read: true},
		ipc.MethodSpec{Name: "browse.artistImage", Desc: "Local file of an artist's image: the library's, else online", Params: []ipc.ParamSpec{lib, id, ipc.Opt("name", "artist name, for online lookups"), ipc.Opt("coverId", "the artist's coverId, when it has one"), ipc.Opt("size", "pixels, if the server scales")}, Read: true},
		ipc.MethodSpec{Name: "browse.artistPlaylists", Desc: "Local and this library's server playlists with tracks of the artist, from indexes; artistPlaylists events say when to ask again", Params: []ipc.ParamSpec{lib, id, ipc.Opt("name", "artist name: also matches tracks of other libraries in local playlists")}, Read: true},
	)
	r.Group("artistInfo", handler,
		ipc.MethodSpec{Name: "artistInfo.status", Desc: "Online artist info: on or off, providers and their keys", Read: true},
		ipc.MethodSpec{Name: "artistInfo.setKey", Desc: "Set or clear a provider's API key (kept in the keyring)", Params: []ipc.ParamSpec{ipc.Req("provider", ProviderFanartTV+"|"+ProviderLastFM), ipc.Req("key", "empty clears")}},
	)
}
