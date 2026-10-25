pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import "../Common/Format.js" as Format

// The M3E player, after the shell's Material media chrome: the header (where the queue came from) and a button
// group on top, title and seekbar beside the art, the transport along the bottom with the far end opposite.
Item {
    id: root

    // the arrangement to show; Settings passes a draft for its live preview
    property var zones: UiConfig.surface("fullM3e").zones
    // shown in place of the art and title (lyrics or the queue in a narrow window), or null
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
    readonly property bool showArt: cover !== null
    readonly property var seekTypes: zones.seek.filter(i => !i.hidden).map(i => i.type)
    readonly property bool hasElapsed: seekTypes.indexOf("elapsed") >= 0
    readonly property bool hasDuration: seekTypes.indexOf("duration") >= 0
    readonly property string durationText: PlayerService.duration > 0 ? Format.duration(PlayerService.duration) : "--:--"

    Item {
        id: block
        anchors.centerIn: parent
        width: Math.min(parent.width, root.showArt ? root.artMax * 2 + Theme.spacingL : root.artMax * 1.5)
        height: Math.min(parent.height, header.height + Theme.spacingL * 2 + (root.showArt ? Math.min(root.artMax, width / 2) : root.artMax / 2) + transport.height)

        NowPlayingZone {
            id: headerStart
            anchors.left: parent.left
            anchors.right: header.left
            anchors.rightMargin: Theme.spacingM
            y: (header.height - height) / 2
            instances: root.zones.header
            context: "view"
            textLook: "m3e"
        }

        NowPlayingZone {
            id: header
            anchors.right: parent.right
            height: Math.max(implicitHeight, Theme.buttonHeightS)
            spacing: Theme.spacingXS
            instances: root.zones.buttons
            context: "view"
            look: "outlined"
            room: block.width - (headerStart.populated ? PlayerMetrics.barInfoMinWidth + Theme.spacingM : 0)
        }

        Item {
            id: hero
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: header.bottom
            anchors.topMargin: Theme.spacingL
            anchors.bottom: transport.top
            anchors.bottomMargin: Theme.spacingL

            AlbumArt {
                id: art
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                width: root.showArt && !root.inPlace ? Math.max(0, Math.min(parent.height, parent.width / 2 - Theme.spacingL, root.artMax)) : 0
                height: width
                visible: width > 0
                radius: PlayerMetrics.mediaInnerRadius
                item: root.track
            }

            NowPlayingZone {
                id: titleZone
                visible: !root.inPlace && titleZone.populated
                anchors.left: parent.left
                anchors.right: art.visible ? art.left : parent.right
                anchors.rightMargin: art.visible ? Theme.spacingL : 0
                anchors.bottom: seekBlock.top
                anchors.bottomMargin: Theme.spacingM
                instances: root.zones.title
                context: "view"
                textLook: "m3e"
                column: true
            }

            Loader {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.top: parent.top
                anchors.bottom: seekBlock.visible ? seekBlock.top : parent.bottom
                anchors.bottomMargin: seekBlock.visible ? Theme.spacingM : 0
                active: root.inPlace !== null
                sourceComponent: root.inPlace
            }

            // the seek zone: the seekbar, then "elapsed / length" in one readout, then anything else placed there
            Row {
                id: seekBlock
                anchors.left: parent.left
                anchors.right: art.visible ? art.left : parent.right
                anchors.rightMargin: art.visible ? Theme.spacingL : 0
                anchors.bottom: parent.bottom
                height: Math.max(PlayerMetrics.mediaSeekbarHeight, readout.implicitHeight)
                visible: root.seekTypes.length > 0
                spacing: Theme.spacingM
                LayoutMirroring.enabled: false
                LayoutMirroring.childrenInherit: true

                MediaSeekbar {
                    id: seekbar
                    anchors.verticalCenter: parent.verticalCenter
                    visible: root.seekTypes.indexOf("seekbar") >= 0
                    width: Math.max(0, parent.width - (readout.visible ? readout.width + parent.spacing : 0) - (extra.visible ? extra.width + parent.spacing : 0))
                }

                NumericText {
                    id: readout
                    anchors.verticalCenter: parent.verticalCenter
                    visible: root.hasElapsed || root.hasDuration
                    width: Math.ceil(reservedWidth)
                    text: [root.hasElapsed ? Format.duration(seekbar.value * PlayerService.duration) : "", root.hasDuration ? root.durationText : ""].filter(Boolean).join(" / ")
                    reserveText: text.replace(/\d/g, "8")
                    isMonospace: false
                    color: ArtColors.text
                    font.pixelSize: Theme.fontSizeLarge
                    font.weight: Theme.fontWeightMedium
                }

                NowPlayingZone {
                    id: extra
                    anchors.verticalCenter: parent.verticalCenter
                    instances: root.zones.seek.filter(i => ["elapsed", "seekbar", "duration"].indexOf(i.type) < 0)
                    context: "view"
                }
            }
        }

        Item {
            id: transport
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: Math.max(playback.implicitHeight, modesRow.implicitHeight)

            NowPlayingZone {
                id: playback
                anchors.left: parent.left
                anchors.verticalCenter: parent.verticalCenter
                instances: root.zones.controls
                context: "view"
                size: block.width < Theme.smallBreakpoint ? "s" : "m"
                look: "m3e"
                room: block.width
            }

            NowPlayingZone {
                id: modesRow
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                instances: root.zones.end
                context: "view"
                size: playback.size
                look: "m3e"
                room: Math.max(0, block.width - playback.implicitWidth - Theme.spacingM)
            }
        }
    }
}
