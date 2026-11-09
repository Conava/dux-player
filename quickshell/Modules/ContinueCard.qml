import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets
import "../Common/Format.js" as Format

// The last queue, as it was left: where it came from, the current track, progress, resume.
Rectangle {
    id: root

    readonly property var track: PlayerService.current

    width: parent?.width ?? 0
    // the narrowest text column worth showing beside the art; below it the text goes under the art
    readonly property real minTextWidth: 220
    readonly property real besideWidth: row.width - art.width - Theme.spacingXL
    readonly property bool stacked: besideWidth < minTextWidth

    implicitHeight: row.height + Theme.spacingL * 2
    radius: Theme.cornerRadiusXL
    color: Theme.primaryContainer
    visible: track !== null
    Accessible.role: Accessible.Grouping
    Accessible.name: I18n.tr("Continue listening", "home shelf with the last queue")

    Item {
        id: row
        x: Theme.spacingL
        y: Theme.spacingL
        width: parent.width - Theme.spacingL * 2
        height: root.stacked ? art.height + Theme.spacingXL + info.height : Math.max(art.height, info.height)

        AlbumArt {
            id: art
            anchors.left: parent.left
            width: PlayerMetrics.shelfArtSize
            height: width
            radius: Theme.cornerRadiusL
            item: root.track
        }

        Column {
            id: info
            anchors.left: root.stacked ? parent.left : art.right
            anchors.leftMargin: root.stacked ? 0 : Theme.spacingXL
            width: root.stacked ? parent.width : root.besideWidth
            // beside the art: centered on it, but a taller column grows downwards, never above the card
            y: root.stacked ? art.height + Theme.spacingXL : Math.max(0, (art.height - height) / 2)
            spacing: Theme.spacingS

            StyledText {
                width: parent.width
                text: PlayerService.queueEdited ? I18n.tr("%1 · edited queue", "continue listening context, the queue was changed with play next or reordering").arg(UiConfig.contextLabel(PlayerService.playContext)) : UiConfig.contextLabel(PlayerService.playContext)
                font.pixelSize: Theme.fontSizeSmall
                font.weight: Theme.fontWeightMedium
                color: Theme.onPrimaryContainer
                elide: Text.ElideRight
                wrapMode: Text.NoWrap
            }

            StyledText {
                width: parent.width
                text: root.track?.title ?? ""
                font.pixelSize: Theme.fontSizeXLarge
                font.weight: Theme.fontWeightMedium
                color: Theme.onPrimaryContainer
                elide: Text.ElideRight
                wrapMode: Text.NoWrap
            }

            StyledText {
                width: parent.width
                text: I18n.tr("%1 · track %2 of %3 · %4 / %5", "continue listening facts: artist · position in queue · elapsed / total").arg(root.track?.artist ?? "").arg(PlayerService.queueIndex + 1).arg(PlayerService.queue.length).arg(Format.duration(PlayerService.position)).arg(Format.duration(PlayerService.duration))
                font.pixelSize: Theme.fontSizeMedium
                color: Theme.onPrimaryContainer
                elide: Text.ElideRight
                wrapMode: Text.NoWrap
            }

            MediaSeekbar {
                width: parent.width
                height: Theme.spacingL
                interactive: false
                fillColor: Theme.onPrimaryContainer
                trackColor: Theme.withAlpha(Theme.onPrimaryContainer, Theme.stateLayerDrag)
            }

            Flow {
                width: parent.width
                spacing: Theme.spacingS
                topPadding: Theme.spacingXS

                DButton {
                    id: playButton
                    text: PlayerService.playing ? I18n.tr("Pause", "transport button") : I18n.tr("Play", "button that starts playback")
                    // the other label's width too, so the button doesn't resize when it switches
                    reserveText: PlayerService.playing ? I18n.tr("Play", "button that starts playback") : I18n.tr("Pause", "transport button")
                    iconName: PlayerService.playing ? "pause" : "play_arrow"
                    onClicked: PlayerService.toggle()

                    DPressFeedback {
                        control: playButton
                    }
                }

                DButton {
                    id: queueButton
                    text: I18n.tr("Queue", "the play queue: panel, toggle")
                    iconName: "queue_music"
                    // a tonal container would melt into the primaryContainer card
                    backgroundColor: Theme.surface
                    textColor: Theme.primary
                    onClicked: Router.panel = "queue"

                    DPressFeedback {
                        control: queueButton
                    }
                }
            }
        }
    }
}
