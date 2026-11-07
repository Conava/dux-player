.pragma library

// The customization model stored as `layout` in ui-settings.json: catalogue, defaults, cleaning, migration from
// the old keys, and checks. Pure: labels and icons live in UiConfig. The stored form leaves out what is default
// (a missing surface, zone, item type or list change means the default), and everything read from the file
// goes through clean first.

const version = 2;
const maxZone = 32;
const maxList = 200;
const maxOverrides = 2000;
const idPattern = /^[a-z0-9-]{1,32}$/;
const typePattern = /^[A-Za-z0-9]{1,32}$/;
const keyPattern = /^[A-Za-z-]{1,32}(:.{1,256})?$/;

const placements = ["auto", "keep", "overflow"];
const starts = ["bar", "side", "full"];
const styles = ["bento", "m3e"];
const openPanes = ["last", "lyrics", "queue", "cover"];
const playerLooks = ["card", "attached"];
const paneLooks = ["player", "card", "attached"];
const paneTypes = ["cover", "lyrics", "queue"];

const groups = dict({
    "trackInfo": ["cover", "title", "artists", "album", "text", "lyricLine", "libraryBadge", "playingFrom", "castTarget"],
    "transport": ["shuffle", "previous", "playPause", "next", "repeat"],
    "seek": ["elapsed", "seekbar", "duration"],
    "buttons": ["favorite", "rating", "volume", "device", "queue", "lyrics", "sleep", "speed", "expand", "dock"],
    "actions": ["addToPlaylist", "goToAlbum", "goToArtist", "details"],
    "layout": ["spacer", "divider"]
});
const groupIds = ["trackInfo", "transport", "seek", "buttons", "actions", "layout"];
const elementTypes = groupIds.reduce((all, g) => all.concat(groups[g]), []);

// catalogues looked up with keys from the file: no Object.prototype names ("constructor", "__proto__")
function dict(o) {
    return Object.assign(Object.create(null), o);
}

const shelfTypes = ["search", "continue", "favoriteTracks", "playlists", "recentTracks", "mostPlayed", "recentlyPlayed", "recentlyAdded", "randomAlbums", "favoriteAlbums", "playlist", "genre", "library", "view", "artist"];

function zone(id, kind, overflows) {
    return {
        "id": id,
        "kind": kind,
        "overflows": overflows === true
    };
}

const aboveSeek = zone("seekAbove", "line", true);
const belowSeek = zone("seekBelow", "line", true);

// In the order they show. kind: "row" | "line" (a row of its own, the width of the surface) | "column" | "seek"
// (the seekbar line, a row) | "cover" | "panes" | "shelves"
const surfaceZones = dict({
    "bar": [zone("left", "row"), aboveSeek, zone("seek", "seek"), belowSeek, zone("controls", "row"), zone("right", "row", true)],
    "side": [zone("panes", "panes"), zone("cover", "cover"), zone("title", "column"), aboveSeek, zone("seek", "seek"), belowSeek, zone("controls", "row", true), zone("top", "row", true), zone("bottom", "row", true)],
    "fullBento": [zone("cover", "cover"), zone("header", "row"), zone("title", "column"), zone("top", "row", true), aboveSeek, zone("seek", "seek"), belowSeek, zone("controls", "row", true), zone("bottom", "row", true)],
    "fullM3e": [zone("header", "row"), zone("buttons", "row", true), zone("cover", "cover"), zone("title", "column"), aboveSeek, zone("seek", "seek"), belowSeek, zone("controls", "row", true), zone("end", "row", true)],
    "home": [zone("shelves", "shelves")]
});
const surfaceIds = ["bar", "side", "fullBento", "fullM3e", "home"];
const nowPlayingSurfaces = ["bar", "side", "fullBento", "fullM3e"];
// the order the side panel reads the bar in, which decides the order of what it takes from it
const barReadOrder = ["left", "controls", "seekAbove", "seek", "seekBelow", "right"];

// Row and menu actions per media type. trailing: what End of row offers; menu: every action of the ⋮ menu
// (On hover offers the same, except keepTrack: its label depends on a lookup the hover buttons don't make)
const itemCatalog = dict({
    "track": {
        "trailing": ["favorite", "rating", "library", "cloud", "duration", "addToPlaylist", "more"],
        "menu": ["playNext", "addToQueue", "addToPlaylist", "favorite", "goToAlbum", "goToArtist", "details", "keepTrack", "removeFromPlaylist"],
        "trailingDefault": ["favorite", "library", "cloud", "duration", "more"],
        "hoverDefault": ["playNext", "addToQueue"],
        "menuDefault": ["playNext", "addToQueue", "addToPlaylist", "favorite", "divider", "goToAlbum", "goToArtist", "details", "keepTrack", "divider", "removeFromPlaylist"]
    },
    "album": {
        "trailing": ["favorite", "rating", "library", "cloud", "play", "more"],
        "menu": ["play", "shuffle", "playNext", "addToQueue", "addToPlaylist", "favorite", "goToArtist", "details", "keepOffline"],
        "trailingDefault": ["play", "more"],
        "hoverDefault": [],
        "menuDefault": ["play", "shuffle", "playNext", "addToQueue", "divider", "addToPlaylist", "favorite", "goToArtist", "details", "keepOffline"]
    },
    "artist": {
        "trailing": ["favorite", "play", "more"],
        "menu": ["play", "shuffle", "favorite", "pin", "details", "keepOffline"],
        "trailingDefault": ["more"],
        "hoverDefault": [],
        "menuDefault": ["play", "shuffle", "favorite", "pin", "details", "keepOffline"]
    },
    "playlist": {
        "trailing": ["play", "more"],
        "menu": ["play", "shuffle", "playNext", "addToQueue", "rename", "duplicate", "export", "addToHome", "pin", "keepOffline", "delete"],
        "trailingDefault": ["play", "more"],
        "hoverDefault": [],
        "menuDefault": ["play", "shuffle", "playNext", "addToQueue", "divider", "rename", "duplicate", "export", "addToHome", "pin", "keepOffline", "divider", "delete"]
    },
    "genre": {
        "trailing": ["more"],
        "menu": ["shuffle", "pin", "addToHome"],
        "trailingDefault": ["more"],
        "hoverDefault": [],
        "menuDefault": ["shuffle", "pin", "addToHome"]
    },
    "queue": {
        "trailing": ["addedByYou", "duration", "addToPlaylist", "remove", "more"],
        "menu": ["playAfter", "stopAfter", "playNext", "addToQueue", "addToPlaylist", "favorite", "goToAlbum", "goToArtist", "details", "keepTrack", "removeFromQueue"],
        "trailingDefault": ["addedByYou", "remove", "more"],
        "hoverDefault": [],
        "menuDefault": ["playAfter", "stopAfter", "addToPlaylist", "favorite", "divider", "goToAlbum", "goToArtist", "details", "keepTrack", "divider", "removeFromQueue"]
    }
});
const itemTypes = ["track", "album", "artist", "playlist", "genre", "queue"];
const itemZoneIds = ["trailing", "hover", "menu", "context"];

// List options. type: the browse type whose lines and sorts apply; multi: one change per item (each playlist);
// sections: the page's own sections, in their default order
const typeDefaults = dict({
    "tracks": {
        "lines": ["artist", "album"],
        "sort": "title"
    },
    "albums": {
        "layout": "grid",
        "lines": ["artist"],
        "sort": "title"
    },
    "artists": {
        "lines": ["albumCount", "trackCount"]
    },
    "playlists": {
        "lines": ["trackCount", "duration", "libraries"],
        "group": "location"
    },
    "genres": {
        "layout": "grid",
        "lines": ["albumCount"]
    }
});
const kinds = dict({
    "tracks": {
        "type": "tracks"
    },
    "albums": {
        "type": "albums"
    },
    "artists": {
        "type": "artists"
    },
    "playlists": {
        "type": "playlists",
        "sections": ["local", "server"]
    },
    "genres": {
        "type": "genres"
    },
    "favorites": {
        "type": "",
        "sections": ["tracks", "albums", "artists"]
    },
    // the Favorites page's tabs keep their own options
    "favorites-tracks": {
        "type": "tracks"
    },
    "favorites-albums": {
        "type": "albums"
    },
    "favorites-artists": {
        "type": "artists"
    },
    "mostPlayed": {
        "type": "tracks",
        "defaults": {
            "sort": "playCount",
            "descending": true,
            "lines": ["artist", "album", "playCount"]
        }
    },
    "history": {
        "type": "tracks"
    },
    "genre": {
        "type": "albums",
        "multi": true
    },
    "library": {
        "type": "albums",
        "multi": true
    },
    "playlist": {
        "type": "tracks",
        "multi": true,
        "defaults": {
            "sort": "position"
        }
    },
    "album": {
        "type": "tracks",
        "multi": true,
        "defaults": {
            "lines": [],
            "group": "disc"
        }
    },
    "artist": {
        "type": "albums",
        "multi": true,
        "sections": ["albums", "tracks"],
        "defaults": {
            "lines": ["year"],
            "sort": "year",
            "descending": true
        }
    },
    "search": {
        "type": "",
        "sections": ["artists", "albums", "tracks", "playlists"],
        "defaults": {
            "lines": ["artist", "album"]
        }
    },
    "queue": {
        "type": "tracks",
        "defaults": {
            "lines": ["artist"]
        }
    }
});
const kindIds = Object.keys(kinds);
const listLayouts = ["list", "grid", "compact"];
const sizes = ["s", "m", "l"];
// a custom page's limit presets, what a click on a track plays, and its icon names
const pageLimits = [10, 25, 50, 100, 200, 500];
const clickModes = ["list", "one"];
const iconPattern = /^[a-z0-9_]{1,48}$/;
const artShapes = ["cookie", "circle", "rounded", "square"];
// facts a track row shows at its end (End of row), never as info lines
const rowFacts = ["duration", "library"];

function isObject(v) {
    return v !== null && typeof v === "object" && !Array.isArray(v);
}

function copy(v) {
    return v === undefined ? undefined : JSON.parse(JSON.stringify(v));
}

function safeString(v, max) {
    return typeof v === "string" && v.length <= max && !/[\u0000-\u001f\u007f]/.test(v);
}

function safeScalar(v) {
    return safeString(v, 256) || typeof v === "boolean" || (typeof v === "number" && isFinite(v));
}

function inst(type, patch) {
    return Object.assign({
        "id": type.toLowerCase(),
        "type": type,
        "hidden": false,
        "placement": "auto",
        "options": {}
    }, patch ?? {});
}

function hiddenInst(type) {
    return inst(type, {
        "hidden": true
    });
}

function fillSpacer() {
    return inst("spacer", {
        "options": {
            "size": "fill"
        }
    });
}

function insts(list) {
    return list.map(t => typeof t === "string" ? inst(t) : t);
}

function shelf(id, type, layout, size) {
    return inst(type, {
        "id": id,
        "options": {
            "layout": layout,
            "size": size
        }
    });
}

// Catalogue

function isKnownType(type) {
    return elementTypes.indexOf(type) >= 0 || shelfTypes.indexOf(type) >= 0 || itemTypes.some(t => itemCatalog[t].trailing.indexOf(type) >= 0 || itemCatalog[t].menu.indexOf(type) >= 0);
}

function isItemType(id) {
    return itemCatalog[id] !== undefined;
}

function zonesOf(id) {
    if (surfaceZones[id])
        return surfaceZones[id];
    if (itemCatalog[id])
        return itemZoneIds.map(z => zone(z, z === "trailing" || z === "hover" ? "row" : "column"));
    return [];
}

function zoneKind(id, zoneId) {
    return (zonesOf(id).find(z => z.id === zoneId) ?? {}).kind ?? "";
}

function groupOf(type) {
    return groupIds.find(g => groups[g].indexOf(type) >= 0) ?? "";
}

// The types the picker offers for a zone. Cleaning doesn't enforce this: an element the user (or an older
// version) placed elsewhere stays where it is.
function typesFor(id, zoneId) {
    const item = itemCatalog[id];
    if (item) {
        switch (zoneId) {
        case "trailing":
            return item.trailing.concat(["spacer"]);
        case "hover":
            return item.menu.filter(t => t !== "keepTrack");
        case "menu":
        case "context":
            return item.menu.concat(["divider"]);
        }
        return [];
    }
    const kind = zoneKind(id, zoneId);
    switch (kind) {
    case "":
        return [];
    case "panes":
        return paneTypes;
    case "cover":
        return ["cover"];
    case "shelves":
        return shelfTypes;
    }
    return elementTypes.filter(type => {
        // needs a line's width: beside other elements it would get what they leave, down to nothing
        if (type === "seekbar")
            return kind !== "row";
        // the full view has nothing to open
        if (type === "expand")
            return id === "bar" || id === "side";
        return true;
    });
}

function defaultTemplate() {
    return "%album%[ \u00b7 %year%]";
}

// {key: {values: [..] | null (any short text), default?}}
const titleFavorite = {
    "values": [true, false],
    "default": false
};

function optionSchema(id, type, zoneId) {
    if (id === "home")
        return type === "search" ? {} : {
            "source": {
                "values": null
            },
            "layout": {
                "values": ["carousel", "grid", "list"],
                "default": "carousel"
            },
            "size": {
                "values": sizes,
                "default": "m"
            }
        };
    if (type === "cover" && zoneId !== "cover")
        return {};
    if (type === "cover" && (id === "fullBento" || id === "fullM3e"))
        return {
            "size": {
                "values": sizes,
                "default": "m"
            },
            "shape": {
                "values": artShapes,
                "default": id === "fullBento" ? "cookie" : "rounded"
            },
            "rotate": {
                "values": [true, false],
                "default": true
            }
        };
    if (type === "cover" && id === "side")
        return {
            "shape": {
                "values": artShapes,
                "default": "rounded"
            },
            "rotate": {
                "values": [true, false],
                "default": true
            }
        };
    if (type === "title" && (id === "fullBento" || id === "side"))
        return {
            "face": {
                "values": ["expressive", "display"],
                "default": "expressive"
            },
            "favorite": titleFavorite
        };
    if (type === "title" && id === "bar")
        return {
            "width": {
                "values": ["s", "m", "l", "xl"],
                "default": "m"
            },
            "adaptive": {
                "values": [true, false],
                "default": false
            },
            "favorite": titleFavorite
        };
    if (type === "title" && id === "fullM3e")
        return {
            "favorite": titleFavorite
        };
    if (type === "text")
        return {
            "template": {
                "values": null,
                "default": defaultTemplate()
            }
        };
    if (type === "lyricLine") {
        const smooth = {
            "smooth": {
                "values": [true, false],
                "default": false
            },
            "hideInLyrics": {
                "values": [true, false],
                "default": true
            }
        };
        // anywhere else the zone sets its width
        return id === "bar" && (zoneId === "left" || zoneId === "right") ? Object.assign({
            "width": {
                "values": ["s", "m", "l", "xl"],
                "default": "m"
            }
        }, smooth) : smooth;
    }
    if (type === "spacer")
        return {
            "size": {
                "values": ["s", "m", "l", "fill"],
                "default": "m"
            }
        };
    return {};
}

// types that appear at most once in a zone
function single(id, zoneId, type) {
    if (itemCatalog[id])
        return zoneId === "trailing" ? type !== "spacer" : type !== "divider";
    return zoneId === "panes" || zoneId === "sections";
}

// whether an element of type can move into zoneId, which holds list
function accepts(id, zoneId, list, type) {
    return typesFor(id, zoneId).indexOf(type) >= 0 && !(single(id, zoneId, type) && list.some(i => i.type === type));
}

// Defaults

function defaultSurface(id) {
    const transport = ["shuffle", "previous", "playPause", "next", "repeat"];
    const seek = ["elapsed", "seekbar", "duration"];
    const heartTitle = () => inst("title", {
            "options": {
                "favorite": true
            }
        });
    const alwaysShown = type => inst(type, {
            "placement": "keep"
        });
    const groupGap = () => inst("spacer", {
            "options": {
                "size": "s"
            }
        });
    const cover = () => inst("cover", {
            "options": {
                "size": "m"
            }
        });
    switch (id) {
    case "bar":
        return {
            "zones": {
                "left": insts(["cover", heartTitle(), "artists", hiddenInst("album"), hiddenInst("libraryBadge"), "castTarget"]),
                "seekAbove": [],
                "seek": insts(seek),
                "seekBelow": [],
                "controls": insts(transport),
                "right": insts([alwaysShown("addToPlaylist"), "lyrics", "queue", "device", "volume", "dock"])
            }
        };
    case "side":
        return {
            "panesFollowBar": true,
            "zones": {
                "panes": [],
                "cover": insts(["cover"]),
                "title": insts([heartTitle(), "artists", "album", hiddenInst("libraryBadge")]),
                "seekAbove": [],
                "seek": insts(seek),
                "seekBelow": [],
                "controls": insts(transport),
                "top": insts(["volume", fillSpacer(), "device"]),
                "bottom": insts([alwaysShown("addToPlaylist"), "sleep", "dock"])
            }
        };
    case "fullBento":
        return {
            "zones": {
                "cover": [cover()],
                "header": insts(["playingFrom"]),
                "title": insts(["title", "artists", "album"]),
                "top": [],
                "seekAbove": [],
                "seek": insts(seek),
                "seekBelow": [],
                "controls": insts(transport),
                "bottom": insts(["lyrics", "queue", groupGap(), "volume", "device", groupGap()]).concat([alwaysShown("favorite"), alwaysShown("addToPlaylist")], ["sleep", "speed", "goToAlbum", "goToArtist", "details"].map(type => inst(type, {
                            "placement": "overflow"
                        })))
            }
        };
    case "fullM3e":
        return {
            "zones": {
                "header": insts(["playingFrom"]),
                "buttons": insts(["volume", "lyrics", "queue", "device"]),
                "cover": [cover()],
                "title": insts(["title", "artists", "album"]),
                "seekAbove": [],
                "seek": insts(["seekbar", "elapsed", "duration"]),
                "seekBelow": [],
                "controls": insts(["previous", "playPause", "next"]),
                "end": insts([alwaysShown("favorite"), alwaysShown("addToPlaylist"), "sleep", "shuffle", "repeat"])
            }
        };
    case "home":
        return {
            "zones": {
                "shelves": [inst("search"), shelf("continue", "continue", "carousel", "m"), shelf("played", "recentlyPlayed", "carousel", "m"), shelf("favorites", "favoriteTracks", "list", "m"), shelf("added", "recentlyAdded", "carousel", "s"), shelf("mostplayed", "mostPlayed", "list", "s"), shelf("playlists", "playlists", "carousel", "s"), shelf("random", "randomAlbums", "grid", "m")]
            }
        };
    }
    return null;
}

function defaultItem(type) {
    const c = itemCatalog[type];
    if (!c)
        return null;
    return {
        "contextSameAsMenu": true,
        "zones": {
            "trailing": insts(c.trailingDefault),
            "hover": insts(c.hoverDefault),
            "menu": insts(c.menuDefault),
            "context": insts(c.menuDefault)
        }
    };
}

// Cleaning

function cleanOptions(schema, stored) {
    const out = {};
    const given = isObject(stored) ? stored : {};
    for (const key of Object.keys(given).slice(0, 16)) {
        const v = given[key];
        if (!typePattern.test(key))
            continue;
        const spec = Object.prototype.hasOwnProperty.call(schema, key) ? schema[key] : undefined;
        if (spec ? (spec.values ? spec.values.indexOf(v) >= 0 : safeString(v, 256)) : safeScalar(v))
            out[key] = v;
    }
    for (const key of Object.keys(schema)) {
        if (out[key] === undefined && schema[key].default !== undefined)
            out[key] = schema[key].default;
    }
    return out;
}

function uniqueId(id, type, seen) {
    let candidate = typeof id === "string" && idPattern.test(id) ? id : "";
    if (candidate === "" || seen.indexOf(candidate) >= 0) {
        const base = type.toLowerCase().slice(0, 28);
        candidate = base;
        for (let n = 2; seen.indexOf(candidate) >= 0; n++)
            candidate = base + "-" + n;
    }
    seen.push(candidate);
    return candidate;
}

// null when stored isn't a list
function cleanZone(id, zoneId, stored, seen) {
    if (!Array.isArray(stored))
        return null;
    const out = [];
    const types = [];
    for (const x of stored.slice(0, maxZone)) {
        if (!isObject(x) || typeof x.type !== "string" || !typePattern.test(x.type))
            continue;
        if (single(id, zoneId, x.type) && types.indexOf(x.type) >= 0)
            continue;
        types.push(x.type);
        out.push({
            "id": uniqueId(x.id, x.type, seen),
            "type": x.type,
            "hidden": x.hidden === true,
            "placement": placements.indexOf(x.placement) >= 0 ? x.placement : "auto",
            "options": cleanOptions(optionSchema(id, x.type, zoneId), x.options)
        });
    }
    return out;
}

// The effective surface: every zone of the catalogue, a missing zone at its default. Zones this version
// doesn't know are left out here; setSurface keeps them in the stored form.
function cleanSurface(id, stored) {
    const zones = surfaceZones[id];
    if (!zones)
        return null;
    const given = isObject(stored) ? stored : {};
    const storedZones = isObject(given.zones) ? given.zones : {};
    const defaults = defaultSurface(id).zones;
    const seen = [];
    const out = {
        "zones": {}
    };
    for (const z of zones)
        out.zones[z.id] = cleanZone(id, z.id, storedZones[z.id], seen) ?? cleanZone(id, z.id, defaults[z.id], seen);
    if (id === "side")
        out.panesFollowBar = given.panesFollowBar !== false;
    // unset, the style decides
    if (typeof given.centerTitle === "boolean")
        out.centerTitle = given.centerTitle;
    return out;
}

function cleanItem(type, stored) {
    const defaults = defaultItem(type);
    if (!defaults)
        return null;
    const given = isObject(stored) ? stored : {};
    const storedZones = isObject(given.zones) ? given.zones : {};
    const same = given.contextSameAsMenu !== false;
    const seen = [];
    const out = {
        "contextSameAsMenu": same,
        "zones": {}
    };
    for (const z of ["trailing", "hover", "menu"])
        out.zones[z] = cleanZone(type, z, storedZones[z], seen) ?? cleanZone(type, z, defaults.zones[z], seen);
    out.zones.context = same ? copy(out.zones.menu) : cleanZone(type, "context", storedZones.context, seen) ?? cleanZone(type, "context", out.zones.menu, seen);
    return out;
}

function cleanNowPlaying(stored) {
    const given = isObject(stored) ? stored : {};
    const style = styles.indexOf(given.style) >= 0 ? given.style : "bento";
    return {
        "start": starts.indexOf(given.start) >= 0 ? given.start : "bar",
        "style": style,
        "barStyle": styles.indexOf(given.barStyle) >= 0 ? given.barStyle : style,
        "sideStyle": styles.indexOf(given.sideStyle) >= 0 ? given.sideStyle : style,
        "openPane": openPanes.indexOf(given.openPane) >= 0 ? given.openPane : "last",
        "playerLook": playerLooks.indexOf(given.playerLook) >= 0 ? given.playerLook : "card",
        "paneLook": paneLooks.indexOf(given.paneLook) >= 0 ? given.paneLook : "player"
    };
}

function isTrackKind(kind) {
    return (kinds[kind] ?? {}).type === "tracks";
}

// A custom text column is the line "custom:<id>"; its title, template, sort and width live in lineOptions.
const customLinePattern = /^custom:[A-Za-z0-9]{1,16}$/;
const maxCustomLines = 8;
// the id the one custom line of the old format gets
const legacyCustomLine = "custom:c1";
const maxRules = 32;
const ruleTypes = ["tracks", "albums", "artists"];
// a column's width multiplier: S, M, L as the Width row sets them; a drag writes any value between the bounds
const columnSizes = {
    "s": 0.7,
    "m": 1,
    "l": 1.5
};
const columnSizeRange = [0.4, 3];

function isCustomLine(line) {
    return typeof line === "string" && customLinePattern.test(line);
}

function cleanLines(kind, lines) {
    const out = [];
    let custom = 0;
    for (const line of lines.slice(0, 16)) {
        if (!safeString(line, 32) || line === "" || out.indexOf(line) >= 0 || isTrackKind(kind) && rowFacts.indexOf(line) >= 0)
            continue;
        if (line.startsWith("custom:") && (!isCustomLine(line) || ++custom > maxCustomLines))
            continue;
        out.push(line);
    }
    return out;
}

function cleanLineOptions(stored) {
    const out = {};
    let custom = 0;
    for (const key of Object.keys(stored).slice(0, 32)) {
        const e = stored[key];
        const isCustom = isCustomLine(key);
        if (!isObject(e) || !isCustom && !typePattern.test(key) || isCustom && ++custom > maxCustomLines)
            continue;
        const entry = {};
        if (isCustom && safeString(e.title, 64) && e.title !== "")
            entry.title = e.title;
        if (isCustom && safeString(e.template, 256))
            entry.template = e.template;
        if (safeString(e.sort, 32) && (typePattern.test(e.sort) || isCustomLine(e.sort)))
            entry.sort = e.sort;
        if (typeof e.size === "number" && isFinite(e.size))
            entry.size = Math.min(columnSizeRange[1], Math.max(columnSizeRange[0], e.size));
        if (Object.keys(entry).length > 0)
            out[key] = entry;
    }
    return out;
}

// The one custom line of the old format (lines: ["custom"], lineTemplate, lineTitle, sort: "custom") as a stored
// record of the new one.
function upgradeCustomLine(stored) {
    const hasLine = Array.isArray(stored.lines) && stored.lines.indexOf("custom") >= 0;
    if (!hasLine && stored.lineTemplate === undefined && stored.lineTitle === undefined && stored.sort !== "custom")
        return stored;
    const out = Object.assign({}, stored);
    delete out.lineTemplate;
    delete out.lineTitle;
    if (hasLine)
        out.lines = stored.lines.map(l => l === "custom" ? legacyCustomLine : l);
    if (stored.sort === "custom")
        out.sort = legacyCustomLine;
    if (safeString(stored.lineTemplate, 256) || safeString(stored.lineTitle, 64)) {
        const entry = {};
        if (safeString(stored.lineTemplate, 256))
            entry.template = stored.lineTemplate;
        if (safeString(stored.lineTitle, 64))
            entry.title = stored.lineTitle;
        out.lineOptions = Object.assign({}, isObject(stored.lineOptions) ? stored.lineOptions : {}, {
            [legacyCustomLine]: entry
        });
    }
    return out;
}

// A filter rule's value: a scalar, or a list of them with null for a blank end of a range. Anything else is
// dropped and the rule stays, without a value.
function cleanRuleValue(v) {
    if (safeScalar(v))
        return v;
    if (Array.isArray(v) && v.length <= maxRules && v.every(e => e === null || safeScalar(e)))
        return v.slice();
    return undefined;
}

// What a rule means is checked in Rules.js against this version; a rule it doesn't know is kept here.
function cleanRules(stored) {
    const given = Array.isArray(stored.list) ? stored.list : [];
    return {
        "match": stored.match === "any" ? "any" : "all",
        "list": given.filter(r => isObject(r) && safeString(r.field, 32) && typePattern.test(r.field) && safeString(r.op, 32) && typePattern.test(r.op)).slice(0, maxRules).map(r => {
            const out = {
                "field": r.field,
                "op": r.op
            };
            const value = cleanRuleValue(r.value);
            if (value !== undefined)
                out.value = value;
            return out;
        })
    };
}

function canAddRule(list) {
    return list.length < maxRules;
}

// Filters from before rules, and the locked filters of a page saved as a view. They hold on top of the rules,
// which a list matching any of two or more can't say, so there they stay filters.
function upgradeFilters(kind, stored) {
    const f = stored.filters;
    if (!isObject(f) || ruleTypes.indexOf(kinds[kind].type) < 0)
        return stored;
    const added = [];
    if (safeString(f.library, 256) && f.library !== "")
        added.push({
            "field": "library",
            "op": "isAny",
            "value": [f.library]
        });
    if (safeString(f.genre, 256) && f.genre !== "")
        added.push({
            "field": "genre",
            "op": "isAny",
            "value": [f.genre]
        });
    if (f.favorites === true)
        added.push({
            "field": "favorite",
            "op": "is",
            "value": true
        });
    const rules = cleanRules(isObject(stored.rules) ? stored.rules : {});
    if (added.length > 0 && rules.match === "any" && rules.list.length > 1)
        return stored;
    const out = Object.assign({}, stored);
    delete out.filters;
    if (added.length > 0)
        out.rules = {
            "match": "all",
            "list": added.concat(rules.list)
        };
    return out;
}

// what a click on a column's header sorts by: its own key unless Sort by says otherwise
function columnSort(o, line) {
    const sort = o.lineOptions?.[line]?.sort;
    return typeof sort === "string" && !(isCustomLine(sort) && (o.lines ?? []).indexOf(sort) < 0) ? sort : line;
}

function columnSize(o, line) {
    return o.lineOptions?.[line]?.size ?? 1;
}

// the options with these columns' widths set (1 removes the stored size, and an entry left empty goes with it)
function withColumnSizes(o, sizes) {
    const lineOptions = Object.assign({}, o.lineOptions);
    for (const line of Object.keys(sizes)) {
        const entry = Object.assign({}, lineOptions[line]);
        if (sizes[line] === 1)
            delete entry.size;
        else
            entry.size = sizes[line];
        if (Object.keys(entry).length > 0)
            lineOptions[line] = entry;
        else
            delete lineOptions[line];
    }
    return Object.assign({}, o, {
        "lineOptions": lineOptions
    });
}

// "s", "m" or "l" for a width the Width row sets, "" for a dragged one
function sizeName(size) {
    return Object.keys(columnSizes).find(k => columnSizes[k] === size) ?? "";
}

function sizeValue(name) {
    return columnSizes[name];
}

// the template a custom sort key sorts by; null for any other key
function sortTemplate(o, key) {
    return isCustomLine(key) ? o.lineOptions?.[key]?.template ?? "" : null;
}

function canAddCustom(lines) {
    return lines.filter(isCustomLine).length < maxCustomLines;
}

// a line key for a new custom column: the first one no column and no stored sort uses, so a removed column's sort doesn't come back
function newCustomLine(lines, options) {
    const used = lines.slice();
    used.push(options?.sort);
    for (const k of Object.keys(options?.lineOptions ?? {}))
        used.push(k, options.lineOptions[k]?.sort);
    let n = 1;
    while (used.indexOf("custom:c" + n) >= 0)
        n++;
    return "custom:c" + n;
}

function cleanSections(kind, stored) {
    const known = kinds[kind].sections;
    const out = cleanZone("sections", "sections", stored.filter(s => isObject(s) && known.indexOf(s.type) >= 0), []);
    for (const type of known) {
        if (!out.some(s => s.type === type))
            out.push(inst(type));
    }
    if (out.every(s => s.hidden))
        out[0].hidden = false;
    return out;
}

// A list options record, only the valid keys that are present. Unknown scalar keys are kept.
function cleanListOptions(kind, stored) {
    const out = {};
    if (!isObject(stored) || !kinds[kind])
        return out;
    stored = upgradeFilters(kind, upgradeCustomLine(stored));
    for (const key of Object.keys(stored).slice(0, 32)) {
        const v = stored[key];
        switch (key) {
        case "lineOptions":
            if (isObject(v))
                out.lineOptions = cleanLineOptions(v);
            break;
        case "rules":
            if (isObject(v))
                out.rules = cleanRules(v);
            break;
        case "layout":
            if (listLayouts.indexOf(v) >= 0)
                out.layout = v;
            break;
        case "tileSize":
            if (sizes.indexOf(v) >= 0)
                out.tileSize = v;
            break;
        case "lines":
        case "tileLines":
            if (Array.isArray(v))
                out[key] = cleanLines(kind, v);
            break;
        case "sort":
        case "group":
            if (safeString(v, 32))
                out[key] = v;
            break;
        case "descending":
        case "shuffleButton":
        case "ranked":
            if (typeof v === "boolean")
                out[key] = v;
            break;
        case "limit":
            if (pageLimits.indexOf(v) >= 0)
                out.limit = v;
            break;
        case "click":
            if (clickModes.indexOf(v) >= 0)
                out.click = v;
            break;
        case "filters":
            if (isObject(v)) {
                const f = {};
                if (safeString(v.library, 256))
                    f.library = v.library;
                if (safeString(v.genre, 256))
                    f.genre = v.genre;
                if (typeof v.favorites === "boolean")
                    f.favorites = v.favorites;
                out.filters = f;
            }
            break;
        case "sections":
            if (Array.isArray(v) && kinds[kind].sections)
                out.sections = cleanSections(kind, v);
            break;
        default:
            if (typePattern.test(key) && safeScalar(v))
                out[key] = v;
        }
    }
    return out;
}

function cleanNavigation(stored) {
    if (!Array.isArray(stored))
        return [];
    return stored.slice(0, maxList).filter(e => isObject(e) && safeString(e.key, 300) && safeString(e.kind, 32) && safeString(e.target, 256)).map(e => {
        const out = {
            "key": e.key,
            "kind": e.kind,
            "target": e.target,
            "label": safeString(e.label, 256) ? e.label : "",
            "hidden": e.hidden === true
        };
        if (safeString(e.name, 256))
            out.name = e.name;
        return out;
    });
}

function cleanSavedViews(stored) {
    if (!Array.isArray(stored))
        return [];
    return stored.slice(0, maxList).filter(v => isObject(v) && safeString(v.id, 64) && safeString(v.name, 256) && typeDefaults[v.type] !== undefined).map(v => {
        const out = {
            "id": v.id,
            "name": v.name,
            "type": v.type,
            "options": cleanListOptions(v.type, v.options)
        };
        if (typeof v.icon === "string" && iconPattern.test(v.icon))
            out.icon = v.icon;
        return out;
    });
}

function overrideKind(key) {
    return key.split(":")[0];
}

function cleanLists(stored) {
    const given = isObject(stored) ? stored : {};
    const out = {
        "defaults": {},
        "overrides": {}
    };
    const defaults = isObject(given.defaults) ? given.defaults : {};
    for (const kind of kindIds) {
        if (isObject(defaults[kind]))
            out.defaults[kind] = cleanListOptions(kind, defaults[kind]);
    }
    const overrides = isObject(given.overrides) ? given.overrides : {};
    for (const key of Object.keys(overrides).slice(0, maxOverrides)) {
        const kind = overrideKind(key);
        if (keyPattern.test(key) && safeString(key, 300) && kinds[kind] && (kinds[kind].multi === true) === (key !== kind))
            out.overrides[key] = cleanListOptions(kind, overrides[key]);
    }
    return out;
}

// Effective layout

function sidePanes(bar, side) {
    if (!side.panesFollowBar) {
        const panes = side.zones.panes.filter(i => !i.hidden && paneTypes.indexOf(i.type) >= 0).map(i => i.type);
        return panes.length > 0 ? panes : ["cover"];
    }
    // the bar never takes a pane away: one whose button the bar doesn't show goes last
    const shown = barReadOrder.reduce((all, z) => all.concat(bar.zones[z].filter(i => !i.hidden).map(i => i.type)), []);
    const at = type => shown.indexOf(type) < 0 ? shown.length : shown.indexOf(type);
    return ["cover"].concat(at("lyrics") <= at("queue") ? ["lyrics", "queue"] : ["queue", "lyrics"]);
}

// The side panel's zones that follow the bar while panesFollowBar is on; the seek line stays its own
function sideFollowedZones() {
    return ["title", "top", "controls", "bottom"];
}

// What the side panel takes from the bar while it follows: the track info (not the cover: the panel has a cover
// pane), the transport, and the buttons, window buttons at the bottom. Lyrics and queue are the panel's tabs, and
// the bar's spacers mean nothing in a column. Order, hidden state and placement come with them; the device chip
// goes to the far end of its row, and the transport is one row in its usual order, as the bar may split it. The
// bar's lines above and below the seekbar stay lines, in a line the panel has no content of its own for.
function sideFromBar(bar, side) {
    const lines = ["seekAbove", "seekBelow"];
    const line = z => side.zones[z].length > 0 ? side.zones[z] : bar.zones[z].filter(i => isKnownType(i.type) && i.type !== "lyrics" && i.type !== "queue").map(i => copy(i));
    const all = barReadOrder.filter(z => lines.indexOf(z) < 0).reduce((list, z) => list.concat(bar.zones[z]), []).filter(i => isKnownType(i.type));
    const of = (types, skip) => all.filter(i => types.indexOf(i.type) >= 0 && skip.indexOf(i.type) < 0).map(i => copy(i));
    const buttons = of(groups.buttons.concat(groups.actions), ["lyrics", "queue"]);
    const windowButtons = ["dock", "expand"];
    const top = buttons.filter(i => windowButtons.indexOf(i.type) < 0);
    const device = top.filter(i => i.type === "device");
    return {
        "title": of(groups.trackInfo, ["cover"]),
        "seekAbove": line("seekAbove"),
        "seekBelow": line("seekBelow"),
        "top": device.length === 0 ? top : top.filter(i => i.type !== "device").concat([fillSpacer()], device),
        "controls": of(groups.transport, []).sort((a, b) => groups.transport.indexOf(a.type) - groups.transport.indexOf(b.type)),
        "bottom": buttons.filter(i => windowButtons.indexOf(i.type) >= 0)
    };
}

function effective(stored) {
    const given = isObject(stored) ? stored : {};
    const surfaces = isObject(given.surfaces) ? given.surfaces : {};
    const items = isObject(given.items) ? given.items : {};
    const out = {
        "nowPlaying": cleanNowPlaying(given.nowPlaying),
        "surfaces": {},
        "items": {},
        "lists": cleanLists(given.lists),
        "navigation": cleanNavigation(given.navigation),
        "savedViews": cleanSavedViews(given.savedViews)
    };
    for (const id of surfaceIds)
        out.surfaces[id] = cleanSurface(id, surfaces[id]);
    for (const type of itemTypes)
        out.items[type] = cleanItem(type, items[type]);
    if (out.surfaces.side.panesFollowBar)
        Object.assign(out.surfaces.side.zones, sideFromBar(out.surfaces.bar, out.surfaces.side));
    out.sidePanes = sidePanes(out.surfaces.bar, out.surfaces.side);
    return out;
}

// next with each part, surface and item type that didn't change replaced by prev's object, so views built
// from them aren't rebuilt (a random shelf keeps its order)
function keepUnchanged(prev, next) {
    if (!isObject(prev))
        return next;
    const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);
    const out = {};
    for (const key of Object.keys(next)) {
        if (same(prev[key], next[key])) {
            out[key] = prev[key];
        } else if (key === "surfaces" || key === "items") {
            out[key] = {};
            for (const id of Object.keys(next[key]))
                out[key][id] = isObject(prev[key]) && same(prev[key][id], next[key][id]) ? prev[key][id] : next[key][id];
        } else {
            out[key] = next[key];
        }
    }
    return out;
}

function visibleTypes(surface, zoneIds) {
    return zoneIds.reduce((all, z) => all.concat((surface.zones[z] ?? []).filter(i => !i.hidden).map(i => i.type)), []);
}

// Problem codes for a surface or an item type: "noPlayPause", "noFullView", "noMenuButton"
function problems(eff, id) {
    const out = [];
    const surface = eff.surfaces[id];
    if (surface && nowPlayingSurfaces.indexOf(id) >= 0) {
        const shown = visibleTypes(surface, Object.keys(surface.zones).filter(z => z !== "panes"));
        if (shown.indexOf("playPause") < 0)
            out.push("noPlayPause");
        if (id === "bar" && !["cover", "title", "expand"].some(t => shown.indexOf(t) >= 0))
            out.push("noFullView");
    }
    const item = eff.items[id];
    if (item && visibleTypes(item, ["trailing"]).indexOf("more") < 0)
        out.push("noMenuButton");
    return out;
}

// Lists: a kind's default, and a page's options (the kind default with the page's change)

function mergeOptions(base, change) {
    const out = Object.assign({}, base, change);
    out.filters = Object.assign({}, base.filters, change.filters ?? {});
    return out;
}

function builtinOptions(kind) {
    const k = kinds[kind];
    const out = mergeOptions(mergeOptions({
        "layout": "list",
        "tileSize": "m",
        "lines": [],
        "lineOptions": {},
        "sort": "name",
        "descending": false,
        "rules": {
            "match": "all",
            "list": []
        },
        "filters": {
            "library": "",
            "genre": "",
            "favorites": false
        }
    }, typeDefaults[k.type] ?? {}), k.defaults ?? {});
    if (k.sections)
        out.sections = k.sections.map(s => inst(s));
    return out;
}

function overrideKey(kind, itemId) {
    const k = kinds[kind];
    if (!k)
        return "";
    if (!k.multi)
        return kind;
    return itemId ? kind + ":" + itemId : "";
}

// a custom sort needs its custom line; without it the kind's own sort applies
function validSort(kind, o) {
    if (isCustomLine(o.sort) && (o.lines ?? []).indexOf(o.sort) < 0)
        o.sort = builtinOptions(kind).sort;
    return o;
}

function kindDefault(eff, kind) {
    if (!kinds[kind])
        return null;
    return validSort(kind, mergeOptions(builtinOptions(kind), eff.lists.defaults[kind] ?? {}));
}

function savedViewOptions(eff, view) {
    return validSort(view.type, mergeOptions(kindDefault(eff, view.type), view.options ?? {}));
}

function pageOptions(eff, kind, itemId) {
    const base = kindDefault(eff, kind);
    if (!base)
        return null;
    const key = overrideKey(kind, itemId);
    return key === "" ? base : validSort(kind, mergeOptions(base, eff.lists.overrides[key] ?? {}));
}

// the keys of o that differ from base, filters one by one
function diff(base, o) {
    const out = {};
    for (const key of Object.keys(o)) {
        const value = key === "filters" ? diff(base.filters ?? {}, o.filters) : o[key];
        if (key === "filters" ? Object.keys(value).length > 0 : JSON.stringify(value) !== JSON.stringify(base[key]))
            out[key] = value;
    }
    return out;
}

function changedPages(eff, kind) {
    return Object.keys(eff.lists.overrides).filter(key => overrideKind(key) === kind && Object.keys(eff.lists.overrides[key]).length > 0).length;
}

// Writing: each returns a new stored layout

function base(stored) {
    const out = isObject(stored) ? copy(stored) : {};
    out.version = version;
    return out;
}

function child(obj, key) {
    if (!isObject(obj[key]))
        obj[key] = {};
    return obj[key];
}

function setSurface(stored, id, value) {
    if (!surfaceZones[id])
        return base(stored);
    const out = base(stored);
    const surfaces = child(out, "surfaces");
    const before = effective(stored);
    const given = isObject(value) ? copy(value) : {};
    given.zones = isObject(given.zones) ? given.zones : {};
    if (id === "side" && typeof given.panesFollowBar !== "boolean")
        given.panesFollowBar = before.surfaces.side.panesFollowBar;
    if (typeof given.centerTitle !== "boolean" && typeof before.surfaces[id].centerTitle === "boolean")
        given.centerTitle = before.surfaces[id].centerTitle;
    // turning "Follow the bottom bar" off starts the panes from what the panel showed
    if (id === "side" && before.surfaces.side.panesFollowBar && given.panesFollowBar === false && !(Array.isArray(given.zones.panes) && given.zones.panes.length > 0))
        given.zones.panes = before.sidePanes.map(t => inst(t));
    const next = cleanSurface(id, given);
    const old = isObject(surfaces[id]) && isObject(surfaces[id].zones) ? surfaces[id].zones : {};
    next.zones = Object.assign({}, old, next.zones);
    surfaces[id] = next;
    return out;
}

function resetSurface(stored, id) {
    const out = base(stored);
    if (isObject(out.surfaces))
        delete out.surfaces[id];
    return out;
}

function setItem(stored, type, patch) {
    if (!itemCatalog[type])
        return base(stored);
    const out = base(stored);
    const current = effective(stored).items[type];
    const given = isObject(patch) ? patch : {};
    const zones = Object.assign({}, current.zones, isObject(given.zones) ? copy(given.zones) : {});
    const same = typeof given.contextSameAsMenu === "boolean" ? given.contextSameAsMenu : current.contextSameAsMenu;
    // turning "Same as ⋮ menu" off starts the right-click menu as a copy of the ⋮ menu
    if (current.contextSameAsMenu && !same && !(isObject(given.zones) && Array.isArray(given.zones.context)))
        zones.context = copy(zones.menu);
    child(out, "items")[type] = storedItem(type, {
        "zones": zones,
        "contextSameAsMenu": same
    });
    return out;
}

// an item's stored form keeps no right-click menu while it follows the ⋮ menu
function storedItem(type, value) {
    const out = cleanItem(type, value);
    if (out.contextSameAsMenu)
        delete out.zones.context;
    return out;
}

function resetItem(stored, type) {
    const out = base(stored);
    if (isObject(out.items))
        delete out.items[type];
    return out;
}

function setPageOptions(stored, kind, itemId, options) {
    const out = base(stored);
    const key = overrideKey(kind, itemId);
    if (key === "")
        return out;
    const eff = effective(stored);
    const change = diff(kindDefault(eff, kind), cleanListOptions(kind, options));
    const overrides = child(child(out, "lists"), "overrides");
    if (Object.keys(change).length === 0)
        delete overrides[key];
    else
        overrides[key] = change;
    return out;
}

// patch over the options of a page, a kind's default (allScope) or a saved view ({kind, itemId, allScope, view}).
// A saved view that is gone changes nothing.
function setOptionsAt(stored, target, patch) {
    const eff = effective(stored);
    if (target.view) {
        return setList(stored, "savedViews", eff.savedViews.map(v => v.id === target.view ? Object.assign({}, v, {
                "options": Object.assign({}, savedViewOptions(eff, v), patch)
            }) : v));
    }
    if (target.allScope)
        return setKindDefault(stored, target.kind, Object.assign({}, kindDefault(eff, target.kind), patch));
    return setPageOptions(stored, target.kind, target.itemId, Object.assign({}, pageOptions(eff, target.kind, target.itemId), patch));
}

function resetPageOptions(stored, kind, itemId) {
    const out = base(stored);
    const key = overrideKey(kind, itemId);
    if (key !== "" && isObject(out.lists) && isObject(out.lists.overrides))
        delete out.lists.overrides[key];
    return out;
}

function setKindDefault(stored, kind, options) {
    const out = base(stored);
    if (!kinds[kind])
        return out;
    const change = diff(builtinOptions(kind), cleanListOptions(kind, options));
    const defaults = child(child(out, "lists"), "defaults");
    if (Object.keys(change).length === 0)
        delete defaults[kind];
    else
        defaults[kind] = change;
    return out;
}

// drops the kind's default; the pages' own changes stay
function resetKindDefault(stored, kind) {
    const out = base(stored);
    if (isObject(out.lists) && isObject(out.lists.defaults))
        delete out.lists.defaults[kind];
    return out;
}

// everything back to the defaults except the saved views
function resetAll(stored) {
    return setList({}, "savedViews", isObject(stored) ? stored.savedViews : []);
}

// drops the kind's default and every page change of that kind
function resetKind(stored, kind) {
    const out = base(stored);
    if (!isObject(out.lists))
        return out;
    if (isObject(out.lists.defaults))
        delete out.lists.defaults[kind];
    if (isObject(out.lists.overrides)) {
        for (const key of Object.keys(out.lists.overrides)) {
            if (overrideKind(key) === kind)
                delete out.lists.overrides[key];
        }
    }
    return out;
}

function setNowPlaying(stored, patch) {
    const out = base(stored);
    const given = isObject(patch) ? patch : {};
    const next = isObject(out.nowPlaying) ? out.nowPlaying : {};
    const allowed = {
        "start": starts,
        "style": styles,
        "barStyle": styles,
        "sideStyle": styles,
        "openPane": openPanes,
        "playerLook": playerLooks,
        "paneLook": paneLooks
    };
    for (const key of Object.keys(allowed)) {
        if (allowed[key].indexOf(given[key]) >= 0)
            next[key] = given[key];
    }
    out.nowPlaying = next;
    return out;
}

// navigation, savedViews: an empty list is the default
function setList(stored, key, list) {
    const out = base(stored);
    if (!Array.isArray(list) || list.length === 0)
        delete out[key];
    else
        out[key] = copy(list);
    return out;
}

// The stored form of a layout from outside (an import): every known piece cleaned, the rest dropped.
function sanitize(stored) {
    const given = isObject(stored) ? stored : {};
    const out = {
        "version": version
    };
    if (isObject(given.nowPlaying))
        out.nowPlaying = cleanNowPlaying(given.nowPlaying);
    if (isObject(given.surfaces)) {
        out.surfaces = {};
        for (const id of surfaceIds) {
            if (isObject(given.surfaces[id]))
                out.surfaces[id] = cleanSurface(id, given.surfaces[id]);
        }
    }
    if (isObject(given.items)) {
        out.items = {};
        for (const type of itemTypes) {
            if (isObject(given.items[type]))
                out.items[type] = storedItem(type, given.items[type]);
        }
    }
    if (isObject(given.lists))
        out.lists = cleanLists(given.lists);
    if (Array.isArray(given.navigation))
        out.navigation = cleanNavigation(given.navigation);
    if (Array.isArray(given.savedViews))
        out.savedViews = cleanSavedViews(given.savedViews);
    return out;
}

// {problem: "", layout} or {problem: "tooLarge" | "notJson" | "notLayout"}
function parseImport(text) {
    if (typeof text !== "string" || text.length > 1048576)
        return {
            "problem": "tooLarge"
        };
    let raw;
    try {
        raw = JSON.parse(text);
    } catch (e) {
        return {
            "problem": "notJson"
        };
    }
    if (!isObject(raw) || typeof raw.version !== "number")
        return {
            "problem": "notLayout"
        };
    return {
        "problem": "",
        "layout": sanitize(raw)
    };
}

// Migration from the keys before version 2

const oldViewIds = ["favorite", "rating", "shuffle", "previous", "playPause", "next", "repeat", "elapsed", "seekbar", "duration", "volume", "queue", "lyrics", "device", "sleep", "speed"];
const oldBarIds = ["art", "info", "expand"].concat(oldViewIds);
const oldQuickActions = dict({
    "playNext": "playNext",
    "queue": "addToQueue",
    "playlist": "addToPlaylist",
    "album": "goToAlbum",
    "artist": "goToArtist",
    "details": "details"
});

function needsMigration(raw) {
    return !isObject(raw) || !isObject(raw.layout) || typeof raw.layout.version !== "number";
}

// Top-level keys of files from before the layout object; migrate() folds them into it.
const oldTopKeys = ["nowPlaying", "nowPlayingBar", "homeShelves", "trackQuickActions", "viewOptions", "artistView", "navigation", "savedViews"];

// What a write must carry over besides the adapter's own keys: the file's other top-level keys.
// A file still in the old format loses the keys migrate() consumed.
function extraTop(raw) {
    if (!isObject(raw))
        return {};
    const drop = needsMigration(raw) ? oldTopKeys : [];
    const out = {};
    for (const k of Object.keys(raw)) {
        if (drop.indexOf(k) < 0)
            out[k] = raw[k];
    }
    return out;
}

// the old cleanup: known ids, each once
function oldZones(stored, zoneIds, ids) {
    const seen = [];
    const out = {};
    for (const z of zoneIds) {
        out[z] = (Array.isArray(stored[z]) ? stored[z] : []).filter((id, at, all) => ids.indexOf(id) >= 0 && seen.indexOf(id) < 0 && all.indexOf(id) === at);
        seen.push(...out[z]);
    }
    return out;
}

function without(list, types) {
    return list.filter(t => types.indexOf(t) < 0);
}

function setHidden(surface, zoneIds, type, hidden) {
    for (const z of zoneIds) {
        for (const i of surface.zones[z] ?? []) {
            if (i.type === type)
                i.hidden = hidden;
        }
    }
}

// The shared view zones of the old full view and side panel, applied to the three surfaces.
function viewFromOld(surfaces, zones) {
    const v = oldZones(zones, ["top", "seek", "controls", "bottom"], oldViewIds);
    const panes = ["lyrics", "queue"];
    const modes = v.controls.filter(t => t === "shuffle" || t === "repeat");
    Object.assign(surfaces.fullBento.zones, {
        "top": insts(v.top),
        "controls": insts(v.controls),
        "seek": insts(v.seek),
        "bottom": insts(v.bottom)
    });
    Object.assign(surfaces.fullM3e.zones, {
        "buttons": insts(v.top),
        "controls": insts(without(v.controls, modes)),
        "seek": insts(v.seek),
        "end": insts(v.bottom.concat(modes))
    });
    Object.assign(surfaces.side.zones, {
        "top": insts(without(v.top, panes)),
        "controls": insts(v.controls),
        "seek": insts(v.seek),
        "bottom": insts(without(v.bottom, panes).concat(["dock"]))
    });
}

// The old bar's zones; the info block becomes its parts, and the dock button the bar always added is placed.
function barFromOld(zones, albumShown, badgeShown) {
    const b = oldZones(zones, ["left", "controls", "seek", "right"], oldBarIds);
    const expand = list => list.reduce((all, id) => {
            if (id === "art")
                return all.concat([inst("cover")]);
            if (id === "info")
                return all.concat([inst("title"), inst("artists"), inst("album", {
                            "hidden": !albumShown
                        }), inst("libraryBadge", {
                            "hidden": !badgeShown
                        }), inst("castTarget")]);
            return all.concat([inst(id)]);
        }, []);
    return {
        "zones": {
            "left": expand(b.left),
            "controls": expand(b.controls),
            "seek": expand(b.seek),
            "right": expand(b.right.concat(["dock"]))
        }
    };
}

// The oldest bar: art and info, the transport, a seekbar style and a list of right-hand buttons with a shown
// flag. Its cast button was the playback device button. It never stored the badge, so the new default applies.
function barFromButtons(old, albumShown) {
    const seek = old.seekbar === "hidden" ? [] : old.seekbar === "times" ? ["elapsed", "seekbar", "duration"] : ["seekbar"];
    const bar = barFromOld({
        "left": ["art", "info"],
        "controls": groups.transport,
        "seek": seek
    }, albumShown, false);
    const right = [];
    for (const b of (Array.isArray(old.buttons) ? old.buttons : []).slice(0, maxZone)) {
        const type = isObject(b) && b.id === "cast" ? "device" : isObject(b) ? b.id : "";
        if (oldBarIds.indexOf(type) < 0 || ["art", "info"].indexOf(type) >= 0 || right.some(i => i.type === type))
            continue;
        right.push(inst(type, {
            "hidden": b.shown === false
        }));
    }
    bar.zones.right = right.concat(bar.zones.right);
    return bar;
}

// The old full view layout: the cover's size and visibility in both styles
function applyOldLayout(surfaces, name) {
    for (const id of ["fullBento", "fullM3e"]) {
        for (const i of surfaces[id].zones.cover) {
            if (name === "bigArt" || name === "minimal" || name === "artLyrics" || name === "artQueue") {
                i.options.size = name === "bigArt" ? "l" : "m";
                i.hidden = name === "minimal";
            }
        }
    }
}

function applyShowAlbum(surfaces, shown) {
    for (const id of ["side", "fullBento", "fullM3e"])
        setHidden(surfaces[id], ["title"], "album", !shown);
}

function migrateShelf(s) {
    if (!isObject(s) || !isObject(s.source) || typeof s.source.type !== "string")
        return null;
    const options = {};
    if (s.source.id !== undefined)
        options.source = s.source.id;
    if (s.layout !== undefined)
        options.layout = s.layout;
    if (s.size !== undefined)
        options.size = s.size;
    return inst(s.source.type, {
        "id": s.id,
        "options": options
    });
}

function migrate(raw) {
    const old = isObject(raw) ? raw : {};
    const out = {
        "version": version
    };
    const np = isObject(old.nowPlaying) ? old.nowPlaying : {};
    const oldBar = isObject(old.nowPlayingBar) ? old.nowPlayingBar : {};

    const nowPlaying = {};
    if (starts.indexOf(np.start) >= 0)
        nowPlaying.start = np.start;
    if (styles.indexOf(np.style) >= 0)
        nowPlaying.style = np.style;
    if (np.layout === "artLyrics" || np.layout === "artQueue")
        nowPlaying.openPane = np.layout === "artLyrics" ? "lyrics" : "queue";
    else if (np.layout === "bigArt" || np.layout === "minimal")
        nowPlaying.openPane = "last";
    if (Object.keys(nowPlaying).length > 0)
        out.nowPlaying = nowPlaying;

    const surfaces = {};
    if (isObject(np.zones) || np.showAlbum === false || np.layout === "bigArt" || np.layout === "minimal") {
        const view = {
            "side": defaultSurface("side"),
            "fullBento": defaultSurface("fullBento"),
            "fullM3e": defaultSurface("fullM3e")
        };
        if (isObject(np.zones))
            viewFromOld(view, np.zones);
        applyOldLayout(view, np.layout);
        applyShowAlbum(view, np.showAlbum !== false);
        Object.assign(surfaces, view);
    }
    if (isObject(oldBar.zones)) {
        surfaces.bar = barFromOld(oldBar.zones, np.showAlbumInBar === true, true);
    } else if (Array.isArray(oldBar.buttons) || typeof oldBar.seekbar === "string") {
        surfaces.bar = barFromButtons(oldBar, np.showAlbumInBar === true);
    } else if (np.showAlbumInBar === true) {
        surfaces.bar = defaultSurface("bar");
        setHidden(surfaces.bar, ["left", "controls", "seek", "right"], "album", false);
    }
    if (Array.isArray(old.homeShelves)) {
        const shelves = old.homeShelves.slice(0, maxZone - 1).map(migrateShelf).filter(s => s !== null);
        // the old Home showed its defaults for an empty list
        if (shelves.length > 0)
            surfaces.home = {
                "zones": {
                    "shelves": [inst("search")].concat(shelves)
                }
            };
    }
    if (Object.keys(surfaces).length > 0)
        out.surfaces = surfaces;

    // the old rows showed their defaults for an empty list
    if (Array.isArray(old.trackQuickActions) && old.trackQuickActions.length > 0)
        out.items = {
            "track": {
                "zones": {
                    "hover": old.trackQuickActions.filter(id => oldQuickActions[id] !== undefined).map(id => inst(oldQuickActions[id]))
                }
            }
        };

    const lists = {
        "defaults": {},
        "overrides": {}
    };
    const viewOptions = isObject(old.viewOptions) ? old.viewOptions : {};
    for (const key of Object.keys(viewOptions).slice(0, maxOverrides)) {
        const record = viewOptions[key];
        if (!isObject(record))
            continue;
        // one record served every playlist, and the Albums page's served the genre and library pages
        if (key === "playlist") {
            lists.defaults.playlist = diff(builtinOptions("playlist"), cleanListOptions("playlist", record));
        } else if (kinds[key] && !kinds[key].multi) {
            lists.overrides[key] = diff(builtinOptions(key), cleanListOptions(key, record));
            if (key === "albums") {
                lists.defaults.genre = diff(builtinOptions("genre"), cleanListOptions("genre", record));
                lists.defaults.library = diff(builtinOptions("library"), cleanListOptions("library", record));
            }
        }
    }
    if (old.artistView === "tracks")
        lists.defaults.artist = {
            "sections": [inst("tracks"), inst("albums")]
        };
    if (Object.keys(lists.defaults).length > 0 || Object.keys(lists.overrides).length > 0)
        out.lists = lists;

    if (Array.isArray(old.navigation) && old.navigation.length > 0)
        out.navigation = copy(old.navigation);
    if (Array.isArray(old.savedViews) && old.savedViews.length > 0)
        out.savedViews = copy(old.savedViews);
    // a layout written without a version (by a build between the two) wins over the old keys
    return Object.assign(out, isObject(old.layout) ? copy(old.layout) : {}, {
        "version": version
    });
}
