.pragma library

// Dux Shell's custom theme JSON: one palette, or "dark"/"light" palettes, optionally with a "variants" block.
// Imported files are untrusted, so only known color keys holding #rrggbb / #aarrggbb strings survive.

var COLOR_KEYS = ["primary", "primaryText", "primaryContainer", "secondary", "secondaryContainer", "tertiary", "tertiaryContainer", "surface", "surfaceText", "surfaceVariant", "surfaceVariantText", "surfaceTint", "background", "backgroundText", "outline", "outlineVariant", "surfaceContainerLowest", "surfaceContainerLow", "surfaceContainer", "surfaceContainerHigh", "surfaceContainerHighest", "inverseSurface", "inverseOnSurface", "error", "errorText", "errorContainer", "errorContainerText", "warning", "info", "success"];
var TEXT_KEYS = ["name", "matugen_type"];
var MAX_TEXT = 128;
var MAX_JSON = 262144;
var COLOR = /^#([0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;

function cleanPalette(raw) {
    if (!raw || typeof raw !== "object" || Array.isArray(raw))
        return null;
    const out = {};
    for (const key of COLOR_KEYS) {
        if (raw[key] === undefined)
            continue;
        if (typeof raw[key] !== "string" || !COLOR.test(raw[key]))
            throw new Error(key + " is not a #rrggbb color");
        out[key] = raw[key];
    }
    for (const key of TEXT_KEYS) {
        if (typeof raw[key] === "string" && raw[key].length <= MAX_TEXT && !/[\u0000-\u001f]/.test(raw[key]))
            out[key] = raw[key];
    }
    return out;
}

function hasColors(palette) {
    return !!palette && COLOR_KEYS.some(key => palette[key] !== undefined);
}

// The variants block keeps only its default choice, already merged, so the stored theme stays flat.
function variantColors(variants, mode) {
    if (!variants || typeof variants !== "object")
        return null;
    const options = Array.isArray(variants.options) ? variants.options : Array.isArray(variants.flavors) ? variants.flavors : [];
    const wanted = variants.type === "multi" ? (variants.defaults?.[mode]?.flavor ?? "") : (variants.default ?? "");
    const option = options.find(o => o && o.id === wanted) ?? options[0];
    if (!option || typeof option !== "object")
        return null;
    return cleanPalette(option[mode] ?? option.dark ?? option.light ?? null);
}

// Returns {theme} with "dark" and "light" palettes, or {error}: "empty" | "tooLarge" | "notTheme" | "noColors".
function parse(text) {
    if (typeof text !== "string" || text.length === 0)
        return {
            "error": "empty"
        };
    if (text.length > MAX_JSON)
        return {
            "error": "tooLarge"
        };
    let raw;
    try {
        raw = JSON.parse(text);
    } catch (e) {
        return {
            "error": "notTheme"
        };
    }
    if (!raw || typeof raw !== "object" || Array.isArray(raw))
        return {
            "error": "notTheme"
        };
    try {
        const split = raw.dark !== undefined || raw.light !== undefined;
        const theme = {};
        for (const mode of ["dark", "light"]) {
            const base = cleanPalette(split ? (raw[mode] ?? raw.dark ?? raw.light) : raw) ?? {};
            theme[mode] = Object.assign(base, variantColors(raw.variants, mode) ?? {});
        }
        if (!hasColors(theme.dark) && !hasColors(theme.light))
            return {
                "error": "noColors"
            };
        const name = cleanPalette(raw)?.name ?? theme.dark.name ?? "";
        theme.name = name;
        return {
            "theme": theme
        };
    } catch (e) {
        return {
            "error": "notTheme"
        };
    }
}

function palette(theme, isLight) {
    return theme ? (isLight ? theme.light : theme.dark) ?? {} : {};
}

// The shell's format, so an exported theme imports into Dux Shell and back.
function serialize(name, dark, light) {
    const pick = p => {
        const out = {
            "name": name
        };
        for (const key of COLOR_KEYS) {
            if (p[key] !== undefined)
                out[key] = String(p[key]);
        }
        return out;
    };
    return JSON.stringify({
        "dark": pick(dark),
        "light": pick(light)
    }, null, 2);
}
