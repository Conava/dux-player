// node quickshell/tests/settings-search.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const dir = path.join(__dirname, "../Modules/Settings");
const source = fs.readFileSync(path.join(dir, "SettingsSearch.js"), "utf8").replace(".pragma library", "");
const S = new Function(source + "; return { search };")();

const entries = [
    { page: "playback", title: "Playback", isPage: true },
    { page: "playback", pageLabel: "Playback", section: "Transitions", title: "Crossfade duration", keywords: "fade" },
    { page: "playback", pageLabel: "Playback", section: "Transitions", title: "Crossfade", keywords: "fade" },
    { page: "playback", pageLabel: "Playback", section: "Volume", title: "Normalize volume", text: "Uses the ReplayGain values in the files. Off Track Album", keywords: "loudness" },
    { page: "appearance", pageLabel: "Appearance", section: "Font", title: "Text size", keywords: "font scale" },
    { page: "lyrics", pageLabel: "Lyrics", section: "Display", title: "Text size", keywords: "font" },
    { page: "general", pageLabel: "General", section: "Background", title: "Show tray icon", text: "Control playback from the system tray." }
];
const titles = (query) => S.search(entries, query).map(e => e.page + ":" + e.title);

// query, expected results in order
const cases = [
    ["", []],
    ["   ", []],
    ["CROSSFADE", ["playback:Crossfade", "playback:Crossfade duration"]],
    ["  cross  fade", ["playback:Crossfade duration", "playback:Crossfade"]],
    ["volume", ["playback:Normalize volume"]],
    ["loudness", ["playback:Normalize volume"]],
    ["lyrics size", ["lyrics:Text size"]],
    ["text size", ["appearance:Text size", "lyrics:Text size"]],
    ["transitions", ["playback:Crossfade duration", "playback:Crossfade"]],
    ["playback", ["playback:Playback", "playback:Crossfade duration", "playback:Crossfade", "playback:Normalize volume", "general:Show tray icon"]],
    // description and option labels, below title, keyword, section and page matches
    ["replaygain", ["playback:Normalize volume"]],
    ["album", ["playback:Normalize volume"]],
    ["system tray", ["general:Show tray icon"]],
    // forgiving: word endings and small typos
    ["crosfade", ["playback:Crossfade duration", "playback:Crossfade"]],
    ["normalizing", ["playback:Normalize volume"]],
    ["fonts", ["appearance:Text size", "lyrics:Text size"]],
    ["zzz", []],
    ["xq", []]
];
for (const [query, expected] of cases)
    assert.deepStrictEqual(titles(query), expected, JSON.stringify(query));

console.log("settings-search: ok");
