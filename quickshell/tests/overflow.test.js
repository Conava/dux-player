// node quickshell/tests/overflow.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Common/Overflow.js"), "utf8").replace(".pragma library", "");
const O = new Function(source + "; return { overflow, least, yieldsHeight };")();

const row = (...specs) => specs.map((s, i) => {
    const [type, placement] = s.split(":");
    return { "key": type + i, "type": type, "placement": placement ?? "auto", "width": 10 };
});

// each item takes 10 + 2 spacing; the ⋮ button 10 + 2
const cases = [
    ["fits: nothing moves", row("lyrics", "queue", "volume"), 36, []],
    ["no limit", row("lyrics", "queue", "volume"), -1, []],
    ["least important first, room for the button", row("lyrics", "queue", "dock", "volume"), 36, ["dock2", "lyrics0"]],
    ["keep never moves", row("dock:keep", "lyrics", "queue", "volume"), 36, ["lyrics1", "queue2"]],
    ["kept items go last when keepMoves", row("dock:keep", "lyrics", "queue", "volume"), 12, ["lyrics1", "queue2", "volume3", "dock0"], true],
    ["always in the menu even with room", row("lyrics:overflow", "volume"), 100, ["lyrics0"]],
    ["the later duplicate goes first", row("lyrics", "lyrics", "volume", "volume"), 36, ["lyrics1", "lyrics0"]],
    ["track info and play never move", row("title", "playPause", "cover", "elapsed"), 10, []],
    ["overflow placement on track info is ignored", row("title:overflow"), 100, []],
    ["spacers go before buttons", row("volume", "spacer", "queue", "device"), 36, ["spacer1", "queue2"]]
];
for (const [name, items, room, want, keepMoves] of cases)
    assert.deepStrictEqual(O.overflow(items, room, 10, 2, keepMoves), want, name);

// a block of track info takes room only for what it shows, and a run takes it once
const info = (type, showing) => ({ "key": type, "type": type, "placement": "auto", "width": 30, "stack": true, "showing": showing });
const infoCases = [
    ["a block that shows nothing moves no button", [info("castTarget", false), ...row("lyrics", "queue")], 24, []],
    ["a block that shows takes its room", [info("castTarget", true), ...row("lyrics", "queue")], 24, ["lyrics0", "queue1"]],
    ["a run counts once", [info("title", true), info("artists", true), ...row("lyrics")], 46, []],
    ["the run's width comes from what shows", [info("castTarget", false), info("title", true), ...row("lyrics")], 41, ["lyrics0"]]
];
for (const [name, items, room, want] of infoCases)
    assert.deepStrictEqual(O.overflow(items, room, 10, 2), want, name);

// The least a row can be: what never moves, one block per run of track info, the ⋮ if anything moves
const fixed = (type, width, stack) => ({ "type": type, "width": width, "stack": stack === true });
const leastCases = [
    ["empty", [], 0],
    ["movable elements leave only the ⋮", [fixed("volume", 40), fixed("queue", 40)], 10],
    ["the lyric line gives way to nothing", [fixed("lyricLine", 280)], 0],
    ["never-moving elements stay with their gaps", [fixed("cover", 48), fixed("playPause", 60)], 110],
    ["a run of track info is one block", [fixed("title", 160, true), fixed("artists", 0, true)], 160],
    ["track info split by something that stays is two", [fixed("title", 160, true), fixed("cover", 48), fixed("album", 160, true)], 372],
    ["a moved button joins the blocks around it", [fixed("title", 160, true), fixed("volume", 40), fixed("album", 160, true)], 172],
    ["track info that shows nothing takes nothing", [Object.assign(fixed("castTarget", 160, true), { "showing": false }), fixed("cover", 48)], 48]
];
for (const [name, items, want] of leastCases)
    assert.strictEqual(O.least(items, 10, 2), want, name);

// lyric only, the other line lyric only, this is the lower line: goes first
const heightCases = [
    [true, false, false, true],
    [true, false, true, true],
    [false, true, false, false],
    [false, true, true, false],
    [true, true, true, true],
    [true, true, false, false],
    [false, false, true, true],
    [false, false, false, false]
];
for (const [lyricOnly, other, lower, want] of heightCases)
    assert.strictEqual(O.yieldsHeight(lyricOnly, other, lower), want, [lyricOnly, other, lower].join(" "));

console.log("overflow ok");
