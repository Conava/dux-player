.pragma library

// A seekbar handle keeps half its width (inset) from both ends, so the 0–1 ratio maps onto the span between them.
function ratioAt(x, width, inset) {
    const travel = width - inset * 2;
    if (travel <= 0)
        return 0;
    return Math.max(0, Math.min(1, (x - inset) / travel));
}

function handleCenter(ratio, width, inset) {
    return inset + Math.max(0, Math.min(1, ratio)) * Math.max(0, width - inset * 2);
}
