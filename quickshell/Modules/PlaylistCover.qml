pragma ComponentBehavior: Bound

import QtQuick
import Quickshell.Widgets
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// A playlist's cover: its own pick, or a mosaic of its first tracks.
ClippingRectangle {
    id: root

    property var playlist: null

    readonly property var cover: playlist?.cover ?? {
        "type": "auto"
    }
    readonly property var tracks: playlist ? PlayerService.playlistTracks(playlist.id) : []
    readonly property var coverTrack: cover.type === "art" ? PlayerService.track(cover) : null
    // the daemon's first album covers until the tracks are loaded
    readonly property var mosaic: playlist?.loaded === false ? (playlist.mosaic ?? []) : tracks.filter((t, i, all) => all.findIndex(o => o.albumId === t.albumId) === i).slice(0, 4)

    radius: Theme.cornerRadiusM
    color: playlist?.kind === "server" ? Theme.tertiaryContainer : Theme.secondaryContainer
    Accessible.ignored: true

    AlbumArt {
        id: single
        anchors.fill: parent
        visible: root.cover.type === "shape" || root.coverTrack !== null || (root.cover.type === "auto" && root.mosaic.length > 0 && root.mosaic.length < 4)
        radius: root.radius
        item: root.cover.type === "shape" ? null : root.coverTrack ?? root.mosaic[0] ?? null
        accent: root.cover.type === "shape" ? root.cover.accent : single.item?.accent ?? ""
        shape: root.cover.type === "shape" ? root.cover.shape : single.item?.shape ?? "circle"
    }

    Grid {
        anchors.fill: parent
        columns: 2
        visible: root.cover.type === "auto" && root.mosaic.length === 4

        Repeater {
            model: root.cover.type === "auto" && root.mosaic.length === 4 ? root.mosaic : []

            AlbumArt {
                required property var modelData
                width: root.width / 2
                height: root.height / 2
                radius: 0
                item: modelData
            }
        }
    }

    // the server's own playlist art, over the mosaic once it loads
    AlbumArt {
        anchors.fill: parent
        visible: (root.playlist?.coverId ?? "") !== "" && source !== ""
        radius: root.radius
        item: (root.playlist?.coverId ?? "") !== "" ? root.playlist : null
    }

    DIcon {
        anchors.centerIn: parent
        visible: root.cover.type === "auto" && root.mosaic.length === 0 && (root.playlist?.coverId ?? "") === ""
        name: root.playlist?.kind === "server" ? "cloud" : "queue_music"
        size: Math.round(parent.width * PlayerMetrics.artShapeRatio)
        color: root.playlist?.kind === "server" ? Theme.onTertiaryContainer : Theme.onSecondaryContainer
    }
}
