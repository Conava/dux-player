.pragma library

// Pure catalog helpers: daemon shapes to the UI's, sorting, play windows, the queue diff and navigation
// entries. No QML here, so tests/catalog.test.js runs them under node.

function key(ref) {
    return ref.library + ":" + ref.id;
}

function refsOf(list) {
    return list.map(t => ({
                "library": t.library,
                "id": t.id
            }));
}

function shuffled(list) {
    const out = list.slice();
    for (let i = out.length - 1; i > 0; i--) {
        const j = Math.floor(Math.random() * (i + 1));
        const tmp = out[i];
        out[i] = out[j];
        out[j] = tmp;
    }
    return out;
}

// Sorts a copy by one value; ties go by label. Values are read once per item, not once per comparison.
function sortBy(list, valueOf, labelOf, descending) {
    const rows = list.map(i => ({
                "v": valueOf(i),
                "l": labelOf(i),
                "i": i
            }));
    const dir = descending ? -1 : 1;
    rows.sort((a, b) => {
        const order = typeof a.v === "number" && typeof b.v === "number" ? a.v - b.v : String(a.v ?? "").localeCompare(String(b.v ?? ""));
        return dir * order || a.l.localeCompare(b.l);
    });
    return rows.map(r => r.i);
}

// list -> {key: [items]}
function groupBy(list, keyOf) {
    const out = {};
    for (const i of list) {
        const k = keyOf(i);
        (out[k] ?? (out[k] = [])).push(i);
    }
    return out;
}

// sums[i] = duration of items[i..]; sums[items.length] = 0
function suffixDurations(items, durationOf) {
    const sums = new Array(items.length + 1);
    sums[items.length] = 0;
    for (let i = items.length - 1; i >= 0; i--)
        sums[i] = sums[i + 1] + (durationOf(items[i]) || 0);
    return sums;
}

function sameKeys(a, b) {
    if (a.length !== b.length)
        return false;
    for (let i = 0; i < a.length; i++) {
        if (a[i] !== b[i])
            return false;
    }
    return true;
}

// The edit that turns list `from` into `to` (arrays of unique keys): {move: [from, to]} when one entry moved,
// else {at, remove, insert: [keys]} for the changed middle, or null when they are equal. A full reorder
// comes out as one big replace.
function diff(from, to) {
    let head = 0;
    while (head < from.length && head < to.length && from[head] === to[head])
        head++;
    if (head === from.length && head === to.length)
        return null;
    let tail = 0;
    while (tail < from.length - head && tail < to.length - head && from[from.length - 1 - tail] === to[to.length - 1 - tail])
        tail++;
    const a = from.slice(head, from.length - tail);
    const b = to.slice(head, to.length - tail);
    if (a.length === b.length && a.length > 1) {
        if (a[0] === b[b.length - 1] && sameKeys(a.slice(1), b.slice(0, -1)))
            return {
                "move": [head, head + a.length - 1]
            };
        if (a[a.length - 1] === b[0] && sameKeys(a.slice(0, -1), b.slice(1)))
            return {
                "move": [head + a.length - 1, head]
            };
    }
    return {
        "at": head,
        "remove": a.length,
        "insert": b
    };
}

// The {from, to} of a reorder that moved one entry, from the new order as old indexes; null if none moved.
function singleMove(order) {
    const moved = order.map((from, to) => from !== to ? to : -1).filter(i => i >= 0);
    if (moved.length === 0)
        return null;
    const first = moved[0];
    const last = moved[moved.length - 1];
    const down = order[last] === first;
    return {
        "from": down ? first : last,
        "to": down ? last : first
    };
}

// Navigation entries {key, kind, target, label, hidden}. Every page has a place (pages missing from `stored`
// are appended, hidden); an unpinned item stays in the list hidden, so pinning it again keeps its place.
function navEntry(kind, target) {
    return {
        "key": kind + ":" + target,
        "kind": kind,
        "target": target,
        "label": ""
    };
}

function navEntries(stored, pages, shownByDefault) {
    const list = stored.length > 0 ? stored.slice() : shownByDefault.map(p => navEntry("page", p));
    for (const p of pages) {
        if (!list.some(e => e.kind === "page" && e.target === p))
            list.push(Object.assign(navEntry("page", p), {
                "hidden": true
            }));
    }
    return list;
}

// name: kept on a new entry for targets the catalog may not list (an artist without albums)
function setPinned(list, kind, target, on, name) {
    const at = list.findIndex(e => e.kind === kind && e.target === target);
    if (at < 0)
        return on ? list.concat([Object.assign(navEntry(kind, target), name ? {
                    "name": name
                } : {})]) : list;
    return list.map((e, i) => i === at ? Object.assign({}, e, {
            "hidden": !on
        }) : e);
}

// Daemon shapes to the UI's. Album, artist and playlist ids become "library:id" so they stay unique across
// libraries; rawId keeps the daemon's.

// [{id, name}] from the daemon's `artists`, or the single artist when a library sends no list; an id is ""
// when the library has no artist page for that name
function artistRefs(library, list, name, id) {
    if (Array.isArray(list) && list.length > 0)
        return list.map(a => ({
                    "id": a.id ? library + ":" + a.id : "",
                    "name": a.name ?? ""
                }));
    return name ? [{
            "id": id ? library + ":" + id : "",
            "name": name
        }] : [];
}

function track(t) {
    return {
        "library": t.library,
        "id": t.id,
        "title": t.title ?? "",
        "artist": t.artist ?? "",
        "artistId": t.artistId ? t.library + ":" + t.artistId : "",
        "artists": artistRefs(t.library, t.artists, t.artist, t.artistId),
        "album": t.album ?? "",
        "albumId": t.albumId ? t.library + ":" + t.albumId : "",
        "number": t.trackNo ?? 0,
        "disc": t.discNo ?? 1,
        "duration": t.duration ?? 0,
        "year": t.year ?? 0,
        "genre": t.genre ?? "",
        "genreId": t.genre ?? "",
        "coverId": t.coverId ?? "",
        "favorite": t.favorite === true,
        "rating": t.rating ?? 0,
        "playCount": t.playCount ?? 0,
        "lastPlayed": t.lastPlayed ?? 0,
        "unavailable": t.unavailable ?? "",
        "unavailableCode": t.unavailableCode ?? ""
    };
}

function album(a) {
    return {
        "library": a.library,
        "id": a.library + ":" + a.id,
        "rawId": a.id,
        "title": a.name ?? "",
        "name": a.name ?? "",
        "artist": a.artist ?? "",
        "artistId": a.artistId ? a.library + ":" + a.artistId : "",
        "artists": artistRefs(a.library, a.artists, a.artist, a.artistId),
        "year": a.year ?? 0,
        "genre": a.genre ?? "",
        "genreId": a.genre ?? "",
        "trackCount": a.trackCount ?? 0,
        "duration": a.duration ?? 0,
        "coverId": a.coverId ?? "",
        "added": a.added ?? 0,
        "favorite": a.favorite === true,
        "rating": a.rating ?? 0,
        "playCount": a.playCount ?? 0,
        "lastPlayed": a.lastPlayed ?? 0
    };
}

// own: the artist's albums when known, for the counts and a fallback cover
function artist(a, own) {
    const albums = own ?? [];
    return {
        "library": a.library,
        "id": a.library + ":" + a.id,
        "rawId": a.id,
        "name": a.name ?? "",
        "albumCount": a.albumCount ?? albums.length,
        "trackCount": albums.reduce((sum, al) => sum + al.trackCount, 0),
        "coverId": a.coverId || (albums.find(al => al.coverId)?.coverId ?? ""),
        "libraries": [a.library],
        "favorite": a.favorite === true,
        "rating": a.rating ?? 0
    };
}

// old: the UI's earlier copy, whose loaded tracks are kept
function playlist(p, old) {
    return {
        "library": p.library,
        "id": p.library + ":" + p.id,
        "rawId": p.id,
        "name": p.name ?? "",
        "kind": "server",
        "owner": p.owner ?? "",
        "readOnly": p.readOnly === true,
        "coverId": p.coverId ?? "",
        "trackCount": p.trackCount ?? 0,
        "duration": p.duration ?? 0,
        "items": old?.items ?? [],
        "loaded": old?.loaded ?? false,
        "mosaic": [],
        "cover": {
            "type": "auto"
        }
    };
}

function localPlaylist(p, old) {
    return {
        "library": "",
        "id": ":" + p.id,
        "rawId": p.id,
        "name": p.name ?? "",
        "kind": "local",
        "readOnly": false,
        "coverId": "",
        "trackCount": p.trackCount ?? 0,
        "duration": p.duration ?? 0,
        "items": old?.items ?? [],
        "loaded": old?.loaded ?? false,
        "mosaic": p.mosaic ?? [],
        "cover": p.cover ?? {
            "type": "auto"
        }
    };
}

// old: the UI's earlier copy; scanning is the UI's own flag (libraries.list has no scan state)
function library(l, old) {
    const config = l.config ?? {};
    return {
        "id": l.id,
        "name": l.name,
        "kind": l.kind,
        "enabled": l.enabled !== false,
        "available": l.available !== false,
        "folders": config.folders ?? [],
        "url": config.url ?? "",
        "user": config.username ?? "",
        "capabilities": l.capabilities ?? [],
        "noticeCodes": l.noticeCodes ?? [],
        "error": l.error ?? "",
        "errorCode": l.errorCode ?? "",
        "scanning": old?.scanning ?? false,
        "lastScan": l.lastScan ?? 0,
        "writeMarks": config.writeMarks === true
    };
}

// a queue origin from the daemon to the UI's play context
function contextOf(origin) {
    switch (origin.kind) {
    case "album":
    case "playlist":
    case "artist":
        return {
            "type": origin.kind,
            "id": (origin.library ?? "") + ":" + origin.id,
            "name": origin.name ?? ""
        };
    case "library":
        return {
            "type": "library",
            "id": origin.library || origin.id
        };
    case "genre":
    case "view":
        return {
            "type": origin.kind,
            "id": origin.id ?? "",
            "name": origin.name ?? ""
        };
    case "favorites":
    case "history":
    case "search":
        return {
            "type": origin.kind
        };
    }
    return {
        "type": "tracks"
    };
}

// the items holding every word of `text` (any case) in their names: title, name, artists, album, genre
function filterText(list, text) {
    const words = (text ?? "").toLowerCase().split(/\s+/).filter(w => w !== "");
    if (words.length === 0)
        return list;
    return list.filter(item => {
        const names = [item.title, item.name, item.artist, item.album, item.genre].concat((item.artists ?? []).map(a => a.name));
        const hay = names.filter(v => typeof v === "string").join("\n").toLowerCase();
        return words.every(w => hay.indexOf(w) >= 0);
    });
}
