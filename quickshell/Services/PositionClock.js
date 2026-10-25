.pragma library

// The playback position between the daemon's position events. A clock is {base, at, rate, running, limit,
// offset, offsetAt}: base seconds at wall time at (ms), moving at rate while running but not past limit. A
// small correction becomes an offset that fades out over easeMs, so the bar never jumps for drift; a big one
// (a seek) jumps.

function create(position) {
    return {
        "base": position,
        "at": 0,
        "rate": 1,
        "running": false,
        "limit": Infinity,
        "offset": 0,
        "offsetAt": 0
    };
}

function offsetAt(clock, now, easeMs) {
    if (clock.offset === 0)
        return 0;
    return clock.offset * Math.exp(-Math.max(0, now - clock.offsetAt) / easeMs);
}

function predict(clock, now, easeMs) {
    const moved = clock.running ? Math.max(0, now - clock.at) / 1000 * clock.rate : 0;
    return Math.min(clock.base + moved, clock.limit) + offsetAt(clock, now, easeMs);
}

// A reported position. at: when the daemon measured it (ms, <= now); lead: how far the clock may run ahead of
// it until the next report (a track that may still be loading); jump: start over without easing (a new track,
// a seek); jumpAt: a correction larger than this (seconds) jumps too.
function sync(clock, report, now, opts) {
    const at = Math.min(now, report.at > 0 ? report.at : now);
    const next = {
        "base": report.position,
        "at": at,
        "rate": report.rate,
        "running": report.running,
        "limit": report.lead > 0 ? report.position + report.lead : Infinity,
        "offset": 0,
        "offsetAt": now
    };
    if (opts.jump)
        return next;
    const shown = predict(clock, now, opts.easeMs);
    const offset = shown - predict(next, now, opts.easeMs);
    if (Math.abs(offset) <= opts.jumpAt)
        next.offset = offset;
    return next;
}
