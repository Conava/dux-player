// node quickshell/tests/template.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Common/Template.js"), "utf8").replace(".pragma library", "");
const T = new Function(source + "; return { format };")();

// the table the Go formatter runs too, so the two can't drift
const table = JSON.parse(fs.readFileSync(path.join(__dirname, "../../core/internal/template/testdata/cases.json"), "utf8"));
for (const c of table.cases)
    assert.strictEqual(T.format(c.template, table.fields), c.want, c.name);

assert.strictEqual(T.format("%a%%a%%a%", {
    "a": "x".repeat(300)
}).length, 512, "output is capped");
console.log("template: ok");
