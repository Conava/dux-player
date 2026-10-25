.pragma library

// Which elements of a now-playing row move into the row's ⋮ menu when it runs short of room.

// least important first; a type not listed (track info, the seek line, the cover, play/pause) never moves
const dropOrder = ["spacer", "divider", "details", "goToArtist", "goToAlbum", "addToPlaylist", "dock", "expand", "speed", "sleep", "rating", "favorite", "lyrics", "queue", "device", "volume", "shuffle", "repeat", "previous", "next"];

function rank(type) {
    return dropOrder.indexOf(type);
}

// items: [{key, type, placement, width}] in row order; room < 0 means no limit. Returns the keys that go into
// the menu: "overflow" always, "auto" least important first (the later of two equals first) until the rest fits
// beside the ⋮ button, "keep" never.
function overflow(items, room, buttonWidth, spacing) {
    const movable = items.filter(i => rank(i.type) >= 0);
    const out = movable.filter(i => i.placement === "overflow");
    const need = list => list.reduce((sum, i) => sum + i.width + spacing, 0);
    let rest = items.filter(i => out.indexOf(i) < 0);
    const fits = () => need(rest) + (out.length > 0 ? buttonWidth + spacing : 0) <= room;
    if (room >= 0 && !fits()) {
        const ranked = movable.filter(i => i.placement !== "keep" && out.indexOf(i) < 0).sort((a, b) => rank(a.type) - rank(b.type) || items.indexOf(b) - items.indexOf(a));
        while (ranked.length > 0 && need(rest) + buttonWidth + spacing > room) {
            const next = ranked.shift();
            out.push(next);
            rest = rest.filter(i => i !== next);
        }
    }
    return out.map(i => i.key);
}
