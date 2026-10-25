pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import "Catalog.js" as Catalog
import "PositionClock.js" as PositionClock

// Playback: transport, queue, what plays from where, lyrics and the sleep timer. The catalog, playlists,
// outputs, settings and history live in their own services; the forwards at the end keep the older views
// working until they use those directly.
Singleton {
    id: root

    readonly property var log: Log.scoped("PlayerService")
    // why a control for a feature the core doesn't have yet is disabled
    readonly property string unavailableText: I18n.tr("Not available yet", "reason shown on controls for features still missing in the daemon")

    // tracks plus a qid that stays the same while the entry moves
    property var queue: []
    // what the queue came from, and whether play next, reordering or removing changed it since
    property var playContext: ({
            "type": "tracks"
        })
    property bool queueEdited: false
    property int queueIndex: -1
    // the player's own track wins: player and queue events can arrive in either order
    readonly property var current: CatalogService.track(_statusTrack ?? queue[queueIndex] ?? null)
    readonly property string currentKey: current ? Catalog.key(current) : ""
    // seconds left in the queue from the current track's start; subtract position for what's left
    readonly property real queueLeft: _queueSums[Math.max(0, queueIndex)] ?? 0
    property bool playing: false
    // a stream URL is still being looked up; the position stands until it plays
    property bool loading: false
    // the running position, refreshed a few times a second while a view holds watchPosition; a view that
    // animates the position reads livePosition() per frame instead
    property real position: 0
    // the position moves: playing, and the daemon has reported the track running
    readonly property bool positionRunning: _clock.running
    readonly property real positionRate: _clock.rate
    property real duration: 0
    property real volume: 0.7
    property bool muted: false
    property bool shuffle: false
    // "off" | "all" | "one"
    property string repeat: "off"
    // the queue refills itself with similar songs at its end; continueKnown: the daemon reports it at all
    property bool continuing: false
    property bool continueKnown: false

    // "idle" | "loading" | "ready" | "none" | "instrumental" | "error"
    readonly property string lyricsState: _lyricsKey === "" || _lyricsKey === currentKey || _lyricsState === "idle" ? _lyricsState : "loading"
    // dux-go lyrics Result JSON for the current track, or null
    readonly property var lyrics: _lyricsKey === "" || _lyricsKey === currentKey ? _lyrics : null

    // "off" | "minutes" | "endOfTrack"
    property string sleepMode: "off"
    property real sleepAt: 0
    property real sleepRemaining: 0

    signal seeked(real position)
    // a position report moved the clock by more than drift; timed views (lyrics) schedule again
    signal clockCorrected
    // a second `dplayer show` asks the open window to come forward
    signal showRequested
    // a result worth a snackbar: a command failed, or part of a selection was skipped
    signal notice(string text)

    // Playback

    function play() {
        DaemonBridge.request(_state === "stopped" ? "player.resume" : "player.toggle", null, I18n.tr("Couldn't start playback", "toast when the daemon refuses to play"));
    }

    function pause() {
        DaemonBridge.request("player.pause");
    }

    function toggle() {
        if (playing)
            return pause();
        play();
    }

    function next() {
        DaemonBridge.request("player.next");
    }

    function previous() {
        DaemonBridge.request("player.previous");
    }

    function seek(seconds) {
        _syncClock({
            "position": Math.max(0, Math.min(duration, seconds))
        }, true);
        seeked(position);
        DaemonBridge.request("player.seek", {
            "position": position
        });
    }

    // A slider sends one step per round trip; the latest waits and the daemon's events don't pull it back.
    function setVolume(value) {
        volume = Math.max(0, Math.min(SettingsService.volumeMax, value));
        if (muted && volume > 0)
            setMuted(false);
        _sendVolume();
    }

    function setMuted(value) {
        muted = value;
        DaemonBridge.request("player.mute", {
            "muted": value
        }, null, null, () => root._refreshStatus());
    }

    function setShuffle(on) {
        shuffle = _shuffleOn(on);
        DaemonBridge.request("player.shuffle", {
            "on": shuffle
        }, null, null, () => root._refreshStatus());
    }

    function toggleShuffle() {
        setShuffle(!shuffle);
    }

    function setRepeat(mode) {
        repeat = mode;
        DaemonBridge.request("player.repeat", {
            "mode": mode
        }, null, null, () => root._refreshStatus());
    }

    // endless continue for the queue playing now, whatever Settings say for new queues
    function continueQueue() {
        continuing = true;
        DaemonBridge.request("queue.continue", {
            "mode": "similar"
        }, I18n.tr("Couldn't continue with similar tracks", "toast when the daemon refuses to keep filling the queue"), null, () => root.continuing = false);
    }

    function cycleRepeat() {
        setRepeat(repeat === "off" ? "all" : repeat === "all" ? "one" : "off");
    }

    // Queue

    // Plays `list` (tracks or refs) from `index`. context: what it was started from, {type, id?} with type
    // album | playlist | favorites | library | genre | view | artist | tracks | history | search; it labels
    // "Continue listening". A whole album or playlist goes by id; any other list goes whole as refs, and the
    // daemon starts once the entry and the next few resolve.
    function playTracks(list, index, context) {
        if (list.length === 0)
            return;
        const ctx = context ?? {
            "type": "tracks"
        };
        const at = Math.max(0, Math.min(index ?? 0, list.length - 1));
        if (ctx.type === "album" && list.length === (CatalogService.album(ctx.id)?.trackCount ?? -1)) {
            playAlbum(ctx.id, false, at);
            return;
        }
        const p = ctx.type === "playlist" ? PlaylistService.playlist(ctx.id) : null;
        // only the playlist in its own order (a sorted or searched view plays as shown)
        if (p?.loaded && list.length === p.items.length && list.every((t, i) => t.position === undefined || t.position === i)) {
            if (p.kind === "local")
                _playLocalPlaylist(p, at, false);
            else
                _playServerPlaylist(p, at);
            return;
        }
        const w = _refsAround(list, at);
        DaemonBridge.request("player.play", {
            "tracks": w.refs,
            "index": w.index,
            "origin": _originOf(ctx)
        }, I18n.tr("Couldn't start playback", "toast when the daemon refuses to play"));
    }

    // the list as refs; past the daemon's bound per request, the part around the entry
    function _refsAround(list, at) {
        const max = PlayerMetrics.queueRefsMax;
        const start = Math.max(0, Math.min(at - Math.floor(max / 2), list.length - max));
        return {
            "refs": Catalog.refsOf(list.slice(start, start + max)),
            "index": at - start
        };
    }

    function _playServerPlaylist(p, index) {
        DaemonBridge.request("player.play", {
            "playlist": p.id,
            "index": index,
            "origin": _originOf({
                "type": "playlist",
                "id": p.id
            })
        }, I18n.tr("Couldn't start playback", "toast when the daemon refuses to play"));
    }

    // Shuffles what a view shows. Unfiltered favorites, libraries, all tracks and playlists shuffle in the
    // daemon, over everything they hold; anything else shuffles a sample of the list here.
    function shuffleTracks(list, context, filtered) {
        const ctx = context ?? {
            "type": "tracks"
        };
        const from = filtered ? "" : _shuffleSource(ctx);
        if (from === "album") {
            playAlbum(ctx.id, true);
            return;
        }
        if (from === "localPlaylist") {
            _playLocalPlaylist(PlaylistService.playlist(ctx.id), 0, true);
            return;
        }
        if (from !== "") {
            const params = {
                "from": from
            };
            if (from === "library")
                params.name = ctx.id;
            else if (from === "playlist")
                params.name = PlaylistService.playlist(ctx.id).rawId;
            DaemonBridge.request("player.shuffleFrom", params, I18n.tr("Couldn't start playback", "toast when the daemon refuses to play"));
            return;
        }
        if (list.length === 0)
            return;
        DaemonBridge.request("player.play", {
            "tracks": Catalog.refsOf(Catalog.shuffled(list).slice(0, PlayerMetrics.queueRefsMax)),
            "shuffle": true,
            "origin": _originOf(ctx)
        }, I18n.tr("Couldn't start playback", "toast when the daemon refuses to play"));
    }

    // every track of an artist, album by album, as the daemon lists them
    function playArtist(artist, shuffleFirst) {
        DaemonBridge.request("player.play", {
            "shuffle": shuffleFirst === true,
            "library": artist.library,
            "artist": artist.rawId,
            "origin": _originOf({
                "type": "artist",
                "id": artist.id,
                "name": artist.name
            })
        }, I18n.tr("Couldn't start playback", "toast when the daemon refuses to play"));
    }

    // album ids are "library:albumId", the ref form the daemon takes
    function playAlbum(albumId, shuffleFirst, index) {
        DaemonBridge.request("player.play", {
            "shuffle": shuffleFirst === true,
            "album": albumId,
            "index": index ?? 0
        }, I18n.tr("Couldn't start playback", "toast when the daemon refuses to play"));
    }

    function playPlaylist(id, shuffleFirst) {
        const p = PlaylistService.playlist(id);
        if (!p)
            return;
        if (shuffleFirst) {
            shuffleTracks([], {
                "type": "playlist",
                "id": id
            });
            return;
        }
        if (p.kind === "local") {
            _playLocalPlaylist(p, 0, false);
            return;
        }
        PlaylistService.loadPlaylist(id, () => root.playTracks(PlaylistService.playlistTracks(id), 0, {
                "type": "playlist",
                "id": id
            }));
    }

    function enqueue(refs, playNext) {
        if (refs.length === 0)
            return;
        DaemonBridge.request("queue.add", {
            "tracks": Catalog.refsOf(refs),
            "next": playNext === true
        }, I18n.tr("Couldn't add to the queue", "toast when adding tracks to the play queue fails"));
    }

    function queueJump(index) {
        DaemonBridge.request("queue.jump", {
            "index": index
        });
    }

    // the queue entry with that qid, wherever it sits now
    function queueItem(qid) {
        return _queueByQid[qid] ?? null;
    }

    // failed() runs when the daemon refuses, e.g. to put back a dragged row (the row moving back says it)
    function queueMove(from, to, failed) {
        if (from === to || from < 0 || to < 0 || from >= queue.length || to >= queue.length) {
            failed?.();
            return;
        }
        DaemonBridge.request("queue.move", {
            "from": from,
            "to": to
        }, null, null, () => {
            root._refreshQueue();
            failed?.();
        });
    }

    // order: the new order as old indexes, from a drag that moved one entry
    function queueReorder(order) {
        const move = Catalog.singleMove(order);
        if (move)
            queueMove(move.from, move.to);
    }

    function queueRemove(index) {
        DaemonBridge.request("queue.remove", {
            "index": index
        });
    }

    // keeps the current track playing
    function queueClear() {
        DaemonBridge.request("queue.clear", {
            "keepCurrent": true
        });
    }

    // Sleep timer

    function setSleepTimer(minutes) {
        DaemonBridge.request("player.sleep", {
            "minutes": minutes
        });
    }

    function setSleepAtEndOfTrack() {
        DaemonBridge.request("player.sleep", {
            "endOfTrack": true
        });
    }

    function cancelSleepTimer() {
        DaemonBridge.request("player.sleep", {
            "off": true
        });
    }

    // the lyrics view holds this while it shows; the daemon looks lyrics up only then
    function watchLyrics(on) {
        if (on && (DaemonBridge.watchers.lyrics ?? 0) === 0) {
            _lyrics = null;
            _lyricsKey = "";
            _lyricsState = current ? "loading" : "idle";
        }
        DaemonBridge.watchTopic("lyrics", on);
    }

    // A view showing the running position holds this while visible; between the daemon's events (about one a
    // second) the position then moves on locally.
    function watchPosition(on) {
        _positionHolds = Math.max(0, _positionHolds + (on ? 1 : -1));
        if (on)
            position = livePosition();
    }

    // the position without the eased-out correction: for timing (lyrics), where being right beats being smooth
    function exactPosition() {
        return Math.max(0, Math.min(duration || Infinity, PositionClock.predict(Object.assign({}, _clock, {
            "offset": 0
        }), Date.now(), PlayerMetrics.positionEase)));
    }

    function livePosition() {
        return Math.max(0, Math.min(duration || Infinity, PositionClock.predict(_clock, Date.now(), PlayerMetrics.positionEase)));
    }

    // Daemon link

    property string _state: "stopped"
    property var _statusTrack: null
    property var _clock: PositionClock.create(0)
    // after a track change the clock runs only a little ahead until the daemon's first position report: the
    // new track may still be loading, and running far ahead of it would jump back
    property string _clockTrack: ""
    property bool _clockUnconfirmed: false
    property int _positionHolds: 0
    property var _queueKeys: []
    property var _queueByQid: ({})
    property var _queueSums: [0]
    property var _lyrics: null
    property string _lyricsState: "idle"
    // the track the lyrics belong to ("" when the daemon didn't say)
    property string _lyricsKey: ""
    property bool _volumeBusy: false
    property bool _volumeDirty: false

    function _sendVolume() {
        if (_volumeBusy) {
            _volumeDirty = true;
            return;
        }
        _volumeBusy = true;
        _volumeDirty = false;
        DaemonBridge.call("player.volume", {
            "level": Math.round(volume * 100)
        }, r => {
            root._volumeBusy = false;
            if (root._volumeDirty)
                root._sendVolume();
            else if (r.error) {
                root.log.warn("player.volume", r.error);
                root._refreshStatus();
            } else
                root._applyStatus(r);
        });
    }

    function _refreshStatus() {
        DaemonBridge.request("player.status", null, null, s => root._applyStatus(s));
    }

    function _refreshQueue() {
        DaemonBridge.request("queue.list", null, null, q => root._applyQueue(q));
    }

    // p: a status or position report; jump: no easing (a seek, a new track)
    function _syncClock(p, jump) {
        const rate = p.speed ?? SettingsService.settings.speed ?? 1;
        _clock = PositionClock.sync(_clock, {
            "position": p.position ?? 0,
            "at": p.at ?? 0,
            "rate": rate > 0 ? rate : 1,
            "running": (p.playing ?? playing) && !loading,
            "lead": _clockUnconfirmed ? PlayerMetrics.positionLead : 0
        }, Date.now(), {
            "jump": jump,
            "jumpAt": PlayerMetrics.seekDetectThreshold,
            "easeMs": PlayerMetrics.positionEase
        });
        position = livePosition();
    }

    function _applyStatus(s) {
        _state = s.state ?? "stopped";
        playing = _state === "playing";
        loading = s.loading === true;
        if (!_volumeBusy)
            volume = (s.volume ?? 0) / 100;
        muted = s.muted === true;
        duration = s.duration || (s.track?.duration ?? 0);
        shuffle = _shuffleOn(s.shuffle);
        repeat = s.repeat ?? "off";
        _applyContinuing(s);
        if (s.systemVolume !== undefined)
            SettingsService.systemVolume = s.systemVolume === true;
        queueIndex = s.index ?? -1;
        const t = s.track ? Catalog.track(s.track) : null;
        if (!t || !_statusTrack || Catalog.key(t) !== Catalog.key(_statusTrack))
            _statusTrack = t;
        const sleep = s.sleep ?? {};
        sleepMode = sleep.mode === "timer" ? "minutes" : sleep.mode === "endOfTrack" ? "endOfTrack" : "off";
        sleepAt = sleep.at ?? 0;
        sleepRemaining = sleepMode === "minutes" ? Math.max(0, sleepAt - Date.now() / 1000) : 0;
        _applyPosition(s, false);
        if (s.error)
            log.warn("player:", s.error);
    }

    // Events come about once a second; the position runs on locally in between. reported: a position event,
    // which only comes while the track really plays.
    function _applyPosition(p, reported) {
        const target = p.position ?? 0;
        const track = currentKey;
        const changed = track !== _clockTrack;
        _clockTrack = track;
        if (changed || reported)
            _clockUnconfirmed = !reported;
        const drift = Math.abs(target - livePosition());
        _syncClock(p, changed);
        if (p.duration)
            duration = p.duration;
        if (!changed && drift > PlayerMetrics.seekDetectThreshold)
            seeked(target);
        else if (!changed && drift > PlayerMetrics.lyricsResyncDrift)
            clockCorrected();
    }

    // the daemon used to send a mode ("off" | "tracks" | "albums"); now a bool
    function _shuffleOn(value) {
        return value === true || typeof value === "string" && value !== "" && value !== "off";
    }

    // An advance only moves the index: the list stays the same object, so the queue view keeps its rows.
    function _applyQueue(q) {
        const items = q.items ?? [];
        const keys = items.map(item => item.uid);
        if (!Catalog.sameKeys(keys, _queueKeys)) {
            const byQid = {};
            queue = items.map(item => byQid[item.uid] = Object.assign(Catalog.track(item.track), {
                    "qid": item.uid,
                    "added": item.added === true
                }));
            _queueKeys = keys;
            _queueByQid = byQid;
            _queueSums = Catalog.suffixDurations(queue, t => t.duration);
        }
        queueIndex = q.index ?? -1;
        playContext = Catalog.contextOf(q.origin ?? {});
        queueEdited = q.origin?.modified === true;
        if (q.shuffle !== undefined)
            shuffle = _shuffleOn(q.shuffle);
        repeat = q.repeat ?? repeat;
        _applyContinuing(q);
    }

    function _applyContinuing(e) {
        if (e.continuing === undefined)
            return;
        continueKnown = true;
        continuing = e.continuing === true;
    }

    function _applyLyrics(e) {
        const r = e.result ?? null;
        _lyricsKey = e.library && e.id ? e.library + ":" + e.id : "";
        switch (e.state) {
        case "loading":
            _lyrics = null;
            _lyricsState = "loading";
            return;
        case "ready":
            _lyrics = r?.found ? r : null;
            _lyricsState = !r?.found ? "none" : r.instrumental ? "instrumental" : "ready";
            return;
        case "error":
            _lyrics = null;
            _lyricsState = "error";
            return;
        }
        _lyrics = null;
        _lyricsState = "idle";
    }

    function _onPlayerError(e) {
        log.warn("playback:", e.message, e.reason ?? "");
        if (e.dropped > 0) {
            notice(I18n.tr("Skipped %1 that no longer exist", "snackbar when tracks of a played list no longer exist, %1 is a track count").arg(Phrases.tracks(e.dropped)));
            return;
        }
        // the daemon sends one per library until that library plays again
        if (e.library)
            notice(I18n.tr("Skipped tracks from %1 because it's unavailable", "snackbar when the queue passes over a library's tracks, %1 is the library").arg(CatalogService.libraryName(e.library)));
        else
            notice(I18n.tr("Skipped a track that couldn't play", "toast when one track fails to play and the queue moves on"));
    }

    // "" when the daemon can't shuffle this context as a whole
    function _shuffleSource(ctx) {
        switch (ctx.type) {
        case "favorites":
            return "favorites";
        case "album":
            return ctx.id ? "album" : "";
        case "library":
            return CatalogService.hasCapability(ctx.id, "randomTracks") ? "library" : "";
        case "tracks":
            return CatalogService.libraries.every(l => !l.enabled || CatalogService.hasCapability(l.id, "randomTracks")) ? "tracks" : "";
        case "playlist":
            {
                const p = PlaylistService.playlist(ctx.id);
                return !p ? "" : p.kind === "local" ? "localPlaylist" : "playlist";
            }
        }
        return "";
    }

    function _playLocalPlaylist(p, index, shuffleFirst) {
        DaemonBridge.request("localPlaylists.play", {
            "id": p.rawId,
            "index": index,
            "shuffle": shuffleFirst
        }, I18n.tr("Couldn't start playback", "toast when the daemon refuses to play"));
    }

    function _originOf(context) {
        switch (context.type) {
        case "album":
            {
                const a = CatalogService.album(context.id);
                return {
                    "kind": "album",
                    "library": a?.library ?? "",
                    "id": a?.rawId ?? "",
                    "name": a?.title ?? ""
                };
            }
        case "playlist":
            {
                const p = PlaylistService.playlist(context.id);
                return {
                    "kind": "playlist",
                    "library": p?.library ?? "",
                    "id": p?.rawId ?? "",
                    "name": p?.name ?? ""
                };
            }
        case "artist":
            {
                // an artist on tracks only isn't in the catalog's list; its id still says where it lives
                const a = CatalogService.artist(context.id);
                const at = context.id.indexOf(":");
                return {
                    "kind": "artist",
                    "library": a?.library ?? context.id.slice(0, at),
                    "id": a?.rawId ?? context.id.slice(at + 1),
                    "name": a?.name ?? context.name ?? ""
                };
            }
        case "library":
            return {
                "kind": "library",
                "library": context.id,
                "id": context.id,
                "name": CatalogService.libraryName(context.id)
            };
        case "genre":
        case "view":
            return {
                "kind": context.type,
                "id": context.id ?? "",
                "name": context.type === "genre" ? (CatalogService.genre(context.id)?.name ?? "") : ""
            };
        case "search":
            return {
                "kind": "search",
                "name": CatalogService.searchQuery
            };
        case "favorites":
        case "history":
            return {
                "kind": context.type
            };
        }
        return {
            "kind": "tracks"
        };
    }

    Component.onCompleted: {
        DaemonBridge.subscribeTopic("player", s => root._applyStatus(s));
        DaemonBridge.subscribeTopic("position", p => root._applyPosition(p, true));
        DaemonBridge.subscribeTopic("queue", q => root._applyQueue(q));
        DaemonBridge.subscribeTopic("playerError", e => root._onPlayerError(e));
        DaemonBridge.subscribeTopic("lyrics", e => root._applyLyrics(e), true);
        DaemonBridge.subscribeTopic("ui", e => {
            if (e.action === "show")
                root.showRequested();
        });
        DaemonBridge.start();
    }

    Connections {
        target: DaemonBridge

        function onNotice(text) {
            root.notice(text);
        }

        // nothing plays for the UI while the link is down; the next subscribe sends the state again
        function onConnectedChanged() {
            if (!DaemonBridge.connected) {
                root.playing = false;
                root._syncClock({
                    "position": root.livePosition()
                }, true);
            }
        }
    }

    Timer {
        interval: PlayerMetrics.positionTick
        repeat: true
        running: root.positionRunning && root._positionHolds > 0
        onTriggered: root.position = root.livePosition()
    }

    Timer {
        interval: 1000
        repeat: true
        running: root.sleepMode === "minutes"
        onTriggered: root.sleepRemaining = Math.max(0, root.sleepAt - Date.now() / 1000)
    }

    // Forwards for views that predate the split. New code uses CatalogService, PlaylistService,
    // OutputService, SettingsService and StatsService directly.

    readonly property var libraries: CatalogService.libraries
    readonly property var libraryKinds: CatalogService.libraryKinds
    readonly property var scanProgress: CatalogService.scanProgress
    readonly property var albums: CatalogService.albums
    readonly property var artists: CatalogService.artists
    readonly property var tracks: CatalogService.tracks
    readonly property bool tracksLoading: CatalogService.tracksLoading
    readonly property var genres: CatalogService.genres
    readonly property var cachedKeys: CatalogService.cachedKeys
    readonly property string searchQuery: CatalogService.searchQuery
    readonly property var searchResults: CatalogService.searchResults
    readonly property var playlists: PlaylistService.playlists
    readonly property var history: StatsService.history
    readonly property bool historyEnd: StatsService.historyEnd
    readonly property var recentTracks: StatsService.recentTracks
    readonly property var recentAlbums: StatsService.recentAlbums
    readonly property var settings: SettingsService.settings
    readonly property var eqBands: SettingsService.eqBands
    readonly property var eqPresetIds: SettingsService.eqPresetIds
    readonly property real volumeMax: SettingsService.volumeMax
    readonly property var lyricsProviders: SettingsService.lyricsProviders
    readonly property var scrobble: SettingsService.scrobble
    readonly property bool colorSchemeAvailable: SettingsService.colorSchemeAvailable
    readonly property int colorScheme: SettingsService.colorScheme
    readonly property var outputDevices: OutputService.outputDevices
    readonly property var selectedOutputs: OutputService.selectedOutputs
    readonly property var outputDelays: OutputService.outputDelays
    readonly property bool resetOutputOnStart: OutputService.resetOutputOnStart
    readonly property bool canMultiOutput: OutputService.canMultiOutput
    readonly property bool testTonePlaying: OutputService.testTonePlaying
    readonly property var currentOutput: OutputService.currentOutput
    readonly property bool multiOutput: OutputService.multiOutput
    readonly property string equalizerState: OutputService.equalizerState
    readonly property string devicesError: OutputService.devicesError
    readonly property var castStatus: OutputService.castStatus
    readonly property bool casting: OutputService.casting
    readonly property string castDeviceName: OutputService.castDeviceName

    signal serverTestFinished(bool ok, string message)

    Connections {
        target: CatalogService

        function onServerTestFinished(ok, message) {
            root.serverTestFinished(ok, message);
        }
    }

    // Browse query: the catalog's lists, playlists, and most played (base "mostPlayed").
    function query(type, options) {
        if (type === "playlists")
            return CatalogService.refine(PlaylistService.listed(options.filters?.library ?? ""), type, options);
        if (type === "tracks" && options.base === "mostPlayed")
            return CatalogService.refine(StatsService.mostPlayed(), type, options);
        return CatalogService.query(type, options);
    }

    function key(ref) {
        return Catalog.key(ref);
    }
    function refsOf(list) {
        return Catalog.refsOf(list);
    }
    function shuffled(list) {
        return Catalog.shuffled(list);
    }
    function track(ref) {
        return CatalogService.track(ref);
    }
    function genre(id) {
        return CatalogService.genre(id);
    }
    function album(id) {
        return CatalogService.album(id);
    }
    function artist(id) {
        return CatalogService.artist(id);
    }
    function library(id) {
        return CatalogService.library(id);
    }
    function libraryName(id) {
        return CatalogService.libraryName(id);
    }
    function hasCapability(libraryId, capability) {
        return CatalogService.hasCapability(libraryId, capability);
    }
    function isRemote(libraryId) {
        return CatalogService.isRemote(libraryId);
    }
    function kindLabel(kind) {
        return CatalogService.kindLabel(kind);
    }
    function featureSummary(libraryId) {
        return CatalogService.featureSummary(libraryId);
    }
    function enabledLibrary(id) {
        return CatalogService.enabledLibrary(id);
    }
    function isCached(t) {
        return CatalogService.isCached(t);
    }
    function unavailableReason(t) {
        return CatalogService.unavailableReason(t);
    }
    function whyNot(action, item, target) {
        return CatalogService.whyNot(action, item, target);
    }
    function allows(action, item, target) {
        return CatalogService.allows(action, item, target);
    }
    function partition(action, items, target) {
        return CatalogService.partition(action, items, target);
    }
    function reportSkipped(count) {
        CatalogService.reportSkipped(count);
    }
    function albumTracks(albumId) {
        return CatalogService.albumTracks(albumId);
    }
    function artistAlbums(artistId) {
        return CatalogService.artistAlbums(artistId);
    }
    function visibleAlbums() {
        return CatalogService.visibleAlbums();
    }
    function favoriteTracks() {
        return CatalogService.favoriteTracks();
    }
    function coverUrl(item) {
        return CatalogService.coverUrl(item);
    }
    function coverColor(item) {
        return CatalogService.coverColor(item);
    }
    function setFavorite(kind, ref, value) {
        CatalogService.setFavorite(kind, ref, value);
    }
    function setRating(kind, ref, stars) {
        CatalogService.setRating(kind, ref, stars);
    }
    function search(text) {
        CatalogService.search(text);
    }
    function addLocalLibrary(name, folders) {
        CatalogService.addLocalLibrary(name, folders);
    }
    function testServer(kind, url, user, password) {
        CatalogService.testServer(kind, url, user, password);
    }
    function addServerLibrary(kind, name, url, user, password) {
        CatalogService.addServerLibrary(kind, name, url, user, password);
    }
    function updateLibrary(id, patch) {
        CatalogService.updateLibrary(id, patch);
    }
    function setLibraryEnabled(id, enabled) {
        CatalogService.setLibraryEnabled(id, enabled);
    }
    function retryLibrary(id) {
        CatalogService.retryLibrary(id);
    }
    function openedLibrary(id) {
        CatalogService.openedLibrary(id);
    }
    function rescanLibrary(id) {
        CatalogService.rescanLibrary(id);
    }
    function removeLibrary(id) {
        CatalogService.removeLibrary(id);
    }
    function loadAlbum(id) {
        CatalogService.loadAlbum(id);
    }
    function playlist(id) {
        return PlaylistService.playlist(id);
    }
    function playlistTracks(id) {
        return PlaylistService.playlistTracks(id);
    }
    function loadPlaylist(id, then) {
        PlaylistService.loadPlaylist(id, then);
    }
    function createPlaylist(name, libraryId, done) {
        PlaylistService.createPlaylist(name, libraryId, done);
    }
    function addToPlaylist(id, refs) {
        return PlaylistService.addToPlaylist(id, refs);
    }
    function renamePlaylist(id, name) {
        PlaylistService.renamePlaylist(id, name);
    }
    function setPlaylistCover(id, cover) {
        PlaylistService.setPlaylistCover(id, cover);
    }
    function reorderPlaylist(id, order) {
        PlaylistService.reorderPlaylist(id, order);
    }
    function duplicatePlaylist(id, done) {
        PlaylistService.duplicatePlaylist(id, done);
    }
    function removeFromPlaylist(id, indexes) {
        PlaylistService.removeFromPlaylist(id, indexes);
    }
    function deletePlaylist(id) {
        PlaylistService.deletePlaylist(id);
    }
    function importPlaylist(path, done) {
        PlaylistService.importPlaylist(path, done);
    }
    function exportPlaylist(id, path) {
        PlaylistService.exportPlaylist(id, path);
    }
    function loadHistory(more) {
        StatsService.loadHistory(more);
    }
    function clearHistory() {
        StatsService.clearHistory();
    }
    function mostPlayed() {
        return StatsService.mostPlayed();
    }
    function setSetting(name, value) {
        SettingsService.setSetting(name, value);
    }
    function setEqualizerPreset(preset) {
        SettingsService.setEqualizerPreset(preset);
    }
    function setEqualizerBand(index, gain) {
        SettingsService.setEqualizerBand(index, gain);
    }
    function setLyricsProviderEnabled(id, enabled) {
        SettingsService.setLyricsProviderEnabled(id, enabled);
    }
    function moveLyricsProviders(order) {
        SettingsService.moveLyricsProviders(order);
    }
    function scrobbleConnect(service, params, done) {
        SettingsService.scrobbleConnect(service, params, done);
    }
    function scrobbleFinishAuth() {
        SettingsService.scrobbleFinishAuth();
    }
    function scrobbleDisconnect(service) {
        SettingsService.scrobbleDisconnect(service);
    }
    function scrobbleTest(service) {
        SettingsService.scrobbleTest(service);
    }
    function setScrobbleEnabled(service, enabled) {
        SettingsService.setScrobbleEnabled(service, enabled);
    }
    function setScrobbleLibrary(libraryId, enabled) {
        SettingsService.setScrobbleLibrary(libraryId, enabled);
    }
    function output(id) {
        return OutputService.output(id);
    }
    function selectOutput(id) {
        OutputService.selectOutput(id);
    }
    function toggleOutput(id) {
        OutputService.toggleOutput(id);
    }
    function setOutputDelay(id, ms) {
        OutputService.setOutputDelay(id, ms);
    }
    function setResetOutputOnStart(enabled) {
        OutputService.setResetOutputOnStart(enabled);
    }
    function playTestTone() {
        OutputService.playTestTone();
    }
    function watchCastDevices(on) {
        OutputService.watchCastDevices(on);
    }
}
