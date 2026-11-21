// node quickshell/tests/scheme.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const read = f => fs.readFileSync(path.join(__dirname, f), "utf8").replace(".pragma library", "");
const hct = read("../DCommon/Common/Hct.js");
const scheme = read("../Common/Scheme.js").replace(/\.import .*\n/, "");
const hex = h => ({ r: parseInt(h.slice(1, 3), 16) / 255, g: parseInt(h.slice(3, 5), 16) / 255, b: parseInt(h.slice(5, 7), 16) / 255 });
const S = new Function("Qt", `const Hct = (() => { ${hct}; return { fromHct, toHct }; })(); ${scheme}; return { fromSeed, primaryContainer };`)({ color: hex, rgba: (r, g, b) => ({ r, g, b, toString: () => "#" + [r, g, b].map(v => Math.round(v * 255).toString(16).padStart(2, "0")).join("") }) });
const H = new Function(hct + "; return { toHct };")();

for (const isLight of [true, false]) {
    // a coloured seed: the same container fromSeed builds
    assert.strictEqual(S.primaryContainer("#d03a2c", isLight), S.fromSeed("#d03a2c", isLight).primaryContainer);
    assert.strictEqual(S.primaryContainer("#d03a2c", isLight, true), S.primaryContainer("#d03a2c", isLight));
    // a grey seed: grey, not the saturated tint of an arbitrary hue
    for (const grey of ["#808080", "#000000", "#ffffff", "#7a7d7c"])
        assert.ok(H.toHct(hex(S.primaryContainer(grey, isLight, true))).chroma < 10, grey);
}
