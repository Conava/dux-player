pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Effects
import qs.Common
import qs.Services

// The playing cover, decoded small and blurred behind the full view, under a scrim of the view's surface. The blur is
// rendered when a cover lands or the player pauses, not while the view moves; the effect and both images exist only
// while this item does. While paused the blur deepens and the scrim thickens a step.
Item {
    id: root
    objectName: "fullBackground"

    // the cover whose colours show, so the cross-fade runs with the colours and the art's blur-in
    readonly property string cover: PlayerService.coverUrl(ArtColors.track)

    // The scrim rises with how far the cover's lightness sits from the surface's: the accent text on a light surface
    // keeps 4.5:1 only while a dark cover shows through at under a tenth, and likewise a bright cover in dark mode.
    readonly property real scrimMax: 0.8
    // the same mismatch also shifts the cover's tone toward the surface's, so the blur keeps its hue under the scrim
    readonly property real toneMax: 0.6
    readonly property real mismatchGain: 1.5
    readonly property real mismatch: cover === "" ? 0 : Math.min(1, mismatchGain * Math.abs(Qt.color(PlayerService.coverColor(ArtColors.track)).hslLightness - ArtColors.surface.hslLightness))

    readonly property real pausedBlurMultiplier: 0.5
    readonly property real pausedScrimStep: 0.1
    // 1 while paused
    property real depth: PlayerService.playing ? 0 : 1
    Behavior on depth {
        enabled: Theme.animationsEnabled && root.visible
        NumberAnimation {
            duration: Theme.expressiveDurations.expressiveEffects
            easing.type: Easing.BezierSpline
            easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
        }
    }

    // the window paints the surface at this alpha, and the blurred layer follows it
    opacity: Theme.floatingWindowTransparency
    visible: front.source.toString() !== ""

    onCoverChanged: {
        const had = front.status === Image.Ready && cover !== "" && Theme.animationsEnabled && visible;
        back.source = had ? front.source : "";
        front.opacity = had ? 0 : 1;
        front.source = cover;
        // a cached cover is Ready at once, and Ready to Ready sends no status change
        if (had && front.status === Image.Ready)
            fade.restart();
    }

    Item {
        id: stage
        anchors.fill: parent

        Layer {
            id: back
        }

        Layer {
            id: front
            source: root.cover
            onStatusChanged: {
                if (status !== Image.Ready || opacity === 1)
                    return;
                fade.restart();
            }
        }
    }

    component Layer: Image {
        anchors.fill: parent
        sourceSize.width: PlayerMetrics.fullBlurSourceSize
        sourceSize.height: PlayerMetrics.fullBlurSourceSize
        fillMode: Image.PreserveAspectCrop
        asynchronous: true
        cache: true
    }

    // a new cover rises over the old one; reduce motion never starts this
    NumberAnimation {
        id: fade
        target: front
        property: "opacity"
        to: 1
        duration: Theme.expressiveDurations.expressiveDefaultSpatial
        easing.type: Easing.BezierSpline
        easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
        onFinished: back.source = ""
    }

    MultiEffect {
        anchors.fill: parent
        source: stage
        autoPaddingEnabled: false
        blurEnabled: true
        blurMax: PlayerMetrics.fullBlurMax
        blur: 1
        blurMultiplier: root.pausedBlurMultiplier * root.depth
        brightness: (Theme.isLightMode ? 1 : -1) * root.toneMax * root.mismatch
    }

    Rectangle {
        anchors.fill: parent
        color: Theme.withAlpha(ArtColors.surface, Math.min(1, PlayerMetrics.fullBlurScrimAlpha + root.pausedScrimStep * root.depth + (root.scrimMax - PlayerMetrics.fullBlurScrimAlpha) * root.mismatch))
    }
}
