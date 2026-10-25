// node quickshell/tests/layouts.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Common/Layouts.js"), "utf8").replace(".pragma library", "");
const L = new Function(source + "; return { migrate, needsMigration, effective, problems, setSurface, setItem, setPageOptions, setKindDefault, resetKind, pageOptions, kindDefault, changedPages, parseImport, cleanSurface, resetAll, resetKindDefault, accepts, keepUnchanged };")();

const types = (list) => list.filter(i => !i.hidden).map(i => i.type);
const zoneTypes = (eff, id, zone) => types(eff.surfaces[id].zones[zone]);

// An old ui-settings.json shaped like a long-used install: every customization key edited.
const edited = {
    "themeMode": "dark",
    "nowPlaying": {
        "start": "side",
        "style": "m3e",
        "layout": "artQueue",
        "showAlbum": false,
        "showAlbumInBar": true,
        "zones": {
            "top": ["volume", "lyrics", "device"],
            "seek": ["elapsed", "seekbar", "duration"],
            "controls": ["shuffle", "previous", "playPause", "next", "repeat"],
            "bottom": ["favorite", "queue", "speed"]
        }
    },
    "nowPlayingBar": {
        "zones": {
            "left": ["art", "info"],
            "controls": ["previous", "playPause", "next"],
            "seek": ["elapsed", "seekbar", "duration"],
            "right": ["queue", "lyrics", "volume", "expand"]
        }
    },
    "trackQuickActions": ["favorite", "playNext", "playlist"],
    "viewOptions": {
        "tracks": {
            "layout": "compact",
            "tileSize": "m",
            "lines": ["artist", "album", "library", "duration", "year"],
            "sort": "artist",
            "descending": true,
            "filters": {
                "library": "",
                "genre": "",
                "favorites": false
            }
        },
        "albums": {
            "layout": "list",
            "tileSize": "l",
            "lines": ["artist", "year"],
            "sort": "year",
            "descending": false,
            "filters": {
                "library": "lib1",
                "genre": "",
                "favorites": false
            }
        },
        "playlist": {
            "lines": ["artist", "duration"],
            "sort": "position",
            "descending": true
        },
        "favorites-albums": {
            "layout": "grid",
            "tileSize": "s",
            "sort": "added"
        }
    },
    "homeShelves": [{
            "id": "continue",
            "source": {
                "type": "continue"
            },
            "layout": "carousel",
            "size": "m"
        }, {
            "id": "shelf-1712345678901",
            "source": {
                "type": "playlist",
                "id": "local:7"
            },
            "layout": "list",
            "size": "l"
        }],
    "navigation": [{
            "key": "page:home",
            "kind": "page",
            "target": "home",
            "label": "",
            "hidden": false
        }, {
            "key": "playlist:local:7",
            "kind": "playlist",
            "target": "local:7",
            "label": "Road",
            "hidden": false
        }],
    "savedViews": [{
            "id": "view-1",
            "name": "Loud",
            "type": "albums",
            "options": {
                "layout": "grid",
                "sort": "year"
            }
        }],
    "artistView": "tracks"
};

// Migration: the old setup comes back unchanged through the shapes the old UI reads
{
    assert.ok(L.needsMigration(edited));
    const layout = L.migrate(edited);
    assert.strictEqual(layout.version, 2);
    assert.ok(!L.needsMigration({
        "layout": layout
    }), "a migrated file is not migrated again");
    const eff = L.effective(layout);
    assert.deepStrictEqual(eff.nowPlaying, {
        "start": "side",
        "style": "m3e",
        "openPane": "queue"
    });
    for (const zone of ["top", "seek", "controls", "bottom"])
        assert.deepStrictEqual(zoneTypes(eff, "fullBento", zone), edited.nowPlaying.zones[zone], "fullBento " + zone);
    assert.deepStrictEqual(zoneTypes(eff, "bar", "controls"), edited.nowPlayingBar.zones.controls);
    assert.deepStrictEqual(zoneTypes(eff, "bar", "seek"), edited.nowPlayingBar.zones.seek);
    assert.strictEqual(eff.surfaces.fullBento.zones.cover[0].options.size, "m", "artQueue keeps the normal cover");
    assert.deepStrictEqual(zoneTypes(eff, "fullBento", "title"), ["title", "artists"], "Show album off hides the album");
    // the heart stays at the end of the row, the rest become hover actions
    assert.deepStrictEqual(types(eff.items.track.zones.trailing), ["favorite", "library", "cloud", "duration", "more"]);
    assert.deepStrictEqual(types(eff.items.track.zones.hover), ["playNext", "addToPlaylist"]);
    // an edited bar keeps its library badge, and the dock button it always added
    assert.deepStrictEqual(zoneTypes(eff, "bar", "left"), ["cover", "title", "artists", "album", "libraryBadge", "castTarget"]);
    assert.deepStrictEqual(zoneTypes(eff, "bar", "right"), ["queue", "lyrics", "volume", "expand", "dock"]);
    // M3E: modes go to the far end, the top zone becomes the button group
    assert.deepStrictEqual(zoneTypes(eff, "fullM3e", "buttons"), ["volume", "lyrics", "device"]);
    assert.deepStrictEqual(zoneTypes(eff, "fullM3e", "controls"), ["previous", "playPause", "next"]);
    assert.deepStrictEqual(zoneTypes(eff, "fullM3e", "end"), ["favorite", "queue", "speed", "shuffle", "repeat"]);
    // side: its own zones get the shared ones without lyrics and queue, plus the dock button (shown once it
    // stops following the bar)
    assert.deepStrictEqual(zoneTypes(layout, "side", "top"), ["volume", "device"]);
    assert.deepStrictEqual(zoneTypes(layout, "side", "bottom"), ["favorite", "speed", "dock"]);
    assert.strictEqual(eff.surfaces.side.panesFollowBar, true);
    assert.deepStrictEqual(eff.sidePanes, ["cover", "queue", "lyrics"]);

    assert.deepStrictEqual(types(L.pageOptions(eff, "artist", "").sections), ["tracks", "albums"]);
    assert.deepStrictEqual(eff.navigation, edited.navigation);
    assert.deepStrictEqual(eff.savedViews.map(v => v.id), ["view-1"]);
    for (const key of ["tracks", "albums", "favorites-albums"]) {
        for (const field of ["layout", "tileSize", "sort", "descending"]) {
            if (edited.viewOptions[key][field] !== undefined)
                assert.deepStrictEqual(L.pageOptions(eff, key, "")[field], edited.viewOptions[key][field], key + "." + field);
        }
    }
    assert.strictEqual(L.pageOptions(eff, "playlist", "p1").sort, "position");
    assert.deepStrictEqual(L.pageOptions(eff, "albums", "").filters.library, "lib1");
    assert.deepStrictEqual(L.pageOptions(eff, "albums", "").lines, ["artist", "year"]);
    assert.deepStrictEqual(eff.surfaces.home.zones.shelves.slice(1).map(s => [s.id, s.type, s.options.source, s.options.layout, s.options.size]), edited.homeShelves.map(s => [s.id, s.source.type, s.source.id, s.layout, s.size]));

    // duration and library leave the track info lines (End of row shows them)
    assert.deepStrictEqual(L.pageOptions(eff, "tracks", "").lines, ["artist", "album", "year"]);
    assert.deepStrictEqual(L.pageOptions(eff, "playlist", "p1").lines, ["artist"]);
    assert.strictEqual(L.pageOptions(eff, "playlist", "p1").descending, true, "one record served every playlist");
    // the Albums page's options served the genre and library pages
    assert.strictEqual(L.pageOptions(eff, "genre", "g1").layout, "list");
    assert.strictEqual(L.pageOptions(eff, "library", "x").tileSize, "l");
    assert.deepStrictEqual(types(eff.surfaces.home.zones.shelves), ["search", "continue", "playlist"]);
    assert.deepStrictEqual(eff.surfaces.home.zones.shelves[2].options, {
        "source": "local:7",
        "layout": "list",
        "size": "l"
    });
    assert.deepStrictEqual(types(eff.items.track.zones.hover), ["playNext", "addToPlaylist"]);
    assert.deepStrictEqual(types(L.pageOptions(eff, "artist", "a1").sections), ["tracks", "albums"]);
}

// Migration table: old key → effective result
const migrations = [
    ["empty file: defaults, library badge hidden in the bar", {}, eff => {
            assert.deepStrictEqual(zoneTypes(eff, "bar", "left"), ["cover", "title", "artists", "castTarget"]);
            assert.deepStrictEqual(eff.nowPlaying, {
                "start": "bar",
                "style": "bento",
                "openPane": "last"
            });
            assert.deepStrictEqual(eff.sidePanes, ["cover", "lyrics", "queue"]);
        }],
    ["unedited bar with the album shown: defaults plus the album", {
            "nowPlaying": {
                "showAlbumInBar": true
            }
        }, eff => assert.deepStrictEqual(zoneTypes(eff, "bar", "left"), ["cover", "title", "artists", "album", "castTarget"])],
    ["big art: large cover in both styles", {
            "nowPlaying": {
                "layout": "bigArt"
            }
        }, eff => {
            assert.strictEqual(eff.surfaces.fullBento.zones.cover[0].options.size, "l");
            assert.strictEqual(eff.surfaces.fullM3e.zones.cover[0].options.size, "l");
            assert.strictEqual(eff.nowPlaying.openPane, "last");
        }],
    ["minimal: cover hidden in both styles", {
            "nowPlaying": {
                "layout": "minimal"
            }
        }, eff => {
            assert.strictEqual(eff.surfaces.fullBento.zones.cover[0].hidden, true);
            assert.strictEqual(eff.surfaces.fullM3e.zones.cover[0].hidden, true);
        }],
    ["art and lyrics opens with lyrics", {
            "nowPlaying": {
                "layout": "artLyrics"
            }
        }, eff => assert.strictEqual(eff.nowPlaying.openPane, "lyrics")],
    ["old zones with unknown and repeated ids are cleaned the old way", {
            "nowPlayingBar": {
                "zones": {
                    "left": ["art", "bogus", "art"],
                    "right": "volume"
                }
            }
        }, eff => {
            assert.deepStrictEqual(zoneTypes(eff, "bar", "left"), ["cover"]);
            assert.deepStrictEqual(zoneTypes(eff, "bar", "right"), ["dock"]);
            assert.deepStrictEqual(zoneTypes(eff, "bar", "controls"), []);
        }],
    ["quick actions without favorite: favorite stays at the end of the row", {
            "trackQuickActions": ["queue", "details"]
        }, eff => {
            assert.deepStrictEqual(types(eff.items.track.zones.hover), ["addToQueue", "details"]);
            assert.ok(types(eff.items.track.zones.trailing).indexOf("favorite") >= 0);
        }],
    ["a layout without a version wins over the old keys", {
            "nowPlaying": {
                "style": "m3e"
            },
            "layout": {
                "nowPlaying": {
                    "style": "bento"
                }
            }
        }, eff => assert.strictEqual(eff.nowPlaying.style, "bento")],
    ["the oldest bar: right-hand buttons with a shown flag and a seekbar style", {
            "nowPlaying": {
                "layout": "artLyrics",
                "showAlbum": true,
                "style": "m3e"
            },
            "nowPlayingBar": {
                "buttons": [{
                        "id": "favorite",
                        "shown": true
                    }, {
                        "id": "lyrics",
                        "shown": true
                    }, {
                        "id": "queue",
                        "shown": true
                    }, {
                        "id": "sleep",
                        "shown": false
                    }, {
                        "id": "cast",
                        "shown": true
                    }, {
                        "id": "volume",
                        "shown": true
                    }, "junk", {
                        "id": "bogus",
                        "shown": true
                    }],
                "seekbar": "times"
            }
        }, eff => {
            const right = eff.surfaces.bar.zones.right;
            assert.deepStrictEqual(right.map(i => i.type), ["favorite", "lyrics", "queue", "sleep", "device", "volume", "dock"]);
            assert.deepStrictEqual(right.filter(i => i.hidden).map(i => i.type), ["sleep"]);
            assert.deepStrictEqual(zoneTypes(eff, "bar", "left"), ["cover", "title", "artists", "castTarget"], "the badge takes the new default");
            assert.deepStrictEqual(zoneTypes(eff, "bar", "seek"), ["elapsed", "seekbar", "duration"]);
            assert.deepStrictEqual(zoneTypes(eff, "bar", "controls"), ["shuffle", "previous", "playPause", "next", "repeat"]);
            assert.deepStrictEqual(eff.sidePanes, ["cover", "lyrics", "queue"]);
            assert.deepStrictEqual(eff.nowPlaying, {
                "start": "bar",
                "style": "m3e",
                "openPane": "lyrics"
            });
        }],
    ["the oldest bar without its seekbar", {
            "nowPlayingBar": {
                "seekbar": "hidden"
            }
        }, eff => {
            assert.deepStrictEqual(zoneTypes(eff, "bar", "seek"), []);
            assert.deepStrictEqual(zoneTypes(eff, "bar", "left"), ["cover", "title", "artists", "castTarget"]);
        }],
    ["garbage values are ignored", {
            "nowPlaying": "x",
            "nowPlayingBar": [],
            "viewOptions": {
                "tracks": 5,
                "nope": {}
            },
            "homeShelves": [7, {
                    "source": "x"
                }],
            "trackQuickActions": "favorite"
        }, eff => {
            assert.deepStrictEqual(eff.lists.overrides, {});
            assert.deepStrictEqual(types(eff.surfaces.home.zones.shelves), types(L.effective({}).surfaces.home.zones.shelves));
        }]
];
for (const [name, raw, check] of migrations) {
    const layout = L.migrate(raw);
    check(L.effective(layout), name);
    assert.strictEqual(L.needsMigration({
        "layout": layout
    }), false, name);
}

// Cleaning: the file is never trusted
{
    const many = Array.from({
        "length": 50
    }, () => ({
            "type": "spacer"
        }));
    const cleaned = L.cleanSurface("bar", {
        "zones": {
            "left": many,
            "controls": [{
                    "id": "Bad Id",
                    "type": "playPause",
                    "placement": "sideways",
                    "hidden": "yes"
                }, {
                    "id": "x",
                    "type": "next"
                }, {
                    "id": "x",
                    "type": "previous"
                }, {
                    "type": "evil\u0000"
                }, "playPause", null, {
                    "type": "fromTheFuture",
                    "options": {
                        "speed": 3,
                        "nested": {
                            "a": 1
                        },
                        "bad key": 1
                    }
                }],
            "seek": {
                "length": 1
            },
            "right": [{
                    "type": "spacer",
                    "options": {
                        "size": "huge"
                    }
                }]
        }
    });
    assert.strictEqual(cleaned.zones.left.length, 32, "zones are capped");
    assert.deepStrictEqual(cleaned.zones.controls.map(i => i.type), ["playPause", "next", "previous", "fromTheFuture"]);
    assert.deepStrictEqual(cleaned.zones.controls.map(i => i.id), ["playpause", "x", "previous", "fromthefuture"]);
    assert.deepStrictEqual(cleaned.zones.controls[0].placement, "auto");
    assert.strictEqual(cleaned.zones.controls[0].hidden, false);
    assert.deepStrictEqual(cleaned.zones.controls[3].options, {
        "speed": 3
    }, "unknown types and plain unknown options are kept");
    assert.deepStrictEqual(cleaned.zones.seek.map(i => i.type), ["elapsed", "seekbar", "duration"], "a non-list zone is the default");
    assert.deepStrictEqual(cleaned.zones.right[0].options, {
        "size": "m"
    });
    const ids = Object.keys(cleaned.zones).reduce((all, z) => all.concat(cleaned.zones[z].map(i => i.id)), []);
    assert.strictEqual(new Set(ids).size, ids.length, "ids are unique within a surface");

    const side = L.cleanSurface("side", {
        "panesFollowBar": false,
        "zones": {
            "panes": [{
                    "type": "lyrics"
                }, {
                    "type": "lyrics"
                }]
        }
    });
    assert.deepStrictEqual(side.zones.panes.map(i => i.type), ["lyrics"], "a pane appears once");

    const item = L.effective({
        "items": {
            "track": {
                "zones": {
                    "menu": [{
                            "type": "playNext"
                        }, {
                            "type": "playNext"
                        }, {
                            "type": "divider"
                        }, {
                            "type": "divider"
                        }],
                    "trailing": [{
                            "type": "spacer"
                        }, {
                            "type": "spacer"
                        }, {
                            "type": "more"
                        }, {
                            "type": "more"
                        }]
                }
            }
        }
    }).items.track;
    assert.deepStrictEqual(types(item.zones.menu), ["playNext", "divider", "divider"]);
    assert.deepStrictEqual(types(item.zones.trailing), ["spacer", "spacer", "more"]);
    assert.deepStrictEqual(types(item.zones.context), types(item.zones.menu), "the right-click menu follows the ⋮ menu");

    const lists = L.effective({
        "lists": {
            "overrides": {
                "tracks": {
                    "layout": "spiral",
                    "lines": ["artist", "artist", "duration", 5, "x\u0007"],
                    "sort": "a".repeat(40),
                    "filters": {
                        "library": 3,
                        "favorites": true
                    },
                    "extra": "kept"
                },
                "playlist": {
                    "sort": "title"
                },
                "albums:1": {
                    "sort": "title"
                },
                "playlist:p1": {
                    "sort": "title"
                },
                "favorites": {
                    "sections": [{
                            "type": "albums",
                            "hidden": true
                        }, {
                            "type": "tracks",
                            "hidden": true
                        }, {
                            "type": "artists",
                            "hidden": true
                        }, {
                            "type": "spam"
                        }]
                }
            }
        }
    }).lists;
    assert.deepStrictEqual(lists.overrides.tracks, {
        "lines": ["artist"],
        "filters": {
            "favorites": true
        },
        "extra": "kept"
    });
    assert.deepStrictEqual(Object.keys(lists.overrides), ["tracks", "playlist:p1", "favorites"], "keys must match the kind");
    const sections = lists.overrides.favorites.sections;
    assert.deepStrictEqual(sections.map(s => s.type), ["albums", "tracks", "artists"]);
    assert.ok(sections.some(s => !s.hidden), "one section stays visible");
}

// Problems: essentials warn
const problemCases = [
    ["defaults have none", "bar", {}, []],
    ["no play/pause", "fullBento", {
            "surfaces": {
                "fullBento": {
                    "zones": {
                        "controls": [{
                                "type": "next"
                            }]
                    }
                }
            }
        }, ["noPlayPause"]],
    ["hidden play/pause counts as none", "side", {
            "surfaces": {
                "side": {
                    "panesFollowBar": false,
                    "zones": {
                        "controls": [{
                                "type": "playPause",
                                "hidden": true
                            }]
                    }
                }
            }
        }, ["noPlayPause"]],
    ["bar without a way to the full view", "bar", {
            "surfaces": {
                "bar": {
                    "zones": {
                        "left": [{
                                "type": "artists"
                            }]
                    }
                }
            }
        }, ["noFullView"]],
    ["expand opens the full view", "bar", {
            "surfaces": {
                "bar": {
                    "zones": {
                        "left": [],
                        "right": [{
                                "type": "expand"
                            }]
                    }
                }
            }
        }, []],
    ["item without ⋮", "album", {
            "items": {
                "album": {
                    "zones": {
                        "trailing": [{
                                "type": "play"
                            }]
                    }
                }
            }
        }, ["noMenuButton"]],
    ["home has no essentials", "home", {
            "surfaces": {
                "home": {
                    "zones": {
                        "shelves": []
                    }
                }
            }
        }, []]
];
for (const [name, id, layout, expected] of problemCases)
    assert.deepStrictEqual(L.problems(L.effective(layout), id), expected, name);

// Side panes derived from the bar
const bar = right => ({
        "surfaces": {
            "bar": {
                "zones": {
                    "right": right.map(t => typeof t === "string" ? {
                            "type": t
                        } : t)
                }
            }
        }
    });
const paneCases = [
    ["bar order", bar(["queue", "lyrics"]), ["cover", "queue", "lyrics"]],
    ["a pane the bar doesn't show goes last", bar(["queue"]), ["cover", "queue", "lyrics"]],
    ["neither in the bar keeps the default order", bar(["volume"]), ["cover", "lyrics", "queue"]],
    ["a hidden button counts as not shown", bar([{
                "type": "lyrics",
                "hidden": true
            }, "queue"]), ["cover", "queue", "lyrics"]],
    ["own panes when not following", {
            "surfaces": {
                "side": {
                    "panesFollowBar": false,
                    "zones": {
                        "panes": [{
                                "type": "queue"
                            }, {
                                "type": "cover"
                            }]
                    }
                }
            }
        }, ["queue", "cover"]],
    ["an empty panel keeps a cover pane", {
            "surfaces": {
                "side": {
                    "panesFollowBar": false,
                    "zones": {
                        "panes": []
                    }
                }
            }
        }, ["cover"]]
];
for (const [name, layout, expected] of paneCases)
    assert.deepStrictEqual(L.effective(layout).sidePanes, expected, name);

// While following, the side panel's shared zones come from the bar; the seek line stays its own
const edBar = {
    "surfaces": {
        "bar": {
            "zones": {
                "left": [{
                        "type": "cover"
                    }, {
                        "type": "title"
                    }, {
                        "type": "album",
                        "hidden": true
                    }],
                "controls": [{
                        "type": "previous"
                    }, {
                        "type": "playPause"
                    }],
                "seek": [{
                        "type": "seekbar"
                    }],
                "right": [{
                        "type": "favorite",
                        "placement": "overflow"
                    }, {
                        "type": "lyrics"
                    }, {
                        "type": "spacer"
                    }, {
                        "type": "dock"
                    }, {
                        "type": "volume"
                    }]
            }
        },
        "side": {
            "zones": {
                "top": [{
                        "type": "sleep"
                    }],
                "seek": [{
                        "type": "elapsed"
                    }]
            }
        }
    }
};
const followCases = [
    ["title without the cover, hidden kept", "title", ["title", "album"], z => z.map(i => i.hidden)[1] === true],
    ["transport in bar order", "controls", ["previous", "playPause"], null],
    ["buttons without tabs, spacers and window buttons, placement kept", "top", ["favorite", "volume"], z => z[0].placement === "overflow"],
    ["window buttons at the bottom", "bottom", ["dock"], null],
    ["the seek line stays the panel's", "seek", ["elapsed"], null]
];
for (const [name, zone, expected, extra] of followCases) {
    const z = L.effective(edBar).surfaces.side.zones[zone];
    assert.deepStrictEqual(z.map(i => i.type), expected, name);
    if (extra)
        assert.ok(extra(z), name);
}
{
    // not following: the panel's own zones
    const own = JSON.parse(JSON.stringify(edBar));
    own.surfaces.side.panesFollowBar = false;
    assert.deepStrictEqual(zoneTypes(L.effective(own), "side", "top"), ["sleep"]);
    // turning following off keeps what the panel showed
    const off = L.setSurface(edBar, "side", {
        "panesFollowBar": false,
        "zones": L.effective(edBar).surfaces.side.zones
    });
    assert.deepStrictEqual(zoneTypes(L.effective(off), "side", "top"), ["favorite", "volume"]);
    assert.deepStrictEqual(L.effective(off).surfaces.side.zones.title.map(i => [i.type, i.hidden]), [["title", false], ["album", true]]);
}

// Turning following off copies the derived order; later edits stick
{
    let layout = L.setSurface(bar(["queue", "lyrics"]), "side", {
        "panesFollowBar": false,
        "zones": L.effective(bar(["queue", "lyrics"])).surfaces.side.zones
    });
    assert.deepStrictEqual(L.effective(layout).sidePanes, ["cover", "queue", "lyrics"]);
    const zones = L.effective(layout).surfaces.side.zones;
    layout = L.setSurface(layout, "side", {
        "panesFollowBar": false,
        "zones": Object.assign({}, zones, {
            "panes": zones.panes.slice(0, 1)
        })
    });
    assert.deepStrictEqual(L.effective(layout).sidePanes, ["cover"]);
    // unknown zones from a newer version survive an edit
    layout = L.setSurface({
        "surfaces": {
            "bar": {
                "zones": {
                    "future": [{
                            "type": "x"
                        }]
                }
            }
        }
    }, "bar", {
        "zones": {}
    });
    assert.ok(Array.isArray(layout.surfaces.bar.zones.future));
}

// An edit that leaves "Follow the bottom bar" out keeps the stored choice
{
    const off = L.setSurface({}, "side", {
        "panesFollowBar": false
    });
    const edited = L.setSurface(off, "side", {
        "zones": L.effective(off).surfaces.side.zones
    });
    assert.strictEqual(L.effective(edited).surfaces.side.panesFollowBar, false);
}

// Moving an element: a zone takes only its types, and a single type only once
const one = type => [{
        "type": type
    }];
const acceptCases = [
    ["new type", "track", "hover", [], "playNext", true],
    ["already there", "track", "hover", one("playNext"), "playNext", false],
    ["already at the row end", "track", "trailing", one("more"), "more", false],
    ["spacers repeat", "track", "trailing", one("spacer"), "spacer", true],
    ["dividers repeat", "track", "menu", one("divider"), "divider", true],
    ["wrong zone", "track", "hover", [], "spacer", false],
    ["a pane once", "side", "panes", one("cover"), "cover", false],
    ["bar buttons repeat", "bar", "right", one("volume"), "volume", true]
];
for (const [name, id, zone, list, type, expected] of acceptCases)
    assert.strictEqual(L.accepts(id, zone, list, type), expected, name);

// Lines a page shows when nothing changed them
const lineCases = [
    ["search tracks", "search", ["artist", "album"]],
    ["tracks", "tracks", ["artist", "album"]],
    ["album", "album", []]
];
for (const [name, kind, expected] of lineCases)
    assert.deepStrictEqual(L.pageOptions(L.effective({}), kind, "").lines, expected, name);

// An empty list meant "the defaults" in the old model
const emptyCases = [
    ["no hover actions", {
            "trackQuickActions": []
        }, eff => types(eff.items.track.zones.hover), types(L.effective({}).items.track.zones.hover)],
    ["no home shelves", {
            "homeShelves": []
        }, eff => zoneTypes(eff, "home", "shelves"), zoneTypes(L.effective({}), "home", "shelves")]
];
for (const [name, old, pick, expected] of emptyCases)
    assert.deepStrictEqual(pick(L.effective(L.migrate(old))), expected, name);

// Names on Object.prototype are not catalogue entries
const protoCases = [
    ["saved view type", () => L.effective({
                "savedViews": [{
                        "id": "v",
                        "name": "v",
                        "type": "constructor"
                    }, {
                        "id": "w",
                        "name": "w",
                        "type": "__proto__"
                    }]
            }).savedViews.length, 0],
    ["list override key", () => Object.keys(L.effective({
                "lists": {
                    "overrides": {
                        "constructor": {
                            "layout": "grid"
                        }
                    }
                }
            }).lists.overrides).length, 0],
    ["old quick action", () => types(L.effective(L.migrate({
                    "trackQuickActions": ["constructor", "playNext"]
                })).items.track.zones.hover), ["playNext"]]
];
for (const [name, run, expected] of protoCases)
    assert.deepStrictEqual(run(), expected, name);

// A write keeps the objects of what it didn't change
{
    const before = L.effective({});
    const after = L.keepUnchanged(before, L.effective(L.setSurface({}, "bar", {
        "zones": {
            "right": one("volume")
        }
    })));
    assert.strictEqual(after.surfaces.home, before.surfaces.home);
    assert.notStrictEqual(after.surfaces.bar, before.surfaces.bar);
    assert.deepStrictEqual(zoneTypes(after, "bar", "right"), ["volume"]);
}

// Turning "Same as ⋮ menu" off starts the right-click menu as a copy
{
    const layout = L.setItem({}, "track", {
        "contextSameAsMenu": false
    });
    const item = L.effective(layout).items.track;
    assert.strictEqual(item.contextSameAsMenu, false);
    assert.deepStrictEqual(types(item.zones.context), types(item.zones.menu));
}

// Lists: a page's change is what differs from its kind's default
{
    let layout = L.setPageOptions({}, "playlist", "p1", Object.assign(L.kindDefault(L.effective({}), "playlist"), {
        "sort": "title"
    }));
    assert.deepStrictEqual(layout.lists.overrides["playlist:p1"], {
        "sort": "title"
    });
    layout = L.setKindDefault(layout, "playlist", {
        "layout": "compact"
    });
    const eff = L.effective(layout);
    assert.strictEqual(L.pageOptions(eff, "playlist", "p1").layout, "compact", "a change keeps following the default");
    assert.strictEqual(L.pageOptions(eff, "playlist", "p2").sort, "position");
    assert.strictEqual(L.changedPages(eff, "playlist"), 1);
    layout = L.resetKind(layout, "playlist");
    assert.strictEqual(L.changedPages(L.effective(layout), "playlist"), 0);
    assert.strictEqual(L.pageOptions(L.effective(layout), "playlist", "p1").layout, "list");
}

// Resets: all of it keeps only the saved views; a kind's default keeps the pages' own changes
{
    const layout = L.migrate(edited);
    assert.deepStrictEqual(L.resetAll(layout), {
        "version": 2,
        "savedViews": edited.savedViews
    });
    const changed = L.setPageOptions(L.setKindDefault(layout, "playlist", {
        "layout": "compact"
    }), "playlist", "p1", {
        "layout": "list",
        "sort": "title"
    });
    const eff = L.effective(L.resetKindDefault(changed, "playlist"));
    assert.strictEqual(L.kindDefault(eff, "playlist").layout, "list");
    assert.strictEqual(L.pageOptions(eff, "playlist", "p1").sort, "title");
}

// Import
const imports = [
    ["not JSON", "{", "notJson"],
    ["not a layout", "[1]", "notLayout"],
    ["no version", "{}", "notLayout"],
    ["too large", " ".repeat(1048577), "tooLarge"],
    ["a layout", JSON.stringify(L.migrate(edited)), ""]
];
for (const [name, text, problem] of imports)
    assert.strictEqual(L.parseImport(text).problem, problem, name);
assert.deepStrictEqual(L.effective(L.parseImport(JSON.stringify(L.migrate(edited))).layout).surfaces.bar, L.effective(L.migrate(edited)).surfaces.bar, "an export imports to the same layout");

console.log("layouts: ok");
