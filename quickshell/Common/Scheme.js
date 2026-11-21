.pragma library
.import "../DCommon/Common/Hct.js" as Hct

// Material tonal-spot schemes from one hue or seed color, in the custom-theme key names (primary,
// primaryText, surface, surfaceContainer, …) that Dux themes use.
const PRIMARY_CHROMA = 48;

function scheme(hue, isLight) {
    const tone = (chroma, h, dark, light) => Hct.fromHct(h, chroma, isLight ? light : dark).toString();
    const p = (d, l) => tone(PRIMARY_CHROMA, hue, d, l);
    const s = (d, l) => tone(16, hue, d, l);
    const t = (d, l) => tone(24, (hue + 60) % 360, d, l);
    const n = (d, l) => tone(4, hue, d, l);
    const nv = (d, l) => tone(8, hue, d, l);
    return {
        "primary": p(80, 40),
        "primaryText": p(20, 100),
        "primaryContainer": p(30, 90),
        "onPrimaryContainer": p(90, 10),
        "secondary": s(80, 40),
        "secondaryContainer": s(30, 90),
        "onSecondaryContainer": s(90, 10),
        "tertiary": t(80, 40),
        "tertiaryContainer": t(30, 90),
        "onTertiaryContainer": t(90, 10),
        "surface": n(6, 98),
        "surfaceText": n(90, 10),
        "background": n(6, 98),
        "backgroundText": n(90, 10),
        "surfaceVariant": nv(30, 90),
        "surfaceVariantText": nv(80, 30),
        "surfaceTint": p(80, 40),
        "outline": nv(60, 50),
        "outlineVariant": nv(30, 80),
        "surfaceContainerLowest": n(4, 100),
        "surfaceContainerLow": n(10, 96),
        "surfaceContainer": n(12, 94),
        "surfaceContainerHigh": n(17, 92),
        "surfaceContainerHighest": n(22, 90),
        "inverseSurface": n(90, 20),
        "inverseOnSurface": n(20, 95),
        "inversePrimary": p(40, 80)
    };
}

// seed: a color or "#rrggbb" string
function fromSeed(seed, isLight) {
    return scheme(Hct.toHct(Qt.color(seed)).hue, isLight);
}

// fromSeed's primaryContainer alone. neutralAware lets a near-grey seed give a near-grey result (its chroma
// times 2, up to the scheme's own) instead of a saturated colour on an arbitrary hue.
function primaryContainer(seed, isLight, neutralAware) {
    const hct = Hct.toHct(Qt.color(seed));
    const chroma = neutralAware ? Math.min(PRIMARY_CHROMA, hct.chroma * 2) : PRIMARY_CHROMA;
    return Hct.fromHct(hct.hue, chroma, isLight ? 90 : 30).toString();
}
