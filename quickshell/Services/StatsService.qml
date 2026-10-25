pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import "Catalog.js" as Catalog

// This device's play history: the History page, recently played and most played shelves.
Singleton {
    id: root

    readonly property var log: Log.scoped("StatsService")

    // [track + {playedAt}], newest first; loaded while the History page asks for it
    property var history: []
    property bool historyEnd: false
    property var recentTracks: []
    property var recentAlbums: []
    // most played tracks over every library, all time; null until first used
    property var topTracks: null

    // more: the next page after what is loaded
    function loadHistory(more) {
        if (_historyBusy || (more && historyEnd))
            return;
        _historyBusy = true;
        const params = {
            "limit": PlayerMetrics.historyPage
        };
        // the daemon pages with played_at < before: ask one second later and drop what is already here, so
        // plays sharing the edge second aren't skipped
        const last = more && history.length > 0 ? history[history.length - 1].playedAt : 0;
        if (last > 0)
            params.before = last + 1;
        DaemonBridge.call("history.list", params, result => {
            root._historyBusy = false;
            if (result.error) {
                root.log.warn("history.list", result.error);
                return;
            }
            const seen = {};
            for (const h of last > 0 ? root.history : []) {
                if (h.playedAt >= last)
                    seen[h.playedAt + "|" + Catalog.key(h)] = true;
            }
            const page = result.map(h => Object.assign(Catalog.track(h.track), {
                    "playedAt": h.playedAt
                })).filter(h => !seen[h.playedAt + "|" + Catalog.key(h)]);
            root.history = last > 0 ? root.history.concat(page) : page;
            root.historyEnd = result.length < PlayerMetrics.historyPage;
            root._historyLoaded = true;
        });
    }

    function clearHistory() {
        DaemonBridge.request("history.clear", null, I18n.tr("Couldn't clear the history", "toast when clearing the play history fails"));
    }

    // fetched on first use and after each counted play
    function mostPlayed() {
        if (topTracks === null) {
            // later, so the asking binding doesn't depend on the state this sets
            Qt.callLater(root._wantTop);
            return [];
        }
        return topTracks.map(t => Object.assign({}, CatalogService.track(t) ?? t, {
                "playCount": t.playCount
            })).filter(t => CatalogService.enabledLibrary(t.library));
    }

    property bool _historyBusy: false
    property bool _historyLoaded: false
    // "" (not used yet) | "wanted" | "pending" | "done"
    property string _topState: ""

    // A play doesn't bump counts in the loaded catalog (that would re-sort every list showing it);
    // the shelves refetch, the catalog shows new counts on its next load
    function _onHistoryEvent(e) {
        if (e.type === "cleared") {
            history = [];
            historyEnd = true;
            // local play counts are this device's history
            CatalogService.refreshLibraries(true);
        } else if (e.track && _historyLoaded)
            history = [Object.assign(Catalog.track(e.track), {
                    "playedAt": e.playedAt
                })].concat(history);
        statsRefresh.restart();
    }

    function _refreshStats() {
        DaemonBridge.request("history.recent", {
            "kind": "track",
            "limit": PlayerMetrics.shelfFetchLimit
        }, null, list => root.recentTracks = list.map(h => Catalog.track(h.track)));
        DaemonBridge.request("history.recent", {
            "kind": "album",
            "limit": PlayerMetrics.shelfFetchLimit
        }, null, list => root.recentAlbums = list.map(h => Catalog.album(h.album)));
        if (_topState !== "")
            _topState = "wanted";
        _refreshTop();
    }

    function _wantTop() {
        if (_topState !== "")
            return;
        _topState = "wanted";
        _refreshTop();
    }

    function _refreshTop() {
        if (_topState !== "wanted")
            return;
        _topState = "pending";
        DaemonBridge.call("history.top", {
            "kind": "track",
            "limit": PlayerMetrics.mostPlayedLimit
        }, list => {
            // a failed fetch is asked again with the next refresh
            root._topState = list.error ? "wanted" : "done";
            if (list.error) {
                root.log.warn("history.top", list.error);
                return;
            }
            root.topTracks = list.map(h => Object.assign(Catalog.track(h.track), {
                    "playCount": h.count
                }));
        });
    }

    Component.onCompleted: {
        DaemonBridge.subscribeTopic("history", e => root._onHistoryEvent(e));
        if (DaemonBridge.subscribed)
            _refreshStats();
    }

    Connections {
        target: DaemonBridge

        function onResubscribed() {
            root._refreshStats();
        }
    }

    // plays come one at a time; shelves refetch once after a burst
    Timer {
        id: statsRefresh
        interval: PlayerMetrics.statsRefreshDelay
        onTriggered: root._refreshStats()
    }
}
