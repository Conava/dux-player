import QtQuick
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
    readonly property bool showsCover: cover.status === Image.Ready || cover.status === Image.Loading && readySource === source

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
        source: root.source
        sourceSize.width: root.decodeSize
        sourceSize.height: root.decodeSize
        retainWhileLoading: true
        fillMode: Image.PreserveAspectCrop
        asynchronous: true
        cache: true
        onStatusChanged: {
            if (status === Image.Ready)
                root.readySource = root.source;
        }
    }
}
