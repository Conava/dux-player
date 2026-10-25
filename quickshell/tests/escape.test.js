// node quickshell/tests/escape.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Common/EscapeStack.js"), "utf8").replace(".pragma library", "");
const E = new Function(source + "; return { raise, drop, top };")();

// steps: [op, layer]; shown: the layers whose owner shows; expected: the one Escape closes
const cases = [
    ["the last activated", [["raise", "selection"], ["raise", "edit"]], null, "edit"],
    ["activated again goes back on top", [["raise", "selection"], ["raise", "edit"], ["raise", "selection"]], null, "selection"],
    ["closed ones leave", [["raise", "selection"], ["raise", "edit"], ["drop", "edit"]], null, "selection"],
    ["a hidden one is passed over", [["raise", "selection"], ["raise", "view"]], ["selection"], "selection"],
    ["nothing open", [["raise", "edit"], ["drop", "edit"], ["drop", "edit"]], null, null]
];
for (const [name, steps, shown, expected] of cases) {
    let stack = [];
    for (const [op, layer] of steps)
        stack = E[op](stack, layer);
    assert.strictEqual(E.top(stack, l => shown === null || shown.indexOf(l) >= 0), expected, name);
}

console.log("escape: ok");
