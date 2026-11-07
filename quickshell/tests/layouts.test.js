// node quickshell/tests/layouts.test.js
const fs = require("fs");
const path = require("path");
const assert = require("assert");

const source = fs.readFileSync(path.join(__dirname, "../Common/Layouts.js"), "utf8").replace(".pragma library", "");
const L = new Function(source + "; return { migrate, needsMigration, effective, problems, setSurface, setItem, setPageOptions, setKindDefault, resetKind, pageOptions, savedViewOptions, kindDefault, changedPages, parseImport, cleanSurface, resetAll, resetKindDefault, accepts, keepUnchanged, setNowPlaying, extraTop, columnSort, columnSize, withColumnSizes, sizeName, sizeValue, canAddCustom, sortTemplate, newCustomLine, setOptionsAt };")();

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
        "openPane": "queue",
        "barStyle": "m3e",
        "sideStyle": "m3e",
        "playerLook": "card",
        "paneLook": "player"
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

    const lined = L.effective({
        "surfaces": {
            "bar": {
                "zones": {
                    "seekAbove": [{
                            "type": "lyricLine"
                        }]
                }
            }
        }
    });
    assert.deepStrictEqual(zoneTypes(lined, "side", "seekAbove"), ["lyricLine"], "a seek line stays one in the side panel");
    assert.ok(zoneTypes(lined, "side", "title").indexOf("lyricLine") < 0);

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
    assert.deepStrictEqual(L.pageOptions(eff, "albums", "").rules.list, [{
            "field": "library",
            "op": "isAny",
            "value": ["lib1"]
        }]);
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
                "openPane": "last",
                "barStyle": "bento",
                "sideStyle": "bento",
                "playerLook": "card",
        "paneLook": "player"
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
                "openPane": "lyrics",
                "barStyle": "m3e",
                "sideStyle": "m3e",
                "playerLook": "card",
        "paneLook": "player"
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
        "rules": {
            "match": "all",
            "list": [{
                    "field": "favorite",
                    "op": "is",
                    "value": true
                }]
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
                        "type": "device"
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
    ["buttons without tabs, spacers and window buttons, placement kept, device at the far end", "top", ["favorite", "volume", "spacer", "device"], z => z[0].placement === "overflow" && z[2].options.size === "fill"],
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
    assert.deepStrictEqual(zoneTypes(L.effective(off), "side", "top"), ["favorite", "volume", "spacer", "device"]);
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
    ["bar buttons repeat", "bar", "right", one("volume"), "volume", true],
    ["add to playlist at the end of a track row", "track", "trailing", [], "addToPlaylist", true],
    ["add to playlist at the end of a queue row", "queue", "trailing", [], "addToPlaylist", true],
    ["add to playlist at the end of an artist row", "artist", "trailing", [], "addToPlaylist", false],
    ["lyric line in a bar row", "bar", "left", [], "lyricLine", true],
    ["lyric line in the transport row", "bar", "controls", [], "lyricLine", true],
    ["lyric line in the seek line", "bar", "seek", [], "lyricLine", true],
    ["lyric line in a full view's row", "fullM3e", "end", [], "lyricLine", true],
    ["lyric line in a title column", "fullBento", "title", [], "lyricLine", true],
    ["a button in a title column", "side", "title", [], "volume", true],
    ["transport in a title column", "fullM3e", "title", [], "playPause", true],
    ["a small cover in a full view's row", "fullBento", "bottom", [], "cover", true],
    ["a cover in the bar's right zone", "bar", "right", [], "cover", true],
    ["times outside the seek line", "bar", "right", [], "elapsed", true],
    ["the dock button in a full view", "fullM3e", "end", [], "dock", true],
    ["the seekbar on a line of its own", "side", "seekBelow", [], "seekbar", true],
    ["the seekbar in a title column", "fullBento", "title", [], "seekbar", true],
    ["no seekbar in a row it shares", "bar", "right", [], "seekbar", false],
    ["no seekbar beside the transport", "fullM3e", "controls", [], "seekbar", false],
    ["expand has nothing to open in a full view", "fullBento", "seekAbove", [], "expand", false],
    ["expand in the bar", "bar", "seekBelow", [], "expand", true],
    ["only the art in the art's place", "fullM3e", "cover", [], "lyricLine", false]
];
for (const [name, id, zone, list, type, expected] of acceptCases)
    assert.strictEqual(L.accepts(id, zone, list, type), expected, name);

// The bar's lyric line is off until it is added
for (const zone of ["left", "controls", "right"])
    assert.ok(!zoneTypes(L.effective({}), "bar", zone).includes("lyricLine"), "not in the default bar: " + zone);

// Every now-playing surface has a zone above and below the seekbar, taking every element but the ones it can't show
const nowPlayingIds = ["bar", "side", "fullBento", "fullM3e"];
for (const id of nowPlayingIds) {
    for (const zone of ["seekAbove", "seekBelow"]) {
        for (const type of ["lyricLine", "title", "text", "cover", "elapsed", "seekbar", "playPause", "volume", "details", "spacer", "divider", "dock"])
            assert.ok(L.accepts(id, zone, [], type), type + " in " + id + " " + zone);
    }
}

// A layout from before these zones reads as empty new zones, with every old zone as it was
const oldBar = {
    "surfaces": {
        "bar": {
            "zones": {
                "left": [{
                        "id": "cover",
                        "type": "cover"
                    }]
            }
        }
    }
};
for (const stored of [{}, oldBar, edited]) {
    const eff = L.effective(L.migrate(stored));
    for (const id of nowPlayingIds) {
        assert.deepStrictEqual(eff.surfaces[id].zones.seekAbove, [], id + " above");
        assert.deepStrictEqual(eff.surfaces[id].zones.seekBelow, [], id + " below");
    }
}
assert.deepStrictEqual(zoneTypes(L.effective({}), "bar", "seek"), ["elapsed", "seekbar", "duration"]);

// The new zones survive a write and a read; the rest of the surface stays
const withZone = (id, zone, list) => {
    const zones = JSON.parse(JSON.stringify(L.effective({}).surfaces[id].zones));
    zones[zone] = list;
    return L.setSurface({}, id, {
        "zones": zones
    });
};
{
    const stored = JSON.parse(JSON.stringify(withZone("fullBento", "seekBelow", [{
                        "type": "lyricLine"
                    }, {
                        "type": "volume"
                    }])));
    const eff = L.effective(stored);
    assert.deepStrictEqual(zoneTypes(eff, "fullBento", "seekBelow"), ["lyricLine", "volume"]);
    assert.deepStrictEqual(zoneTypes(eff, "fullBento", "bottom"), zoneTypes(L.effective({}), "fullBento", "bottom"));
}

// The width option of a lyric line only means something in the bar's left and right zones, and the art's options in the art's place
{
    const options = (id, zone, type) => L.effective(withZone(id, zone, [{
                    "type": type
                }])).surfaces[id].zones[zone][0].options;
    assert.strictEqual(options("bar", "right", "lyricLine").width, "m");
    assert.strictEqual(options("bar", "seekAbove", "lyricLine").width, undefined);
    assert.strictEqual(options("fullBento", "seekBelow", "lyricLine").width, undefined);
    assert.strictEqual(options("fullM3e", "title", "lyricLine").smooth, false);
    assert.strictEqual(options("fullBento", "cover", "cover").size, "m");
    assert.strictEqual(options("fullBento", "bottom", "cover").size, undefined);
}

// The title's alignment stays unset until it is chosen, and later edits of the surface keep it
{
    assert.strictEqual(L.effective({}).surfaces.side.centerTitle, undefined);
    const centered = L.setSurface({}, "fullM3e", {
        "zones": {},
        "centerTitle": true
    });
    const moved = L.setSurface(centered, "fullM3e", {
        "zones": L.effective(centered).surfaces.fullM3e.zones
    });
    assert.strictEqual(L.effective(moved).surfaces.fullM3e.centerTitle, true);
}

// While the side panel follows, the bar's lines stay lines there, and the side's own lines win
{
    const stored = withZone("bar", "seekAbove", [{
                "type": "lyricLine"
            }, {
                "type": "sleep"
            }, {
                "type": "spacer"
            }]);
    const side = L.effective(stored).surfaces.side;
    assert.ok(!types(side.zones.title).includes("lyricLine"));
    assert.ok(!types(side.zones.top).includes("sleep"));
    assert.deepStrictEqual(types(side.zones.seekAbove), ["lyricLine", "sleep", "spacer"]);
    const own = L.setSurface(stored, "side", Object.assign({}, side, {
        "zones": Object.assign({}, side.zones, {
            "seekBelow": [{
                    "type": "rating"
                }]
        })
    }));
    assert.deepStrictEqual(zoneTypes(L.effective(own), "side", "seekBelow"), ["rating"]);
    assert.strictEqual(L.effective(own).surfaces.side.panesFollowBar, true);
}

// What the side panel takes from the bar keeps the old reading order: transport row before the seek line
{
    const zones = JSON.parse(JSON.stringify(L.effective({}).surfaces.bar.zones));
    zones.seek.push({
        "type": "sleep"
    }, {
        "type": "lyrics"
    });
    zones.controls.push({
        "type": "speed"
    }, {
        "type": "queue"
    });
    const eff = L.effective(L.setSurface({}, "bar", {
        "zones": zones
    }));
    const top = types(eff.surfaces.side.zones.top);
    assert.ok(top.indexOf("speed") < top.indexOf("sleep"), "transport row first");
    assert.deepStrictEqual(eff.sidePanes, ["cover", "queue", "lyrics"]);
}

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

// The default bar has shuffle and repeat around the transport, and the side dock following it the same row
{
    const eff = L.effective({});
    assert.deepStrictEqual(zoneTypes(eff, "bar", "controls"), ["shuffle", "previous", "playPause", "next", "repeat"]);
    assert.deepStrictEqual(zoneTypes(eff, "side", "controls"), ["shuffle", "previous", "playPause", "next", "repeat"]);
}

// Styles per surface: each follows the full view until set, and stays when that changes
{
    const eff = L.effective(L.setNowPlaying({}, {
        "style": "m3e"
    })).nowPlaying;
    assert.strictEqual(eff.barStyle, "m3e");
    assert.strictEqual(eff.sideStyle, "m3e");
    const mixed = L.effective(L.setNowPlaying(L.setNowPlaying({}, {
        "barStyle": "bento",
        "sideStyle": "m3e"
    }), {
        "style": "bento"
    })).nowPlaying;
    assert.deepStrictEqual([mixed.barStyle, mixed.sideStyle, mixed.style], ["bento", "m3e", "bento"]);
}

// Player look: only known values are stored
{
    assert.strictEqual(L.effective(L.setNowPlaying({}, {
        "playerLook": "x"
    })).nowPlaying.playerLook, "card");
    assert.strictEqual(L.effective(L.setNowPlaying({}, {
        "playerLook": "attached"
    })).nowPlaying.playerLook, "attached");
}

// Art shape and rotation: a stored value outside the schema falls back to the surface's own default
{
    const cover = (id, options) => L.cleanSurface(id, {
            "zones": {
                "cover": [{
                        "type": "cover",
                        "options": options
                    }]
            }
        }).zones.cover[0].options;
    const cases = [
        ["unknown shape on Bento", "fullBento", {
                "shape": "blob"
            }, "shape", "cookie"],
        ["Material defaults to rounded", "fullM3e", {}, "shape", "rounded"],
        ["the side panel defaults to rounded", "side", {}, "shape", "rounded"],
        ["a known shape stays on the side panel", "side", {
                "shape": "cookie"
            }, "shape", "cookie"],
        ["a known shape stays", "fullM3e", {
                "shape": "circle"
            }, "shape", "circle"],
        ["rotation off stays off", "fullBento", {
                "rotate": false
            }, "rotate", false],
        ["rotation as text is dropped", "fullBento", {
                "rotate": "no"
            }, "rotate", true]
    ];
    for (const [name, id, options, key, want] of cases)
        assert.strictEqual(cover(id, options)[key], want, name);
}

// Title style: Bento and the side panel offer it, defaulting to the expressive face
{
    const title = (id, options) => L.cleanSurface(id, {
            "zones": {
                "title": [{
                        "type": "title",
                        "options": options
                    }]
            }
        }).zones.title[0].options.face;
    assert.strictEqual(title("fullBento", {}), "expressive");
    assert.strictEqual(title("side", {
        "face": "display"
    }), "display");
    assert.strictEqual(title("side", {
        "face": "comic"
    }), "expressive");
    assert.strictEqual(title("fullM3e", {}), undefined);
    // the default layouts carry the heart; a customized title keeps what it had
    assert.strictEqual(L.effective({}).surfaces.bar.zones.left.find(i => i.type === "title").options.favorite, true);
    assert.strictEqual(L.setSurface({}, "side", {
        "zones": {
            "title": [{
                    "type": "title"
                }]
        }
    }).surfaces.side.zones.title[0].options.favorite, false);
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

{
    const withCustom = L.setKindDefault({}, "tracks", Object.assign(L.kindDefault(L.effective({}), "tracks"), {
        "lines": ["custom:k1"]
    }));
    const stored = L.setPageOptions(withCustom, "playlist", "p1", Object.assign(L.pageOptions(L.effective(withCustom), "playlist", "p1"), {
        "sort": "custom:k1",
        "lines": ["custom:k1"]
    }));
    assert.strictEqual(L.pageOptions(L.effective(stored), "playlist", "p1").sort, "custom:k1");
    const gone = L.setPageOptions(stored, "playlist", "p1", Object.assign(L.pageOptions(L.effective(stored), "playlist", "p1"), {
        "lines": []
    }));
    assert.strictEqual(L.pageOptions(L.effective(gone), "playlist", "p1").sort, "position", "custom sort without the line falls back to the kind's sort");
    const view = {
        "type": "tracks",
        "options": {
            "sort": "custom:k1"
        }
    };
    assert.strictEqual(L.savedViewOptions(L.effective({}), view).sort, L.kindDefault(L.effective({}), "tracks").sort, "saved view");
}

// Several custom text columns, each with its own template, sort and width
{
    const read = record => L.kindDefault(L.effective(L.setKindDefault({}, "tracks", record)), "tracks");
    const record = {
        "lines": ["artist", "custom:k1", "custom:p9", "year"],
        "sort": "custom:p9",
        "lineOptions": {
            "title": {"size": 1.5},
            "custom:k1": {"title": "Release", "template": "%album% (%year%)", "sort": "year", "size": 1},
            "custom:p9": {"title": "Quality", "template": "%format%", "size": 0.7}
        }
    };
    const got = read(record);
    assert.deepStrictEqual(got.lines, record.lines, "two custom columns are kept");
    assert.deepStrictEqual(got.lineOptions, record.lineOptions, "each keeps its own template, sort and size");
    assert.strictEqual(got.sort, "custom:p9");

    const nine = Array.from({length: 10}, (_, i) => "custom:c" + i);
    assert.strictEqual(read({"lines": nine}).lines.length, 8, "custom columns are capped");
    assert.deepStrictEqual(read({"lines": ["custom:a b", "custom:", "custom:ok"]}).lines, ["custom:ok"], "bad custom keys are dropped");

    const hostile = read({
        "lineOptions": {
            "custom:k1": {"title": "x".repeat(65), "template": "a\u0000b", "sort": "bad sort!", "size": 99},
            "artist": {"title": "no title on a built-in column", "template": "%x%", "size": 0.01},
            "bad key": {"size": 1},
            "year": "text"
        }
    }).lineOptions;
    assert.deepStrictEqual(hostile, {"custom:k1": {"size": 3}, "artist": {"size": 0.4}}, "hostile values are rejected or clamped");

    // the old format: lines ["custom"], lineTemplate, lineTitle, sort "custom"
    const old = read({"lines": ["artist", "custom"], "lineTemplate": "%year%", "lineTitle": "Y", "sort": "custom"});
    assert.deepStrictEqual(old.lines, ["artist", "custom:c1"]);
    assert.deepStrictEqual(old.lineOptions, {"custom:c1": {"title": "Y", "template": "%year%"}}, "the old template and title carry over");
    assert.strictEqual(old.sort, "custom:c1");
    assert.strictEqual(old.lineTemplate, undefined);

    const o = {"lines": ["artist", "custom:k1"], "lineOptions": {"artist": {"sort": "albumArtist"}, "custom:k1": {"template": "%year%", "sort": "custom:zz"}}};
    assert.strictEqual(L.columnSort(o, "artist"), "albumArtist", "Sort by changes what a header sorts");
    assert.strictEqual(L.columnSort(o, "album"), "album", "no Sort by means its own key");
    assert.strictEqual(L.columnSort(o, "custom:k1"), "custom:k1", "a sort on a column that is gone falls back to the column's text");
    assert.strictEqual(L.sortTemplate(o, "custom:k1"), "%year%");
    assert.strictEqual(L.sortTemplate(o, "custom:k2"), "", "a custom sort without options sorts by empty text");
    assert.strictEqual(L.sortTemplate(o, "year"), null);
    assert.strictEqual(L.columnSize(o, "artist"), 1);
    assert.deepStrictEqual(["s", "m", "l"].map(k => L.sizeName(({"s": 0.7, "m": 1, "l": 1.5})[k])), ["s", "m", "l"]);
    assert.strictEqual(L.sizeName(1.2), "", "a dragged width lights no preset");
    assert.strictEqual(L.newCustomLine(["custom:c1", "custom:c3"]), "custom:c2");
    assert.strictEqual(L.newCustomLine(["custom:c1"], {"sort": "custom:c2"}), "custom:c3", "a stored page sort keeps its id taken");
    assert.strictEqual(L.newCustomLine(["custom:c1"], {"lineOptions": {"artist": {"sort": "custom:c2"}}}), "custom:c3", "a column's Sort by keeps its id taken");
    assert.deepStrictEqual(["s", "m", "l"].map(L.sizeValue), [0.7, 1, 1.5]);
    assert.strictEqual(L.canAddCustom(nine.slice(0, 7)), true);
    assert.strictEqual(L.canAddCustom(nine.slice(0, 8)), false, "the eighth custom column is the last");
}

// Filter rules
{
    const rule = (field, op, value) => ({ field, op, value });
    const genre = rule("genre", "isAny", ["Rock", "Metal"]);
    const stored = rules => ({
        "lists": {
            "overrides": {
                "tracks": { rules }
            }
        }
    });
    const clean = rules => L.effective(stored(rules)).lists.overrides.tracks.rules;

    // the file's round trip through a page edit
    const written = L.setPageOptions({}, "tracks", "", Object.assign({}, L.pageOptions(L.effective({}), "tracks", ""), {
        "rules": {
            "match": "any",
            "list": [genre, rule("year", "between", [1990, null])]
        }
    }));
    const read = L.pageOptions(L.effective(JSON.parse(JSON.stringify(written))), "tracks", "");
    assert.strictEqual(read.rules.match, "any");
    assert.deepStrictEqual(read.rules.list, [genre, rule("year", "between", [1990, null])], "a blank end of a range is kept");
    assert.deepStrictEqual(Object.keys(L.setPageOptions({}, "tracks", "", L.pageOptions(L.effective({}), "tracks", "")).lists?.overrides ?? {}), [], "no rules is no change");

    // a page's rules replace the kind's
    const withDefault = L.setKindDefault({}, "albums", Object.assign({}, L.kindDefault(L.effective({}), "albums"), {
        "rules": {
            "match": "all",
            "list": [genre]
        }
    }));
    const onPage = L.setPageOptions(withDefault, "genre", "Rock", Object.assign({}, L.pageOptions(L.effective(withDefault), "genre", "Rock"), {
        "rules": {
            "match": "all",
            "list": [rule("year", "is", 1)]
        }
    }));
    assert.strictEqual(L.pageOptions(L.effective(withDefault), "albums", "").rules.list.length, 1, "the kind default holds rules");
    assert.deepStrictEqual(L.pageOptions(L.effective(onPage), "genre", "Rock").rules.list, [rule("year", "is", 1)]);

    // a saved view: the same cleaning, on the view's own rules
    const view = L.effective({
        "savedViews": [{
                "id": "v",
                "name": "V",
                "type": "tracks",
                "options": {
                    "rules": {
                        "match": "any",
                        "list": [genre, "junk"]
                    }
                }
            }]
    }).savedViews[0];
    assert.deepStrictEqual(view.options.rules, {
        "match": "any",
        "list": [genre]
    });

    // hostile and broken input: nothing throws, what can't be trusted goes
    assert.deepStrictEqual(clean({
        "match": "some",
        "list": [null, 3, "x", {}, {
                "field": "bad field",
                "op": "is"
            }, {
                "field": 5,
                "op": "is"
            }, {
                "op": "is"
            }, {
                "field": "a".repeat(33),
                "op": "is"
            }]
    }), {
        "match": "all",
        "list": []
    });
    assert.deepStrictEqual(clean({ "list": "genre" }), { "match": "all", "list": [] }, "a list that isn't one");
    assert.strictEqual(L.effective(stored("junk")).lists.overrides.tracks.rules, undefined, "rules that aren't an object");
    const values = v => clean({
        "list": [rule("genre", "isAny", v)]
    }).list[0].value;
    assert.strictEqual(values("a\u0007"), undefined, "control characters");
    assert.strictEqual(values("a".repeat(257)), undefined, "a long value");
    assert.strictEqual(values(Infinity), undefined, "an infinite number");
    assert.strictEqual(values({ "a": 1 }), undefined, "an object");
    assert.strictEqual(values(["ok", "a\u0007"]), undefined, "a bad entry in a list");
    assert.strictEqual(values([["nested"]]), undefined, "a nested list");
    assert.strictEqual(values(new Array(33).fill("a")), undefined, "33 values");
    assert.strictEqual(values(new Array(32).fill("a")).length, 32, "32 values");
    assert.strictEqual(clean({
        "list": new Array(40).fill(genre)
    }).list.length, 32, "at most 32 rules");
    assert.deepStrictEqual(clean({
        "list": [{ "field": "mood", "op": "soundsLike", "value": "sad" }]
    }).list, [{ "field": "mood", "op": "soundsLike", "value": "sad" }], "a rule from a newer version is kept");

    // old filters become rules once, on the kinds that have rules
    const filters = (kind, f, more) => L.effective({
        "lists": {
            "overrides": {
                [kind]: Object.assign({ "filters": f }, more)
            }
        }
    }).lists.overrides[kind];
    const old = filters("tracks", { "library": "l1", "genre": "Rock", "favorites": true });
    assert.deepStrictEqual(old, {
        "rules": {
            "match": "all",
            "list": [rule("library", "isAny", ["l1"]), rule("genre", "isAny", ["Rock"]), rule("favorite", "is", true)]
        }
    });
    assert.deepStrictEqual(filters("tracks", { "library": "", "genre": "", "favorites": false }), {}, "empty filters leave nothing");
    assert.deepStrictEqual(filters("playlists", { "library": "l1" }).filters, { "library": "l1" }, "playlists keep their library filter");
    assert.strictEqual(filters("playlists", { "library": "l1" }).rules, undefined);

    // saving a page as a view turns its locked filter into a rule next to the page's rules
    const locked = filters("albums", { "genre": "Rock" }, {
        "rules": {
            "match": "all",
            "list": [rule("year", "is", 1)]
        }
    });
    assert.deepStrictEqual(locked.rules.list, [rule("genre", "isAny", ["Rock"]), rule("year", "is", 1)]);
    const anyOfTwo = filters("albums", { "genre": "Rock" }, {
        "rules": {
            "match": "any",
            "list": [rule("year", "is", 1), rule("year", "is", 2)]
        }
    });
    assert.strictEqual(anyOfTwo.filters.genre, "Rock", "a filter over any of two rules stays a filter");
    assert.strictEqual(anyOfTwo.rules.list.length, 2);
    assert.strictEqual(filters("albums", { "genre": "Rock" }, {
        "rules": {
            "match": "any",
            "list": [rule("year", "is", 1)]
        }
    }).rules.match, "all", "any of one rule joins the filter");
}

console.log("layouts: ok");

// Unknown top-level keys survive a write; only the old-format keys that migrate() consumed go.
assert.deepStrictEqual(L.extraTop({ "layout": { "version": 1 }, "future": { "a": 1 }, "nowPlaying": {} }), { "layout": { "version": 1 }, "future": { "a": 1 }, "nowPlaying": {} });
assert.deepStrictEqual(L.extraTop({ "themeMode": "dark", "future": 2, "nowPlaying": {}, "savedViews": [] }), { "themeMode": "dark", "future": 2 });
assert.deepStrictEqual(L.extraTop("junk"), {});

// setOptionsAt writes to the captured target, whatever page is current
{
    const list = [{ "field": "year", "op": "is", "value": 2000 }];
    const rules = { "match": "all", "list": list };
    let s = L.setOptionsAt({}, { kind: "tracks", itemId: "", allScope: false }, { rules });
    assert.deepStrictEqual(L.pageOptions(L.effective(s), "tracks", "").rules.list.length, 1);
    assert.strictEqual(L.pageOptions(L.effective(s), "albums", "").rules.list.length, 0, "other pages are untouched");
    const sorted = L.setPageOptions({}, "tracks", "", Object.assign(L.pageOptions(L.effective({}), "tracks", ""), { "sort": "year" }));
    assert.strictEqual(L.pageOptions(L.effective(L.setOptionsAt(sorted, { kind: "tracks", itemId: "", allScope: false }, { rules })), "tracks", "").sort, "year", "other options stay");
    s = L.setOptionsAt({}, { kind: "playlist", itemId: "p1", allScope: true }, { rules });
    assert.strictEqual(L.kindDefault(L.effective(s), "playlist").rules.list.length, 1, "all pages: the kind's default");
    assert.strictEqual(L.pageOptions(L.effective(s), "playlist", "p1").rules.list.length, 1);
    const view = { id: "v1", name: "V", type: "tracks", options: {} };
    const withView = { savedViews: [view] };
    s = L.setOptionsAt(withView, { view: "v1" }, { rules });
    assert.strictEqual(L.savedViewOptions(L.effective(s), L.effective(s).savedViews[0]).rules.list.length, 1);
    assert.deepStrictEqual(L.effective(L.setOptionsAt({}, { view: "gone" }, { rules })).savedViews, [], "a deleted view is not recreated");
    assert.strictEqual(L.effective(L.setOptionsAt(withView, { view: "gone" }, { rules })).savedViews[0].options.rules, undefined, "another view is not touched");
}

// a column drag stores both sizes at once; 1 removes the size and an emptied entry
{
    const o = { "lineOptions": { "artist": { "size": 2, "sort": "album" }, "album": { "size": 0.5 }, "year": { "size": 1.5 } } };
    const r = L.withColumnSizes(o, { "artist": 1, "album": 1, "genre": 0.8 });
    assert.deepStrictEqual(r.lineOptions, { "artist": { "sort": "album" }, "year": { "size": 1.5 }, "genre": { "size": 0.8 } });
    assert.strictEqual(o.lineOptions.album.size, 0.5);
}

// a custom page's own options are checked on load: a bad value is dropped, a good one survives
{
    const view = (options, icon) => ({ "savedViews": [{ "id": "v1", "name": "V", "type": "tracks", "options": options, "icon": icon }] });
    const read = (options, icon) => L.effective(view(options, icon)).savedViews[0];
    const good = read({ "limit": 50, "click": "one", "ranked": true, "shuffleButton": false, "sort": "random", "group": "artist" }, "star");
    assert.deepStrictEqual([good.options.limit, good.options.click, good.options.ranked, good.options.shuffleButton, good.options.sort, good.options.group, good.icon], [50, "one", true, false, "random", "artist", "star"]);
    const bad = read({ "limit": 7, "click": "all", "ranked": "yes", "shuffleButton": 0 }, "../x");
    assert.deepStrictEqual([bad.options.limit, bad.options.click, bad.options.ranked, bad.options.shuffleButton, bad.icon], [undefined, undefined, undefined, undefined, undefined]);
    assert.strictEqual(read({ "limit": "50" }).options.limit, undefined, "a number as text");
    assert.strictEqual(read({}, "x".repeat(49)).icon, undefined, "an icon name has a bound");
    assert.strictEqual(L.savedViewOptions(L.effective({}), read({})).limit, undefined, "no limit unless set");
}

// editing a custom page keeps its icon and the new options
{
    const s = { "savedViews": [{ "id": "v1", "name": "V", "type": "tracks", "options": {}, "icon": "bolt" }] };
    const next = L.effective(L.setOptionsAt(s, { view: "v1" }, { "limit": 25, "ranked": true })).savedViews[0];
    assert.deepStrictEqual([next.icon, next.options.limit, next.options.ranked], ["bolt", 25, true]);
}
