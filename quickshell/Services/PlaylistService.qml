pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import "Catalog.js" as Catalog

// Server and local playlists. Ids are "library:playlistId", local ones ":playlistId"; their tracks load on
// first open.
Singleton {
    id: root

    readonly property var log: Log.scoped("PlaylistService")

    property var playlists: []
    // id -> true while a delete waits for its undo time; such a playlist is gone everywhere but here
    property var hiddenIds: ({})

    function playlist(id) {
        return hiddenIds[id] ? null : _find(id);
    }

    function _find(id) {
        return playlists.find(p => p.id === id) ?? null;
    }

    function playlistTracks(id) {
        return (playlist(id)?.items ?? []).map(ref => CatalogService.track(ref)).filter(t => t !== null);
    }

    // the playlists to browse, with counts and libraries from their loaded tracks; filter and sort with
    // CatalogService.refine
    function listed(libraryId) {
        let list = playlists.filter(p => !hiddenIds[p.id] && (p.kind === "local" || CatalogService.enabledLibrary(p.library))).map(p => {
            const items = playlistTracks(p.id);
            return Object.assign({}, p, {
                "trackCount": p.loaded === false ? p.trackCount : items.length,
                "duration": p.loaded === false ? p.duration : items.reduce((sum, t) => sum + t.duration, 0),
                "libraries": p.kind === "server" ? [p.library] : items.map(t => t.library).filter((id, i, all) => all.indexOf(id) === i)
            });
        });
        if (libraryId)
            list = list.filter(p => p.library === libraryId || (p.kind === "local" && p.libraries.indexOf(libraryId) >= 0));
        return list;
    }

    // done(id) runs once the playlist exists; libraryId "" makes a local playlist. tracks: refs to start with
    // (a server playlist takes its own server's)
    function createPlaylist(name, libraryId, done, tracks) {
        const created = p => {
            // the playlists event may have listed it already
            root.playlists = root.playlists.filter(o => o.id !== p.id).concat([p]);
            done?.(p.id);
        };
        if (!libraryId) {
            DaemonBridge.request("localPlaylists.create", {
                "name": name,
                "tracks": Catalog.refsOf(tracks ?? [])
            }, I18n.tr("Couldn't create the playlist", "toast when making a new playlist fails"), result => created(Catalog.localPlaylist(result, null)));
            return;
        }
        DaemonBridge.request("playlists.create", {
            "library": libraryId,
            "name": name,
            "tracks": (tracks ?? []).filter(t => t.library === libraryId).map(t => t.id)
        }, I18n.tr("Couldn't create the playlist", "toast when making a new playlist fails"), result => created(Catalog.playlist(result, null)));
    }

    // Adds what fits (a server playlist takes only its server's tracks) and returns how many were skipped.
    function addToPlaylist(id, refs) {
        const target = playlist(id);
        if (!target)
            return 0;
        const parts = CatalogService.partition("playlistAdd", refs.map(r => CatalogService.track(r) ?? r), target);
        if (parts.ok.length > 0) {
            if (target.kind === "local")
                _editLocal(target, "localPlaylists.add", {
                    "tracks": Catalog.refsOf(parts.ok)
                });
            else
                _updatePlaylist(target, {
                    "add": parts.ok.map(t => t.id)
                });
        }
        return parts.skipped;
    }

    function renamePlaylist(id, name) {
        const p = playlist(id);
        if (p?.kind === "local")
            _editLocal(p, "localPlaylists.rename", {
                "name": name
            });
        else
            _updatePlaylist(p, {
                "name": name
            });
    }

    // cover: {type: "auto"} (mosaic of the first albums) | {type: "art", library, id} | {type: "shape", accent, shape}
    function setPlaylistCover(id, cover) {
        const p = playlist(id);
        if (!CatalogService.allows("playlistCover", p))
            return;
        _patchPlaylist(id, {
            "cover": cover
        });
        _editLocal(p, "localPlaylists.setCover", {
            "cover": cover
        }, () => root._patchPlaylist(id, {
                "cover": p.cover
            }));
    }

    // order: the new order as old indexes, from a drag that moved one track
    function reorderPlaylist(id, order) {
        const p = playlist(id);
        const move = Catalog.singleMove(order);
        if (!CatalogService.allows("playlistReorder", p) || !move)
            return;
        if (p.kind === "local") {
            _editLocal(p, "localPlaylists.move", move);
            return;
        }
        DaemonBridge.request("playlists.move", {
            "library": p.library,
            "id": p.rawId,
            "from": move.from,
            "to": move.to
        }, I18n.tr("Couldn't change the playlist", "toast when renaming, adding to, removing from or reordering a playlist fails"));
    }

    // A copy is always local, so it can take tracks from any library. done(id) runs with the copy's id.
    function duplicatePlaylist(id, done) {
        const p = playlist(id);
        if (!p)
            return;
        const params = {
            "id": p.rawId,
            "name": I18n.tr("%1 (copy)", "name of a duplicated playlist").arg(p.name)
        };
        if (p.kind === "server")
            params.library = p.library;
        DaemonBridge.request("localPlaylists.duplicate", params, I18n.tr("Couldn't duplicate the playlist", "toast when copying a playlist fails"), result => {
            const copy = Catalog.localPlaylist(result, null);
            root.playlists = root.playlists.filter(o => o.id !== copy.id).concat([copy]);
            done?.(copy.id);
        });
    }

    function removeFromPlaylist(id, indexes) {
        const p = playlist(id);
        if (p?.kind === "local")
            _editLocal(p, "localPlaylists.remove", {
                "positions": indexes
            });
        else
            _updatePlaylist(p, {
                "remove": indexes
            });
    }

    // finished() runs once the daemon answered, either way
    function deletePlaylist(id, finished) {
        const p = _find(id);
        if (!p || !CatalogService.allows("playlistDelete", p)) {
            setHidden(id, false);
            finished?.();
            return;
        }
        const done = () => {
            root.playlists = root.playlists.filter(o => o.id !== id);
            root.setHidden(id, false);
            finished?.();
        };
        const failed = () => {
            root.setHidden(id, false);
            finished?.();
        };
        if (p.kind === "local")
            DaemonBridge.request("localPlaylists.delete", {
                "id": p.rawId
            }, I18n.tr("Couldn't delete the playlist", "toast when deleting a playlist fails"), done, failed);
        else
            DaemonBridge.request("playlists.delete", {
                "library": p.library,
                "id": p.rawId
            }, I18n.tr("Couldn't delete the playlist", "toast when deleting a playlist fails"), done, failed);
    }

    // hides a playlist while its delete can still be undone
    function setHidden(id, hidden) {
        if ((hiddenIds[id] === true) === hidden)
            return;
        const next = Object.assign({}, hiddenIds);
        if (hidden)
            next[id] = true;
        else
            delete next[id];
        hiddenIds = next;
    }

    // [{position, track}] for the entries at these positions, to put back after a removal
    function entriesAt(id, positions) {
        const items = playlist(id)?.items ?? [];
        return positions.slice().sort((a, b) => a - b).filter(i => items[i] !== undefined).map(i => ({
                    "position": i,
                    "track": items[i]
                }));
    }

    // Puts removed entries back where they were, lowest position first, each request after the last.
    // lengthAfter: the playlist's length right after the removal (a server appends, then moves into place).
    function restoreEntries(id, entries, lengthAfter) {
        const p = playlist(id);
        if (!p || entries.length === 0)
            return;
        const failText = I18n.tr("Couldn't change the playlist", "toast when renaming, adding to, removing from or reordering a playlist fails");
        if (p.kind === "local") {
            const next = i => {
                if (i < entries.length)
                    DaemonBridge.request("localPlaylists.add", {
                        "id": p.rawId,
                        "tracks": Catalog.refsOf([entries[i].track]),
                        "at": entries[i].position
                    }, failText, () => next(i + 1));
            };
            next(0);
            return;
        }
        const reorder = CatalogService.allows("playlistReorder", p);
        const move = i => {
            if (reorder && i < entries.length)
                DaemonBridge.request("playlists.move", {
                    "library": p.library,
                    "id": p.rawId,
                    "from": lengthAfter + i,
                    "to": entries[i].position
                }, failText, () => move(i + 1));
        };
        DaemonBridge.request("playlists.update", {
            "library": p.library,
            "id": p.rawId,
            "add": entries.map(e => e.track.id)
        }, failText, () => move(0));
    }

    // every track of a playlist into the queue, at the end or next
    function enqueuePlaylist(id, next) {
        const p = playlist(id);
        if (!p)
            return;
        const failText = I18n.tr("Couldn't add to the queue", "toast when adding tracks to the play queue fails");
        if (p.kind === "server") {
            DaemonBridge.request("queue.add", {
                "playlist": p.id,
                "next": next === true
            }, failText);
            return;
        }
        loadPlaylist(id, () => {
            const refs = Catalog.refsOf(root.playlist(id)?.items ?? []);
            if (refs.length > 0)
                DaemonBridge.request("queue.add", {
                    "tracks": refs,
                    "next": next === true
                }, failText);
        });
    }

    // path: an absolute .m3u, .m3u8 or .pls file; done(id) with the new playlist
    function importPlaylist(path, done) {
        DaemonBridge.request("localPlaylists.import", {
            "path": path
        }, I18n.tr("Couldn't import the playlist", "toast when reading a playlist file fails"), result => {
            const created = Catalog.localPlaylist(result.playlist, null);
            root.playlists = root.playlists.filter(o => o.id !== created.id).concat([created]);
            if (result.missed > 0)
                DaemonBridge.notice(I18n.tr("Imported. %1 tracks weren't found in your libraries.", "snackbar after a playlist import, %1 is a count").arg(result.missed));
            done?.(created.id);
        });
    }

    // path: an absolute .m3u8 or .pls file to write
    function exportPlaylist(id, path) {
        const p = playlist(id);
        if (p?.kind !== "local")
            return;
        DaemonBridge.request("localPlaylists.export", {
            "id": p.rawId,
            "path": path
        }, I18n.tr("Couldn't export the playlist", "toast when writing a playlist file fails"), result => {
            if (result.skipped > 0)
                DaemonBridge.notice(I18n.tr("Exported %1 tracks. %2 without a local file were skipped.", "snackbar after a playlist export").arg(result.written).arg(result.skipped));
            else
                DaemonBridge.notice(I18n.tr("Exported %1 tracks", "snackbar after a playlist export").arg(result.written));
        });
    }

    // then() runs once the playlist's tracks are in
    function loadPlaylist(id, then) {
        const p = playlist(id);
        if (!p) {
            then?.();
            return;
        }
        const local = p.kind === "local";
        DaemonBridge.call(local ? "localPlaylists.get" : "playlists.get", local ? {
            "id": p.rawId
        } : {
            "library": p.library,
            "id": p.rawId
        }, result => {
            if (result.error) {
                root.log.warn("playlist", id, result.error);
                DaemonBridge.notice(I18n.tr("Couldn't load the playlist", "toast when a playlist's tracks can't be loaded"));
                return;
            }
            const old = root.playlist(id);
            root._patchPlaylist(id, Object.assign(local ? Catalog.localPlaylist(result, old) : Catalog.playlist(result, old), {
                "loaded": true,
                "items": (result.tracks ?? []).map(t => Catalog.track(t))
            }));
            then?.();
        });
    }

    function _listServer(libraryId, then) {
        if (!CatalogService.hasCapability(libraryId, "playlists"))
            return;
        DaemonBridge.request("playlists.list", {
            "library": libraryId
        }, null, list => {
            root.playlists = root.playlists.filter(p => p.library !== libraryId).concat(list.map(p => Catalog.playlist(p, root.playlist(p.library + ":" + p.id))));
            then?.();
        });
    }

    // a server playlist changed, here or on another device
    function _onPlaylistsEvent(e) {
        _listServer(e.library, () => {
            if (root.playlist(e.library + ":" + e.id)?.loaded)
                root.loadPlaylist(e.library + ":" + e.id);
        });
    }

    // changedId: the local playlist an edit touched, reloaded when it is open
    function _refreshLocal(changedId) {
        DaemonBridge.request("localPlaylists.list", null, null, list => {
            const local = list.map(p => Catalog.localPlaylist(p, root.playlist(":" + p.id)));
            root.playlists = root.playlists.filter(p => p.kind !== "local").concat(local);
            for (const p of local) {
                if (p.loaded && (changedId === undefined || p.id === ":" + changedId))
                    root.loadPlaylist(p.id);
            }
        });
    }

    function _patchPlaylist(id, patch) {
        playlists = playlists.map(p => p.id === id ? Object.assign({}, p, patch) : p);
    }

    // the playlists and localPlaylists events reload the playlist afterwards
    function _updatePlaylist(p, params) {
        if (!p)
            return;
        DaemonBridge.request("playlists.update", Object.assign({
            "library": p.library,
            "id": p.rawId
        }, params), I18n.tr("Couldn't change the playlist", "toast when renaming, adding to, removing from or reordering a playlist fails"));
    }

    function _editLocal(p, method, params, failed) {
        if (!p)
            return;
        DaemonBridge.request(method, Object.assign({
            "id": p.rawId
        }, params), I18n.tr("Couldn't change the playlist", "toast when renaming, adding to, removing from or reordering a playlist fails"), null, failed);
    }

    Component.onCompleted: {
        DaemonBridge.subscribeTopic("localPlaylists", e => root._refreshLocal(e.id));
        DaemonBridge.subscribeTopic("playlists", e => root._onPlaylistsEvent(e));
        if (DaemonBridge.subscribed)
            _refreshLocal();
    }

    Connections {
        target: DaemonBridge

        function onResubscribed() {
            root._refreshLocal();
        }
    }

    Connections {
        target: CatalogService

        function onLibraryLoaded(id) {
            root._listServer(id);
        }

        function onLibraryDropped(id) {
            root.playlists = root.playlists.filter(p => p.library !== id);
        }
    }
}
