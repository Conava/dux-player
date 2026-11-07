// node quickshell/tests/columndrag.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Common/ColumnDrag.js"), "utf8").replace(".pragma library", "");
const D = new Function(source + "; return { resize, stored };")();

// what the layout makes of the sizes: fill columns share what the fixed ones leave
const layout = (cols, total) => {
    const fixed = cols.filter(c => !c.fill).reduce((n, c) => n + c.base * c.size, 0);
    const weights = cols.filter(c => c.fill).reduce((n, c) => n + c.base * c.size, 0);
    return cols.map(c => c.fill ? (total - fixed) * c.base * c.size / weights : c.base * c.size);
};
const make = (specs, total) => {
    const cols = specs.map(s => Object.assign({ "size": 1, "min": 0, "px": 0 }, s));
    layout(cols, total).forEach((px, i) => cols[i].px = px);
    return cols;
};
const near = (a, b, msg) => assert.ok(Math.abs(a - b) < 0.01, msg + ": " + a + " vs " + b);

// the border follows the pointer and no other column moves, for every kind of pair
for (const [i, dx] of [[0, 60], [0, -60], [1, 40], [2, -25], [3, 30], [3, -30]]) {
    const cols = make([{ "fill": true, "base": 3 }, { "fill": true, "base": 2 }, { "fill": false, "base": 120 }, { "fill": false, "base": 80 }, { "fill": true, "base": 2, "size": 1.5 }], 1000);
    const before = layout(cols, 1000);
    const r = D.resize(cols, i, dx);
    near(r.dx, dx, "free move " + i);
    cols[i].size = r.left;
    cols[i + 1].size = r.right;
    const after = layout(cols, 1000);
    after.forEach((px, k) => near(px, before[k] + (k === i ? dx : k === i + 1 ? -dx : 0), "column " + k + " of pair " + i));
}

// the stored sizes keep the border where it was released, on a list as wide as a 4K window
{
    const wide = make([{ "fill": true, "base": 3 }, { "fill": true, "base": 2 }, { "fill": true, "base": 2 }], 3600);
    const r = D.resize(wide, 0, 137);
    const held = wide.map(c => Object.assign({}, c));
    held[0].size = r.left;
    held[1].size = r.right;
    const saved = wide.map(c => Object.assign({}, c));
    saved[0].size = D.stored(r.left);
    saved[1].size = D.stored(r.right);
    assert.ok(Math.abs(layout(saved, 3600)[0] - layout(held, 3600)[0]) < 0.5, "border stays within a pixel");
}

// a drag stops where a size would leave its range or a column its minimum width
let cols = make([{ "fill": true, "base": 3, "min": 450 }, { "fill": true, "base": 2 }], 1000);
let r = D.resize(cols, 0, 5000);
assert.ok(r.right >= 0.4 - 1e-9 && r.left <= 3, "sizes in range");
near(r.right, 0.4, "the far side stops at the smallest size");
near(r.dx, 240, "and the border stops with it");
assert.ok(r.dx < 5000 && r.dx > 0);
r = D.resize(cols, 0, -5000);
assert.ok(cols[0].px + r.dx >= 450 - 0.01, "title keeps its minimum");
assert.ok(r.dx < -100, "and still moves up to it");

// a lone fill column beside a fixed one changes no size of its own
cols = make([{ "fill": true, "base": 3 }, { "fill": false, "base": 120 }], 1000);
r = D.resize(cols, 0, 50);
near(r.left, 1, "lone fill keeps its size");
near(r.right, (120 - 50) / 120, "fixed shrinks");
console.log("columndrag ok");
