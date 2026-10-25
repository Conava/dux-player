pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import qs.Services
import "../Common/EscapeStack.js" as Escape

// Navigation and transient UI state of the main window.
Singleton {
    id: root

    readonly property var navEntries: UiConfig.shownNavigation

    property string page: navEntries.length > 0 ? UiConfig.entryRoute(navEntries[0]).page : "home"
    property string itemId: ""
    property var history: []
    // the main window's side panel beside the pages: "" | "queue" | "lyrics"
    property string panel: ""
    property bool nowPlayingOpen: false
    // the pane beside (or in place of) the full view's art: "" | "lyrics" | "queue"
    property string viewPane: ""
    property bool devicePickerOpen: false
    property bool sleepPickerOpen: false
    property bool speedPickerOpen: false
    // the shown page's own search field, which Ctrl+F focuses before the global search
    property Item pageSearch: null
    // the list picked in last (a track list, search results or the queue): Ctrl+A acts on its selection
    property Item selectionOwner: null
    // the active EscapeLayers, the last activated on top
    property var escapeLayers: []
    // the shown page is in edit mode; set by editPage, cleared by any navigation
    property bool editing: false
    // what the page editor edits: 0 this page, 1 every page of its kind. Held here because a layout switch
    // rebuilds the editor; any navigation resets it.
    property int editScope: 0
    // how now playing shows right now, "bar" | "side": the saved default until the bar's move button overrides
    // it for this run; the expanded view is nowPlayingOpen on top of either
    readonly property string savedDock: UiConfig.nowPlaying.start === "side" ? "side" : "bar"
    property string dockOverride: ""
    readonly property string dockMode: dockOverride !== "" ? dockOverride : savedDock
    // a new saved default applies at once
    onSavedDockChanged: dockOverride = ""
    // the side panel's tab, remembered across sessions: one of UiConfig.sidePanes
    readonly property string sideTab: UiConfig.sidePanes.indexOf(SettingsData.sideTab) >= 0 ? SettingsData.sideTab : UiConfig.sidePanes[0]

    // tracks waiting for the add-to-playlist dialog
    property var playlistPick: []
    // tracks being dragged, for drop targets (queue, playlists)
    property var dragTracks: []
    // the playlist a drag started in, so dropping back onto it doesn't duplicate tracks
    property string dragSource: ""

    // the snackbar's text, "" when none shows
    property string notice: ""
    // the undo the snackbar offers, null when it offers none
    property var noticeUndo: null
    property var _pendingCommit: null
    // {kind: "track"|"album", item} for the details dialog, null when closed
    property var details: null
    // the Libraries settings page opens its add dialog
    property bool wantAddLibrary: false

    signal settingsRequested(string page)
    // the main window should come to the front (from the settings window)
    signal raiseRequested
    // the window shows one text prompt; callback(text) runs on confirm
    signal promptRequested(string title, string initial, string confirmText, var callback, string label)
    // the window asks before something is removed; callback() runs on confirm
    signal confirmRequested(string title, string text, string confirmText, var callback)

    readonly property bool canGoBack: history.length > 0
    // detail pages light up the entry they were reached from
    readonly property int navIndex: {
        const exact = navEntries.findIndex(e => {
            const route = UiConfig.entryRoute(e);
            return route.page === page && route.itemId === itemId;
        });
        if (exact >= 0)
            return exact;
        const parent = {
            "album": "albums",
            "artist": "artists",
            "playlist": "playlists",
            "genre": "genres",
            "library": "libraries"
        }[page] ?? "";
        return navEntries.findIndex(e => e.kind === "page" && e.target === parent);
    }

    // the search page over everything, its field focused
    function searchEverything() {
        nowPlayingOpen = false;
        if (page === "search")
            pageSearch?.forceActiveFocus();
        else
            go("search");
    }

    function go(target, id) {
        if (target === page && (id ?? "") === itemId)
            return;
        history = history.concat([
            {
                "page": page,
                "itemId": itemId
            }
        ]);
        page = target;
        itemId = id ?? "";
        nowPlayingOpen = false;
        editing = false;
        editScope = 0;
    }

    // whether a list with a selection takes Ctrl+A: it was picked in last, or the last one has none
    function ownsSelectionKeys(list) {
        const owner = selectionOwner;
        return owner === list || owner === null || !owner.visible || !owner.selecting;
    }

    function raiseEscape(layer) {
        escapeLayers = Escape.raise(escapeLayers, layer);
    }

    function dropEscape(layer) {
        const next = Escape.drop(escapeLayers, layer);
        if (next !== escapeLayers)
            escapeLayers = next;
    }

    // a page in edit mode (Settings → Pages)
    function editPage(target, id) {
        go(target, id);
        editing = true;
        raiseRequested();
    }

    function openEntry(entry) {
        const route = UiConfig.entryRoute(entry);
        history = [];
        page = route.page;
        itemId = route.itemId;
        nowPlayingOpen = false;
        editing = false;
        editScope = 0;
    }

    function openItem(kind, item) {
        go(kind, item.id);
    }

    function back() {
        if (history.length === 0)
            return;
        const last = history[history.length - 1];
        history = history.slice(0, -1);
        page = last.page;
        itemId = last.itemId;
        editing = false;
        editScope = 0;
    }

    // back, or to the first navigation entry when there is no history (the page was opened from the nav)
    function leave() {
        if (canGoBack) {
            back();
            return;
        }
        if (navEntries.length > 0)
            openEntry(navEntries[0]);
    }

    // pane: "lyrics" | "queue" | "" for the one Settings → Now playing opens with
    function openNowPlaying(pane) {
        viewPane = pane || UiConfig.startPane;
        nowPlayingOpen = true;
    }

    // shows pane beside the full view's art, or hides it when it already shows; remembered for "Last used"
    function toggleViewPane(pane) {
        viewPane = viewPane === pane ? "" : pane;
        SettingsData.lastPane = viewPane;
    }

    // opens the side panel on view, or closes it when it already shows view
    function togglePanel(view) {
        panel = panel === view ? "" : view;
    }

    function setSideTab(tab) {
        SettingsData.sideTab = tab;
    }

    function toggleDock() {
        dockOverride = dockMode === "side" ? "bar" : "side";
    }

    // Settings → Now playing on a surface's editor: "bar", "side", "fullBento" or "fullM3e"
    function customize(surface) {
        openSettings("nowplaying/" + surface);
    }

    // a now-playing button's action. context: "bar" (lyrics and the queue open the main window's panel), "side"
    // (they switch the side panel's tab) or "view" (the full view's pane)
    function runElement(type, context) {
        const track = PlayerService.current;
        switch (type) {
        case "shuffle":
            PlayerService.toggleShuffle();
            return;
        case "repeat":
            PlayerService.cycleRepeat();
            return;
        case "previous":
            PlayerService.previous();
            return;
        case "next":
            PlayerService.next();
            return;
        case "playPause":
            PlayerService.toggle();
            return;
        case "expand":
            openNowPlaying("");
            return;
        case "dock":
            toggleDock();
            return;
        case "speed":
            speedPickerOpen = true;
            return;
        case "sleep":
            sleepPickerOpen = true;
            return;
        case "device":
        case "castTarget":
            devicePickerOpen = true;
            return;
        case "favorite":
            PlayerService.setFavorite("track", track, CatalogService.withMarks(track)?.favorite !== true);
            return;
        case "lyrics":
        case "queue":
        case "playingFrom":
            {
                const pane = type === "playingFrom" ? "queue" : type;
                if (context === "view")
                    toggleViewPane(pane);
                else if (context === "bar")
                    togglePanel(pane);
                else if (UiConfig.sidePanes.indexOf(pane) >= 0)
                    setSideTab(sideTab === pane ? "cover" : pane);
                else
                    openNowPlaying(pane);
                return;
            }
        case "addToPlaylist":
            if (track)
                pickPlaylist([track]);
            return;
        case "goToAlbum":
            if (track?.albumId)
                go("album", track.albumId);
            return;
        case "goToArtist":
            {
                const artist = (track?.artists ?? []).find(a => a.id !== "")?.id ?? track?.artistId ?? "";
                if (artist !== "")
                    go("artist", artist);
                return;
            }
        case "details":
            if (track)
                showDetails("track", track);
            return;
        }
    }

    function pickPlaylist(tracks) {
        playlistPick = tracks;
    }

    // label: the field's label; empty means "Name"
    function prompt(title, initial, confirmText, callback, label) {
        promptRequested(title, initial, confirmText, callback, label ?? "");
    }

    function openSettings(page) {
        settingsRequested(page);
    }

    function confirm(title, text, confirmText, callback) {
        confirmRequested(title, text, confirmText, callback);
    }

    function showDetails(kind, item) {
        details = {
            "kind": kind,
            "item": item
        };
    }

    // Removes playlist entries (positions in the playlist) and offers Undo, which puts them back.
    function removeFromPlaylist(id, positions) {
        const p = PlaylistService.playlist(id);
        const entries = PlaylistService.entriesAt(id, positions);
        if (!p || entries.length === 0)
            return;
        const lengthAfter = p.items.length - entries.length;
        askRemove(I18n.tr("Remove from %1?", "remove tracks from a playlist confirmation title, %1 is the playlist").arg(p.name), "", () => {
            PlaylistService.removeFromPlaylist(id, entries.map(e => e.position));
            offerUndo(I18n.tr("Removed from %1", "toast after removing tracks from a playlist, %1 is the playlist").arg(p.name), () => PlaylistService.restoreEntries(id, entries, lengthAfter));
        });
    }

    // Removes a queue entry and offers Undo, which puts the track back at its place.
    // indexes: queue positions; done() runs once they are removed
    function removeFromQueue(indexes, done) {
        const at = indexes.filter(i => PlayerService.queue[i]).sort((a, b) => a - b);
        if (at.length === 0)
            return;
        const refs = at.map(i => ({
                    "library": PlayerService.queue[i].library,
                    "id": PlayerService.queue[i].id
                }));
        askRemove(I18n.tr("Remove from the queue?", "remove a track from the queue confirmation title"), "", () => {
            for (let k = at.length - 1; k >= 0; k--)
                PlayerService.queueRemove(at[k]);
            if (done)
                done();
            offerUndo(I18n.tr("Removed from the queue", "toast after removing a track from the queue"), () => DaemonBridge.request("queue.add", {
                    "tracks": refs
                }, I18n.tr("Couldn't add to the queue", "toast when adding tracks to the play queue fails"), q => {
                    // appended in queue order; moving each back in that order leaves the later ones in place
                    const start = (q.items ?? []).length - refs.length;
                    at.forEach((to, k) => {
                        if (start + k > to)
                            DaemonBridge.request("queue.move", {
                                "from": start + k,
                                "to": to
                            });
                    });
                }));
        });
    }

    // Deletes a playlist: held back behind the undo toast, or asked first in confirm mode.
    function deletePlaylist(id) {
        const p = PlaylistService.playlist(id);
        if (!p)
            return;
        const run = () => {
            PlaylistService.setHidden(id, true);
            if (page === "playlist" && itemId === id)
                leave();
            offerUndo(I18n.tr("Deleted \"%1\"", "toast after deleting a playlist").arg(p.name), () => PlaylistService.setHidden(id, false), done => {
                UiConfig.removeEntry("playlist", id);
                PlaylistService.deletePlaylist(id, done);
            });
        };
        const server = p.kind === "server";
        askRemove(I18n.tr("Delete \"%1\"?", "delete playlist confirmation title").arg(p.name), server ? I18n.tr("The playlist is deleted from %1 and every device that uses it. The tracks stay in their libraries.", "delete server playlist confirmation").arg(CatalogService.libraryName(p.library)) : I18n.tr("The tracks stay in their libraries.", "delete local playlist confirmation"), run, I18n.tr("Delete", "confirm button that deletes an item"));
    }

    function addLibrary() {
        wantAddLibrary = true;
        openSettings("libraries");
    }

    function notify(text) {
        _settle(null);
        noticeUndo = null;
        notice = text;
        noticeTimer.interval = PlayerMetrics.snackbarDuration;
        noticeTimer.restart();
    }

    // Runs a removal at once, or after a confirm dialog when Settings → General asks for one.
    function askRemove(title, text, run, confirmText) {
        if (UiConfig.removeMode === "confirm")
            confirm(title, text, confirmText ?? I18n.tr("Remove", "button that removes an item"), run);
        else
            run();
    }

    // After removing something: a toast with Undo in undo mode. commit(done), when given, is the removal
    // itself, held back until the toast ends; in the other modes it runs at once.
    function offerUndo(text, undo, commit) {
        if (UiConfig.removeMode !== "undo") {
            commit?.(null);
            return;
        }
        _settle(null);
        notice = text;
        noticeUndo = undo;
        _pendingCommit = commit ?? null;
        noticeTimer.interval = UiConfig.undoSeconds * 1000;
        noticeTimer.restart();
    }

    function undo() {
        const run = noticeUndo;
        _pendingCommit = null;
        noticeUndo = null;
        notice = "";
        noticeTimer.stop();
        run?.();
    }

    // runs a held-back removal before the window quits; done() once the daemon has it
    function flushUndo(done) {
        if (!_pendingCommit) {
            done();
            return;
        }
        quitFallback.callback = done;
        quitFallback.start();
        _settle(() => {
            quitFallback.stop();
            done();
        });
    }

    function _settle(done) {
        const commit = _pendingCommit;
        _pendingCommit = null;
        commit?.(done);
    }

    readonly property Timer _noticeTimer: Timer {
        id: noticeTimer
        interval: PlayerMetrics.snackbarDuration
        onTriggered: {
            root.notice = "";
            root.noticeUndo = null;
            root._settle(null);
        }
    }

    // a daemon that doesn't answer mustn't keep the window open
    readonly property Timer _quitFallback: Timer {
        id: quitFallback
        property var callback: null
        interval: 2000
        onTriggered: callback?.()
    }

    readonly property Connections _serviceNotices: Connections {
        target: PlayerService
        function onNotice(text) {
            root.notify(text);
        }
    }
}
