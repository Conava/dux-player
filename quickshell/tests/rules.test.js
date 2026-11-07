// node quickshell/tests/rules.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Services/Rules.js"), "utf8").replace(".pragma library", "");
const R = new Function(source + "; return { fields, ops, shape, active, uses, onlyFavorites, newRule, withField, withOp, compile, picked };")();

const now = 1000 * 86400;
const daysAgo = n => now - n * 86400;
const track = (id, patch) => Object.assign({
    "id": id,
    "title": "",
    "artist": "",
    "album": "",
    "albumArtist": "",
    "composer": "",
    "genre": "",
    "library": "a",
    "format": "",
    "year": 0,
    "duration": 0,
    "rating": 0,
    "playCount": 0,
    "bitrate": 0,
    "added": 0,
    "lastPlayed": 0,
    "favorite": false
}, patch);

const tracks = [
    track("rock", { "title": "Thunder Road", "artist": "Ärger", "genre": "Rock", "library": "a", "format": "FLAC", "year": 1975, "duration": 190, "rating": 5, "playCount": 10, "added": daysAgo(5), "lastPlayed": daysAgo(1), "favorite": true }),
    track("metal", { "title": "thunderstruck", "artist": "ACDC", "genre": "Metal", "library": "b", "format": "MP3", "year": 1990, "duration": 240, "rating": 3, "added": daysAgo(50), "lastPlayed": daysAgo(100) }),
    track("mixed", { "title": "Hybrid", "composer": "Bach", "genre": "Rock; Metal", "library": "b", "format": "flac", "year": 2000, "duration": 239, "added": daysAgo(200) }),
    track("bare", {})
];

const names = (rules, type, list) => {
    const test = R.compile(rules, type, now);
    return (test ? list.filter(test) : list).map(i => i.id);
};
const one = (field, op, value) => ({
    "match": "all",
    "list": [{ field, op, value }]
});

const cases = [
    ["contains ignores case", "title", "contains", "THUNDER", ["rock", "metal"]],
    ["contains ignores case beyond ASCII", "artist", "contains", "ärg", ["rock"]],
    ["contains trims the value", "title", "contains", " hybrid ", ["mixed"]],
    ["doesn't contain", "title", "notContains", "thunder", ["mixed", "bare"]],
    ["is takes the whole text", "title", "is", "thunder road", ["rock"]],
    ["is not", "title", "isNot", "thunder road", ["metal", "mixed", "bare"]],
    ["starts with", "title", "startsWith", "thunders", ["metal"]],
    ["is empty", "composer", "isEmpty", undefined, ["rock", "metal", "bare"]],
    ["is not empty", "composer", "isNotEmpty", undefined, ["mixed"]],
    ["is one of matches whole tags", "genre", "isAny", ["rock", "METAL"], ["rock", "metal"]],
    ["is none of", "genre", "isNone", ["Rock"], ["metal", "mixed", "bare"]],
    ["pick contains reaches into a joined tag", "genre", "contains", "metal", ["metal", "mixed"]],
    ["library is one of", "library", "isAny", ["b"], ["metal", "mixed"]],
    ["format ignores case", "format", "isAny", ["flac"], ["rock", "mixed"]],
    ["number is", "year", "is", 1990, ["metal"]],
    ["number is not", "year", "isNot", 1990, ["rock", "mixed", "bare"]],
    ["at least", "year", "atLeast", 1990, ["metal", "mixed"]],
    ["at most", "year", "atMost", 1975, ["rock", "bare"]],
    ["between holds both ends", "year", "between", [1975, 1990], ["rock", "metal"]],
    ["between takes its ends in any order", "year", "between", [1990, 1975], ["rock", "metal"]],
    ["rating 0 is unrated", "rating", "is", 0, ["mixed", "bare"]],
    ["duration at least is in minutes", "duration", "atLeast", 4, ["metal"]],
    ["duration at most is in minutes", "duration", "atMost", 3.5, ["rock", "bare"]],
    ["duration between is in minutes", "duration", "between", [3, 3.5], ["rock"]],
    ["duration is means the shown minutes", "duration", "is", 3, ["rock", "mixed"]],
    ["in the last days", "added", "inLast", 60, ["rock", "metal"]],
    ["added not in the last skips an unknown date", "added", "notInLast", 60, ["mixed"]],
    ["last played in the last skips never played", "lastPlayed", "inLast", 2000, ["rock", "metal"]],
    ["last played not in the last holds never played", "lastPlayed", "notInLast", 30, ["metal", "mixed", "bare"]],
    ["never", "lastPlayed", "never", undefined, ["mixed", "bare"]],
    ["flag yes", "favorite", "is", true, ["rock"]],
    ["flag no", "favorite", "is", false, ["metal", "mixed", "bare"]]
];
for (const [name, field, op, value, expected] of cases)
    assert.deepStrictEqual(names(one(field, op, value), "tracks", tracks), expected, name);

// All and Any
const pair = {
    "list": [{ "field": "genre", "op": "isAny", "value": ["Rock"] }, { "field": "year", "op": "atLeast", "value": 1990 }]
};
assert.deepStrictEqual(names(Object.assign({ "match": "all" }, pair), "tracks", tracks), [], "all needs every rule");
assert.deepStrictEqual(names(Object.assign({ "match": "any" }, pair), "tracks", tracks), ["rock", "metal", "mixed"], "any needs one rule");
assert.deepStrictEqual(names(pair, "tracks", tracks), [], "a missing match is all");

// rules that don't filter
const idle = [
    ["no rules", undefined],
    ["a list that isn't one", { "match": "all", "list": "genre" }],
    ["an empty list", { "match": "all", "list": [] }],
    ["a rule that isn't an object", { "list": [null, 3, "x"] }],
    ["an unknown field", one("mood", "contains", "sad")],
    ["an unknown condition", one("title", "matches", "x")],
    ["a field name from the prototype", one("constructor", "contains", "x")],
    ["a condition of another kind", one("year", "contains", "19")],
    ["a text rule without text", one("title", "contains", "  ")],
    ["a list without entries", one("genre", "isAny", [])],
    ["a list of the wrong type", one("genre", "isAny", [3])],
    ["a text value on a number", one("year", "atLeast", "1990")],
    ["an infinite number", one("year", "atLeast", Infinity)],
    ["half a range", one("year", "between", [1990, null])],
    ["zero days", one("added", "inLast", 0)],
    ["a text value on a flag", one("favorite", "is", "yes")],
    ["never on date added", one("added", "never", undefined)],
    ["contains on a library", one("library", "contains", "a")]
];
for (const [name, rules] of idle) {
    assert.strictEqual(R.compile(rules, "tracks", now), null, name);
    assert.strictEqual(R.active(rules, "tracks").length, 0, name);
}
assert.strictEqual(R.compile(one("title", "contains", "x"), "playlists", now), null, "a type without rules");
assert.deepStrictEqual(names({
    "match": "all",
    "list": [{ "field": "mood", "op": "is", "value": "sad" }, { "field": "year", "op": "is", "value": 1975 }]
}, "tracks", tracks), ["rock"], "a rule from a newer version leaves the others working");

// a list from outside may lack a field
assert.deepStrictEqual(names(one("title", "contains", "x"), "tracks", [{ "id": "none" }]), [], "text on a missing field");
assert.deepStrictEqual(names(one("year", "atMost", 5), "tracks", [{ "id": "none" }]), ["none"], "number on a missing field");

// types
const albums = [
    { "id": "big", "title": "Big", "artist": "X", "trackCount": 20, "genre": "Rock", "library": "a" },
    { "id": "small", "title": "Small", "artist": "Y", "trackCount": 2, "genre": "Pop", "library": "b" }
];
assert.deepStrictEqual(names(one("trackCount", "atLeast", 10), "albums", albums), ["big"], "album track count");
assert.deepStrictEqual(names(one("library", "isAny", ["b"]), "albums", albums), ["small"], "album library");
assert.strictEqual(R.compile(one("composer", "contains", "x"), "albums", now), null, "albums have no composer");

const artists = [
    { "id": "two", "name": "Two", "genres": ["Rock", "Pop"], "libraries": ["a", "b"], "albumCount": 2 },
    { "id": "solo", "name": "Solo", "genres": ["Jazz"], "libraries": ["b"], "albumCount": 1 },
    { "id": "nothing", "name": "Nothing", "genres": [], "libraries": [], "albumCount": 0 }
];
assert.deepStrictEqual(names(one("genre", "isAny", ["pop"]), "artists", artists), ["two"], "an artist is in a genre through any album");
assert.deepStrictEqual(names(one("genre", "isNone", ["pop"]), "artists", artists), ["solo", "nothing"], "is none of covers every album");
assert.deepStrictEqual(names(one("library", "isAny", ["a"]), "artists", artists), ["two"], "an artist's libraries");
assert.deepStrictEqual(names(one("name", "startsWith", "s"), "artists", artists), ["solo"], "artist name");

// what the query needs to know
assert.strictEqual(R.uses(one("genre", "isAny", ["Rock"]), "artists", "genre"), true, "uses a field");
assert.strictEqual(R.uses(one("genre", "isAny", []), "artists", "genre"), false, "a rule without a value uses nothing");
const yes = { "field": "favorite", "op": "is", "value": true };
const year = { "field": "year", "op": "is", "value": 1975 };
assert.strictEqual(R.onlyFavorites({ "match": "all", "list": [yes, year] }, "tracks"), true, "favorite in a match-all list");
assert.strictEqual(R.onlyFavorites({ "match": "any", "list": [yes, year] }, "tracks"), false, "any can let others through");
assert.strictEqual(R.onlyFavorites({ "match": "any", "list": [yes] }, "tracks"), true, "any of one rule is that rule");
assert.strictEqual(R.onlyFavorites(one("favorite", "is", false), "tracks"), false, "favorite is No");

// editing
assert.deepStrictEqual(R.newRule("artists"), { "field": "name", "op": "contains" }, "a new rule starts empty on the first field");
assert.strictEqual(R.active({ "list": [R.newRule("tracks")] }, "tracks").length, 0, "a new rule does not filter");
for (const type of ["tracks", "albums", "artists"]) {
    for (const field of R.fields(type)) {
        assert.ok(R.ops(type, field).length > 0, field);
        for (const op of R.ops(type, field))
            assert.notStrictEqual(R.shape(type, { field, op }), "", field + " " + op);
    }
}
assert.deepStrictEqual(R.withField("tracks", { "field": "title", "op": "is", "value": "x" }, "artist"), { "field": "artist", "op": "is", "value": "x" }, "the same kind of field keeps condition and value");
assert.deepStrictEqual(R.withField("tracks", { "field": "title", "op": "contains", "value": "x" }, "year"), { "field": "year", "op": "is" }, "another kind starts over");
assert.deepStrictEqual(R.withField("tracks", { "field": "year", "op": "is", "value": 3 }, "favorite"), { "field": "favorite", "op": "is" }, "the same condition with another value drops it");
assert.deepStrictEqual(R.withOp("tracks", { "field": "year", "op": "is", "value": 3 }, "atLeast"), { "field": "year", "op": "atLeast", "value": 3 }, "the same value shape stays");
assert.deepStrictEqual(R.withOp("tracks", { "field": "year", "op": "is", "value": 3 }, "between"), { "field": "year", "op": "between" }, "another value shape goes");
assert.deepStrictEqual(R.withOp("tracks", { "field": "genre", "op": "isAny", "value": ["x"] }, "isNone"), { "field": "genre", "op": "isNone", "value": ["x"] }, "is none of keeps the picks");
assert.deepStrictEqual(R.withField("tracks", { "field": "added", "op": "inLast", "value": 7 }, "lastPlayed"), { "field": "lastPlayed", "op": "inLast", "value": 7 }, "days stay across date fields");
assert.deepStrictEqual(R.withField("tracks", { "field": "lastPlayed", "op": "never" }, "added"), { "field": "added", "op": "inLast" }, "never doesn't reach date added");

console.log("rules tests passed");

// a scalar on a list rule from a hand-edited file reads as no picks
assert.deepStrictEqual(R.picked("Rock"), []);
assert.deepStrictEqual(R.picked(undefined), []);
assert.deepStrictEqual(R.picked(["a"]), ["a"]);
