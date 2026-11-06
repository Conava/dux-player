// node quickshell/tests/lyrics-model.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Common/LyricsModel.js"), "utf8").replace(".pragma library", "");
const L = new Function(source + "; return { buildItems, centreItem, layoutRows, fillEdge, smoothEdge, isRtl };")();

const line = (t, e, x = "words", background = false) => ({ t, e, move: 0.5, parts: [{ x, background, w: [] }] });
const kinds = items => items.map(item => item.kind).join(" ");

// buildItems
assert.strictEqual(kinds(L.buildItems([line(6, 8)], 5, true)), "break line", "a 6 s intro is a break");
assert.strictEqual(kinds(L.buildItems([line(4, 8)], 5, true)), "line", "a 4 s intro is not");
assert.strictEqual(kinds(L.buildItems([line(0, 2), line(8, 9)], 5, true)), "line break line", "a 6 s gap is a break");
assert.strictEqual(kinds(L.buildItems([line(0, 2), line(6, 9)], 5, true)), "line line", "a 4 s gap is not");
const blank = L.buildItems([line(0, 3), line(3, 20, " "), line(20, 22)], 5, true);
assert.strictEqual(kinds(blank), "line break line", "a blank LRC line opens a break, not a row");
assert.deepStrictEqual([blank[1].t, blank[1].e], [3, 20], "the break spans the blank line");
assert.strictEqual(kinds(L.buildItems([line(0, 3), line(3, 4, ""), line(4, 6)], 5, true)), "line line", "a short blank line leaves no row");
const vocals = [{ t: 0, e: 2, move: 0.5, parts: [{ x: "lead", background: false }, { x: "ooh", background: true }] }, line(2, 3, "ooh", true)];
assert.strictEqual(L.buildItems(vocals, 5, true).length, 2, "background vocals on");
assert.deepStrictEqual(L.buildItems(vocals, 5, false).map(item => item.text), ["lead"], "background vocals off drops the parts and lines with nothing left");

// centreItem: a break from 2 to 8 between lines 0 and 1
const items = L.buildItems([line(0, 2), line(8, 10)], 5, true);
assert.strictEqual(L.centreItem(items, 1, 0), 0, "the line being sung");
assert.strictEqual(L.centreItem(items, 3, 0), 1, "the break wins once it starts");
assert.strictEqual(L.centreItem(items, 7.75, 1), 2, "the next line wins before it starts, once the controller moved on");
assert.strictEqual(L.centreItem(items, 7.75, 0), 1, "not before the controller moved on");

// fillEdge
const segments = [{ x0: 0, x1: 100, t: 10, e: 12 }, { x0: 100, x1: 200, t: 12, e: 13 }];
assert.strictEqual(L.fillEdge(segments, 9, true, 20), -20, "the lead waits behind the row before the first cue");
assert.strictEqual(L.fillEdge(segments, 11, true, 0), 50, "linear inside a cue");
assert.strictEqual(L.fillEdge(segments, 11, true, 20) + 20, 50, "the soft lead ends at the sung position, not past it");
assert.strictEqual(L.fillEdge(segments, 12, true, 20) + 20, 100, "the second cue starts where the first ended");
assert.strictEqual(L.fillEdge(segments, 13, true, 20), 200, "the row's last cue lights the row's end");
assert.strictEqual(L.fillEdge(segments, 20, true, 0), 200, "full after the last cue");
assert.strictEqual(L.fillEdge(segments, Infinity, true, 0), 200, "a line behind the centre is full");
assert.strictEqual(L.fillEdge(segments, 10.1, false, 0), 100, "without the sweep a started cue fills at once");
assert.strictEqual(L.smoothEdge(segments, 9, 20, 0.24), -20, "smoothing starts no earlier than half a window");
const instant = [{ x0: 0, x1: 100, t: 10, e: 10 }];
assert.ok(L.smoothEdge(instant, 10, 0, 0.24) > 0 && L.smoothEdge(instant, 10, 0, 0.24) < 100, "a cue with no length glides");

assert.strictEqual(L.isRtl("123 שלום"), true, "digits don't decide");
assert.strictEqual(L.isRtl("hello שלום"), false);

// layoutRows: one pixel per character
const measure = text => text.length;
const rows = L.layoutRows("aaa bbb", [{ start: 0, end: 4, t: 0, e: 2 }, { start: 4, end: 7, t: 2, e: 4 }], 4, measure);
assert.deepStrictEqual(rows.map(row => row.text), ["aaa", "bbb"], "wraps at the space");
assert.deepStrictEqual(rows[1].segments, [{ x0: 0, x1: 3, t: 2, e: 4 }], "a row's cues count from its own start");
assert.deepStrictEqual(L.layoutRows("aaaaaa", [], 4, measure).map(row => row.text), ["aaaa", "aa"], "a word wider than the row breaks");

console.log("lyrics-model ok");
