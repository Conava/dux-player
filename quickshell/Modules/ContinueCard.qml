import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import "../Common/Format.js" as Format

// The last queue, as it was left: where it came from, the current track, progress, resume.
Rectangle {
    id: root

    readonly property var track: PlayerService.current

    width: parent?.width ?? 0
    implicitHeight: row.implicitHeight + Theme.spacingL * 2
    radius: Theme.cornerRadiusXL
    color: Theme.primaryContainer
    visible: track !== null
    Accessible.role: Accessible.Grouping
    Accessible.name: I18n.tr("Continue listening", "home shelf with the last queue")

    Row {
        id: row
        x: Theme.spacingL
        y: Theme.spacingL
        width: parent.width - Theme.spacingL * 2
        spacing: Theme.spacingXL

        AlbumArt {
            id: art
            width: PlayerMetrics.shelfArtSize
            height: width
            radius: Theme.cornerRadiusL
            item: root.track
        }

        Column {
            width: parent.width - art.width - parent.spacing
            anchors.verticalCenter: art.verticalCenter
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

            Row {
                spacing: Theme.spacingS
                topPadding: Theme.spacingXS

                DButton {
                    text: PlayerService.playing ? I18n.tr("Pause", "transport button") : I18n.tr("Resume", "continue listening action")
                    iconName: PlayerService.playing ? "pause" : "play_arrow"
                    onClicked: PlayerService.toggle()
                }

                DButton {
                    text: I18n.tr("Queue", "the play queue: panel, toggle")
                    iconName: "queue_music"
                    backgroundColor: Theme.surfaceContainerHighest
                    textColor: Theme.onSurface
                    onClicked: Router.panel = "queue"
                }
            }
        }
    }
}
