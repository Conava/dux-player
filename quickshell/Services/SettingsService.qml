pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import "Catalog.js" as Catalog

// The daemon's settings and accounts: playback settings and the equalizer, lyrics providers, scrobbling,
// the offline cache, and the desktop's color scheme.
Singleton {
    id: root

    readonly property var log: Log.scoped("SettingsService")

    property var settings: ({
            "gapless": true,
            "replayGain": "track",
            "keepRunning": true,
            "speed": 1,
            "equalizer": {
                "enabled": false,
                "preset": "flat",
                "bands": [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
            },
            // "off" | "random" | "similar"
            "autoContinue": "off",
            // albumGapless: tracks of one album play gapless instead
            "crossfade": {
                "enabled": false,
                "seconds": 5,
                "albumGapless": true
            },
            // seconds into a track after which Previous restarts it
            "previousThreshold": 5,
            "allowVolumeBoost": false,
            // "app" (mpv's own volume) | "system" (the default output's)
            "volumeTarget": "app",
            "artistInfoOnline": true,
            "showTrayIcon": true,
            "skipSilenceStart": false,
            "skipSilenceEnd": false,
            // upcoming server tracks downloaded ahead, 0-3
            "prefetchTracks": 1,
            "prefetchOnMetered": false
        })
    property var eqBands: [31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000]
    property var eqPresets: ({})
    readonly property var eqPresetIds: Object.keys(eqPresets)
    // the daemon can drive the system output's volume; the status of settings.get or the player says so
    property bool systemVolume: false
    // status.ffmpeg: silence and loudness can be measured
    property bool ffmpeg: true
    // the daemon's volume ceiling, 1.3 with the boost on
    readonly property real volumeMax: settings.allowVolumeBoost ? 1.3 : 1

    // [{id, name, enabled}] in priority order
    property var lyricsProviders: []
    // scrobble.status: {listenbrainz, lastfm, disabledLibraries, queued}
    property var scrobble: ({
            "listenbrainz": {},
            "lastfm": {},
            "disabledLibraries": [],
            "queued": 0
        })
    // cache.status: {limit, used, count, queued, downloading, full, pinned, pinnedOverLimit,
    // libraries: [{library, used, count, rules}], localRules}; a rule: {kind, id, limit, key, local, tracks,
    // cached, size}
    property var cache: ({
            "libraries": [],
            "localRules": []
        })
    // "library|key" of rules removed but held back by an Undo toast
    property var hiddenRules: ({})
    // marks.fileStatus: {pending, job: {library, done, total, written, failed}, last, waiting, problems: [{path, code, reason, skipped}]}
    property var markFiles: ({
            "problems": []
        })
    // the last finished pass ({library, done, total, written, failed}) and the last that couldn't start
    // ({library, reason}); the daemon only tells them by event
    property var markFilesDone: null
    property var markFilesFailed: null

    // the daemon reports these settings (an older core doesn't)
    property bool artistInfoKnown: false
    property bool trayKnown: false
    // artistInfo.status: {online, providers: [{id, name, uses, needsKey, hasKey}]}
    // the daemon's build: {version, commit}; empty until it answers
    property var daemonBuild: ({})
    property var artistInfo: ({
            "providers": []
        })

    // provider: "fanarttv" | "lastfm"; "" clears the key
    function setArtistInfoKey(provider, key) {
        DaemonBridge.request("artistInfo.setKey", {
            "provider": provider,
            "key": key
        }, I18n.tr("Couldn't save the API key", "toast when storing an artist info API key fails"), r => root.artistInfo = r);
    }

    property bool colorSchemeAvailable: false
    // org.freedesktop.appearance color-scheme: 0 = no preference, 1 = prefer dark, 2 = prefer light
    property int colorScheme: 0

    function setSetting(name, value) {
        const params = {};
        switch (name) {
        case "gapless":
            // the daemon also knows "yes" (always gapless); turning it back on keeps whichever it had
            params.gaplessMode = value ? _gaplessOn : "no";
            break;
        case "replayGain":
            params.replayGainMode = value;
            break;
        case "keepRunning":
            params.keepPlayingOnClose = value;
            break;
        case "equalizer":
            _sendEqualizer(value);
            return;
        case "speed":
        case "autoContinue":
        case "crossfade":
        case "previousThreshold":
        case "allowVolumeBoost":
        case "volumeTarget":
        case "artistInfoOnline":
        case "showTrayIcon":
        case "skipSilenceStart":
        case "skipSilenceEnd":
        case "prefetchTracks":
        case "prefetchOnMetered":
            params[name] = value;
            break;
        default:
            return;
        }
        DaemonBridge.request("settings.set", params, I18n.tr("Couldn't save the setting", "toast when the daemon refuses a settings change"), r => root._applySettings(r), () => root._refreshSettings());
    }

    function setEqualizerPreset(preset) {
        const bands = (eqPresets[preset] ?? _eqBandsNow()).slice();
        _eqDraft = bands;
        _sendEqualizer({
            "preset": preset,
            "bands": bands
        });
    }

    function setEqualizerBand(index, gain) {
        const bands = _eqBandsNow().slice();
        bands[index] = gain;
        _eqDraft = bands;
        _sendEqualizer({
            "preset": "custom",
            "bands": bands
        });
    }

    function setLyricsProviderEnabled(id, enabled) {
        _setProviders(lyricsProviders.map(p => p.id === id ? Object.assign({}, p, {
                "enabled": enabled
            }) : p));
    }

    function moveLyricsProviders(order) {
        _setProviders(order.map(i => lyricsProviders[i]));
    }

    // Scrobbling. service: "listenbrainz" | "lastfm"

    // params: ListenBrainz {token}; Last.fm {apiKey, apiSecret}, or {} for the build's own key; then the browser
    // opens the approval page
    function scrobbleConnect(service, params, done) {
        DaemonBridge.request("scrobble.connect", Object.assign({
            "service": service
        }, params), I18n.tr("Couldn't sign in", "toast when connecting a scrobbling account fails"), result => {
            if (/^https:\/\//.test(result.authUrl ?? ""))
                Qt.openUrlExternally(result.authUrl);
            else
                root._applyScrobble(result);
            done?.();
        });
    }

    function scrobbleFinishAuth() {
        DaemonBridge.call("scrobble.finishAuth", null, result => {
            if (result.error) {
                root.log.warn("scrobble.finishAuth", result.error);
                DaemonBridge.notice(I18n.tr("Not approved on Last.fm yet", "snackbar when the Last.fm sign-in isn't approved"));
                return;
            }
            root._applyScrobble(result);
        });
    }

    function scrobbleDisconnect(service) {
        DaemonBridge.request("scrobble.disconnect", {
            "service": service
        }, I18n.tr("Couldn't sign out", "toast when disconnecting a scrobbling account fails"), result => root._applyScrobble(result));
    }

    function scrobbleTest(service) {
        DaemonBridge.call("scrobble.test", {
            "service": service
        }, result => DaemonBridge.notice(result.ok ? I18n.tr("Signed in as %1", "scrobbling account state, %1 is the user name").arg(result.user ?? "") : I18n.tr("Couldn't reach the account", "snackbar after a failed scrobbling account test")));
    }

    function setScrobbleEnabled(service, enabled) {
        DaemonBridge.request("scrobble.setEnabled", {
            "service": service,
            "enabled": enabled
        }, I18n.tr("Couldn't save the setting", "toast when the daemon refuses a settings change"), result => root._applyScrobble(result));
    }

    function setScrobbleLibrary(libraryId, enabled) {
        DaemonBridge.request("scrobble.setLibrary", {
            "library": libraryId,
            "enabled": enabled
        }, I18n.tr("Couldn't save the setting", "toast when the daemon refuses a settings change"), result => root._applyScrobble(result));
    }

    // Cache for server libraries

    // a library's offline rules ("" for the local playlists kept on this device), without those held back
    function cacheRules(libraryId) {
        const rules = libraryId === "" ? cache.localRules ?? [] : (cache.libraries ?? []).find(c => c.library === libraryId)?.rules ?? [];
        return rules.filter(r => !hiddenRules[libraryId + "|" + r.key]);
    }

    // the capability a server needs to list a rule kind's tracks; "" when any server can
    function ruleCapability(kind) {
        switch (kind) {
        case "everything":
        case "album":
        case "artist":
            return "browse";
        case "playlist":
            return "playlists";
        case "favorites":
            return "favorites";
        }
        return "";
    }

    // limit: {songs} or {bytes}, for played and mostPlayed
    function addCacheRule(libraryId, kind, id, limit) {
        const params = {
            "library": libraryId,
            "kind": kind
        };
        if (id)
            params.id = id;
        if (limit)
            params.limit = limit;
        DaemonBridge.request("cache.addRule", params, I18n.tr("Couldn't keep it on this device", "toast when adding an offline rule fails"));
    }

    function updateCacheRule(libraryId, kind, limit) {
        DaemonBridge.request("cache.updateRule", {
            "library": libraryId,
            "kind": kind,
            "limit": limit
        }, I18n.tr("Couldn't save the offline setting", "toast when an offline cache setting is refused"));
    }

    // done() once the status without the rule is in, or the removal failed
    function removeCacheRule(libraryId, kind, id, done) {
        DaemonBridge.request("cache.removeRule", {
            "library": libraryId,
            "kind": kind,
            "id": id ?? ""
        }, I18n.tr("Couldn't remove it from this device", "toast when removing an offline rule fails"), () => DaemonBridge.request("cache.status", null, null, st => {
                root.cache = st;
                done?.();
            }, () => done?.()), () => done?.());
    }

    // hidden while an Undo toast holds the removal back
    function hideRule(libraryId, key, hidden) {
        const next = Object.assign({}, hiddenRules);
        if (hidden)
            next[libraryId + "|" + key] = true;
        else
            delete next[libraryId + "|" + key];
        hiddenRules = next;
    }

    // "Keep on this device" of a playlist, album or artist: {shown, kept, blocker}. Ids are "library:id",
    // local playlists ":id".
    function keepState(kind, item) {
        const id = item?.id ?? "";
        const cut = id.indexOf(":");
        if (cut < 0)
            return {
                "shown": false,
                "kept": false,
                "blocker": ""
            };
        const lib = id.substring(0, cut);
        const raw = id.substring(cut + 1);
        const rules = cacheRules(lib);
        const kept = rules.some(r => r.kind === kind && r.id === raw);
        if (lib === "")
            return {
                "shown": CatalogService.libraries.some(l => CatalogService.isRemote(l.id)),
                "kept": kept,
                "blocker": ""
            };
        const everything = rules.some(r => r.kind === "everything");
        return {
            "shown": CatalogService.isRemote(lib) && CatalogService.hasCapability(lib, ruleCapability(kind)),
            "kept": kept || everything,
            "blocker": everything ? I18n.tr("All of %1 is kept on this device", "why Keep on this device is locked on, %1 is the library").arg(CatalogService.libraryName(lib)) : ""
        };
    }

    function setKept(kind, item, keep) {
        const id = item.id;
        const cut = id.indexOf(":");
        if (keep)
            addCacheRule(id.substring(0, cut), kind, id.substring(cut + 1));
        else
            removeCacheRule(id.substring(0, cut), kind, id.substring(cut + 1));
    }

    // Ratings and favorites in files (local libraries with writeMarks)

    function setWriteMarks(libraryId, enabled) {
        DaemonBridge.request("marks.setWriteToFiles", {
            "library": libraryId,
            "enabled": enabled
        }, I18n.tr("Couldn't save the setting", "toast when the daemon refuses a settings change"), () => CatalogService.refreshLibraries(false));
    }

    function writeMarksNow(libraryId) {
        markFilesDone = null;
        markFilesFailed = null;
        DaemonBridge.request("marks.writeFiles", {
            "library": libraryId
        }, I18n.tr("Couldn't write ratings to files", "toast when writing ratings and favorites to files can't start"), () => root._refreshMarkFiles());
    }

    function _refreshMarkFiles() {
        DaemonBridge.call("marks.fileStatus", null, r => {
            if (r.error)
                return;
            root.markFiles = r;
            // the last finished pass, so its result shows after a restart of the window too
            if (r.last?.state === "done") {
                root.markFilesDone = r.last;
                root.markFilesFailed = null;
            } else if (r.last?.state === "failed") {
                root.markFilesFailed = r.last;
                root.markFilesDone = null;
            }
        });
    }

    function setCacheLimit(mb) {
        DaemonBridge.request("cache.setLimit", {
            "mb": Math.max(100, Math.round(mb))
        }, I18n.tr("Couldn't save the offline setting", "toast when an offline cache setting is refused"));
    }

    // done(list of {track, size, added, used}), most recently used first
    function cacheList(libraryId, done) {
        DaemonBridge.request("cache.list", {
            "library": libraryId
        }, I18n.tr("Couldn't load downloaded tracks", "toast when the offline cache's track list can't be read"), list => done(list.map(c => Object.assign({}, c, {
                    "track": Catalog.track(c.track)
                }))));
    }

    function clearCache(libraryId) {
        DaemonBridge.request("cache.clear", libraryId ? {
            "library": libraryId
        } : null, I18n.tr("Couldn't clear downloads", "toast when emptying the offline cache fails"));
    }

    // the last gapless mode other than "no"
    property string _gaplessOn: "weak"
    // bands sent but not confirmed yet, so two quick band changes both count
    property var _eqDraft: null
    property bool _eqBusy: false
    property var _eqQueued: null

    function _eqBandsNow() {
        return _eqDraft ?? settings.equalizer.bands;
    }

    // Each change rebuilds the daemon's filter chain, so one goes at a time and the latest waits.
    function _sendEqualizer(value) {
        if (_eqBusy) {
            _eqQueued = value;
            return;
        }
        _eqBusy = true;
        DaemonBridge.call("settings.set", {
            "equalizer": value
        }, r => {
            root._eqBusy = false;
            if (r.error) {
                root.log.warn("settings.set equalizer", r.error);
                DaemonBridge.notice(I18n.tr("Couldn't save the setting", "toast when the daemon refuses a settings change"));
            } else
                root._applySettings(r);
            const next = root._eqQueued;
            root._eqQueued = null;
            if (next)
                root._sendEqualizer(next);
            else
                root._eqDraft = null;
        });
    }

    function _setProviders(list) {
        lyricsProviders = list;
        DaemonBridge.request("lyrics.setProviders", {
            "providers": list.map(p => ({
                        "id": p.id,
                        "enabled": p.enabled
                    }))
        }, I18n.tr("Couldn't save the setting", "toast when the daemon refuses a settings change"), null, () => DaemonBridge.request("lyrics.providers", null, null, l => root.lyricsProviders = l));
    }

    function _applySettings(r) {
        const s = r.settings ?? {};
        const status = r.status ?? {};
        if (s.gaplessMode && s.gaplessMode !== "no")
            _gaplessOn = s.gaplessMode;
        settings = Object.assign({}, settings, {
            "gapless": s.gaplessMode !== "no",
            "replayGain": s.replayGainMode ?? "track",
            "keepRunning": s.keepPlayingOnClose !== false,
            "speed": s.speed ?? 1,
            "equalizer": s.equalizer ?? settings.equalizer,
            "autoContinue": s.autoContinue ?? "off",
            "crossfade": s.crossfade ?? settings.crossfade,
            "previousThreshold": s.previousThreshold ?? 5,
            "allowVolumeBoost": s.allowVolumeBoost === true,
            "volumeTarget": s.volumeTarget === "system" ? "system" : "app",
            "artistInfoOnline": s.artistInfoOnline !== false,
            "showTrayIcon": s.showTrayIcon !== false,
            "skipSilenceStart": s.skipSilenceStart === true,
            "skipSilenceEnd": s.skipSilenceEnd === true,
            "prefetchTracks": s.prefetchTracks ?? 1,
            "prefetchOnMetered": s.prefetchOnMetered === true
        });
        artistInfoKnown = s.artistInfoOnline !== undefined;
        trayKnown = s.showTrayIcon !== undefined;
        if (status.systemVolume !== undefined)
            systemVolume = status.systemVolume === true;
        if (status.ffmpeg !== undefined)
            ffmpeg = status.ffmpeg === true;
        if (status.equalizerBands)
            eqBands = status.equalizerBands;
        if (status.equalizerPresets)
            eqPresets = status.equalizerPresets;
    }

    function _applyScrobble(s) {
        if (s.listenbrainz)
            scrobble = s;
    }

    function _refreshSettings() {
        DaemonBridge.request("settings.get", null, null, r => root._applySettings(r));
    }

    function _refreshAll() {
        _refreshSettings();
        DaemonBridge.request("lyrics.providers", null, null, list => root.lyricsProviders = list);
        DaemonBridge.request("scrobble.status", null, null, s => root._applyScrobble(s));
        DaemonBridge.request("cache.status", null, null, s => root.cache = s);
        _refreshMarkFiles();
        DaemonBridge.call("version", null, r => {
            if (!r.error)
                root.daemonBuild = r;
        });
        DaemonBridge.call("artistInfo.status", null, r => {
            if (!r.error)
                root.artistInfo = r;
        });
        DaemonBridge.call("appearance.colorScheme", null, r => {
            if (r.scheme === undefined)
                return;
            root.colorScheme = r.scheme;
            root.colorSchemeAvailable = true;
        });
    }

    Component.onCompleted: {
        DaemonBridge.subscribeTopic("settings", r => root._applySettings(r));
        DaemonBridge.subscribeTopic("lyricsProviders", list => root.lyricsProviders = list);
        DaemonBridge.subscribeTopic("scrobble", s => root._applyScrobble(s));
        DaemonBridge.subscribeTopic("cache", () => DaemonBridge.request("cache.status", null, null, s => root.cache = s));
        DaemonBridge.subscribeTopic("markFiles", e => {
            if (e.type === "done")
                root.markFilesDone = e.job;
            else if (e.type === "failed")
                root.markFilesFailed = e;
            root._refreshMarkFiles();
        });
        DaemonBridge.subscribeTopic("colorScheme", e => {
            root.colorScheme = e.scheme ?? 0;
            root.colorSchemeAvailable = true;
        });
        if (DaemonBridge.subscribed)
            _refreshAll();
    }

    Connections {
        target: DaemonBridge

        function onResubscribed() {
            root._refreshAll();
        }
    }
}
