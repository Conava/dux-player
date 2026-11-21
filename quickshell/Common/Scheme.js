.pragma library
.import "../DCommon/Common/Hct.js" as Hct

// Material schemes from one hue or seed color, in the custom-theme key names (primary, primaryText,
// surface, surfaceContainer, …) that Dux themes use. Tonal spot is Dux's own (more chroma than
// Material's); the other styles are Material Color Utilities' 2021 variants.
const PRIMARY_CHROMA = 48;

const VIBRANT_HUES = [0, 41, 61, 101, 131, 181, 251, 301, 360];
const EXPRESSIVE_HUES = [0, 21, 51, 121, 151, 191, 271, 321, 360];

// MCU's DynamicScheme.getRotatedHue: lower breakpoint inclusive, upper exclusive, no match keeps the hue.
function rotatedHue(hue, breakpoints, rotations) {
    for (let i = 0; i < breakpoints.length - 1; i++)
        if (hue >= breakpoints[i] && hue < breakpoints[i + 1])
            return (hue + rotations[i]) % 360;
    return hue;
}

// [hue, chroma] of the primary, secondary, tertiary, neutral and neutral variant palettes
function palettes(hue, style) {
    const h = d => (hue + d) % 360;
    switch (style) {
    case "vibrant":
        return [[hue, 200], [rotatedHue(hue, VIBRANT_HUES, [18, 15, 10, 12, 15, 18, 15, 12, 12]), 24], [rotatedHue(hue, VIBRANT_HUES, [35, 30, 20, 25, 30, 35, 30, 25, 25]), 32], [hue, 10], [hue, 12]];
    case "expressive":
        return [[h(240), 40], [rotatedHue(hue, EXPRESSIVE_HUES, [45, 95, 45, 20, 45, 90, 45, 45, 45]), 24], [rotatedHue(hue, EXPRESSIVE_HUES, [120, 120, 20, 45, 20, 15, 20, 120, 120]), 32], [h(15), 8], [h(15), 12]];
    case "neutral":
        return [[hue, 12], [hue, 8], [hue, 16], [hue, 2], [hue, 2]];
    case "monochrome":
        return [[hue, 0], [hue, 0], [hue, 0], [hue, 0], [hue, 0]];
    default:
        return [[hue, PRIMARY_CHROMA], [hue, 16], [h(60), 24], [hue, 4], [hue, 8]];
    }
}

// style: "tonalSpot" (also anything unknown), "vibrant", "expressive", "neutral" or "monochrome"
function scheme(hue, isLight, style) {
    const pal = palettes(hue, style);
    const mono = style === "monochrome";
    const palette = i => (d, l) => Hct.fromHct(pal[i][0], pal[i][1], isLight ? l : d).toString();
    const p = palette(0), s = palette(1), t = palette(2), n = palette(3), nv = palette(4);
    // the last two tones are Monochrome's own, from MCU's 2021 color spec
    const m = (f, dark, light, monoDark, monoLight) => mono ? f(monoDark, monoLight) : f(dark, light);
    return {
        "primary": m(p, 80, 40, 100, 0),
        "primaryText": m(p, 20, 100, 10, 90),
        "primaryContainer": m(p, 30, 90, 85, 25),
        "onPrimaryContainer": m(p, 90, 10, 0, 100),
        "secondary": s(80, 40),
        "secondaryContainer": m(s, 30, 90, 30, 85),
        "onSecondaryContainer": s(90, 10),
        "tertiary": m(t, 80, 40, 90, 25),
        "tertiaryContainer": m(t, 30, 90, 60, 49),
        "onTertiaryContainer": m(t, 90, 10, 0, 100),
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
function fromSeed(seed, isLight, style) {
    return scheme(Hct.toHct(Qt.color(seed)).hue, isLight, style);
}

// fromSeed's primaryContainer alone. neutralAware lets a near-grey seed give a near-grey result (its chroma
// times 2, up to the scheme's own) instead of a saturated colour on an arbitrary hue.
function primaryContainer(seed, isLight, neutralAware) {
    const hct = Hct.toHct(Qt.color(seed));
    const chroma = neutralAware ? Math.min(PRIMARY_CHROMA, hct.chroma * 2) : PRIMARY_CHROMA;
    return Hct.fromHct(hct.hue, chroma, isLight ? 90 : 30).toString();
}
