pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import "Catalog.js" as Catalog

// Libraries and what they hold: albums, artists, tracks (loaded on demand), covers, search, favorites and
// ratings. Lists are replaced, never edited in place, so bindings see each change; `revision` moves when
// tracks or marks change without a new list.
Singleton {
    id: root

    readonly property var log: Log.scoped("CatalogService")

    // {id, name, kind, enabled, available, folders, url, user, capabilities, noticeCodes, error, scanning,
    // lastScan (unix s), writeMarks}
    property var libraries: []
    // libraries.list has answered once, so an empty `libraries` means there are none
    property bool librariesLoaded: false
    // library id -> {done, total} while it scans; apart from libraries so progress doesn't re-run everything
    property var scanProgress: ({})
    // kinds the daemon can add: local, subsonic, jellyfin
    property var libraryKinds: []
    property var albums: []
    property var artists: []
    // tracks seen so far: album pages, playlists, and every page once a full track list is shown
    property var tracks: []
    property bool tracksLoading: false
    // moves when tracks or favorites/ratings change; track() and withMarks() depend on it
    property int revision: 0
    property var favoriteTrackList: []
    // "library:id" of cached tracks, for libraries that are unavailable right now
    property var cachedKeys: ({})

    property string searchQuery: ""
    property var searchResults: ({
            "albums": [],
            "artists": [],
            "tracks": []
        })

    // "library:coverId" → file URL / dominant "#rrggbb"; "" while it loads or when there is none
    property var covers: ({})
    property var coverColors: ({})
    // artist id → the image browse.artistImage found (library, artist.jpg or online), "" when there is none
    property var artistImages: ({})

    readonly property var genres: {
        const byId = {};
        const on = _enabledSet();
        for (const a of albums) {
            if (!on[a.library] || !a.genreId)
                continue;
            const g = byId[a.genreId] ?? (byId[a.genreId] = {
                    "id": a.genreId,
                    "name": a.genre,
                    "albumCount": 0,
                    "trackCount": 0,
                    "libraries": []
                });
            g.albumCount++;
            g.trackCount += a.trackCount;
            if (g.libraries.indexOf(a.library) < 0)
                g.libraries.push(a.library);
        }
        return Object.keys(byId).map(id => byId[id]);
    }
    readonly property var albumsByArtist: Catalog.groupBy(albums, a => a.artistId)
    readonly property var tracksByAlbum: Catalog.groupBy(tracks, t => t.albumId)

    signal serverTestFinished(bool ok, string message)
    // a library's catalog was (re)loaded, or went away (disabled, removed)
    signal libraryLoaded(string id)
    signal libraryDropped(string id)
    // a library's index of which playlists hold which artists changed
    signal artistPlaylistsChanged(string library)

    function key(ref) {
        return ref.library + ":" + ref.id;
    }

    // Catalog tracks win, so favorites stay current; queue, history and playlist entries carry their own copy
    // for tracks the catalog doesn't hold.
    function track(ref) {
        if (!ref || revision < 0)
            return null;
        return withMarks(_trackIndex[key(ref)] ?? (ref.title !== undefined ? ref : null));
    }

    // The item with the favorite and rating set since it was loaded; the same object when nothing changed.
    function withMarks(t) {
        if (!t || revision < 0)
            return t;
        const m = _marks[key(t)];
        return m ? Object.assign({}, t, m) : t;
    }

    // an artist with the marks set since it was loaded (an artist page's copy isn't in `artists`)
    function withArtistMarks(a) {
        if (!a || revision < 0)
            return a;
        const m = _marks["artist|" + a.library + ":" + a.rawId];
        return m ? Object.assign({}, a, m) : a;
    }

    function genre(id) {
        return genres.find(g => g.id === id) ?? null;
    }

    function album(id) {
        return albums.find(a => a.id === id) ?? null;
    }

    function artist(id) {
        return artists.find(a => a.id === id) ?? null;
    }

    function library(id) {
        return libraries.find(l => l.id === id) ?? null;
    }

    function libraryName(id) {
        return library(id)?.name ?? "";
    }

    function hasCapability(libraryId, capability) {
        return (library(libraryId)?.capabilities ?? []).indexOf(capability) >= 0;
    }

    // remote sources are the ones that can be pinged
    function isRemote(libraryId) {
        return hasCapability(libraryId, "ping");
    }

    function kindLabel(kind) {
        switch (kind) {
        case "local":
            return I18n.tr("Folders", "library kind option");
        case "subsonic":
            return I18n.tr("Subsonic", "library kind option");
        case "jellyfin":
            return I18n.tr("Jellyfin", "library kind option");
        }
        return kind;
    }

    // What a server library supports, compactly: "Favorites · Playlists · Scrobbling · No star ratings"
    function featureSummary(libraryId) {
        if (!isRemote(libraryId))
            return "";
        const features = [["favorites", I18n.tr("Favorites", "favorite tracks: page, cache mode, library feature"), I18n.tr("No favorites", "library feature missing")], ["ratings", I18n.tr("Star ratings", "library feature"), I18n.tr("No star ratings", "library feature missing")], ["playlists", I18n.tr("Playlists", "playlists: page, section, library feature"), I18n.tr("No playlists", "library feature missing")], ["scrobble", I18n.tr("Scrobbling", "scrobbling: feature and settings tab"), I18n.tr("No scrobbling", "library feature missing")]];
        const have = features.filter(f => hasCapability(libraryId, f[0])).map(f => f[1]);
        const missing = features.filter(f => !hasCapability(libraryId, f[0])).map(f => f[2]);
        return have.concat(missing).join(" · ");
    }

    function enabledLibrary(id) {
        return library(id)?.enabled ?? false;
    }

    function isCached(t) {
        return cachedKeys[key(t)] === true;
    }

    // Why a track can't play right now ("" when it can): its file is gone, or its library is unreachable and
    // no cached copy exists.
    function unavailableReason(t) {
        if (!t)
            return "";
        const lib = library(t.library);
        if (t.unavailable)
            return Phrases.reason(t.unavailableCode, t.unavailable, lib?.folders?.[0]);
        if (!lib || lib.available !== false || !lib.enabled || isCached(t))
            return "";
        return Phrases.libraryError(lib) || I18n.tr("%1 is unavailable", "why a track can't play, %1 is its library").arg(lib.name);
    }

    // One answer to "can this item do X", for tracks, albums, artists and playlists. Returns "" when it can,
    // else the reason to show. action: favorite | rate | playlistEdit | playlistReorder | playlistCover |
    // playlistDelete | playlistAdd (target: the playlist the item would go into)
    function whyNot(action, item, target) {
        if (!item)
            return "";
        const lib = item.library ?? "";
        const name = libraryName(lib);
        switch (action) {
        case "favorite":
            return hasCapability(lib, "favorites") ? "" : I18n.tr("%1 doesn't support favorites", "why the heart is disabled, %1 is the library").arg(name);
        case "rate":
            return hasCapability(lib, "ratings") ? "" : I18n.tr("%1 doesn't support star ratings", "why the stars are disabled, %1 is the library").arg(name);
        case "playlistAdd":
            if (!target)
                return "";
            if (target.kind !== "local" && lib !== target.library)
                return I18n.tr("Only tracks from %1 can be added", "why a server playlist cannot take the picked tracks, %1 is the server library").arg(libraryName(target.library));
            return whyNot("playlistEdit", target);
        case "playlistEdit":
        case "playlistReorder":
        case "playlistCover":
        case "playlistDelete":
            return playlistBlocker(action, item);
        }
        return "";
    }

    function playlistBlocker(action, playlist) {
        if (playlist.kind === "local")
            return "";
        const name = libraryName(playlist.library);
        // servers show their own playlist art, and none lets a client set it
        if (action === "playlistCover")
            return I18n.tr("%1 manages this playlist's cover.", "cover dialog note for server playlists without custom covers").arg(name);
        if (playlist.readOnly)
            return I18n.tr("This playlist belongs to another user on %1", "why a server playlist cannot be edited, %1 is the server library").arg(name);
        if (action === "playlistDelete")
            return hasCapability(playlist.library, "playlistDelete") ? "" : I18n.tr("%1 can't delete playlists", "why deleting is disabled, %1 is the server library").arg(name);
        if (!hasCapability(playlist.library, "playlistEdit"))
            return I18n.tr("%1 doesn't support editing playlists", "why playlist editing is disabled, %1 is the server library").arg(name);
        if (action === "playlistReorder" && !hasCapability(playlist.library, "playlistReorder"))
            return I18n.tr("%1 can't reorder playlists", "why reordering is disabled, %1 is the server library").arg(name);
        return "";
    }

    function allows(action, item, target) {
        return whyNot(action, item, target) === "";
    }

    // Splits a mixed selection into the items that support the action and a count of the rest.
    function partition(action, items, target) {
        const ok = items.filter(i => allows(action, i, target));
        return {
            "ok": ok,
            "skipped": items.length - ok.length
        };
    }

    function reportSkipped(count) {
        if (count > 0)
            DaemonBridge.notice(I18n.tr("%1 skipped: their library doesn't support this", "snackbar after acting on a mixed selection, %1 is a count").arg(count));
    }

    function albumTracks(albumId) {
        return (tracksByAlbum[albumId] ?? []).slice().sort((a, b) => a.disc - b.disc || a.number - b.number);
    }

    function artistAlbums(artistId) {
        const on = _enabledSet();
        return (albumsByArtist[artistId] ?? []).filter(a => on[a.library]).sort((a, b) => b.year - a.year);
    }

    function visibleAlbums() {
        const on = _enabledSet();
        return albums.filter(a => on[a.library]);
    }

    function visibleTracks() {
        const on = _enabledSet();
        return tracks.filter(t => on[t.library]);
    }

    function favoriteTracks() {
        const on = _enabledSet();
        return favoriteTrackList.map(t => track(t) ?? t).filter(t => t.favorite && on[t.library]);
    }

    // Browse query over the loaded catalog (albums, artists, tracks, genres): filters {library, genre,
    // favorites}, a sort key, descending.
    function query(type, options) {
        const lib = options.filters?.library ?? "";
        let list;
        switch (type) {
        case "albums":
            list = visibleAlbums();
            break;
        case "artists":
            {
                const on = _enabledSet();
                list = [];
                for (const a of artists) {
                    const own = (albumsByArtist[a.id] ?? []).filter(al => on[al.library]);
                    if (own.length > 0)
                        list.push(Object.assign({}, a, {
                            "albumCount": own.length
                        }));
                }
                if (lib !== "")
                    list = list.filter(a => a.libraries.indexOf(lib) >= 0);
                break;
            }
        case "tracks":
            if (options.filters?.favorites)
                list = favoriteTracks();
            else {
                list = visibleTracks();
                // later, so this binding doesn't depend on the flag it sets
                Qt.callLater(root._wantAllTracks);
            }
            break;
        case "genres":
            list = genres.slice();
            if (lib !== "")
                list = list.filter(g => g.libraries.indexOf(lib) >= 0);
            break;
        default:
            return [];
        }
        return refine(list, type, options);
    }

    // The filters and sort of query() over a list from elsewhere (playlists, most played).
    // Sorts use the values the list was loaded with; a new favorite or play shows on the row at
    // once (withMarks) and moves it on the next load, so a play doesn't re-sort a 100k-track list.
    function refine(list, type, options) {
        const filters = options.filters ?? {};
        const lib = filters.library ?? "";
        if (lib !== "" && (type === "albums" || type === "tracks"))
            list = list.filter(i => i.library === lib);
        if ((filters.genre ?? "") !== "")
            list = list.filter(i => i.genreId === filters.genre);
        if (filters.favorites)
            list = list.filter(i => i.favorite);
        const sortKey = options.sort ?? "name";
        const value = sortKey === "library" ? i => libraryName(i.library) : sortKey === "name" ? i => i.name ?? i.title : i => i[sortKey];
        return Catalog.sortBy(list, value, i => String(i.title ?? i.name ?? ""), options.descending === true);
    }

    // A file URL for an item's cover (album, track, artist, playlist entry), fetched on first use.
    function coverUrl(item) {
        const k = _coverKey(item);
        return k === "" ? "" : covers[k] ?? "";
    }

    // The cover's dominant color, "#rrggbb", or "" until it loads.
    // An artist's image for a tile or row, "" until it arrives; asked once per artist when a view shows it.
    function artistImageUrl(artist) {
        const id = artist?.id ?? "";
        if (id.indexOf(":") < 0)
            return "";
        const url = artistImages[id];
        if (url === undefined && !_artistImageAsked[id]) {
            _artistImageAsked[id] = true;
            _artistImageQueue.push(artist);
            // asked from bindings: the request goes out later, so the binding doesn't depend on the bridge
            Qt.callLater(root._requestArtistImages);
        }
        return url ?? "";
    }

    function coverColor(item) {
        const k = _coverKey(item);
        return k === "" ? "" : coverColors[k] ?? "";
    }

    // Favorites and ratings, synced to the server for server libraries. kind: "track" (ref {library, id}) |
    // "album" | "artist" (the item). The heart changes at once and goes back if the daemon refuses.
    function setFavorite(kind, ref, value) {
        const mark = {
            "library": ref.library,
            "kind": kind,
            "id": kind === "track" ? ref.id : ref.rawId
        };
        _applyMark(Object.assign({
            "favorite": value
        }, mark));
        DaemonBridge.request("marks.setFavorite", Object.assign({
            "favorite": value
        }, mark), I18n.tr("Couldn't save the favorite", "toast when setting or clearing a favorite fails"), null, () => root._applyMark(Object.assign({
                "favorite": !value
            }, mark)));
    }

    function setRating(kind, ref, stars) {
        DaemonBridge.request("marks.setRating", {
            "library": ref.library,
            "kind": kind,
            "id": kind === "track" ? ref.id : ref.rawId,
            "rating": Math.max(0, Math.min(5, stars))
        }, I18n.tr("Couldn't save the rating", "toast when setting a star rating fails"));
    }

    // Search: every enabled, reachable library; each library's results show as they come in.

    function search(text) {
        searchQuery = text;
        searchDelay.restart();
        // the slowest library mustn't keep results for the old text
        if (text.trim().length < PlayerMetrics.searchMinLength)
            searchResults = _emptyResults();
    }

    function _emptyResults() {
        return {
            "albums": [],
            "artists": [],
            "tracks": []
        };
    }

    function _runSearch() {
        const text = searchQuery;
        const q = text.trim();
        // servers match nothing on one letter
        if (q.length < PlayerMetrics.searchMinLength)
            return;
        searchResults = _emptyResults();
        for (const l of libraries) {
            if (!l.enabled || l.available === false || !hasCapability(l.id, "browse"))
                continue;
            const lib = l.id;
            DaemonBridge.call("browse.search", {
                "library": lib,
                "query": q,
                "limit": PlayerMetrics.searchLimit
            }, result => {
                if (root.searchQuery !== text)
                    return;
                if (result.error) {
                    root.log.warn("search failed in", lib, result.error);
                    return;
                }
                const r = root.searchResults;
                root.searchResults = {
                    "albums": r.albums.concat((result.albums ?? []).map(a => Catalog.album(a))),
                    "artists": r.artists.concat((result.artists ?? []).map(a => Catalog.artist(a))),
                    "tracks": r.tracks.concat((result.tracks ?? []).map(t => Catalog.track(t)))
                };
            });
        }
    }

    // Libraries

    function addLocalLibrary(name, folders) {
        DaemonBridge.request("libraries.add", {
            "kind": "local",
            "name": name,
            "folders": folders
        }, I18n.tr("Couldn't add the library", "toast when the daemon refuses a new local library"), lib => DaemonBridge.request("libraries.scan", {
                "library": lib.id
            }));
    }

    // The password goes to the keyring in the daemon; it never lands in libraries or settings.
    function testServer(kind, url, user, password) {
        DaemonBridge.call("libraries.test", {
            "kind": kind,
            "url": url,
            "username": user,
            "password": password
        }, info => root.serverTestFinished(!info.error, info.error ? Phrases.reason(info.code, info.error) : ""));
    }

    function addServerLibrary(kind, name, url, user, password) {
        DaemonBridge.request("libraries.add", {
            "kind": kind,
            "name": name || url.replace(/^https?:\/\//, ""),
            "url": url,
            "username": user,
            "password": password
        }, I18n.tr("Couldn't add the server", "toast when the daemon refuses a new server library"));
    }

    // patch: name, folders (local), url, user, password (servers; empty keeps the saved one)
    function updateLibrary(id, patch) {
        const params = {
            "library": id,
            "name": patch.name
        };
        if (patch.folders !== undefined)
            params.folders = patch.folders;
        if (patch.url !== undefined)
            params.url = patch.url;
        if (patch.user !== undefined)
            params.username = patch.user;
        if (patch.password)
            params.password = patch.password;
        DaemonBridge.request("libraries.update", params, I18n.tr("Couldn't save the library", "toast when editing a library's name, folders or server fails"), () => {
            // a rename or a new url doesn't change what the daemon's libraries event compares
            root.refreshLibraries(false);
            if (patch.folders !== undefined)
                root.rescanLibrary(id);
            else
                root._loadLibrary(id);
        });
    }

    function setLibraryEnabled(id, enabled) {
        DaemonBridge.request("libraries.setEnabled", {
            "library": id,
            "enabled": enabled
        }, I18n.tr("Couldn't change the library", "toast when turning a library on or off fails"));
    }

    function retryLibrary(id) {
        DaemonBridge.request("libraries.retry", {
            "library": id
        }, I18n.tr("Couldn't reach %1", "toast when retrying an unavailable library fails, %1 is the library").arg(libraryName(id)), info => {
            if (info.available === false)
                DaemonBridge.notice(I18n.tr("%1 is still unavailable", "snackbar after a retry, %1 is the library").arg(root.libraryName(id)));
            root.refreshLibraries(false);
        });
    }

    // Opening an unavailable library's page checks it again, at most every 10 s, quietly: the page shows the state.
    function openedLibrary(id) {
        const lib = library(id);
        const now = Date.now();
        if (!lib || !lib.enabled || lib.available !== false || now - (_openRetries[id] ?? 0) < 10000)
            return;
        _openRetries[id] = now;
        DaemonBridge.request("libraries.retry", {
            "library": id
        }, null, () => root.refreshLibraries(false));
    }

    // Local libraries rescan their folders; servers are fetched again.
    function rescanLibrary(id) {
        if (!hasCapability(id, "scan")) {
            _loadLibrary(id);
            return;
        }
        _setScanning(id, true);
        DaemonBridge.request("libraries.scan", {
            "library": id
        }, I18n.tr("Couldn't start the scan", "toast when a library scan can't start"), null, () => root._setScanning(id, false));
    }

    function removeLibrary(id) {
        DaemonBridge.request("libraries.remove", {
            "library": id
        }, I18n.tr("Couldn't remove the library", "toast when removing a library fails"));
    }

    // every track of an album into the queue, at the end or next
    function enqueueAlbum(id, next) {
        DaemonBridge.request("queue.add", {
            "album": id,
            "next": next === true
        }, I18n.tr("Couldn't add to the queue", "toast when adding tracks to the play queue fails"));
    }

    // then(tracks) with the album's tracks in order, fetched when they aren't all here yet
    function withAlbumTracks(id, then) {
        const a = album(id);
        if (!a)
            return;
        if ((tracksByAlbum[id] ?? []).length >= a.trackCount) {
            then(albumTracks(id));
            return;
        }
        DaemonBridge.request("browse.album", {
            "library": a.library,
            "id": a.rawId
        }, I18n.tr("Couldn't load %1", "toast when a library's albums or tracks can't be loaded, %1 is the library").arg(libraryName(a.library)), result => {
            const list = (result.tracks ?? []).map(t => Catalog.track(t));
            root._ingest(list);
            root._publishTracks();
            then(list);
        });
    }

    // Everything the daemon can tell about a track or album, for the details dialog; then({}) when it can't.
    function details(kind, item, then) {
        DaemonBridge.call("browse.details", {
            "library": item.library,
            "kind": kind,
            "id": kind === "track" ? item.id : item.rawId
        }, result => {
            if (result.error)
                root.log.info("details", item.library, result.error);
            then(result.error ? {} : result);
        });
    }

    function loadAlbum(id) {
        const a = album(id);
        if (!a || _albumLoads[id] || _paging[a.library]?.done)
            return;
        _albumLoads[id] = true;
        DaemonBridge.call("browse.album", {
            "library": a.library,
            "id": a.rawId
        }, result => {
            delete root._albumLoads[id];
            if (result.error) {
                root.log.warn("album", id, result.error);
                return;
            }
            root._ingest((result.tracks ?? []).map(t => Catalog.track(t)));
            publishDelay.restart();
        });
    }

    // Artist pages. Ids are "library:artistId". Each then() runs once; a failed lookup hands over the empty
    // value, since the page shows whatever it has.

    function _splitId(id) {
        const at = id.indexOf(":");
        return {
            "library": id.slice(0, at),
            "id": id.slice(at + 1)
        };
    }

    // browse.artist: the catalog's artist plus biography, biographySource {name, url}, similar artists
    function artistInfo(id, then) {
        DaemonBridge.call("browse.artist", _splitId(id), a => {
            if (a.error) {
                root.log.info("artist", id, a.error);
                then(null);
                return;
            }
            then(Object.assign(Catalog.artist(a), {
                "trackCount": a.trackCount ?? 0,
                "biography": a.biography ?? "",
                "biographySource": a.biographySource ?? null,
                "musicBrainzId": a.musicBrainzId ?? "",
                "similar": (a.similar ?? []).map(s => Catalog.artist(s))
            }));
        });
    }

    // then(fileUrl): the library's image, a local artist.jpg, or one found online; "" when none
    function artistImage(artist, then) {
        DaemonBridge.call("browse.artistImage", {
            "library": artist.library,
            "id": artist.rawId,
            "name": artist.name,
            "coverId": artist.coverId ?? ""
        }, r => then(r.error || !r.path ? "" : "file://" + r.path.split("/").map(encodeURIComponent).join("/")));
    }

    // every album the artist appears on, also as a track artist only
    function artistAlbumList(id, then) {
        DaemonBridge.call("browse.albums", {
            "library": _splitId(id).library,
            "artist": _splitId(id).id
        }, list => then(list.error ? [] : list.map(a => Catalog.album(a)).sort((a, b) => b.year - a.year)));
    }

    // The first page (trackPage) of the artist's tracks; paging only if an artist outgrows it
    function artistTracks(id, then) {
        DaemonBridge.call("browse.tracks", {
            "library": _splitId(id).library,
            "artist": _splitId(id).id,
            "limit": PlayerMetrics.trackPage
        }, r => {
            if (r.error) {
                root.log.info("artist tracks", id, r.error);
                then([]);
                return;
            }
            const list = (r.tracks ?? []).map(t => Catalog.track(t));
            root._ingest(list);
            publishDelay.restart();
            then(list);
        });
    }

    // then([playlist id]): local and server playlists holding the artist's tracks, most matches first
    function artistPlaylists(artist, then) {
        DaemonBridge.call("browse.artistPlaylists", {
            "library": artist.library,
            "id": artist.rawId,
            "name": artist.name
        }, list => then(list.error ? [] : list.slice().sort((a, b) => (b.matches ?? 0) - (a.matches ?? 0)).map(p => (p.library ?? "") + ":" + p.id)));
    }

    // Daemon link

    // the index behind tracks, filled page by page and published in one go: key -> track, key -> position
    property var _trackIndex: ({})
    property var _trackPos: ({})
    property var _trackList: []
    // "library:id" -> {favorite?, rating?} from marks events since that track was loaded
    property var _marks: ({})
    // library id -> when opening its page last retried it
    property var _openRetries: ({})
    property var _coverPending: ({})
    property var _coverInbox: ({})
    // "library:coverId" -> {library, id} whose lookup failed; asked again when the library is back
    property var _coverFailed: ({})
    // per library, so a reply from an older load is dropped
    property var _loadGeneration: ({})
    // track paging per library: {offset, seen, generation, done}
    property var _paging: ({})
    property var _albumLoads: ({})
    // libraries with an indexChanged waiting for the watcher to settle
    property var _indexChanged: ({})
    property bool _allTracksWanted: false

    function _enabledSet() {
        const on = {};
        for (const l of libraries) {
            if (l.enabled)
                on[l.id] = true;
        }
        return on;
    }

    property var _artistImageAsked: ({})
    property var _artistImageQueue: []
    property var _artistImageInbox: ({})

    function _requestArtistImages() {
        const queue = _artistImageQueue;
        _artistImageQueue = [];
        for (const a of queue)
            artistImage(a, url => {
                root._artistImageInbox[a.id] = url;
                Qt.callLater(root._publishArtistImages);
            });
    }

    // answers arrive one by one; the map is replaced once per batch so tiles don't re-bind per answer
    function _publishArtistImages() {
        const inbox = _artistImageInbox;
        if (Object.keys(inbox).length === 0)
            return;
        _artistImageInbox = {};
        artistImages = Object.assign({}, artistImages, inbox);
    }

    function _coverKey(item) {
        if (!item?.coverId)
            return "";
        const k = item.library + ":" + item.coverId;
        if (covers[k] === undefined && !_coverPending[k] && !_coverFailed[k] && hasCapability(item.library, "cover")) {
            // asked from bindings: the request goes out later, so the binding doesn't depend on the bridge
            _coverPending[k] = {
                "library": item.library,
                "id": item.coverId
            };
            Qt.callLater(root._requestCovers);
        }
        return k;
    }

    function _requestCovers() {
        for (const k in _coverPending) {
            const want = _coverPending[k];
            if (want === true)
                continue;
            _coverPending[k] = true;
            DaemonBridge.call("browse.cover", {
                "library": want.library,
                "id": want.id,
                "size": PlayerMetrics.coverFetchSize
            }, result => {
                delete root._coverPending[k];
                if (result.error) {
                    root._coverFailed[k] = want;
                    return;
                }
                root._coverInbox[k] = {
                    "url": result.path ? "file://" + result.path.split("/").map(encodeURIComponent).join("/") : "",
                    "color": result.color ?? ""
                };
                coverFlush.restart();
            });
        }
    }

    function _retryCovers(libraryId) {
        let any = false;
        for (const k in _coverFailed) {
            if (_coverFailed[k].library !== libraryId)
                continue;
            _coverPending[k] = _coverFailed[k];
            delete _coverFailed[k];
            any = true;
        }
        if (any)
            _requestCovers();
    }

    function _setScanning(id, on) {
        if ((library(id)?.scanning ?? false) !== on)
            _patchLibrary(id, {
                "scanning": on
            });
        if (!on && scanProgress[id] !== undefined) {
            const next = Object.assign({}, scanProgress);
            delete next[id];
            scanProgress = next;
        }
    }

    function _applyMark(m) {
        const patch = {};
        if (m.favorite !== undefined)
            patch.favorite = m.favorite;
        if (m.rating !== undefined)
            patch.rating = m.rating;
        switch (m.kind) {
        case "track":
            {
                const k = m.library + ":" + m.id;
                _marks[k] = Object.assign({}, _marks[k], patch);
                revision++;
                if (m.favorite !== undefined)
                    Qt.callLater(root._fetchFavorites);
                return;
            }
        case "album":
        case "artist":
            {
                if (m.kind === "artist") {
                    const k = "artist|" + m.library + ":" + m.id;
                    _marks[k] = Object.assign({}, _marks[k], patch);
                    revision++;
                }
                const hit = a => a.library === m.library && a.rawId === m.id ? Object.assign({}, a, patch) : a;
                if (m.kind === "album")
                    albums = albums.map(hit);
                else
                    artists = artists.map(hit);
                const list = m.kind + "s";
                if (searchResults[list].some(a => a.library === m.library && a.rawId === m.id))
                    searchResults = Object.assign({}, searchResults, {
                        [list]: searchResults[list].map(hit)
                    });
                return;
            }
        }
    }

    function _onLibrariesEvent(e) {
        switch (e.type) {
        case "changed":
            refreshLibraries(false);
            return;
        case "indexChanged":
            // the folder watcher sends one every few seconds while files change; reload once it settles
            _indexChanged[e.library] = true;
            indexSettle.restart();
            return;
        case "scanProgress":
            {
                _setScanning(e.library, true);
                const next = Object.assign({}, scanProgress);
                next[e.library] = {
                    "done": e.done ?? 0,
                    "total": e.total ?? 0
                };
                scanProgress = next;
                return;
            }
        case "scanDone":
            _setScanning(e.library, false);
            _patchLibrary(e.library, {
                "lastScan": Date.now() / 1000,
                "error": "",
                "errorCode": ""
            });
            _loadLibrary(e.library);
            return;
        case "scanFailed":
            _setScanning(e.library, false);
            _patchLibrary(e.library, {
                "error": e.error ?? "",
                "errorCode": e.code ?? ""
            });
            DaemonBridge.notice(I18n.tr("Couldn't scan %1", "toast when a library scan fails, %1 is the library").arg(libraryName(e.library)));
            return;
        }
    }

    function _refreshAll() {
        DaemonBridge.request("libraries.kinds", null, null, kinds => root.libraryKinds = kinds);
        // after a drop, events may have been missed: reload what is there
        refreshLibraries(true);
        _fetchFavorites();
    }

    function _fetchFavorites() {
        DaemonBridge.request("marks.favorites", null, null, r => root.favoriteTrackList = (r.tracks ?? []).map(t => Catalog.track(t)));
    }

    // only libraries that are unreachable need to know which tracks still play from the cache
    function _refreshCachedKeys() {
        const down = libraries.filter(l => l.enabled && l.available === false && isRemote(l.id));
        if (down.length === 0) {
            if (Object.keys(cachedKeys).length > 0)
                cachedKeys = {};
            return;
        }
        const keys = {};
        let pending = down.length;
        for (const l of down)
            DaemonBridge.call("cache.list", {
                "library": l.id
            }, list => {
                if (!list.error) {
                    for (const c of list)
                        keys[root.key(c.track)] = true;
                }
                if (--pending === 0)
                    root.cachedKeys = keys;
            });
    }

    // Loads the catalog of libraries that are new, just turned on or reachable again (every ready one with
    // reloadAll), and drops what went away.
    function refreshLibraries(reloadAll) {
        DaemonBridge.request("libraries.list", null, null, list => {
            const ready = l => l.enabled && l.available !== false;
            const before = root.libraries.filter(ready).map(l => l.id);
            const wasEnabled = root.libraries.filter(l => l.enabled).map(l => l.id);
            root.libraries = list.map(l => Catalog.library(l, reloadAll ? null : root.library(l.id)));
            root.librariesLoaded = true;
            const now = root.libraries.filter(ready).map(l => l.id);
            for (const id of wasEnabled) {
                if (!root.enabledLibrary(id))
                    root._dropLibrary(id);
            }
            for (const id of now) {
                if (reloadAll || before.indexOf(id) < 0) {
                    root._loadLibrary(id);
                    root._retryCovers(id);
                }
            }
            if (reloadAll)
                root.scanProgress = {};
            root._refreshCachedKeys();
        });
    }

    function _loadLibrary(id) {
        const generation = (_loadGeneration[id] ?? 0) + 1;
        _loadGeneration[id] = generation;
        const stale = () => root._loadGeneration[id] !== generation;
        libraryLoaded(id);
        if (!hasCapability(id, "browse"))
            return;
        const failText = I18n.tr("Couldn't load %1", "toast when a library's albums or tracks can't be loaded, %1 is the library").arg(libraryName(id));
        DaemonBridge.request("browse.artists", {
            "library": id
        }, failText, artistList => DaemonBridge.request("browse.albums", {
                "library": id
            }, failText, albumList => {
                if (stale())
                    return;
                const libAlbums = albumList.map(a => Catalog.album(a));
                const own = Catalog.groupBy(libAlbums, a => a.artistId);
                root.artists = root.artists.filter(a => a.library !== id).concat(artistList.map(a => Catalog.artist(a, own[a.library + ":" + a.id])));
                root.albums = root.albums.filter(a => a.library !== id).concat(libAlbums);
            }));
        if (_paging[id] !== undefined || _allTracksWanted)
            _pageTracks(id);
    }

    // The first full track list shown pages every library in once (500 per call) and keeps it;
    // the UI sorts and filters locally. Paging by scroll position would need server-side sorts across libraries.
    function _wantAllTracks() {
        if (_allTracksWanted)
            return;
        _allTracksWanted = true;
        for (const l of libraries) {
            if (l.enabled && l.available !== false && hasCapability(l.id, "browse"))
                _pageTracks(l.id);
        }
    }

    // Pages into the index quietly and publishes twice: after the first page, so lists fill at once, and
    // when the library is complete. Each publish re-sorts whatever shows the tracks.
    function _pageTracks(id) {
        const state = {
            "offset": 0,
            "seen": {},
            "generation": (_paging[id]?.generation ?? 0) + 1,
            "done": false
        };
        _paging[id] = state;
        tracksLoading = true;
        const nextPage = () => {
            DaemonBridge.call("browse.tracks", {
                "library": id,
                "offset": state.offset,
                "limit": PlayerMetrics.trackPage
            }, result => {
                if (root._paging[id] !== state)
                    return;
                const page = (result.tracks ?? []).map(t => Catalog.track(t));
                for (const t of page)
                    state.seen[t.id] = true;
                root._ingest(page);
                if (state.offset === 0)
                    root._publishTracks();
                state.offset += page.length;
                if (!result.error && page.length > 0 && state.offset < (result.total ?? 0)) {
                    nextPage();
                    return;
                }
                if (result.error) {
                    root.log.warn("browse.tracks", id, result.error);
                    DaemonBridge.notice(I18n.tr("Couldn't load %1", "toast when a library's albums or tracks can't be loaded, %1 is the library").arg(root.libraryName(id)));
                } else
                    // a full pass drops tracks the library no longer has
                    root._forget(t => t.library === id && !state.seen[t.id]);
                state.done = true;
                root._publishTracks();
                root._updateTracksLoading();
            });
        };
        nextPage();
    }

    function _updateTracksLoading() {
        tracksLoading = Object.keys(_paging).some(l => !_paging[l].done);
    }

    // new and refreshed tracks go into the index; they show with the next _publishTracks
    function _ingest(list) {
        for (const t of list) {
            const k = key(t);
            const at = _trackPos[k];
            if (at === undefined) {
                _trackPos[k] = _trackList.length;
                _trackList.push(t);
            } else
                _trackList[at] = t;
            _trackIndex[k] = t;
            delete _marks[k];
        }
    }

    function _forget(gone) {
        const kept = [];
        const pos = {};
        for (const t of _trackList) {
            const k = key(t);
            if (gone(t))
                delete _trackIndex[k];
            else {
                pos[k] = kept.length;
                kept.push(t);
            }
        }
        _trackList = kept;
        _trackPos = pos;
    }

    function _publishTracks() {
        tracks = _trackList.slice();
        revision++;
    }

    function _dropLibrary(id) {
        _loadGeneration[id] = (_loadGeneration[id] ?? 0) + 1;
        artists = artists.filter(a => a.library !== id);
        albums = albums.filter(a => a.library !== id);
        _forget(t => t.library === id);
        _publishTracks();
        delete _paging[id];
        _updateTracksLoading();
        libraryDropped(id);
    }

    function _patchLibrary(id, patch) {
        libraries = libraries.map(l => l.id === id ? Object.assign({}, l, patch) : l);
    }

    Component.onCompleted: {
        DaemonBridge.subscribeTopic("libraries", e => root._onLibrariesEvent(e));
        DaemonBridge.subscribeTopic("marks", m => root._applyMark(m));
        DaemonBridge.subscribeTopic("cache", () => root._refreshCachedKeys());
        DaemonBridge.subscribeTopic("artistPlaylists", e => root.artistPlaylistsChanged(e.library ?? ""));
        if (DaemonBridge.subscribed)
            _refreshAll();
    }

    Connections {
        target: DaemonBridge

        function onResubscribed() {
            root._refreshAll();
        }
    }

    // typing sends one search per pause, not one per key
    Timer {
        id: searchDelay
        interval: PlayerMetrics.searchDelay
        onTriggered: root._runSearch()
    }

    // album loads come in bursts (an artist page); one publish per burst
    Timer {
        id: publishDelay
        interval: PlayerMetrics.coverFlushDelay
        onTriggered: root._publishTracks()
    }

    Timer {
        id: indexSettle
        interval: PlayerMetrics.indexSettleDelay
        onTriggered: {
            const ids = Object.keys(root._indexChanged);
            root._indexChanged = {};
            for (const id of ids)
                root._loadLibrary(id);
        }
    }

    // cover replies come in bursts; one map update per burst
    Timer {
        id: coverFlush
        interval: PlayerMetrics.coverFlushDelay
        onTriggered: {
            const urls = Object.assign({}, root.covers);
            const colors = Object.assign({}, root.coverColors);
            for (const k in root._coverInbox) {
                urls[k] = root._coverInbox[k].url;
                colors[k] = root._coverInbox[k].color;
            }
            root._coverInbox = {};
            root.covers = urls;
            root.coverColors = colors;
        }
    }
}
