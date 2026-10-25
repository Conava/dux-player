package ipc

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/duxlabs-dev/dux-go/ipc/params"

	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
	"github.com/Conava/dux-player/core/internal/source/local"
)

// maxPathTracks bounds a play-by-path request; every file's tags are read
// before playback starts.
const maxPathTracks = 5000

const (
	tracksDesc = "track refs: library:id strings or {library,id} objects"
	pathsDesc  = "absolute files or folders, played through the built-in file library"
	albumDesc  = "library:albumId, the album's tracks in order"
	listDesc   = "library id: its whole track list (with sort, artist as in browse.tracks), at most 5000 around index"
)

func RegisterPlayer(r *Registry, d *Deps) {
	r.Topics(player.TopicStatus, player.TopicPosition, player.TopicQueue, player.TopicError)
	// A new subscriber gets the current state right away instead of waiting
	// for the next change.
	r.Greet(func(send func(string, any)) {
		_ = d.Player.Snapshot(d.Ctx, func(st player.Status, q player.QueueSnapshot) {
			send(player.TopicStatus, st)
			send(player.TopicQueue, q)
		})
	})
	r.Group("player", func(ctx context.Context, w *ConnWriter, req Request) { handlePlayer(ctx, w, req, *d) },
		MethodSpec{Name: "player.status", Desc: "Playback state, position, volume, modes and current track", Read: true},
		MethodSpec{Name: "player.play", Desc: "Replace the queue and play; with nothing given, resume", Params: []ParamSpec{Opt("tracks", tracksDesc), Opt("paths", pathsDesc), Opt("album", albumDesc), Opt("playlist", "library:playlistId, a server playlist"), Opt("library", listDesc), Opt("sort", "with library"), Opt("artist", "with library"), Opt("favorites", "true: every favorite track"), Opt("index", "queue index (with library or favorites: list row) to start at"), Opt("shuffle", "true: shuffle these tracks; otherwise they play in order with shuffle off"), Opt("origin", "{kind,library,id,name}: what the UI is playing (playlist, favorites, …)")}},
		MethodSpec{Name: "player.resume", Desc: "Play the saved queue from the saved position"},
		MethodSpec{Name: "player.shuffleFrom", Desc: "Replace the queue with shuffled tracks and play", Params: []ParamSpec{Req("from", "tracks|favorites|library|playlist"), Opt("name", "library or playlist name (or id)")}},
		MethodSpec{Name: "player.shuffle", Desc: "Shuffle the queue, or restore its order", Params: []ParamSpec{Opt("on", "true|false"), Opt("mode", "old form of on: off|tracks|albums")}},
		MethodSpec{Name: "player.repeat", Desc: "Set the repeat mode", Params: []ParamSpec{Req("mode", "off|all|one")}},
		MethodSpec{Name: "player.pause", Desc: "Pause"},
		MethodSpec{Name: "player.toggle", Desc: "Play or pause"},
		MethodSpec{Name: "player.stop", Desc: "Stop and unload the current track"},
		MethodSpec{Name: "player.next", Desc: "Skip to the next track"},
		MethodSpec{Name: "player.previous", Desc: "Restart the track, or go back if it just started"},
		MethodSpec{Name: "player.seek", Desc: "Seek", Params: []ParamSpec{Req("position", "seconds"), Opt("relative", "true: seek by position instead of to it")}},
		MethodSpec{Name: "player.mute", Desc: "Mute or unmute; kept across restarts", Params: []ParamSpec{Opt("muted", "true|false; default: toggle")}},
		MethodSpec{Name: "player.sleep", Desc: "Sleep timer: pause after minutes, at the end of the track, or off", Params: []ParamSpec{Opt("minutes", "1-1440"), Opt("endOfTrack", "true: pause when the current track ends"), Opt("off", "true: cancel")}},
		MethodSpec{Name: "player.volume", Desc: "Set the volume", Params: []ParamSpec{Req("level", "0-100, 130 with volume boost"), Opt("relative", "true: change by level")}},
	)
	r.Group("queue", func(ctx context.Context, w *ConnWriter, req Request) { handleQueue(ctx, w, req, *d) },
		MethodSpec{Name: "queue.list", Desc: "The queue and the current index", Read: true},
		MethodSpec{Name: "queue.add", Desc: "Add tracks to the queue", Params: []ParamSpec{Opt("tracks", tracksDesc), Opt("paths", pathsDesc), Opt("album", albumDesc), Opt("playlist", "library:playlistId, a server playlist"), Opt("library", listDesc), Opt("sort", "with library"), Opt("artist", "with library"), Opt("favorites", "true: every favorite track"), Opt("next", "true: insert after the current track")}},
		MethodSpec{Name: "queue.remove", Desc: "Remove a queue entry", Params: []ParamSpec{Req("index", "")}},
		MethodSpec{Name: "queue.move", Desc: "Move a queue entry", Params: []ParamSpec{Req("from", ""), Req("to", "")}},
		MethodSpec{Name: "queue.jump", Desc: "Play the queue entry at index", Params: []ParamSpec{Req("index", "")}},
		MethodSpec{Name: "queue.clear", Desc: "Stop and empty the queue", Params: []ParamSpec{Opt("keepCurrent", "true: keep the current track playing and drop the rest")}},
		MethodSpec{Name: "queue.continue", Desc: "Extend this queue endlessly, whatever autoContinue says, starting now; status continuing turns true", Params: []ParamSpec{Opt("mode", "similar|random; default similar")}},
	)
}

// shuffleParam reads on; mode is the form from before shuffle was only on or
// off ("tracks" and "albums" both mean on).
// Mode is kept for one release; drop it after 0.1.0.
func shuffleParam(p map[string]any) (bool, error) {
	if on, ok := Bool(p, "on"); ok {
		return on, nil
	}
	switch Str(p, "mode") {
	case "off":
		return false, nil
	case "tracks", "albums":
		return true, nil
	}
	return false, errors.New("on must be true or false")
}

var originKinds = map[string]bool{"track": true, "album": true, "playlist": true, "library": true, "favorites": true,
	"tracks": true, "adhoc": true, "artist": true, "genre": true, "view": true, "search": true, "history": true}

// tracksFrom gathers tracks from the tracks, paths, album and playlist
// params, in that order, or a whole list (library or favorites), and works
// out where they came from. An explicit origin param wins. A list is cut to
// maxListTracks around row index; skip is the rows cut before it.
func tracksFrom(ctx context.Context, p map[string]any, deps Deps, index int) ([]source.Track, player.Origin, int, error) {
	var tracks []source.Track
	var origin player.Origin
	var skip int
	var err error
	if Str(p, "library") != "" || params.BoolLoose(p, "favorites") {
		tracks, origin, skip, err = listFrom(ctx, p, deps, index)
	} else {
		tracks, origin, err = pickedFrom(ctx, p, deps)
	}
	if err != nil {
		return nil, origin, 0, err
	}
	if o, ok, oerr := originParam(p); oerr != nil || ok {
		return tracks, o, skip, oerr
	}
	return tracks, origin, skip, nil
}

// originParam is the origin the UI says it plays from, if it sent one.
func originParam(p map[string]any) (player.Origin, bool, error) {
	o, ok := p["origin"].(map[string]any)
	if text, isText := p["origin"].(string); isText {
		// The CLI sends the object as JSON text.
		ok = json.Unmarshal([]byte(text), &o) == nil
	}
	if !ok {
		return player.Origin{}, false, nil
	}
	kind := Str(o, "kind")
	if !originKinds[kind] {
		return player.Origin{}, false, fmt.Errorf("origin kind %q is unknown", kind)
	}
	return player.Origin{Kind: kind, Library: Str(o, "library"), ID: Str(o, "id"), Name: source.Clean(Str(o, "name"))}, true, nil
}

func listFrom(ctx context.Context, p map[string]any, deps Deps, index int) ([]source.Track, player.Origin, int, error) {
	for _, k := range []string{"tracks", "paths", "album", "playlist"} {
		if p[k] != nil {
			return nil, player.Origin{}, 0, fmt.Errorf("library and favorites play a whole list; leave out %s", k)
		}
	}
	if params.BoolLoose(p, "favorites") {
		f, err := deps.Libraries.Favorites(ctx)
		tracks, skip := around(f.Tracks, index)
		return tracks, player.Origin{Kind: "favorites", Name: "Favorites"}, skip, err
	}
	lib := Str(p, "library")
	b, err := browser(deps, lib)
	if err != nil {
		return nil, player.Origin{}, 0, err
	}
	tracks, skip, err := listTracks(ctx, b, p, index)
	origin := player.Origin{Kind: "library", Library: lib, ID: lib}
	if l, lerr := deps.Libraries.Library(ctx, lib); lerr == nil {
		origin.Name = l.Name
	}
	if artist := Str(p, "artist"); artist != "" {
		origin = player.Origin{Kind: "artist", Library: lib, ID: artist}
		if len(tracks) > 0 {
			origin.Name = tracks[0].Artist
		}
	}
	return tracks, origin, skip, err
}

func pickedFrom(ctx context.Context, p map[string]any, deps Deps) ([]source.Track, player.Origin, error) {
	var origin player.Origin
	rs, err := Refs(p, "tracks")
	if err != nil {
		return nil, origin, err
	}
	paths := Strs(p, "paths")
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return nil, origin, fmt.Errorf("path %q is not absolute", path)
		}
		files, err := local.Collect(ctx, filepath.Clean(path))
		if err != nil {
			return nil, origin, err
		}
		for _, f := range files {
			rs = append(rs, source.Ref{Library: local.FilesLibrary, ID: f})
		}
		if len(rs) > maxPathTracks {
			return nil, origin, fmt.Errorf("more than %d files; add a library for this folder instead", maxPathTracks)
		}
	}
	tracks, err := deps.Libraries.Tracks(ctx, rs)
	if err != nil {
		return nil, origin, err
	}
	switch {
	case len(tracks) == 1 && len(paths) == 0:
		origin = player.Origin{Kind: "track", Library: tracks[0].Library, ID: tracks[0].ID, Name: tracks[0].Title}
	case len(paths) == 1:
		origin = player.Origin{Kind: "adhoc", Name: filepath.Base(paths[0])}
	case len(tracks) > 0:
		origin = player.Origin{Kind: "adhoc"}
	}
	if album := Str(p, "album"); album != "" {
		lib, id, ok := strings.Cut(album, ":")
		if !ok {
			return nil, origin, fmt.Errorf("album %q is not library:id", album)
		}
		b, err := browser(deps, lib)
		if err != nil {
			return nil, origin, err
		}
		a, err := b.Album(ctx, id)
		if err != nil {
			return nil, origin, err
		}
		if len(tracks) == 0 {
			origin = player.Origin{Kind: "album", Library: lib, ID: id, Name: a.Name}
		} else {
			origin = player.Origin{Kind: "adhoc"}
		}
		tracks = append(tracks, a.Tracks...)
	}
	if ref := Str(p, "playlist"); ref != "" {
		lib, id, ok := strings.Cut(ref, ":")
		if !ok {
			return nil, origin, fmt.Errorf("playlist %q is not library:id", ref)
		}
		pls, err := Capability[source.Playlists](deps.Libraries, lib, "server playlists")
		if err != nil {
			return nil, origin, err
		}
		pl, err := pls.Playlist(ctx, id)
		if err != nil {
			return nil, origin, err
		}
		if len(tracks) == 0 {
			origin = player.Origin{Kind: "playlist", Library: lib, ID: id, Name: pl.Name}
		} else {
			origin = player.Origin{Kind: "adhoc"}
		}
		tracks = append(tracks, pl.Tracks...)
	}
	return tracks, origin, nil
}

func handlePlayer(ctx context.Context, w *ConnWriter, req Request, deps Deps) {
	pl := deps.Player
	p := req.Params
	var err error
	switch req.Method {
	case "player.status":
	case "player.play":
		var index int
		if index, err = Int(p, "index", 0); err != nil {
			break
		}
		if refs, ok, rerr := refsOnly(p); rerr != nil || ok {
			if err = rerr; err == nil {
				err = playRefs(ctx, p, deps, refs, index)
			}
			break
		}
		tracks, origin, skip, terr := tracksFrom(ctx, p, deps, index)
		if err = terr; err != nil {
			break
		}
		if len(tracks) == 0 {
			err = pl.Resume(ctx)
			break
		}
		if params.BoolLoose(p, "shuffle") {
			err = pl.PlayShuffled(ctx, tracks, origin)
			break
		}
		err = pl.Play(ctx, tracks, index-skip, origin)
	case "player.resume":
		err = pl.Resume(ctx)
	case "player.shuffleFrom":
		sh, serr := deps.Libraries.ShuffleSource(ctx, Str(p, "from"), Str(p, "name"))
		switch {
		case serr != nil:
			err = serr
		case sh.Feed != nil:
			err = pl.PlayFeed(ctx, sh.Feed, sh.Origin)
		default:
			err = pl.PlayShuffled(ctx, sh.Tracks, sh.Origin)
		}
	case "player.shuffle":
		on, serr := shuffleParam(p)
		if err = serr; err == nil {
			err = pl.SetShuffle(ctx, on)
		}
	case "player.repeat":
		err = pl.SetRepeat(ctx, Str(p, "mode"))
	case "player.pause":
		err = pl.Pause(ctx)
	case "player.toggle":
		err = pl.Toggle(ctx)
	case "player.stop":
		err = pl.Stop(ctx)
	case "player.next":
		err = pl.Next(ctx)
	case "player.previous":
		err = pl.Previous(ctx)
	case "player.seek":
		pos, ok, perr := Num(p, "position")
		switch {
		case perr != nil:
			err = perr
		case !ok:
			err = errors.New("missing position")
		default:
			err = pl.Seek(ctx, pos, params.BoolLoose(p, "relative"))
		}
	case "player.mute":
		st, _ := pl.Status(ctx)
		muted := !st.Muted
		if _, ok := p["muted"]; ok {
			muted = params.BoolLoose(p, "muted")
		}
		err = pl.SetMute(ctx, muted)
	case "player.sleep":
		minutes, ok, perr := Num(p, "minutes")
		switch {
		case perr != nil:
			err = perr
		case params.BoolLoose(p, "off"):
			err = pl.SetSleep(ctx, player.SleepOff, 0)
		case params.BoolLoose(p, "endOfTrack"):
			err = pl.SetSleep(ctx, player.SleepEndOfTrack, 0)
		case ok:
			err = pl.SetSleep(ctx, player.SleepTimer, time.Duration(minutes*float64(time.Minute)))
		default:
			err = errors.New("give minutes, endOfTrack=true or off=true")
		}
	case "player.volume":
		level, ok, perr := Num(p, "level")
		switch {
		case perr != nil:
			err = perr
		case !ok:
			err = errors.New("missing level")
		default:
			err = pl.SetVolume(ctx, level, params.BoolLoose(p, "relative"))
		}
	}
	if err != nil {
		w.RespondError(req.ID, err.Error())
		return
	}
	st, err := pl.Status(ctx)
	if err != nil {
		w.RespondError(req.ID, err.Error())
		return
	}
	w.Respond(req.ID, st)
}

func handleQueue(ctx context.Context, w *ConnWriter, req Request, deps Deps) {
	pl := deps.Player
	p := req.Params
	var err error
	switch req.Method {
	case "queue.list":
	case "queue.add":
		tracks, _, _, terr := tracksFrom(ctx, p, deps, 0)
		if err = terr; err == nil {
			if len(tracks) == 0 {
				err = errors.New("nothing to add")
				break
			}
			err = pl.Enqueue(ctx, tracks, params.BoolLoose(p, "next"))
		}
	case "queue.remove":
		var i int
		if i, err = Int(p, "index", -1); err == nil {
			err = pl.Remove(ctx, i)
		}
	case "queue.move":
		from, ferr := Int(p, "from", -1)
		to, terr := Int(p, "to", -1)
		if err = errors.Join(ferr, terr); err == nil {
			err = pl.Move(ctx, from, to)
		}
	case "queue.jump":
		var i int
		if i, err = Int(p, "index", -1); err == nil {
			err = pl.Jump(ctx, i)
		}
	case "queue.clear":
		err = pl.Clear(ctx, params.BoolLoose(p, "keepCurrent"))
	case "queue.continue":
		err = pl.Continue(ctx, cmp.Or(Str(p, "mode"), player.ContinueSimilar))
	}
	if err != nil {
		w.RespondError(req.ID, err.Error())
		return
	}
	q, err := pl.Queue(ctx)
	if err != nil {
		w.RespondError(req.ID, err.Error())
		return
	}
	w.Respond(req.ID, q)
}

// refsOnly reports a play of nothing but track refs, longer than what
// resolves before playback starts: those play while the rest resolves.
func refsOnly(p map[string]any) ([]source.Ref, bool, error) {
	for _, k := range []string{"paths", "album", "playlist", "library", "favorites"} {
		if p[k] != nil {
			return nil, false, nil
		}
	}
	refs, err := Refs(p, "tracks")
	if err != nil {
		return nil, false, err
	}
	if len(refs) > maxListTracks {
		return nil, false, fmt.Errorf("more than %d tracks; play the library or playlist instead", maxListTracks)
	}
	return refs, len(refs) > 1, nil
}

func playRefs(ctx context.Context, p map[string]any, deps Deps, refs []source.Ref, index int) error {
	origin, ok, err := originParam(p)
	if err != nil {
		return err
	}
	if !ok {
		origin = player.Origin{Kind: "adhoc"}
	}
	return deps.Player.PlayRefs(ctx, refs, index, origin, params.BoolLoose(p, "shuffle"), func(ctx context.Context, refs []source.Ref) ([]source.Track, int) {
		return resolveEach(ctx, deps, refs)
	})
}

// resolveEach resolves refs together, or one by one when some fail, leaving
// those out.
func resolveEach(ctx context.Context, deps Deps, refs []source.Ref) ([]source.Track, int) {
	if tracks, err := deps.Libraries.Tracks(ctx, refs); err == nil {
		return tracks, 0
	}
	out := make([]source.Track, 0, len(refs))
	for _, r := range refs {
		t, err := deps.Libraries.Tracks(ctx, []source.Ref{r})
		if err != nil || len(t) == 0 {
			continue
		}
		out = append(out, t[0])
	}
	return out, len(refs) - len(out)
}
