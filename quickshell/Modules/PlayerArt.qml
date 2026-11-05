pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Common as DC
import qs.Widgets

// The now-playing artwork in a shape (the cover's shape option). The cookie is a circle while paused, springs
// into the cookie while playing and, with rotate on, turns slowly; the turn stops whenever nobody sees it.
DArtShape {
    id: root

    property var item: null
    property bool rotate: true
    // covered (lyrics over the art) or a Settings preview: no motion
    property bool still: false

    readonly property bool cookie: shape === "cookie"
    readonly property bool playing: PlayerService.playing
    readonly property bool shown: visible && opacity > 0 && Window.visibility !== Window.Minimized && Window.visibility !== Window.Hidden
    readonly property bool spinning: cookie && rotate && playing && shown && !still && Theme.animationsEnabled
    readonly property var morphSpring: Theme.springPreset("fast", Theme.expressiveDurations.expressiveFastSpatial)
    property real turn: 0

    morph: cookie ? morphMotion.value : 1
    spin: rotate ? turn : 0
    onPlayingChanged: still ? morphMotion.snapTo(playing ? 1 : 0) : morphMotion.retarget(playing ? 1 : 0)

    DC.SpringMotion {
        id: morphMotion
        enabled: !Theme.springMotionDisabled
        stiffness: root.morphSpring.stiffness
        damping: root.morphSpring.damping
        Component.onCompleted: snapTo(root.playing ? 1 : 0)
    }

    NumberAnimation on turn {
        from: 0
        to: 2 * Math.PI
        duration: PlayerMetrics.mediaArtRotationDuration
        loops: Animation.Infinite
        running: root.cookie && root.rotate
        paused: running && !root.spinning
    }

    AlbumArt {
        anchors.fill: parent
        radius: 0
        item: root.item
    }
}
