package ipc

import (
	"context"
	"errors"
	"fmt"

	"github.com/duxlabs-dev/dux-go/ipc/params"
	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/pendingmarks"
	"github.com/Conava/dux-player/core/internal/settings"
	"github.com/Conava/dux-player/core/internal/source"
)

const (
	TopicSettings = "settings"
	TopicUI       = "ui"
	TopicMarks    = library.TopicMarks
)

func RegisterSettings(r *Registry, d *Deps) {
	r.Topics(TopicSettings)
	r.Group("settings", func(ctx context.Context, w *ConnWriter, req Request) { handleSettings(ctx, w, req, *d) },
		MethodSpec{Name: "settings.get", Desc: "Playback settings and what limits them", Read: true},
		MethodSpec{Name: "settings.set", Desc: "Change playback settings; all or nothing", Params: []ParamSpec{Opt("previousThreshold", "seconds, 0-60"), Opt("allowVolumeBoost", "true|false"), Opt("gaplessMode", "weak|yes|no"), Opt("replayGainMode", "track|album|off"), Opt("keepPlayingOnClose", "true|false"), Opt("speed", "0.5-2; needs mpv (player status.canChangeSpeed)"), Opt("equalizer", "{enabled, preset, bands[10]} (JSON from the CLI)"), Opt("crossfade", "{enabled, seconds 1-12, albumGapless} (JSON from the CLI); needs mpv (player status.canCrossfade)"), Opt("autoContinue", "off|random|similar"), Opt("playNextOrder", "added|newest: Play next picks line up in the order added, or the newest plays first"), Opt("shuffleMode", "tracks|albums"), Opt("skipRules", "{minRating 0-5, genres[], libraries[] (library ids), shorterThan seconds} for what the daemon picks by itself (JSON from the CLI)"), Opt("historyMonths", "0-120 months; 0 keeps the history forever"), Opt("mpvIdleExit", "minutes mpv stays with nothing playing, 0-1440; 0 keeps it"), Opt("volumeTarget", "app|system: what the volume controls drive; system needs wpctl (status.systemVolume)"), Opt("skipSilenceStart", "true|false; needs ffmpeg (status.ffmpeg)"), Opt("skipSilenceEnd", "true|false; needs ffmpeg (status.ffmpeg)"), Opt("prefetchTracks", "upcoming server tracks downloaded ahead, 0-3"), Opt("prefetchOnMetered", "true|false: prefetch on a metered connection too"), Opt("lyrics", "true|false: look up lyrics"), Opt("lyricsForDownloads", "true|false: find lyrics when a track is downloaded"), Opt("lyricsAudioTiming", "true|false: fit word timing to the audio; needs ffmpeg (status.ffmpeg)"), Opt("discordPresence", "true|false: show the playing track in Discord")}},
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
	// A refused sign-in is not an outage, and Retry would send the password again.
	if err != nil && !errors.Is(err, source.ErrSignInRefused) && queueMark(ctx, deps, event) {
		err = nil
	}
	if err != nil {
		w.RespondError(req.ID, err.Error())
		return
	}
	var fav *bool
	var rating *int
	if v, ok := event["favorite"].(bool); ok {
		fav = &v
	}
	if v, ok := event["rating"].(int); ok {
		rating = &v
	}
	deps.Libraries.MarkChanged(ctx, lib, kind, id, fav, rating)
	if kind == "track" {
		if err := deps.Player.Mark(ctx, source.Ref{Library: lib, ID: id}, fav, rating); err != nil {
			log.Warnf("queue marks: %v", err)
		}
	}
	deps.Bus.Publish(TopicMarks, event)
	w.Respond(req.ID, event)
}

// queueMark keeps a favorite or rating its server couldn't take because it
// is unreachable, to send once it is back. The event is the change; false
// when it failed for another reason.
func queueMark(ctx context.Context, deps Deps, event map[string]any) bool {
	fav, hasFav := event["favorite"].(bool)
	rating, hasRating := event["rating"].(int)
	if deps.Marks == nil || !hasFav && !hasRating {
		return false
	}
	lib := event["library"].(string)
	if info, err := deps.Libraries.Retry(ctx, lib); err != nil || info.ErrorCode != library.CodeServerUnreachable {
		return false
	}
	c := pendingmarks.Change{Library: lib, Kind: event["kind"].(string), ID: event["id"].(string)}
	if hasFav {
		c.Favorite = &fav
	}
	if hasRating {
		c.Rating = &rating
	}
	if err := deps.Marks.Add(c); err != nil {
		log.Warnf("queue a mark: %v", err)
		return false
	}
	return true
}

// FlushMarks sends the queued favorites and ratings of the libraries that
// are reachable again.
func FlushMarks(ctx context.Context, deps Deps) {
	if deps.Marks == nil || deps.Marks.Len() == 0 {
		return
	}
	infos, err := deps.Libraries.List(ctx)
	if err != nil {
		log.Warnf("send queued marks: %v", err)
		return
	}
	online := make(map[string]bool, len(infos))
	for _, info := range infos {
		online[info.ID] = info.Available
	}
	deps.Marks.Flush(ctx, online, func(ctx context.Context, c pendingmarks.Change) error {
		err := sendMark(ctx, deps, c)
		if err != nil {
			if info, rerr := deps.Libraries.Retry(ctx, c.Library); rerr == nil && info.ErrorCode == library.CodeServerUnreachable {
				return fmt.Errorf("%w: %w", pendingmarks.ErrUnreachable, err)
			}
		}
		return err
	})
}

func sendMark(ctx context.Context, deps Deps, c pendingmarks.Change) error {
	ctx, cancel := context.WithTimeout(ctx, source.CallTimeout)
	defer cancel()
	event := map[string]any{"library": c.Library, "kind": c.Kind, "id": c.ID}
	if c.Favorite != nil {
		f, err := Capability[source.Favorites](deps.Libraries, c.Library, "favorites")
		if err == nil {
			err = f.SetFavorite(ctx, c.Kind, c.ID, *c.Favorite)
		}
		if err != nil {
			return err
		}
		event["favorite"] = *c.Favorite
	}
	if c.Rating != nil {
		r, err := Capability[source.Ratings](deps.Libraries, c.Library, "ratings")
		if err == nil {
			err = r.SetRating(ctx, c.Kind, c.ID, *c.Rating)
		}
		if err != nil {
			return err
		}
		event["rating"] = *c.Rating
	}
	// The cache's favorites rule resyncs on it.
	deps.Bus.Publish(TopicMarks, event)
	return nil
}
