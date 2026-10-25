pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// The Bento player, after the shell's Bento media chrome: the art card beside the header ("Playing from"), a title
// card and a control card (top row, transport, seekbar, bottom row). Narrow, the cards go below the art.
Item {
    id: root

    // the arrangement to show; Settings passes a draft for its live preview
    property var zones: UiConfig.surface("fullBento").zones
    // shown in the art's place (lyrics or the queue in a narrow window), or null
    property Component inPlace: null

    readonly property var track: PlayerService.current
    // the first shown cover sets the art's size; without one the player has no art
    readonly property var cover: zones.cover.find(i => i.type === "cover" && !i.hidden) ?? null
    readonly property real artMax: {
        switch (cover?.options?.size) {
        case "s":
            return PlayerMetrics.expandedArtMaxSmall;
        case "l":
            return PlayerMetrics.expandedArtMaxLarge;
        }
        return PlayerMetrics.expandedArtMax;
    }
    readonly property bool showArt: cover !== null || inPlace !== null
    readonly property real gap: Theme.spacingXL
    readonly property real blockWidth: Math.min(width, showArt ? artMax * 2 + gap : artMax)
    readonly property real halfWidth: Math.max(0, (blockWidth - gap) / 2)
    readonly property real headerHeight: header.populated ? header.height + Theme.spacingS : 0
    readonly property real paneMinHeight: headerHeight + controls.implicitHeight
    readonly property bool compact: showArt && (width < Theme.smallBreakpoint || halfWidth < paneMinHeight || inPlace !== null)
    readonly property real artworkSize: {
        if (!showArt)
            return 0;
        if (compact)
            return Math.round(Math.max(0, Math.min(artMax, blockWidth, height - gap - paneMinHeight)));
        // sized from the controls column, never from this item's height, so the split doesn't settle wherever the
        // current track put it
        return Math.round(Math.min(halfWidth, Math.max(Math.min(artMax, halfWidth, height), paneMinHeight)));
    }
    readonly property real columnHeight: compact ? artworkSize + gap + paneMinHeight : Math.max(artworkSize, paneMinHeight)

    Item {
        id: block
        anchors.centerIn: parent
        width: root.blockWidth
        height: root.columnHeight

        Item {
            id: art
            visible: root.showArt
            x: root.compact ? (parent.width - width) / 2 : I18n.isRtl ? parent.width - width : 0
            y: root.compact ? 0 : (parent.height - height) / 2
            width: root.compact && root.inPlace ? parent.width : root.artworkSize
            height: root.artworkSize

            AlbumArt {
                anchors.fill: parent
                visible: root.inPlace === null
                radius: PlayerMetrics.mediaInnerRadius
                item: root.track
            }

            Loader {
                anchors.fill: parent
                active: root.inPlace !== null
                sourceComponent: root.inPlace
            }
        }

        Item {
            id: contentPane
            x: root.compact || !root.showArt || I18n.isRtl ? 0 : art.width + root.gap
            y: root.compact ? root.artworkSize + root.gap : 0
            width: root.compact || !root.showArt ? parent.width : parent.width - art.width - root.gap
            height: root.compact ? root.paneMinHeight : parent.height

            NowPlayingZone {
                id: header
                y: root.compact || !root.showArt ? 0 : art.y
                width: parent.width
                height: Math.max(implicitHeight, Theme.buttonHeightXS)
                instances: root.zones.header
                context: "view"
                textLook: "bento"
                centered: true
                room: width
            }

            Column {
                id: controls
                y: root.compact || !root.showArt ? parent.height - height : art.y + art.height - height
                width: parent.width
                spacing: Theme.spacingS

                Rectangle {
                    width: parent.width
                    height: titleZone.implicitHeight + Theme.spacingS * 2
                    visible: titleZone.populated
                    radius: PlayerMetrics.mediaInnerRadius
                    color: ArtColors.cardColor
                    border.width: Theme.layerOutlineWidth
                    border.color: Theme.outlineMedium

                    NowPlayingZone {
                        id: titleZone
                        x: Theme.spacingL
                        y: Theme.spacingS
                        width: parent.width - Theme.spacingL * 2
                        instances: root.zones.title
                        context: "view"
                        textLook: "bento"
                        centered: true
                        column: true
                    }
                }

                Rectangle {
                    width: parent.width
                    height: playback.implicitHeight + Theme.spacingL * 2
                    visible: topZone.populated || controlsZone.populated || seekZone.populated || bottomZone.populated
                    radius: PlayerMetrics.mediaInnerRadius
                    color: ArtColors.cardColor
                    border.width: Theme.layerOutlineWidth
                    border.color: Theme.outlineMedium

                    Column {
                        id: playback
                        x: Theme.spacingL
                        y: Theme.spacingL
                        width: parent.width - Theme.spacingL * 2
                        spacing: Theme.spacingS

                        NowPlayingZone {
                            id: topZone
                            width: parent.width
                            instances: root.zones.top
                            context: "view"
                            look: "row"
                            room: width
                        }

                        NowPlayingZone {
                            id: controlsZone
                            width: parent.width
                            instances: root.zones.controls
                            context: "view"
                            size: "m"
                            look: "transport"
                            fill: true
                            room: width
                        }

                        NowPlayingZone {
                            id: seekZone
                            width: parent.width
                            instances: root.zones.seek
                            context: "view"
                        }

                        NowPlayingZone {
                            id: bottomZone
                            anchors.horizontalCenter: parent.horizontalCenter
                            instances: root.zones.bottom
                            context: "view"
                            room: parent.width
                        }
                    }
                }
            }
        }
    }
}
