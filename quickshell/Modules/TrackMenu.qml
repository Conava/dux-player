import QtQuick
import qs.Common
import qs.Services
import qs.Widgets

// A track's actions as set up for its type (Edit page → Rows and menus): the ⋮ menu, the right-click menu and
// the hover buttons. "queue" is a queue entry: the track's actions plus the queue's own.
Item {
    id: root

    // "track" | "queue"
    property string type: "track"
    property var tracks: []
    // set when the tracks sit in a playlist the user may edit
    property string playlistId: ""
    property var playlistIndexes: []
    // queue entries: their places in the queue
    property var queueIndexes: []
    // "menu" (the ⋮ button, the Menu key) | "context" (right-click)
    property string zone: "menu"

    readonly property bool opened: menu.opened || artistMenu.opened
    readonly property var hoverEntries: entries("hover", true).filter(e => !e.divider)
    readonly property var first: tracks[0] ?? null
    // a mixed selection acts on the tracks whose library supports favorites
    readonly property var favorable: PlayerService.partition("favorite", tracks)
    readonly property bool allFavorite: favorable.ok.length > 0 && favorable.ok.every(t => CatalogService.withMarks(t).favorite)
    readonly property string removeBlocker: playlistId !== "" ? PlayerService.whyNot("playlistEdit", PlayerService.playlist(playlistId)) : ""
    readonly property var linkedArtists: (first?.artists ?? []).filter(a => a.id !== "")

    // removing queue entries is the list's job (one undo for many)
    signal queueRemoveRequested

    // without x, y: the ⋮ menu under anchor; with them: the right-click menu at that point
    function open(anchor, x, y) {
        zone = x === undefined ? "menu" : "context";
        if (x === undefined)
            menu.open(anchor);
        else
            menu.open(anchor, x, y);
    }

    // null: the action doesn't apply in this list. inert: it doesn't apply to these tracks (a selection, the
    // current queue entry); hover keeps its place so the rows stay aligned.
    function action(id) {
        const entry = {
            "id": id,
            "text": UiConfig.actionLabel(type, id),
            "icon": UiConfig.actionIcon(type, id)
        };
        switch (id) {
        case "playNext":
        case "addToQueue":
        case "addToPlaylist":
            return entry;
        case "favorite":
            return Object.assign(entry, {
                "text": allFavorite ? I18n.tr("Remove from favorites", "track menu action") : I18n.tr("Add to favorites", "track menu action"),
                "iconFilled": !allFavorite,
                "enabled": favorable.ok.length > 0,
                "description": favorable.ok.length === 0 ? PlayerService.whyNot("favorite", first) : ""
            });
        case "goToAlbum":
        case "goToArtist":
        case "details":
            entry.inert = tracks.length !== 1;
            return entry;
        case "removeFromPlaylist":
            if (playlistId === "")
                return null;
            return Object.assign(entry, {
                "enabled": removeBlocker === "",
                "description": removeBlocker
            });
        case "playAfter":
            if (type !== "queue")
                return null;
            entry.inert = queueIndexes.length !== 1 || queueIndexes[0] === PlayerService.queueIndex;
            return entry;
        case "removeFromQueue":
            return queueIndexes.length > 0 ? entry : null;
        }
        return null;
    }

    // A zone's entries in order, dividers collapsed
    function entries(zone, keepInert) {
        const out = [];
        for (const i of UiConfig.item(type)?.zones[zone] ?? []) {
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
            if (e && (keepInert || !e.inert))
                out.push(e);
        }
        while (out.length > 0 && out[out.length - 1].divider)
            out.pop();
        return out;
    }

    // anchor: the hover button the action ran from, for a menu that follows (several artists)
    function run(id, anchor) {
        switch (id) {
        case "playNext":
            PlayerService.enqueue(tracks, true);
            break;
        case "addToQueue":
            PlayerService.enqueue(tracks, false);
            break;
        case "addToPlaylist":
            Router.pickPlaylist(tracks);
            break;
        case "favorite":
            {
                const value = !allFavorite;
                for (const t of favorable.ok)
                    PlayerService.setFavorite("track", t, value);
                PlayerService.reportSkipped(favorable.skipped);
                break;
            }
        case "goToAlbum":
            Router.go("album", first.albumId);
            break;
        case "goToArtist":
            if (linkedArtists.length > 1)
                // after this menu's popup is gone, so the new one can take the grab
                Qt.callLater(() => anchor ? artistMenu.open(anchor) : artistMenu.openLike(menu));
            else
                Router.go("artist", linkedArtists[0]?.id ?? first.artistId);
            break;
        case "details":
            Router.showDetails("track", first);
            break;
        case "removeFromPlaylist":
            Router.removeFromPlaylist(playlistId, playlistIndexes);
            break;
        case "playAfter":
            {
                const from = queueIndexes[0];
                const current = PlayerService.queueIndex;
                PlayerService.queueMove(from, from < current ? current : current + 1);
                break;
            }
        case "removeFromQueue":
            queueRemoveRequested();
            break;
        }
    }

    DMenu {
        id: menu
        model: {
            const list = root.entries(root.zone, false);
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
