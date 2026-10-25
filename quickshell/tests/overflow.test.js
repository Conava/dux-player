// node quickshell/tests/overflow.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Common/Overflow.js"), "utf8").replace(".pragma library", "");
const O = new Function(source + "; return { overflow };")();

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
    ["always in the menu even with room", row("lyrics:overflow", "volume"), 100, ["lyrics0"]],
    ["the later duplicate goes first", row("lyrics", "lyrics", "volume", "volume"), 36, ["lyrics1", "lyrics0"]],
    ["track info and play never move", row("title", "playPause", "cover", "elapsed"), 10, []],
    ["overflow placement on track info is ignored", row("title:overflow"), 100, []],
    ["spacers go before buttons", row("volume", "spacer", "queue", "device"), 36, ["spacer1", "queue2"]]
];
for (const [name, items, room, want] of cases)
    assert.deepStrictEqual(O.overflow(items, room, 10, 2), want, name);

console.log("overflow ok");
