pragma ComponentBehavior: Bound

import QtQuick
import qs.Common

// Turns the daemon's lyrics result into timed line groups, timed by PlayerService's position clock.
// Line preparation follows the shell's LyricsController so both views time and group lines alike.
QtObject {
    id: root

    property bool enabled: true
    readonly property var result: PlayerService.lyrics
    readonly property string serviceState: PlayerService.lyricsState
    readonly property bool playing: PlayerService.positionRunning
    readonly property real duration: PlayerService.duration
    readonly property string trackKey: PlayerService.currentKey
    readonly property real rate: PlayerService.positionRate
    readonly property bool canSeekLines: synced && duration > 0

    property var lines: []
    property var plainLines: []
    property bool synced: false
    property string attributionName: ""
    property string attributionUrl: ""
    property int activeIndex: -1
    property int wordRevision: 0
    property real sampleTime: 0
    property var cueTimes: []
    property var focusedGroups: []
    readonly property string state: serviceState === "ready" && !synced && plainLines.length === 0 ? "none" : serviceState
    readonly property bool pending: serviceState === "loading"
    readonly property string shownResult: result ? JSON.stringify([trackKey, result.source ?? ""]) : ""

    // the daemon looks lyrics up only while a view holds them; held only changes on a real transition, so a
    // view created hidden never gives back a hold it didn't take
    property bool held: false

    onResultChanged: load()
    onPlayingChanged: resync()
    onRateChanged: resync()
    onEnabledChanged: {
        hold(enabled);
        if (enabled)
            resync();
        else
            tick.stop();
    }
    Component.onCompleted: {
        hold(enabled);
        load();
    }
    Component.onDestruction: hold(false)

    function hold(on) {
        if (held === on)
            return;
        held = on;
        PlayerService.watchLyrics(on);
    }

    property Connections seekWatch: Connections {
        target: PlayerService
        function onSeeked() {
            root.resync();
        }
        // the line was scheduled from a clock sample a later report corrected (a track still starting or
        // loading when the lyrics arrived); without this the line stays late until the next cue
        function onClockCorrected() {
            root.resync();
        }
    }

    function load() {
        tick.stop();
        const r = result;
        lines = r && Array.isArray(r.synced) ? prepareLines(r.synced, r.voices ?? {}) : [];
        synced = lines.length > 0;
        plainLines = r && typeof r.plain === "string" && r.plain.trim() !== "" ? r.plain.trim().split("\n") : [];
        attributionName = r?.attribution?.name ?? "";
        attributionUrl = r?.attribution?.url ?? "";
        activeIndex = -1;
        focusedGroups = [];
        wordRevision++;
        resync();
    }

    function prepareLines(source, voices) {
        const parts = source.filter(line => Number.isFinite(line.t) && line.t >= 0 && typeof line.x === "string").map(line => ({
                    t: line.t,
                    e: Number.isFinite(line.e) && line.e > line.t ? line.e : 0,
                    x: line.x,
                    w: wordsFor(line),
                    voice: typeof line.voice === "string" ? line.voice : "",
                    background: line.background === true,
                    group: Number.isInteger(line.group) && line.group > 0 ? line.group : 0
                })).sort((a, b) => a.t - b.t);
        const singers = [...new Set(parts.filter(part => part.voice && !part.background && voices[part.voice]?.type !== "group").map(part => part.voice))];
        const nextStarts = new Map();
        const events = new Set();
        for (let i = parts.length - 1; i >= 0; i--) {
            const part = parts[i];
            const lane = part.voice + "\u0000" + part.background;
            const following = nextStarts.get(lane);
            const next = following?.t === part.t ? following.next : following?.t ?? Infinity;
            const cues = part.w.slice().sort((a, b) => a.t - b.t);
            const last = cues[cues.length - 1];
            if (part.e <= part.t) {
                const trackEnd = duration > part.t ? duration : Infinity;
                part.e = last?.e > last?.t ? Math.max(...cues.map(word => word.e)) : next > part.t ? Math.min(next, trackEnd) : trackEnd;
            }
            if (last && part.e <= last.t)
                part.e = next > last.t ? next : Infinity;
            if (next > part.t)
                part.e = Math.min(part.e, next);
            nextStarts.set(lane, {
                t: part.t,
                next
            });
            part.cues = cues;
            const voice = voices[part.voice];
            const singerIndex = singers.indexOf(part.voice);
            part.voiceIndex = Math.max(0, singerIndex);
            part.chorus = singers.length > 1 && singerIndex < 0 && voice?.type === "group";
            part.side = singers.length > 1 && singerIndex >= 0 ? (singerIndex % 2 === 0 ? -1 : 1) : 0;
            part.voiceName = typeof voice?.name === "string" ? voice.name : "";
            events.add(part.t);
            if (Number.isFinite(part.e))
                events.add(part.e);
            for (const word of cues)
                events.add(word.t);
        }
        const groups = new Map();
        for (const part of parts) {
            const groupKey = part.group ? "group:" + part.group : "time:" + part.t;
            let group = groups.get(groupKey);
            if (!group) {
                group = {
                    t: part.t,
                    e: part.e,
                    parts: [],
                    x: ""
                };
                groups.set(groupKey, group);
            }
            group.t = Math.min(group.t, part.t);
            group.e = Math.max(group.e, part.e);
            group.parts.push(part);
        }
        const prepared = [...groups.values()].sort((a, b) => a.t - b.t);
        for (const group of prepared) {
            group.parts.sort((a, b) => Number(a.background) - Number(b.background) || a.t - b.t);
            group.x = group.parts.map(part => part.x).join("\n");
        }
        cueTimes = [...events].sort((a, b) => a - b).map(t => ({
                    t
                }));
        return prepared;
    }

    function wordsFor(line) {
        if (!Array.isArray(line.w) || !line.w.every(word => Number.isFinite(word.t) && word.t >= line.t && typeof word.x === "string"))
            return [];
        if (line.w.map(word => word.x).join("") !== line.x)
            return [];
        return line.w.map(word => ({
                    t: word.t,
                    e: Number.isFinite(word.e) && word.e > word.t ? word.e : word.t,
                    x: word.x
                }));
    }

    function seekToLine(index) {
        const line = lines[index];
        if (!canSeekLines || !line)
            return;
        PlayerService.seek(line.t);
    }

    function currentTime() {
        return PlayerService.exactPosition();
    }

    function resync() {
        tick.stop();
        if (!enabled || !synced)
            return;
        const at = currentTime();
        sampleTime = at;
        const nextIndex = indexFor(at, lines);
        const focused = [];
        for (let i = 0; i <= nextIndex; i++) {
            if (i === nextIndex || lines[i].parts.some(part => part.t <= at && at < part.e))
                focused.push(i);
        }
        if (focused.join() !== focusedGroups.join())
            focusedGroups = focused;
        activeIndex = nextIndex;
        wordRevision++;
        const next = cueTimes[indexFor(at, cueTimes) + 1]?.t ?? Infinity;
        if (!playing || !Number.isFinite(next))
            return;
        tick.interval = Math.max(1, Math.min(2147483647, (next - at) * 1000 / rate));
        tick.start();
    }

    function wordTiming(part, at) {
        const cues = part?.cues ?? [];
        const index = indexFor(at, cues);
        const word = cues[index];
        if (!word)
            return {
                t: -1,
                e: -1
            };
        return {
            t: word.t,
            e: Math.min(part.e, cues[index + 1]?.t ?? Infinity, word.e)
        };
    }

    function indexFor(at, timeline) {
        let low = 0;
        let high = timeline.length - 1;
        let found = -1;
        while (low <= high) {
            const mid = (low + high) >> 1;
            if (timeline[mid].t <= at) {
                found = mid;
                low = mid + 1;
                continue;
            }
            high = mid - 1;
        }
        return found;
    }

    property Timer tick: Timer {
        onTriggered: root.resync()
    }
}
