pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// Picks a playlist cover: the automatic mosaic, one of its albums' art, or a color and shape.
WindowDialog {
    id: root

    property var playlist: null

    readonly property string blocker: PlayerService.whyNot("playlistCover", playlist)
    readonly property bool allowed: playlist !== null && blocker === ""
    readonly property var albumTracks: playlist ? PlayerService.playlistTracks(playlist.id).filter((t, i, all) => all.findIndex(o => o.albumId === t.albumId) === i) : []
    readonly property var accents: ["red", "orange", "yellow", "green", "teal", "blue", "purple", "pink"]
    readonly property var shapes: ["sunny", "cookie9", "clover4", "flower", "heart", "puffy", "softBurst", "gem"]
    property string accent: "blue"

    function pick(cover) {
        PlayerService.setPlaylistCover(playlist.id, cover);
        opened = false;
    }

    title: I18n.tr("Playlist cover", "cover dialog title")
    supportingText: blocker
    acceptEnabled: false
    onRejected: opened = false

    component Swatch: StyledButton {
        id: swatch
        property bool current: false
        width: Theme.buttonHeightM
        height: width
        radius: Theme.cornerRadiusM
        color: "transparent"
        enabled: root.allowed
        border.width: current ? Theme.outlineWidthFocused : 0
        border.color: Theme.primary

        StateLayer {
            control: swatch
            stateColor: Theme.onSurface
        }
    }

    StyledText {
        width: parent.width
        text: I18n.tr("From the playlist", "cover dialog section")
        font.pixelSize: Theme.fontSizeMedium
        font.weight: Theme.fontWeightMedium
        color: Theme.primary
    }

    Flow {
        width: parent.width
        spacing: Theme.spacingS

        Swatch {
            current: (root.playlist?.cover?.type ?? "auto") === "auto"
            Accessible.name: I18n.tr("Automatic", "option picked automatically")
            onClicked: root.pick({
                "type": "auto"
            })

            PlaylistCover {
                anchors.fill: parent
                anchors.margins: Theme.spacingXS
                playlist: root.playlist ? Object.assign({}, root.playlist, {
                    "cover": {
                        "type": "auto"
                    }
                }) : null
            }
        }

        Repeater {
            model: root.albumTracks

            Swatch {
                required property var modelData
                current: root.playlist?.cover?.type === "art" && root.playlist.cover.id === modelData.id
                Accessible.name: modelData.album
                onClicked: root.pick({
                    "type": "art",
                    "library": modelData.library,
                    "id": modelData.id
                })

                AlbumArt {
                    anchors.fill: parent
                    anchors.margins: Theme.spacingXS
                    item: parent.modelData
                }
            }
        }
    }

    StyledText {
        width: parent.width
        text: I18n.tr("Color and shape", "cover dialog section")
        font.pixelSize: Theme.fontSizeMedium
        font.weight: Theme.fontWeightMedium
        color: Theme.primary
    }

    Flow {
        width: parent.width
        spacing: Theme.spacingS

        Repeater {
            model: root.accents

            Swatch {
                required property string modelData
                width: Theme.buttonHeightXS
                current: root.accent === modelData
                Accessible.name: modelData
                onClicked: root.accent = modelData

                Rectangle {
                    anchors.fill: parent
                    anchors.margins: Theme.spacingXS
                    radius: Theme.fullRadius(width, height)
                    color: Theme.accent(parent.modelData)?.container ?? Theme.secondaryContainer
                }
            }
        }
    }

    Flow {
        width: parent.width
        spacing: Theme.spacingS

        Repeater {
            model: root.shapes

            Swatch {
                required property string modelData
                current: root.playlist?.cover?.type === "shape" && root.playlist.cover.shape === modelData && root.playlist.cover.accent === root.accent
                Accessible.name: modelData
                onClicked: root.pick({
                    "type": "shape",
                    "accent": root.accent,
                    "shape": modelData
                })

                AlbumArt {
                    anchors.fill: parent
                    anchors.margins: Theme.spacingXS
                    accent: root.accent
                    shape: parent.modelData
                }
            }
        }
    }

    actions: [
        DButton {
            text: I18n.tr("Close", "dialog dismiss button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: root.opened = false
        }
    ]
}
