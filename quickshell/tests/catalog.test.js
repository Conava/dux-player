// node quickshell/tests/catalog.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Services/Catalog.js"), "utf8").replace(".pragma library", "");
const C = new Function(source + "; return { sortBy, suffixDurations, diff, singleMove, navEntries, setPinned, track, filterText };")();

function apply(list, edit) {
    const out = list.slice();
    if (!edit)
        return out;
    if (edit.move) {
        const [from, to] = edit.move;
        out.splice(to, 0, out.splice(from, 1)[0]);
        return out;
    }
    out.splice(edit.at, edit.remove, ...edit.insert);
    return out;
}

// diff: every edit reproduces the target; single moves come out as moves
const cases = [
    [["a", "b", "c"], ["a", "b", "c"], null],
    [["a", "b", "c", "d"], ["a", "c", "d", "b"], "move"],
    [["a", "b", "c", "d"], ["d", "a", "b", "c"], "move"],
    [["a", "b", "c"], ["a", "c"], "replace"],
    [["a", "c"], ["a", "b", "c"], "replace"],
    [["a", "b", "c"], ["c", "b", "a"], "replace"],
    [[], ["a", "b"], "replace"],
    [["a", "b"], [], "replace"],
    [["a", "b"], ["b", "a"], "move"]
];
for (const [from, to, kind] of cases) {
    const edit = C.diff(from, to);
    assert.deepStrictEqual(apply(from, edit), to, JSON.stringify([from, to]));
    assert.strictEqual(edit === null ? null : edit.move ? "move" : "replace", kind, JSON.stringify([from, to]));
}


// sortBy: numbers numerically, strings by locale, ties by label, descending flips
const items = [{
        "n": 10,
        "s": "b"
    }, {
        "n": 9,
        "s": "a"
    }, {
        "n": 10,
        "s": "a"
    }];
assert.deepStrictEqual(C.sortBy(items, i => i.n, i => i.s, false).map(i => i.n + i.s), ["9a", "10a", "10b"]);
assert.deepStrictEqual(C.sortBy(items, i => i.s, i => String(i.n), true).map(i => i.n + i.s), ["10b", "10a", "9a"]);

assert.deepStrictEqual(C.suffixDurations([{
        "d": 1
    }, {
        "d": 2
    }, {}], i => i.d), [3, 2, 0, 0]);
assert.deepStrictEqual(C.singleMove([0, 2, 3, 1]), {
    "from": 1,
    "to": 3
});
// navigation: every page gets a place; unpin and pin again keeps the entry's place
const nav = C.navEntries([], ["home", "albums", "genres"], ["home", "albums"]);
assert.deepStrictEqual(nav.map(e => e.target + (e.hidden ? "-" : "")), ["home", "albums", "genres-"]);
let pinned = C.setPinned(C.setPinned(nav, "playlist", "p1", true), "page", "genres", true);
pinned = C.setPinned(pinned, "page", "home", false);
assert.strictEqual(pinned[0].hidden, true);
pinned = C.setPinned(pinned, "page", "home", true);
assert.deepStrictEqual(pinned.map(e => e.target + (e.hidden ? "-" : "")), ["home", "albums", "genres", "p1"]);
assert.deepStrictEqual(C.navEntries([{"kind": "page", "target": "albums"}], ["home", "albums"], ["home"]).map(e => e.target + (e.hidden ? "-" : "")), ["albums", "home-"]);
// artists: the daemon's list with library-scoped ids, else the single artist; no page without an id
const artistsOf = t => C.track(Object.assign({"library": "l", "id": "t"}, t)).artists;
assert.deepStrictEqual(artistsOf({"artist": "A • B", "artistId": "a", "artists": [{"id": "a", "name": "A"}, {"id": "", "name": "B"}]}), [{"id": "l:a", "name": "A"}, {"id": "", "name": "B"}]);
assert.deepStrictEqual(artistsOf({"artist": "A", "artistId": "a"}), [{"id": "l:a", "name": "A"}]);
assert.deepStrictEqual(artistsOf({"artist": "A"}), [{"id": "", "name": "A"}]);
assert.deepStrictEqual(artistsOf({}), []);
// filterText: every word must match some name, in any case; an empty text keeps everything
const rows = [
    {"title": "Blue Monday", "artist": "New Order", "album": "Substance"},
    {"name": "Road trip"},
    {"title": "Ceremony", "artists": [{"id": "", "name": "Joy Division"}]}
];
const titles = (text) => C.filterText(rows, text).map(r => r.title ?? r.name);
assert.deepStrictEqual(titles(""), ["Blue Monday", "Road trip", "Ceremony"]);
assert.deepStrictEqual(titles("  "), ["Blue Monday", "Road trip", "Ceremony"]);
assert.deepStrictEqual(titles("blue order"), ["Blue Monday"]);
assert.deepStrictEqual(titles("SUBSTANCE"), ["Blue Monday"]);
assert.deepStrictEqual(titles("trip"), ["Road trip"]);
assert.deepStrictEqual(titles("joy"), ["Ceremony"]);
assert.deepStrictEqual(titles("blue joy"), []);
console.log("catalog.test.js: ok");
