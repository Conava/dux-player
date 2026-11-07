.pragma library

// Dragging the border between two track list columns, as pure arithmetic.
// A column is { fill, base, size, px, min }: fill columns share the leftover width by weight base * size, the
// others are base * size pixels wide. px is the width on screen now, min the narrowest it may get.

const sizeRange = [0.4, 3];

function weight(c) {
    return c.base * c.size;
}

// a size as stored: fine enough that a released border stays under the pointer on the widest list
function stored(size) {
    return Math.round(size * 10000) / 10000;
}

// the new sizes of columns i and i + 1 when the border between them moves dx pixels to the end side (the sum of
// their widths stays, so no other column moves)
function sizesAt(cols, i, dx) {
    const a = cols[i];
    const b = cols[i + 1];
    const fillPx = cols.filter(c => c.fill).reduce((n, c) => n + c.px, 0);
    const fillWeight = cols.filter(c => c.fill).reduce((n, c) => n + weight(c), 0);
    // the size a fill column needs to be `px` wide while the leftover shrinks by what the fixed column gained
    const fillSize = (c, px) => {
        const rest = fillPx - c.px;
        return rest < 1 ? c.size : Math.max(0, px) * (fillWeight - weight(c)) / rest / c.base;
    };
    if (a.fill && b.fill) {
        const total = weight(a) + weight(b);
        const left = total * (a.px + dx) / (a.px + b.px);
        return [left / a.base, (total - left) / b.base];
    }
    if (!a.fill && !b.fill)
        return [(a.px + dx) / a.base, (b.px - dx) / b.base];
    if (!a.fill)
        return [(a.px + dx) / a.base, fillSize(b, b.px - dx)];
    return [fillSize(a, a.px + dx), (b.px - dx) / b.base];
}

function fits(cols, i, dx) {
    const a = cols[i];
    const b = cols[i + 1];
    if (a.px + dx < a.min || b.px - dx < b.min)
        return false;
    const s = sizesAt(cols, i, dx);
    return s.every(v => isFinite(v) && v >= sizeRange[0] - 1e-9 && v <= sizeRange[1] + 1e-9);
}

// { left, right, dx }: the sizes of the pair after moving the border dx pixels, held at the farthest the sizes
// and minimum widths allow
function resize(cols, i, dx) {
    let t = 1;
    if (!fits(cols, i, dx)) {
        let lo = 0;
        let hi = 1;
        for (let n = 0; n < 24; n++) {
            const mid = (lo + hi) / 2;
            if (fits(cols, i, dx * mid))
                lo = mid;
            else
                hi = mid;
        }
        t = lo;
    }
    const s = sizesAt(cols, i, dx * t);
    const clamp = v => Math.min(sizeRange[1], Math.max(sizeRange[0], v));
    return {
        "left": clamp(s[0]),
        "right": clamp(s[1]),
        "dx": dx * t
    };
}
