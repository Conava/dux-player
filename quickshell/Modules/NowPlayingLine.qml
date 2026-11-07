import QtQuick
import "../Common/Overflow.js" as Overflow

// A zone above or below the seekbar: a row of its own across the surface. What doesn't fit its width moves into its
// ⋮ menu; when the surface is too short for it, the whole line goes.
NowPlayingZone {
    id: root

    // the line's width
    required property real available
    // the height the surface can give up for its two lines, their gaps included
    property real spare: Number.POSITIVE_INFINITY
    property real gap: 0
    // the surface's other line, and whether this one sits below it
    property NowPlayingLine other: null
    property bool lower: false
    readonly property bool lyricOnly: shown.every(i => i.type === "lyricLine")
    readonly property bool yields: other !== null && other.hasContent && Overflow.yieldsHeight(lyricOnly, other.lyricOnly, lower)
    // what to set as width outside a layout: the row's own, or all of it when something in it stretches
    readonly property real lineWidth: spreads ? available : fitWidth

    line: true
    keepMoves: true
    room: available
    fits: implicitHeight + gap + (yields && other.populated ? other.implicitHeight + other.gap : 0) <= spare
}
