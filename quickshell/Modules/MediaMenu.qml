import QtQuick
import qs.Common
import qs.Services
import qs.Widgets

// The actions of an album, artist, playlist or genre wherever one shows (tiles, rows, its own page), as set up for
// its type: the ⋮ menu, the right-click menu and the hover buttons. A page adds its own entries with `extra` and
// handles them in `extraTriggered`.
Item {
    id: root

    // "album" | "artist" | "playlist" | "genre"
    property string kind: "album"
    property var item: null
    property var extra: []
    // "menu" (the ⋮ button, the Menu key) | "context" (right-click)
    property string zone: "menu"

    readonly property string type: kind
    readonly property bool opened: menu.opened || artistMenu.opened
    readonly property var hoverEntries: item ? entries("hover").filter(e => !e.divider) : []
    readonly property bool favorite: (kind === "artist" ? CatalogService.withArtistMarks(item) : item)?.favorite === true
    readonly property var linkedArtists: (item?.artists ?? []).filter(a => a.id !== "")
    readonly property bool server: kind === "playlist" && item?.kind === "server"
    readonly property bool onHome: (UiConfig.surface("home")?.zones.shelves ?? []).some(s => s.type === kind && s.options.source === (item?.id ?? ""))
    readonly property bool pinned: kind !== "album" && UiConfig.isPinned(kind, item?.id ?? "")

    property Item _anchor: null
    property real _x: 0
    property real _y: 0
    property bool _atPointer: false

    signal extraTriggered(string id)

    // without x, y: the ⋮ menu under anchor; with them: the right-click menu at that point
    function open(anchor, x, y) {
        _anchor = anchor;
        _atPointer = x !== undefined;
        _x = x ?? 0;
        _y = y ?? 0;
        zone = _atPointer ? "context" : "menu";
        if (_atPointer)
            menu.open(anchor, x, y);
        else
            menu.open(anchor);
    }

    // a page's own entry (extra)
    function entry(id, text, icon, blocker) {
        return {
            "id": id,
            "text": text,
            "icon": icon,
            "enabled": (blocker ?? "") === "",
            "description": blocker ?? ""
        };
    }

    // null: the action doesn't apply to this kind (or this item)
    function action(id) {
        const plain = entry(id, UiConfig.actionLabel(kind, id), UiConfig.actionIcon(kind, id));
        switch (id) {
        case "play":
            return kind !== "genre" ? plain : null;
        case "shuffle":
            return plain;
        case "playNext":
        case "addToQueue":
            return kind === "album" || kind === "playlist" ? plain : null;
        case "addToPlaylist":
        case "goToArtist":
            return kind === "album" ? plain : null;
        case "details":
            return kind === "album" || kind === "artist" ? plain : null;
        case "favorite":
            if (kind !== "album" && kind !== "artist")
                return null;
            return Object.assign(entry(id, favorite ? I18n.tr("Remove from favorites", "track menu action") : I18n.tr("Add to favorites", "track menu action"), "favorite", PlayerService.whyNot("favorite", item)), {
                "iconFilled": !favorite
            });
        case "keepOffline":
            {
                const keep = SettingsService.keepState(kind, item);
                if (kind === "genre" || !keep.shown)
                    return null;
                return Object.assign(entry(id, keep.kept ? I18n.tr("Remove from this device", "menu action that stops keeping an item offline") : plain.text, plain.icon, keep.blocker), {
                    "iconFilled": !keep.kept
                });
            }
        case "pin":
            if (kind === "album")
                return null;
            return Object.assign(entry(id, pinned ? I18n.tr("Unpin from navigation", "pin button") : plain.text, plain.icon), {
                "iconFilled": !pinned
            });
        case "addToHome":
            if (kind !== "playlist" && kind !== "genre")
                return null;
            return entry(id, plain.text, plain.icon, onHome ? I18n.tr("Already on home", "an item that is already a home shelf") : "");
        case "rename":
            return kind === "playlist" ? entry(id, plain.text, plain.icon, PlayerService.whyNot("playlistEdit", item)) : null;
        case "duplicate":
            return kind === "playlist" ? plain : null;
        case "export":
            return kind === "playlist" ? entry(id, plain.text, plain.icon, server ? I18n.tr("Only local playlists can be exported", "why export is disabled for a server playlist") : "") : null;
        case "delete":
            return kind === "playlist" ? entry(id, plain.text, plain.icon, PlayerService.whyNot("playlistDelete", item)) : null;
        }
        return null;
    }

    // A zone's entries in order, dividers collapsed
    function entries(zone) {
        const out = [];
        for (const i of UiConfig.item(kind)?.zones[zone] ?? []) {
            if (i.hidden || !UiConfig.isKnown(i.type))
                continue;
            if (i.type === "divider") {
                if (out.length > 0 && !out[out.length - 1].divider)
                    out.push({
                        "divider": true
                    });
                continue;
            }
            const e = action(i.type);
            if (e)
                out.push(e);
        }
        while (out.length > 0 && out[out.length - 1].divider)
            out.pop();
        return out;
    }

    // anchor: the hover button the action ran from, for a menu that follows (several artists)
    function run(id, anchor) {
        const it = item;
        switch (id) {
        case "play":
        case "shuffle":
            if (kind === "album")
                PlayerService.playAlbum(it.id, id === "shuffle");
            else if (kind === "artist")
                PlayerService.playArtist(it, id === "shuffle");
            else if (kind === "genre")
                // Shuffles the genre's tracks loaded so far; a daemon-side genre shuffle when libraries get big
                PlayerService.shuffleTracks(CatalogService.query("tracks", {
                    "filters": {
                        "genre": it.id
                    }
                }), {
                    "type": "genre",
                    "id": it.id
                }, false);
            else
                PlayerService.playPlaylist(it.id, id === "shuffle");
            return;
        case "playNext":
        case "addToQueue":
            if (kind === "album")
                CatalogService.enqueueAlbum(it.id, id === "playNext");
            else
                PlaylistService.enqueuePlaylist(it.id, id === "playNext");
            return;
        case "addToPlaylist":
            CatalogService.withAlbumTracks(it.id, tracks => Router.pickPlaylist(tracks));
            return;
        case "favorite":
            PlayerService.setFavorite(kind, it, !root.favorite);
            return;
        case "goToArtist":
            if (root.linkedArtists.length > 1)
                Qt.callLater(() => anchor ? artistMenu.open(anchor) : root._atPointer ? artistMenu.open(root._anchor, root._x, root._y) : artistMenu.open(root._anchor));
            else
                Router.go("artist", root.linkedArtists[0]?.id ?? it.artistId);
            return;
        case "details":
            Router.showDetails(kind, it);
            return;
        case "keepOffline":
            SettingsService.setKept(kind, it, !SettingsService.keepState(kind, it).kept);
            return;
        case "rename":
            Router.prompt(I18n.tr("Rename playlist", "playlist menu action"), it.name, I18n.tr("Save", "dialog confirm button"), name => PlayerService.renamePlaylist(it.id, name));
            return;
        case "duplicate":
            PlayerService.duplicatePlaylist(it.id, copy => Router.go("playlist", copy));
            return;
        case "export":
            Router.prompt(I18n.tr("Export playlist", "playlist menu action"), "~/" + it.name.replace(/\//g, "-") + ".m3u8", I18n.tr("Export", "custom theme action"), path => PlayerService.exportPlaylist(it.id, Paths.expandTilde(path)), I18n.tr("File (.m3u8 or .pls)", "playlist export path field"));
            return;
        case "addToHome":
            {
                const home = UiConfig.surface("home");
                UiConfig.setSurface("home", {
                    "zones": {
                        "shelves": home.zones.shelves.concat([
                            {
                                "type": kind,
                                "options": {
                                    "source": it.id
                                }
                            }
                        ])
                    }
                });
                return;
            }
        case "pin":
            if (pinned)
                UiConfig.unpin(kind, it.id);
            else
                UiConfig.pin(kind, it.id, it.name);
            return;
        case "delete":
            Router.deletePlaylist(it.id);
            return;
        }
        extraTriggered(id);
    }

    DMenu {
        id: menu
        model: {
            if (!root.item)
                return [];
            const list = root.entries(root.zone);
            if (root.extra.length > 0)
                return list.concat(list.length > 0 ? [
                    {
                        "divider": true
                    }
                ] : []).concat(root.extra);
            return list.length > 0 ? list : [
                {
                    "id": "",
                    "text": I18n.tr("No actions. Use Edit page to add some.", "an item menu with every action removed"),
                    "enabled": false
                }
            ];
        }
        onTriggered: id => root.run(id)
    }

    DMenu {
        id: artistMenu
        model: root.linkedArtists.map(a => ({
                    "id": a.id,
                    "text": a.name,
                    "icon": "artist"
                }))
        onTriggered: id => Router.go("artist", id)
    }
}
