// node quickshell/tests/position.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Services/PositionClock.js"), "utf8").replace(".pragma library", "");
const C = new Function(source + "; return { create, predict, sync };")();
const opts = { "jump": false, "jumpAt": 1.5, "easeMs": 600 };
const near = (a, b, msg) => assert.ok(Math.abs(a - b) < 1e-3, msg + ": " + a + " vs " + b);

// a running clock moves with the rate; a paused one stands still
let c = C.sync(C.create(0), { "position": 10, "rate": 1.5, "running": true }, 1000, Object.assign({}, opts, { "jump": true }));
near(C.predict(c, 3000, 600), 13, "running at 1.5x");
c = C.sync(c, { "position": 13, "rate": 1, "running": false }, 3000, opts);
near(C.predict(c, 9000, 600), 13, "paused");

// a small correction doesn't move what shows, then fades into the report
c = C.sync(C.create(0), { "position": 20, "rate": 1, "running": true }, 0, Object.assign({}, opts, { "jump": true }));
const before = C.predict(c, 1000, 600);
c = C.sync(c, { "position": 20.6, "rate": 1, "running": true }, 1000, opts);
near(C.predict(c, 1000, 600), before, "no jump on a small correction");
near(C.predict(c, 7000, 600), 26.6, "eased into the report");

// the report's own timestamp counts: measured 200 ms ago means 200 ms further now
c = C.sync(C.create(0), { "position": 5, "at": 800, "rate": 1, "running": true }, 1000, Object.assign({}, opts, { "jump": true }));
near(C.predict(c, 1000, 600), 5.2, "report timestamp");

// a seek or a big correction jumps
c = C.sync(C.create(0), { "position": 20, "rate": 1, "running": true }, 0, Object.assign({}, opts, { "jump": true }));
c = C.sync(c, { "position": 90, "rate": 1, "running": true }, 1000, opts);
near(C.predict(c, 1000, 600), 90, "big correction jumps");
c = C.sync(c, { "position": 90.5, "rate": 1, "running": true }, 1000, Object.assign({}, opts, { "jump": true }));
near(C.predict(c, 1000, 600), 90.5, "jump flag");

// a track that may still be loading runs at most lead seconds ahead of its start
c = C.sync(C.create(0), { "position": 0, "rate": 1, "running": true, "lead": 1.25 }, 0, Object.assign({}, opts, { "jump": true }));
near(C.predict(c, 5000, 600), 1.25, "capped while unconfirmed");
c = C.sync(c, { "position": 1.04, "rate": 1, "running": true }, 5000, opts);
near(C.predict(c, 5000, 600), 1.25, "no jump back at the first report");
near(C.predict(c, 12000, 600), 8.04, "runs freely once confirmed");

console.log("position: ok");
