pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Effects
import Quickshell.Widgets
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// An item's cover from the daemon; until it loads, or without one, the accent with a Material shape, or
// without an accent the placeholder icon.
ClippingRectangle {
    id: root

    // a track, album or artist; its cover and accent
    property var item: null
    property string accent: item?.accent ?? ""
    property string shape: item?.shape ?? "circle"

    // an image found elsewhere (an artist's), in place of the item's cover
    property string imageUrl: ""
    property string placeholderIcon: "album"

    readonly property var pair: Theme.accent(accent)
    readonly property string source: imageUrl !== "" ? imageUrl : item ? PlayerService.coverUrl(item) : ""
    // Decoded in power-of-two steps, so resizing the window rarely reloads the cover; a reload keeps showing it.
    readonly property int decodeSize: Math.pow(2, Math.ceil(Math.log2(Math.max(width, height, 1) * Screen.devicePixelRatio)))
    property string readySource: ""
    readonly property bool showsCover: cover.status === Image.Ready || cover.status === Image.Loading && readySource === cover.source.toString()

    // Now-playing art: a new cover replaces the old one by a blur hand-off, not a cut. Tiles and rows leave it off.
    property bool handoff: false
    // the source `cover` shows while handing off, and how blurred it is (1 fully)
    property string shown: ""
    property real blur: 0
    property bool swapPending: false
    property bool entering: false
    readonly property bool handingOff: swapPending || blurOut.running || blurIn.running

    // the old cover is gone and the new one decoded
    function land() {
        if (!swapPending || blurOut.running || next.status === Image.Loading || next.status === Image.Null)
            return;
        shown = source;
        swapPending = false;
        if (next.status !== Image.Ready)
            return settle();
        entering = true;
        blurIn.restart();
    }

    function settle() {
        blurOut.stop();
        blurIn.stop();
        swapPending = false;
        blur = 0;
    }

    onSourceChanged: {
        if (!handoff)
            return;
        if (source === shown)
            return settle();
        if (shown === "" || source === "" || !showsCover || !visible || !Theme.animationsEnabled) {
            settle();
            shown = source;
            return;
        }
        blurIn.stop();
        swapPending = true;
        blurOut.restart();
    }
    onHandoffChanged: {
        settle();
        shown = handoff ? source : "";
    }
    Component.onCompleted: if (handoff)
        shown = source

    color: pair?.container ?? Theme.secondaryContainer
    radius: Theme.cornerRadiusM
    Accessible.ignored: true

    DMaterialShape {
        anchors.centerIn: parent
        visible: !root.showsCover && root.pair !== null
        width: Math.round(parent.width * PlayerMetrics.artShapeRatio)
        height: width
        shape: root.shape
        color: root.pair?.onContainer ?? Theme.onSecondaryContainer
    }

    DIcon {
        anchors.centerIn: parent
        visible: !root.showsCover && root.pair === null
        name: root.placeholderIcon
        size: Math.round(parent.width * PlayerMetrics.artShapeRatio)
        color: Theme.onSecondaryContainer
    }

    Image {
        id: cover
        anchors.fill: parent
        visible: root.showsCover
        source: root.handoff ? root.shown : root.source
        opacity: 1 - 0.6 * Math.min(1, Math.max(0, root.blur))
        scale: root.entering ? 1 + 0.05 * Math.max(0, root.blur) : 1
        layer.enabled: root.handingOff
        layer.effect: MultiEffect {
            blurEnabled: true
            blurMax: 32
            blur: Math.min(1, Math.max(0, root.blur))
        }
        sourceSize.width: root.decodeSize
        sourceSize.height: root.decodeSize
        retainWhileLoading: true
        fillMode: Image.PreserveAspectCrop
        asynchronous: true
        cache: true
        onStatusChanged: {
            if (status === Image.Ready)
                root.readySource = cover.source.toString();
        }
    }

    // decodes the incoming cover while the old one blurs out
    Image {
        id: next
        visible: false
        source: root.handoff ? root.source : ""
        sourceSize.width: root.decodeSize
        sourceSize.height: root.decodeSize
        asynchronous: true
        cache: true
        onStatusChanged: root.land()
    }

    NumberAnimation {
        id: blurOut
        target: root
        property: "blur"
        to: 1
        duration: Theme.expressiveDurations.expressiveEffects
        easing.type: Easing.BezierSpline
        easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
        onFinished: root.land()
    }

    NumberAnimation {
        id: blurIn
        target: root
        property: "blur"
        to: 0
        duration: Theme.expressiveDurations.expressiveDefaultSpatial
        easing.type: Easing.BezierSpline
        easing.bezierCurve: Theme.expressiveCurves.expressiveDefaultSpatial
        onRunningChanged: if (!running)
            root.entering = false
    }
}
