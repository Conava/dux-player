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

// descending reverses the whole order, # last, with the same first indexes
const desc = L.index(values, true);
assert.deepStrictEqual(desc.map(e => e.letter), asc.map(e => e.letter).reverse());
assert.strictEqual(desc[desc.length - 1].letter, "#");
assert.deepStrictEqual(firsts(desc), firsts(asc));

// letterOf edge cases
const cases = [["", "#"], [null, "#"], ["  x", "#"], ["_x", "#"], ["éclair", "E"], ["Çelik", "C"], ["日本", "日"], ["가", "가"], ["The Who", "T"]];
for (const [text, expected] of cases)
    assert.strictEqual(L.letterOf(text), expected, String(text));

console.log("letters ok");
