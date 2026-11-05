// node quickshell/tests/seektrack.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Common/SeekTrack.js"), "utf8").replace(".pragma library", "");
const S = new Function(source + "; return { ratioAt, handleCenter };")();

// a 104 px track with a 4 px handle: the handle's centre runs from 2 to 102
const cases = [
    ["left end", 0, 0],
    ["the handle's half width is the start", 2, 0],
    ["middle", 52, 0.5],
    ["width minus the half width is the end", 102, 1],
    ["right end", 104, 1],
    ["past the end clamps", 200, 1]
];
for (const [name, x, ratio] of cases) {
    assert.strictEqual(S.ratioAt(x, 104, 2), ratio, name);
    if (x >= 2 && x <= 102)
        assert.strictEqual(S.handleCenter(ratio, 104, 2), x, name + " (back)");
}
assert.strictEqual(S.ratioAt(1, 2, 2), 0, "no travel");

console.log("seektrack ok");
