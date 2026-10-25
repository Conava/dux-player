package ipc

import (
	"context"
	"errors"

	"github.com/duxlabs-dev/dux-go/ipc/params"
	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/settings"
	"github.com/Conava/dux-player/core/internal/source"
)

const (
	TopicSettings = "settings"
	TopicUI       = "ui"
	TopicMarks    = "marks"
)

func RegisterSettings(r *Registry, d *Deps) {
	r.Topics(TopicSettings)
	r.Group("settings", func(ctx context.Context, w *ConnWriter, req Request) { handleSettings(ctx, w, req, *d) },
		MethodSpec{Name: "settings.get", Desc: "Playback settings and what limits them", Read: true},
		MethodSpec{Name: "settings.set", Desc: "Change playback settings; all or nothing", Params: []ParamSpec{Opt("previousThreshold", "seconds, 0-60"), Opt("allowVolumeBoost", "true|false"), Opt("gaplessMode", "weak|yes|no"), Opt("replayGainMode", "track|album|off"), Opt("keepPlayingOnClose", "true|false"), Opt("speed", "0.5-2"), Opt("equalizer", "{enabled, preset, bands[10]} (JSON from the CLI)"), Opt("crossfade", "{enabled, seconds 1-12, albumGapless} (JSON from the CLI)"), Opt("autoContinue", "off|random|similar"), Opt("historyMonths", "1-120"), Opt("mpvIdleExit", "minutes mpv stays with nothing playing, 0-1440; 0 keeps it"), Opt("volumeTarget", "app|system: what the volume controls drive; system needs wpctl (status.systemVolume)"), Opt("skipSilenceStart", "true|false; needs ffmpeg (status.ffmpeg)"), Opt("skipSilenceEnd", "true|false; needs ffmpeg (status.ffmpeg)")}},
	)
}

func RegisterUI(r *Registry, d *Deps) {
	r.Topics(TopicUI)
	r.Group("ui", func(ctx context.Context, w *ConnWriter, req Request) { handleUI(ctx, w, req, *d) },
		MethodSpec{Name: "ui.show", Desc: "Open the window, or raise the open one", Params: []ParamSpec{Opt("shellDir", "quickshell dir instead of the embedded UI")}},
		MethodSpec{Name: "ui.status", Desc: "Whether the window process runs", Read: true},
	)
}

func RegisterMarks(r *Registry, d *Deps) {
	r.Topics(TopicMarks)
	r.Group("marks", func(ctx context.Context, w *ConnWriter, req Request) { handleMarks(ctx, w, req, *d) },
		MethodSpec{Name: "marks.setFavorite", Desc: "Set or clear a favorite", Params: []ParamSpec{Req("library", ""), Req("kind", "track|album|artist"), Req("id", ""), Req("favorite", "true|false")}},
		MethodSpec{Name: "marks.setRating", Desc: "Set a 1-5 star rating, 0 clears", Params: []ParamSpec{Req("library", ""), Req("kind", "track|album|artist"), Req("id", ""), Req("rating", "0-5")}},
		MethodSpec{Name: "marks.favorites", Desc: "Favorite tracks, albums and artists of every library", Read: true},
	)
}

func handleSettings(ctx context.Context, w *ConnWriter, req Request, deps Deps) {
	switch req.Method {
	case "settings.get":
		w.Respond(req.ID, settingsReply(deps))
	case "settings.set":
		next, err := deps.Settings.Set(req.Params)
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		deps.Libraries.Meter().SetWanted(next.ReplayGainMode != "off", next.SkipSilenceStart || next.SkipSilenceEnd)
		if err := deps.Player.Configure(ctx, next.PlayerOptions()); err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		reply := settingsReply(deps)
		deps.Bus.Publish(TopicSettings, reply)
		w.Respond(req.ID, reply)
	}
}

// The reply carries what the settings page needs to explain a disabled or
// ineffective option, such as loudness measuring without ffmpeg.
func settingsReply(deps Deps) map[string]any {
	return map[string]any{
		"settings": deps.Settings.Get(),
		"status": map[string]any{"ffmpeg": deps.FFmpeg, "equalizerPresets": settings.Presets, "equalizerBands": settings.EQBands,
			"systemVolume": deps.Player.SystemVolumeAvailable()},
	}
}

func handleUI(_ context.Context, w *ConnWriter, req Request, deps Deps) {
	switch req.Method {
	case "ui.show":
		// The window process owns the window; a running one is asked to
		// raise itself, the way Dux Files does it.
		started, err := deps.Window.Start(Str(req.Params, "shellDir"))
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		if !started {
			deps.Bus.Publish(TopicUI, map[string]any{"action": "show"})
		}
		w.Respond(req.ID, map[string]bool{"started": started})
	case "ui.status":
		w.Respond(req.ID, map[string]bool{"running": deps.Window.Running()})
	}
}

func handleMarks(ctx context.Context, w *ConnWriter, req Request, deps Deps) {
	p := req.Params
	if req.Method == "marks.favorites" {
		r, err := deps.Libraries.Favorites(ctx)
		annotate(deps, &r)
		Reply(w, req.ID)(r, err)
		return
	}
	lib, kind, id := Str(p, "library"), Str(p, "kind"), Str(p, "id")
	if !source.ValidItemKind(kind) || id == "" {
		w.RespondError(req.ID, "kind must be track, album or artist, and id is required")
		return
	}
	event := map[string]any{"library": lib, "kind": kind, "id": id}
	var err error
	switch req.Method {
	case "marks.setFavorite":
		var f source.Favorites
		if f, err = Capability[source.Favorites](deps.Libraries, lib, "favorites"); err == nil {
			fav := params.BoolLoose(p, "favorite")
			err = f.SetFavorite(ctx, kind, id, fav)
			event["favorite"] = fav
		}
	case "marks.setRating":
		var r source.Ratings
		var rating int
		if r, err = Capability[source.Ratings](deps.Libraries, lib, "ratings"); err != nil {
			break
		}
		if rating, err = Int(p, "rating", -1); err != nil {
			break
		}
		if rating < 0 || rating > 5 {
			err = errors.New("rating must be 0 to 5")
			break
		}
		err = r.SetRating(ctx, kind, id, rating)
		event["rating"] = rating
	}
	if err != nil {
		w.RespondError(req.ID, err.Error())
		return
	}
	if kind == "track" {
		var fav *bool
		var rating *int
		if v, ok := event["favorite"].(bool); ok {
			fav = &v
		}
		if v, ok := event["rating"].(int); ok {
			rating = &v
		}
		if err := deps.Player.Mark(ctx, source.Ref{Library: lib, ID: id}, fav, rating); err != nil {
			log.Warnf("queue marks: %v", err)
		}
	}
	deps.Bus.Publish(TopicMarks, event)
	w.Respond(req.ID, event)
}
