import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// One line under a title: the track's artists or its album, each name a link. Hidden when the track has none.
Item {
    id: root

    property var track: null
    // "artists" | "album"
    property string line: "artists"
    property bool centered: true
    property real fontSize: Theme.fontSizeSmall

    readonly property Item content: line === "album" ? album : artists

    function alignedX(itemWidth) {
        if (centered)
            return (width - itemWidth) / 2;
        return I18n.isRtl ? width - itemWidth : 0;
    }

    visible: line === "album" ? (track?.album ?? "") !== "" : (track?.artists ?? []).length > 0
    implicitWidth: content.implicitWidth
    implicitHeight: content.implicitHeight

    ArtistLinks {
        id: artists
        x: root.alignedX(width)
        visible: root.line === "artists"
        width: Math.min(implicitWidth, root.width)
        artists: root.track?.artists ?? []
        fontSize: root.fontSize
        color: ArtColors.textVariant
    }

    LinkText {
        id: album
        x: root.alignedX(width)
        visible: root.line === "album"
        width: Math.min(implicitWidth, root.width)
        text: root.track?.album ?? ""
        linked: (root.track?.albumId ?? "") !== ""
        font.pixelSize: root.fontSize
        color: ArtColors.textVariant
        onActivated: Router.go("album", root.track.albumId)
    }
}
