pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// Art for any browse item: album and track covers, round artist images, playlist covers, genre tiles.
Item {
    id: root

    property string kind: "album"
    property var item: null
    property real radius: Theme.cornerRadiusM

    Loader {
        anchors.fill: parent
        sourceComponent: root.kind === "playlist" ? playlistCover : albumArt
    }

    Component {
        id: albumArt

        AlbumArt {
            id: art
            radius: root.kind === "artist" ? Theme.fullRadius(art.width, art.height) : root.radius
            item: root.item
            // the same image chain as the artist page, for every artist tile and row
            imageUrl: root.kind === "artist" ? CatalogService.artistImageUrl(root.item) : ""
            placeholderIcon: root.kind === "artist" ? "person" : root.kind === "genre" ? "category" : "album"
        }
    }

    Component {
        id: playlistCover

        PlaylistCover {
            playlist: root.item
            radius: root.radius
        }
    }
}
