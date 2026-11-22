pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// The floating player's content: the cover inside a progress ring, the title and artists, then previous, play and
// next. The ring follows the position's few ticks a second and nothing here moves on its own; a track change slides the texts.
Item {
    id: root

    readonly property var track: PlayerService.current
    readonly property bool shown: visible && Window.visibility !== Window.Minimized && Window.visibility !== Window.Hidden
    // the ring and the play button sit concentric with the pill's round ends
    readonly property real ringInset: (height - PlayerMetrics.floatingRingSize) / 2
    readonly property real playInset: (height - Theme.minimumTouchTargetSize) / 2

    implicitHeight: PlayerMetrics.floatingHeight

    PositionHold {
        active: root.shown
    }

    Row {
        id: row
        anchors.fill: parent
        anchors.leftMargin: root.ringInset
        anchors.rightMargin: root.playInset
        spacing: Theme.spacingXS

        StyledButton {
            id: ring
            objectName: "floatingRing"
            anchors.verticalCenter: parent.verticalCenter
            width: PlayerMetrics.floatingRingSize
            height: width
            radius: width / 2
            enabled: root.track !== null
            Accessible.name: I18n.tr("Open now playing", "now playing bar button")
            onClicked: Router.openNowPlaying("")

            DRingGauge {
                anchors.fill: parent
                strokeWidth: PlayerMetrics.floatingRingWidth
                ringColor: ArtColors.accent
                trackColor: ArtColors.nowPlaying?.outlineVariant ?? Theme.outlineVariant
                animated: false
                value: PlayerService.duration > 0 ? Math.min(1, PlayerService.position / PlayerService.duration) : 0
            }

            PlayerArt {
                anchors.centerIn: parent
                width: PlayerMetrics.floatingArtSize
                height: width
                shape: "cookie"
                rotate: false
                item: root.track
            }

            StateLayer {
                control: ring
                stateColor: ArtColors.text
            }
        }

        StyledButton {
            id: info
            anchors.verticalCenter: parent.verticalCenter
            width: Math.max(0, row.width - ring.width - transport.width - row.spacing * 2)
            height: PlayerMetrics.floatingRingSize
            radius: Theme.cornerRadiusM
            enabled: root.track !== null
            Accessible.name: I18n.tr("Open now playing", "now playing bar button")
            onClicked: Router.openNowPlaying("")

            StateLayer {
                control: info
                stateColor: ArtColors.text
            }

            Column {
                x: Theme.spacingS
                width: parent.width - Theme.spacingS * 2
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.spacingXXS

                StyledText {
                    id: title
                    width: parent.width
                    text: root.track?.title ?? ""
                    font.pixelSize: Theme.fontSizeMedium
                    font.weight: Theme.fontWeightMedium
                    color: ArtColors.text
                    wrapMode: Text.NoWrap
                    elide: Text.ElideRight
                    opacity: titleSlide.fade
                    transform: Translate {
                        x: titleSlide.shift
                    }

                    TextSlide {
                        id: titleSlide
                        owner: title
                        text: title.text
                        font: title.font
                        color: title.color
                        horizontalAlignment: title.horizontalAlignment
                    }
                }

                StyledText {
                    id: artist
                    width: parent.width
                    visible: text !== ""
                    text: (root.track?.artists ?? []).map(a => a.name).join(", ")
                    font.pixelSize: Theme.fontSizeSmall
                    color: ArtColors.textVariant
                    wrapMode: Text.NoWrap
                    elide: Text.ElideRight
                    opacity: artistSlide.fade
                    transform: Translate {
                        x: artistSlide.shift
                    }

                    TextSlide {
                        id: artistSlide
                        owner: artist
                        text: artist.text
                        font: artist.font
                        color: artist.color
                        horizontalAlignment: artist.horizontalAlignment
                    }
                }
            }
        }

        NowPlayingZone {
            id: transport
            anchors.verticalCenter: parent.verticalCenter
            instances: ["previous", "playPause", "next"].map(type => ({
                        "id": type.toLowerCase(),
                        "type": type,
                        "hidden": false,
                        "placement": "keep",
                        "options": {}
                    }))
            buttonHeight: Theme.minimumTouchTargetSize
            look: "m3e"
        }
    }
}
