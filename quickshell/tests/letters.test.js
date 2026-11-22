// node quickshell/tests/letters.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Common/Letters.js"), "utf8").replace(".pragma library", "");
const L = new Function(source + "; return { letterOf, index };")();

const firsts = list => Object.fromEntries(list.filter(e => e.first >= 0).map(e => [e.letter, e.first]));

// a mixed list: diacritics fold, digits go to #, Ø stays its own letter, other scripts are appended
const values = ["Ábba", "abc", "Björk", "2pac", "Ølafur", "Мумий"];
const asc = L.index(values, false);
assert.deepStrictEqual(asc.map(e => e.letter).slice(0, 27), ["#"].concat("ABCDEFGHIJKLMNOPQRSTUVWXYZ".split("")));
assert.deepStrictEqual(asc.map(e => e.letter).slice(26), ["Z", "Ø", "М"]);
assert.deepStrictEqual(firsts(asc), { "#": 3, A: 0, B: 2, "Ø": 4, "М": 5 });
assert.strictEqual(asc.find(e => e.letter === "C").first, -1);

// descending reverses the whole order, # last, with the same first indexes of the reversed list
const desc = L.index(values.slice().reverse(), true);
assert.deepStrictEqual(desc.map(e => e.letter), asc.map(e => e.letter).reverse());
assert.strictEqual(desc[desc.length - 1].letter, "#");
assert.deepStrictEqual(firsts(desc), { "#": 2, A: 4, B: 3, "Ø": 1, "М": 0 });

// other letters follow the list's own order, so a descending list shows them last to first
const mixed = L.index(["Мумий", "Ølafur", "Zed"], true).map(e => e.letter);
assert.deepStrictEqual(mixed.slice(0, 2), ["М", "Ø"]);
assert.strictEqual(mixed[mixed.length - 1], "#");

// letterOf edge cases
const cases = [["", "#"], [null, "#"], ["  x", "#"], ["_x", "#"], ["éclair", "E"], ["Çelik", "C"], ["日本", "日"], ["가", "가"], ["The Who", "T"], ["😀 party", "#"], ["★ star", "#"], ["→", "#"]];
for (const [text, expected] of cases)
    assert.strictEqual(L.letterOf(text), expected, String(text));

console.log("letters ok");
