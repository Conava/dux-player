// node quickshell/tests/scheme.test.js
// scheme-fixture.json: "tonalSpot" is fromSeed's output before palette styles existed; "styles" was
// generated once from @material/material-color-utilities 0.4.0 (npm): for each style, seed and mode,
// DynamicScheme (2021 spec, contrast 0) palette.tone(t) at this file's tone for each role, and
// MaterialDynamicColors for the roles the Monochrome spec gives their own tones.
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const read = f => fs.readFileSync(path.join(__dirname, f), "utf8").replace(".pragma library", "");
const hct = read("../DCommon/Common/Hct.js");
const scheme = read("../Common/Scheme.js").replace(/\.import .*\n/, "");
const hex = h => ({ r: parseInt(h.slice(1, 3), 16) / 255, g: parseInt(h.slice(3, 5), 16) / 255, b: parseInt(h.slice(5, 7), 16) / 255 });
const S = new Function("Qt", `const Hct = (() => { ${hct}; return { fromHct, toHct }; })(); ${scheme}; return { fromSeed, primaryContainer };`)({ color: hex, rgba: (r, g, b) => ({ r, g, b, toString: () => "#" + [r, g, b].map(v => Math.round(v * 255).toString(16).padStart(2, "0")).join("") }) });
const H = new Function(hct + "; return { toHct };")();
const fixture = JSON.parse(fs.readFileSync(path.join(__dirname, "scheme-fixture.json"), "utf8"));
const modes = { dark: false, light: true };

for (const isLight of [true, false]) {
    // a coloured seed: the same container fromSeed builds
    assert.strictEqual(S.primaryContainer("#d03a2c", isLight), S.fromSeed("#d03a2c", isLight).primaryContainer);
    assert.strictEqual(S.primaryContainer("#d03a2c", isLight, true), S.primaryContainer("#d03a2c", isLight));
    // a grey seed: grey, not the saturated tint of an arbitrary hue
    for (const grey of ["#808080", "#000000", "#ffffff", "#7a7d7c"])
        assert.ok(H.toHct(hex(S.primaryContainer(grey, isLight, true))).chroma < 10, grey);
}

// tonal spot, a missing style and an unknown one keep the palette from before palette styles
for (const seed of fixture.seeds)
    for (const mode in modes)
        for (const style of [undefined, "tonalSpot", "rainbow"]) {
            const out = S.fromSeed(seed, modes[mode], style);
            assert.deepStrictEqual(Object.keys(out), fixture.roles);
            assert.deepStrictEqual(fixture.roles.map(k => out[k]), fixture.tonalSpot[seed][mode], `${seed} ${mode} ${style}`);
        }

// every other style matches Material Color Utilities role by role
for (const style in fixture.styles)
    for (const seed of fixture.seeds)
        for (const mode in modes) {
            const out = S.fromSeed(seed, modes[mode], style);
            assert.deepStrictEqual(Object.keys(out), fixture.roles);
            assert.deepStrictEqual(fixture.roles.map(k => out[k]), fixture.styles[style][seed][mode], `${style} ${seed} ${mode}`);
        }

// Monochrome tones from upstream's dart/test/scheme_monochrome_test.dart (seed 0xff0000ff)
const monochrome = {
    dark: { primary: 100, primaryText: 10, primaryContainer: 85, onPrimaryContainer: 0, secondary: 80, secondaryContainer: 30, onSecondaryContainer: 90, tertiary: 90, tertiaryContainer: 60, onTertiaryContainer: 0 },
    light: { primary: 0, primaryText: 90, primaryContainer: 25, onPrimaryContainer: 100, secondary: 40, secondaryContainer: 85, onSecondaryContainer: 10, tertiary: 25, tertiaryContainer: 49, onTertiaryContainer: 100 }
};
for (const mode in monochrome) {
    const out = S.fromSeed("#0000ff", modes[mode], "monochrome");
    for (const role in monochrome[mode])
        assert.ok(Math.abs(H.toHct(hex(out[role])).tone - monochrome[mode][role]) <= 1, `${mode} ${role}`);
}
