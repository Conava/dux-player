pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// Home shelf: Shuffle all, then the navigation's pinned entries, then recently played albums, as square tiles.
// The tile size sets the rows.
Column {
    id: root

    required property var shelf
    required property string title

    readonly property int rows: shelf.options.size === "s" ? 1 : shelf.options.size === "l" ? 3 : 2
    readonly property int columns: Math.max(1, Math.floor(width / PlayerMetrics.speedDialMinTileWidth))
    readonly property real tileSize: (width - Theme.spacingS * (columns - 1)) / columns
    readonly property bool hasContent: CatalogService.libraries.some(l => l.enabled)
    readonly property var accents: ({
            "genre": "green",
            "library": "teal",
            "playlist": "blue",
            "view": "purple"
        })
    // pins and albums fill what Shuffle all leaves; an album that is also pinned never shows twice
    readonly property var tiles: {
        const slots = columns * rows - 1;
        const pins = UiConfig.navigation.filter(e => e.kind !== "page" && e.kind !== "settings" && !e.hidden && UiConfig.entryValid(e));
        const seen = new Set();
        const albums = PlayerService.recentAlbums.map(a => PlayerService.album(a.id) ?? a).filter(a => PlayerService.enabledLibrary(a.library) && !seen.has(a.id) && seen.add(a.id));
        return pins.map(e => ({
                    "kind": e.kind,
                    "caption": UiConfig.entryLabel(e),
                    "route": UiConfig.entryRoute(e),
                    "icon": UiConfig.entryIcon(e),
                    "item": e.kind === "playlist" ? PlayerService.playlist(e.target) : e.kind === "artist" ? PlayerService.artist(e.target) ?? {
                        "id": e.target,
                        "name": e.name
                    } : null
                })).concat(albums.map(a => ({
                    "kind": "album",
                    "caption": a.title,
                    "route": {
                        "page": "album",
                        "itemId": a.id
                    },
                    "item": a
                }))).slice(0, Math.max(0, slots));
    }

    width: parent?.width ?? 0
    spacing: Theme.spacingS

    ShelfHeader {
        title: root.title
    }

    Grid {
        width: parent.width
        columns: root.columns
        spacing: Theme.spacingS

        SpeedDialTile {
            caption: I18n.tr("Shuffle all", "speed dial tile that shuffles the whole library")
            iconName: "shuffle"
            plate: Theme.primaryContainer
            plateText: Theme.onPrimaryContainer
            onClicked: {
                PlayerService.shuffleTracks([], {
                    "type": "tracks"
                }, false);
            }
        }

        Repeater {
            model: root.tiles

            SpeedDialTile {
                id: tile
                required property var modelData
                readonly property var pair: Theme.accent(root.accents[modelData.kind] ?? "blue")
                readonly property bool hasArt: modelData.kind === "album" || modelData.kind === "playlist" || modelData.kind === "artist"

                caption: modelData.caption
                iconName: modelData.icon ?? ""
                // a playlist without a cover gets the accent plate and small icon of the other pins
                plate: hasArt && !artBare ? Theme.secondaryContainer : pair?.container ?? Theme.secondaryContainer
                plateText: hasArt && !artBare ? Theme.onSecondaryContainer : pair?.onContainer ?? Theme.onSecondaryContainer
                art: modelData.kind === "playlist" ? playlistArt : modelData.kind === "artist" ? artistArt : modelData.kind === "album" ? albumArt : null
                // Router.go keeps the history that openEntry would clear
                onClicked: {
                    Router.go(modelData.route.page, modelData.route.itemId);
                }

                Component {
                    id: playlistArt
                    PlaylistCover {
                        playlist: tile.modelData.item
                        radius: Theme.cornerRadiusL
                    }
                }

                Component {
                    id: artistArt
                    AlbumArt {
                        radius: Theme.cornerRadiusL
                        item: tile.modelData.item
                        imageUrl: CatalogService.artistImageUrl(tile.modelData.item)
                        placeholderIcon: "person"
                    }
                }

                Component {
                    id: albumArt
                    AlbumArt {
                        radius: Theme.cornerRadiusL
                        item: tile.modelData.item
                    }
                }
            }
        }
    }

    // a square tile: art (or an icon on a coloured plate) with its caption at the bottom
    component SpeedDialTile: StyledButton {
        id: dial

        property string caption
        property string iconName
        property color plate: Theme.secondaryContainer
        property color plateText: Theme.onSecondaryContainer
        // a component for the full-bleed art; none shows the icon on the plate
        property Component art: null
        // caption colour over the dark scrim, whatever the theme's mode
        // the art has nothing to show but its placeholder, so the plate and icon take over
        readonly property bool artBare: (artLoader.item as PlaylistCover)?.bare ?? false
        readonly property bool showsArt: art !== null && !artBare
        readonly property color imageText: Theme.isLightMode ? Theme.surface : Theme.surfaceText

        width: root.tileSize
        height: width
        radius: Theme.cornerRadiusL
        color: plate
        Accessible.role: Accessible.Button
        Accessible.name: caption

        Loader {
            id: artLoader
            anchors.fill: parent
            sourceComponent: dial.art
            active: dial.art !== null
            visible: !dial.artBare
        }

        DIcon {
            visible: !dial.showsArt
            anchors.centerIn: parent
            anchors.verticalCenterOffset: -captionText.height / 2
            name: dial.iconName
            filled: true
            size: Theme.iconSizeLarge
            color: dial.plateText
        }

        Rectangle {
            visible: dial.showsArt
            anchors.fill: parent
            radius: dial.radius
            gradient: Gradient {
                GradientStop {
                    position: 0.55
                    color: Qt.alpha(Theme.scrimColor, 0)
                }
                GradientStop {
                    position: 1
                    color: Qt.alpha(Theme.scrimColor, Theme.scrimAlpha)
                }
            }
        }

        StyledText {
            id: captionText
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            anchors.margins: Theme.spacingM
            text: dial.caption
            font.pixelSize: Theme.fontSizeMedium
            font.weight: Theme.fontWeightMedium
            color: dial.showsArt ? dial.imageText : dial.plateText
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
        }

        StateLayer {
            control: dial
            stateColor: dial.showsArt ? dial.imageText : dial.plateText
        }

        DPressFeedback {
            control: dial
        }
    }
}
