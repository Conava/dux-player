.pragma library

// Which elements of a now-playing row move into the row's ⋮ menu when it runs short of room.

// least important first; a type not listed (track info, the seek line, the cover, play/pause) never moves
const dropOrder = ["spacer", "divider", "details", "goToArtist", "goToAlbum", "addToPlaylist", "dock", "expand", "speed", "sleep", "rating", "favorite", "lyrics", "queue", "device", "volume", "shuffle", "repeat", "previous", "next"];

function rank(type) {
    return dropOrder.indexOf(type);
}

// track info with nothing to show (stack set, showing false) takes no room
function counted(items) {
    return items.filter(i => !i.stack || i.showing !== false);
}

// items: [{key, type, placement, width, stack, showing}] in row order; room < 0 means no limit. A run of track info
// is one block, the width of its first member. Returns the keys that go into the menu: "overflow" always, "auto"
// least important first (the later of two equals first) until the rest fits beside the ⋮ button, "keep" never,
// unless keepMoves: then kept items go too, after the "auto" ones.
function overflow(all, room, buttonWidth, spacing, keepMoves) {
    const items = counted(all).map((i, n, list) => i.stack && list[n - 1]?.stack ? Object.assign({}, i, {
            "width": 0
        }) : i);
    const movable = items.filter(i => rank(i.type) >= 0);
    const out = movable.filter(i => i.placement === "overflow");
    const need = list => list.reduce((sum, i) => sum + i.width + spacing, 0);
    let rest = items.filter(i => out.indexOf(i) < 0);
    const fits = () => need(rest) + (out.length > 0 ? buttonWidth + spacing : 0) <= room;
    if (room >= 0 && !fits()) {
        const ranked = movable.filter(i => (keepMoves || i.placement !== "keep") && out.indexOf(i) < 0).sort((a, b) => (a.placement === "keep") - (b.placement === "keep") || rank(a.type) - rank(b.type) || items.indexOf(b) - items.indexOf(a));
        while (ranked.length > 0 && need(rest) + buttonWidth + spacing > room) {
            const next = ranked.shift();
            out.push(next);
            rest = rest.filter(i => i !== next);
        }
    }
    return out.map(i => i.key);
}

// items: [{type, width, stack, showing}] in row order, stack marking track info, which shares a block with the track
// info beside it. The least a row can shrink to: what never moves, the lyric lines down to nothing, and the ⋮ button.
function least(items, buttonWidth, spacing) {
    const fixed = counted(items).filter(i => rank(i.type) < 0 && i.type !== "lyricLine");
    const blocks = fixed.filter((i, n) => !i.stack || !fixed[n - 1]?.stack);
    const more = items.some(i => rank(i.type) >= 0);
    return blocks.reduce((sum, i) => sum + i.width + spacing, 0) + (more ? buttonWidth + spacing : 0) - (blocks.length > 0 || more ? spacing : 0);
}

// Of the two lines around the seekbar, whether this one goes first when the height runs short: a line holding only a
// lyric gives way to one that holds anything else, since a lost button has no ⋮ to reach it by. Else the lower one.
function yieldsHeight(lyricOnly, otherLyricOnly, lower) {
    return lyricOnly === otherLyricOnly ? lower : lyricOnly;
}
