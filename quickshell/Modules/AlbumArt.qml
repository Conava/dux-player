import QtQuick
import Quickshell.Widgets
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// An item's cover from the daemon; until it loads, or without one, the accent with a Material shape.
ClippingRectangle {
    id: root

    // a track, album or artist; its cover and accent
    property var item: null
    property string accent: item?.accent ?? ""
    property string shape: item?.shape ?? "circle"

    // an image found elsewhere (an artist's), in place of the item's cover
    property string imageUrl: ""

    readonly property var pair: Theme.accent(accent)
    readonly property string source: imageUrl !== "" ? imageUrl : item ? PlayerService.coverUrl(item) : ""

    color: pair?.container ?? Theme.surfaceContainerHighest
    radius: Theme.cornerRadiusM
    Accessible.ignored: true

    DMaterialShape {
        anchors.centerIn: parent
        visible: cover.status !== Image.Ready
        width: Math.round(parent.width * PlayerMetrics.artShapeRatio)
        height: width
        shape: root.shape
        color: root.pair?.onContainer ?? Theme.onSurfaceVariant
    }

    Image {
        id: cover
        anchors.fill: parent
        visible: status === Image.Ready
        source: root.source
        sourceSize.width: width * Screen.devicePixelRatio
        sourceSize.height: height * Screen.devicePixelRatio
        fillMode: Image.PreserveAspectCrop
        asynchronous: true
        cache: true
    }
}
