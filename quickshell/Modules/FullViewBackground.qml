pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Effects
import qs.Common
import qs.Services

// The playing cover, decoded small and blurred behind the full view, under a scrim of the view's surface. The blur is
// rendered when a cover lands, not while the view moves; the effect and both images exist only while this item does.
Item {
    id: root
    objectName: "fullBackground"

    readonly property string cover: PlayerService.current ? PlayerService.coverUrl(PlayerService.current) : ""

    // The scrim rises with how far the cover's lightness sits from the surface's: the accent text on a light surface
    // keeps 4.5:1 only while a dark cover shows through at under a tenth, and likewise a bright cover in dark mode.
    readonly property real scrimMax: 0.8
    // the same mismatch also shifts the cover's tone toward the surface's, so the blur keeps its hue under the scrim
    readonly property real toneMax: 0.6
    readonly property real mismatchGain: 1.5
    readonly property real mismatch: cover === "" ? 0 : Math.min(1, mismatchGain * Math.abs(Qt.color(PlayerService.coverColor(PlayerService.current)).hslLightness - ArtColors.surface.hslLightness))

    // the window paints the surface at this alpha, and the blurred layer follows it
    opacity: Theme.floatingWindowTransparency
    visible: front.source.toString() !== ""

    onCoverChanged: {
        const had = front.status === Image.Ready && cover !== "" && Theme.animationsEnabled && visible;
        back.source = had ? front.source : "";
        front.opacity = had ? 0 : 1;
        front.source = cover;
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
        duration: Theme.expressiveDurations.expressiveEffects
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
        brightness: (Theme.isLightMode ? 1 : -1) * root.toneMax * root.mismatch
    }

    Rectangle {
        anchors.fill: parent
        color: Theme.withAlpha(ArtColors.surface, PlayerMetrics.fullBlurScrimAlpha + (root.scrimMax - PlayerMetrics.fullBlurScrimAlpha) * root.mismatch)
    }
}
