package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/source"
)

const defaultSearchLimit = 20

func browser(deps Deps, lib string) (source.Browser, error) {
	return Capability[source.Browser](deps.Libraries, lib, "browsing")
}

func RegisterLibraries(r *Registry, d *Deps) {
	r.Topics(library.TopicLibraries, TopicPlaylists)
	r.Group("libraries", func(ctx context.Context, w *ConnWriter, req Request) { handleLibraries(ctx, w, req, *d) },
		MethodSpec{Name: "libraries.list", Desc: "Configured libraries with capabilities and errors", Read: true},
		MethodSpec{Name: "libraries.kinds", Desc: "Library kinds that can be added: local (folders), subsonic ((Open)Subsonic), jellyfin", Read: true},
		MethodSpec{Name: "libraries.add", Desc: "Add a library", Params: []ParamSpec{Req("kind", "one of libraries.kinds"), Req("name", ""), Opt("folders", "local: absolute folders"), Opt("url", "server address"), Opt("username", "server account"), Opt("password", "server password; stored in the keyring"), Opt("auth", "(Open)Subsonic: auto|token|password|apiKey")}},
		MethodSpec{Name: "libraries.remove", Desc: "Remove a library and its index", Params: []ParamSpec{Req("library", "")}},
		MethodSpec{Name: "libraries.scan", Desc: "Rescan a library; progress on the libraries topic", Params: []ParamSpec{Req("library", "")}},
		MethodSpec{Name: "libraries.ping", Desc: "Check a remote library's server", Params: []ParamSpec{Req("library", "")}, Read: true},
		MethodSpec{Name: "libraries.update", Desc: "Rename a library or change its folders, server or password", Params: []ParamSpec{Req("library", ""), Opt("name", ""), Opt("folders", "local: absolute folders, replacing the old ones"), Opt("url", ""), Opt("username", ""), Opt("password", "stored in the keyring"), Opt("auth", "(Open)Subsonic: auto|token|password|apiKey")}},
		MethodSpec{Name: "libraries.setEnabled", Desc: "Turn a library on or off", Params: []ParamSpec{Req("library", ""), Req("enabled", "true|false")}},
		MethodSpec{Name: "libraries.retry", Desc: "Check an unavailable library again now", Params: []ParamSpec{Req("library", "")}},
		MethodSpec{Name: "libraries.test", Desc: "Ping a server without adding it", Params: []ParamSpec{Req("kind", "a server kind from libraries.kinds"), Req("url", ""), Opt("username", ""), Opt("password", ""), Opt("auth", "(Open)Subsonic: auto|token|password|apiKey")}, Read: true},
	)
	r.Group("browse", func(ctx context.Context, w *ConnWriter, req Request) { handleBrowse(ctx, w, req, *d) },
		MethodSpec{Name: "browse.artists", Desc: "Artists of a library", Params: []ParamSpec{Req("library", "")}, Read: true},
		MethodSpec{Name: "browse.albums", Desc: "Albums of a library or one artist", Params: []ParamSpec{Req("library", ""), Opt("artist", "artist id")}, Read: true},
		MethodSpec{Name: "browse.album", Desc: "An album with its tracks", Params: []ParamSpec{Req("library", ""), Req("id", "album id")}, Read: true},
		MethodSpec{Name: "browse.tracks", Desc: "One page of all tracks of a library", Params: []ParamSpec{Req("library", ""), Opt("offset", "default 0"), Opt("limit", "1-500, default 500"), Opt("sort", "title|album|year|added; default album artist, album, track"), Opt("artist", "artist id: only this artist's tracks, album by album")}, Read: true},
		MethodSpec{Name: "browse.search", Desc: "Search one library", Params: []ParamSpec{Req("library", ""), Req("query", ""), Opt("limit", "per kind, default 20")}, Read: true},
		MethodSpec{Name: "browse.details", Desc: "Everything known about a track or album: its fields plus file, format, tags, ReplayGain, cache", Params: []ParamSpec{Req("library", ""), Req("kind", "track|album"), Req("id", "track id or album id")}, Read: true},
		MethodSpec{Name: "browse.cover", Desc: "Local file path of cover art", Params: []ParamSpec{Req("library", ""), Req("id", "coverId"), Opt("size", "pixels, if the server scales")}, Read: true},
	)
	r.Group("playlists", func(ctx context.Context, w *ConnWriter, req Request) { handlePlaylists(ctx, w, req, *d) },
		MethodSpec{Name: "playlists.list", Desc: "Server playlists of a library", Params: []ParamSpec{Req("library", "")}, Read: true},
		MethodSpec{Name: "playlists.get", Desc: "A server playlist with its tracks", Params: []ParamSpec{Req("library", ""), Req("id", "")}, Read: true},
		MethodSpec{Name: "playlists.create", Desc: "Create a server playlist", Params: []ParamSpec{Req("library", ""), Req("name", ""), Opt("tracks", "track ids in that library")}},
		MethodSpec{Name: "playlists.update", Desc: "Rename or edit a server playlist", Params: []ParamSpec{Req("library", ""), Req("id", ""), Opt("name", ""), Opt("add", "track ids to append"), Opt("remove", "positions to remove")}},
		MethodSpec{Name: "playlists.move", Desc: "Move a track within a server playlist", Params: []ParamSpec{Req("library", ""), Req("id", ""), Req("from", "position"), Req("to", "position")}},
		MethodSpec{Name: "playlists.delete", Desc: "Delete a server playlist", Params: []ParamSpec{Req("library", ""), Req("id", "")}},
	)
}

func handleLibraries(ctx context.Context, w *ConnWriter, req Request, deps Deps) {
	p := req.Params
	lib := Str(p, "library")
	switch req.Method {
	case "libraries.list":
		Reply(w, req.ID)(deps.Libraries.List(ctx))
	case "libraries.kinds":
		w.Respond(req.ID, deps.Libraries.Kinds())
	case "libraries.add":
		config, secrets := libraryParams(p, false)
		added, err := deps.Libraries.Add(ctx, Str(p, "kind"), Str(p, "name"), config, secrets)
		if err == nil {
			deps.Bus.Publish(library.TopicLibraries, map[string]any{"type": "changed"})
		}
		Reply(w, req.ID)(added, err)
	case "libraries.remove":
		err := deps.Libraries.Remove(ctx, lib)
		if err == nil {
			deps.Bus.Publish(library.TopicLibraries, map[string]any{"type": "changed"})
		}
		ReplyOK(w, req.ID, err)
	case "libraries.scan":
		if _, err := Capability[source.Scanner](deps.Libraries, lib, "scanning"); err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		go scan(deps, lib)
		w.Respond(req.ID, map[string]bool{"started": true})
	case "libraries.ping":
		pinger, err := Capability[source.Pinger](deps.Libraries, lib, "ping")
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		Reply(w, req.ID)(pinger.Ping(ctx))
	case "libraries.update":
		config, secrets := libraryParams(p, true)
		Reply(w, req.ID)(deps.Libraries.Update(ctx, lib, Str(p, "name"), config, secrets))
	case "libraries.setEnabled":
		enabled, ok := Bool(p, "enabled")
		if !ok {
			w.RespondError(req.ID, "enabled must be true or false")
			return
		}
		ReplyOK(w, req.ID, deps.Libraries.SetEnabled(ctx, lib, enabled))
	case "libraries.retry":
		Reply(w, req.ID)(deps.Libraries.Retry(ctx, lib))
	case "libraries.test":
		config, secrets := libraryParams(p, true)
		Reply(w, req.ID)(deps.Libraries.Test(ctx, Str(p, "kind"), config, secrets))
	}
}

// libraryParams splits a request into config keys and secrets. Each kind's
// factory checks the keys it needs, so IPC never switches on the kind. An
// edit keeps empty values, which clear a key; a new library drops them.
func libraryParams(p map[string]any, keepEmpty bool) (map[string]any, map[string]string) {
	config := map[string]any{}
	if _, ok := p["folders"]; ok {
		folders := []any{}
		for _, f := range Strs(p, "folders") {
			folders = append(folders, f)
		}
		config["folders"] = folders
	}
	for _, k := range []string{"url", "username", "auth"} {
		if v, ok := p[k].(string); ok && (keepEmpty || v != "") {
			config[k] = v
		}
	}
	secrets := map[string]string{}
	// An empty password is a real one: Jellyfin accounts may have none.
	if pw, ok := p["password"].(string); ok {
		secrets["password"] = pw
	}
	return config, secrets
}

// RescanStale scans the libraries whose index has files to reread: after an
// index migration or a change of the artist separators.
func RescanStale(deps Deps) {
	ids, err := deps.Libraries.Store().Stale(deps.Ctx)
	if err != nil {
		log.Warnf("libraries to rescan: %v", err)
		return
	}
	for _, id := range ids {
		if _, err := Capability[source.Scanner](deps.Libraries, id, "scanning"); err == nil {
			scan(deps, id)
		}
	}
}

func scan(deps Deps, lib string) {
	progress := func(done, total int) {
		deps.Bus.Publish(library.TopicLibraries, map[string]any{"type": "scanProgress", "library": lib, "done": done, "total": total})
	}
	stats, err := deps.Libraries.Scan(deps.Ctx, lib, progress)
	if errors.Is(err, library.ErrScanning) {
		return
	}
	if err != nil {
		log.Warnf("scan %s: %v", lib, err)
		deps.Bus.Publish(library.TopicLibraries, map[string]any{"type": "scanFailed", "library": lib,
			"code": library.CodeScanFailed, "error": "Couldn't scan the library"})
		return
	}
	log.Infof("scan %s: %+v", lib, stats)
	deps.Bus.Publish(library.TopicLibraries, map[string]any{"type": "scanDone", "library": lib, "stats": stats})
}

func handleBrowse(ctx context.Context, w *ConnWriter, req Request, deps Deps) {
	p := req.Params
	lib := Str(p, "library")
	if req.Method == "browse.cover" {
		c, err := Capability[source.CoverArt](deps.Libraries, lib, "cover art")
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		size, _ := Int(p, "size", 0)
		path, err := c.Cover(ctx, Str(p, "id"), size)
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		w.Respond(req.ID, map[string]string{"path": path, "color": source.CoverColor(filepath.Join(deps.CacheDir, "colors"), path)})
		return
	}
	if req.Method == "browse.details" {
		Reply(w, req.ID)(details(ctx, deps, lib, Str(p, "kind"), Str(p, "id")))
		return
	}
	b, err := browser(deps, lib)
	if err != nil {
		w.RespondError(req.ID, err.Error())
		return
	}
	switch req.Method {
	case "browse.artists":
		Reply(w, req.ID)(b.Artists(ctx))
	case "browse.albums":
		as, err := b.Albums(ctx, Str(p, "artist"))
		annotate(deps, as)
		Reply(w, req.ID)(as, err)
	case "browse.album":
		a, err := b.Album(ctx, Str(p, "id"))
		annotate(deps, &a)
		Reply(w, req.ID)(a, err)
	case "browse.tracks":
		page, err := trackPage(ctx, b, p)
		annotate(deps, &page)
		Reply(w, req.ID)(page, err)
	case "browse.search":
		limit, err := Int(p, "limit", defaultSearchLimit)
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		r, err := b.Search(ctx, Str(p, "query"), min(max(limit, 1), 500))
		annotate(deps, &r)
		Reply(w, req.ID)(r, err)
	}
}

// details is the Track or Album with the source's Details laid over it,
// plus libraryName and, for a track, cached. A source without Details still
// answers with the item's own fields.
func details(ctx context.Context, deps Deps, lib, kind, id string) (map[string]any, error) {
	var item any
	switch kind {
	case "track":
		tracks, err := deps.Libraries.Tracks(ctx, []source.Ref{{Library: lib, ID: id}})
		if err != nil {
			return nil, err
		}
		if len(tracks) == 0 {
			return nil, source.ErrNotFound
		}
		annotate(deps, &tracks[0])
		item = tracks[0]
	case "album":
		b, err := browser(deps, lib)
		if err != nil {
			return nil, err
		}
		a, err := b.Album(ctx, id)
		if err != nil {
			return nil, err
		}
		annotate(deps, &a)
		a.Tracks = nil
		item = a
	default:
		return nil, errors.New("kind must be track or album")
	}
	out := map[string]any{}
	if err := mergeJSON(out, item); err != nil {
		return nil, err
	}
	src, err := deps.Libraries.Get(lib)
	if err != nil {
		return nil, err
	}
	out["libraryName"] = src.Library().Name
	var d source.Details
	if dt, ok := src.(source.Detailer); ok {
		if d, err = dt.Details(ctx, kind, id); err != nil {
			// The item's own fields still make a useful answer.
			log.Infof("details of %s %s: %v", kind, id, err)
		}
	}
	if _, remote := src.(source.Pinger); remote && kind == "track" {
		cached, gain := deps.Libraries.Cached(ctx, source.Ref{Library: lib, ID: id})
		out["cached"] = cached
		if gain != nil {
			if d.ReplayGain == nil {
				d.ReplayGain = &source.GainDetails{}
			}
			d.ReplayGain.Measured = gain
		}
	}
	return out, mergeJSON(out, d)
}

// mergeJSON lays v's JSON fields over m.
func mergeJSON(m map[string]any, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &m)
}

// PlaylistChanged publishes a server playlist edit, so every window reloads it.
func PlaylistChanged(bus *Bus, lib, id string) {
	bus.Publish(TopicPlaylists, map[string]string{"library": lib, "id": id})
}

func handlePlaylists(ctx context.Context, w *ConnWriter, req Request, deps Deps) {
	p := req.Params
	lib := Str(p, "library")
	done := func(err error) {
		if err == nil {
			PlaylistChanged(deps.Bus, lib, Str(p, "id"))
		}
		ReplyOK(w, req.ID, err)
	}
	switch req.Method {
	case "playlists.list", "playlists.get":
		pls, err := Capability[source.Playlists](deps.Libraries, lib, "server playlists")
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		if req.Method == "playlists.list" {
			Reply(w, req.ID)(pls.Playlists(ctx))
			return
		}
		pl, err := pls.Playlist(ctx, Str(p, "id"))
		annotate(deps, &pl)
		Reply(w, req.ID)(pl, err)
	case "playlists.create", "playlists.update":
		ed, err := Capability[source.PlaylistEditor](deps.Libraries, lib, "editing playlists")
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		if req.Method == "playlists.create" {
			name := source.Clean(Str(p, "name"))
			if name == "" {
				w.RespondError(req.ID, "a playlist needs a name")
				return
			}
			pl, err := ed.CreatePlaylist(ctx, name, Strs(p, "tracks"))
			if err == nil {
				PlaylistChanged(deps.Bus, lib, pl.ID)
			}
			Reply(w, req.ID)(pl, err)
			return
		}
		edit := source.PlaylistEdit{Add: Strs(p, "add")}
		if name, ok := p["name"].(string); ok {
			edit.Name = &name
		}
		remove, err := Ints(p, "remove")
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		edit.RemoveIndexes = remove
		done(ed.UpdatePlaylist(ctx, Str(p, "id"), edit))
	case "playlists.move":
		mv, err := Capability[source.PlaylistReorderer](deps.Libraries, lib, "reordering playlists")
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		from, ferr := Int(p, "from", -1)
		to, terr := Int(p, "to", -1)
		if err := errors.Join(ferr, terr); err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		done(mv.MovePlaylistTrack(ctx, Str(p, "id"), from, to))
	case "playlists.delete":
		del, err := Capability[source.PlaylistDeleter](deps.Libraries, lib, "deleting playlists")
		if err != nil {
			w.RespondError(req.ID, err.Error())
			return
		}
		done(del.DeletePlaylist(ctx, Str(p, "id")))
	}
}

// annotate adds play counts to the tracks and albums in a reply.
func annotate(deps Deps, v any) {
	if deps.Stats != nil {
		deps.Stats.Annotate(v)
	}
}

const maxTrackPage = 500

func trackPage(ctx context.Context, b source.Browser, p map[string]any) (source.TrackPage, error) {
	offset, oerr := Int(p, "offset", 0)
	limit, lerr := Int(p, "limit", maxTrackPage)
	if err := errors.Join(oerr, lerr); err != nil {
		return source.TrackPage{}, err
	}
	q := source.TrackQuery{Offset: max(offset, 0), Limit: min(max(limit, 1), maxTrackPage), Sort: Str(p, "sort")}
	if !source.ValidTrackSort(q.Sort) {
		return source.TrackPage{}, fmt.Errorf("sort %q is not title, album, year or added", q.Sort)
	}
	if artist := Str(p, "artist"); artist != "" {
		all, err := artistTracks(ctx, b, artist)
		return source.TrackPage{Total: len(all), Tracks: source.Window(all, q.Offset, q.Limit), Sort: artistSort(q.Sort)}, err
	}
	tp, ok := b.(source.TrackPager)
	if !ok {
		return source.TrackPage{}, errors.New("this library can't list all its tracks")
	}
	page, err := tp.TrackPage(ctx, q)
	if page.Tracks == nil {
		page.Tracks = []source.Track{}
	}
	return page, err
}

// An artist's tracks come album by album, in the order the library lists
// the albums; no other sort.
func artistSort(asked string) string {
	if asked == source.SortDefault {
		return source.SortDefault
	}
	return source.SortNone
}

// albumLookups bounds the album requests artistTracks runs at once.
const albumLookups = 8

func artistTracks(ctx context.Context, b source.Browser, artist string) ([]source.Track, error) {
	albums, err := b.Albums(ctx, artist)
	if err != nil {
		return nil, err
	}
	tracks := make([][]source.Track, len(albums))
	errs := make([]error, len(albums))
	sem := make(chan struct{}, albumLookups)
	var wg sync.WaitGroup
	for i, a := range albums {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()
			full, err := b.Album(ctx, a.ID)
			tracks[i], errs[i] = full.Tracks, err
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	out := []source.Track{}
	for i, ts := range tracks {
		whole := source.HasArtist(albums[i].Artists, albums[i].ArtistID, artist)
		for _, t := range ts {
			// An album the artist only appears on gives just their tracks.
			if whole || source.HasArtist(t.Artists, t.ArtistID, artist) {
				out = append(out, t)
			}
		}
	}
	return out, nil
}

// maxListTracks bounds a list played or queued without refs; every track of
// it goes into the queue (and its snapshots).
const maxListTracks = 5000

// around cuts all to maxListTracks around row index.
func around(all []source.Track, index int) ([]source.Track, int) {
	skip := min(max(index-maxListTracks/2, 0), max(len(all)-maxListTracks, 0))
	return source.Window(all, skip, maxListTracks), skip
}

// listTracks is up to maxListTracks of a library's track list (sort and
// artist as in browse.tracks) around row index; skip is how many rows come
// before the first returned one.
func listTracks(ctx context.Context, b source.Browser, p map[string]any, index int) (tracks []source.Track, skip int, err error) {
	skip = max(index-maxListTracks/2, 0)
	if artist := Str(p, "artist"); artist != "" {
		all, err := artistTracks(ctx, b, artist)
		tracks, skip := around(all, index)
		return tracks, skip, err
	}
	sort := Str(p, "sort")
	for len(tracks) < maxListTracks {
		q := map[string]any{"offset": float64(skip + len(tracks)), "limit": float64(min(maxTrackPage, maxListTracks-len(tracks))), "sort": sort}
		page, err := trackPage(ctx, b, q)
		if err != nil {
			return nil, 0, err
		}
		// Near the end, the window starts earlier so it stays full.
		if start := max(page.Total-maxListTracks, 0); len(tracks) == 0 && skip > start {
			skip = start
			continue
		}
		tracks = append(tracks, page.Tracks...)
		if len(page.Tracks) == 0 || skip+len(tracks) >= page.Total {
			break
		}
	}
	return tracks, skip, nil
}
