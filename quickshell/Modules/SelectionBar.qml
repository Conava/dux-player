import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// Floating actions for the selected tracks.
Rectangle {
    id: root

    property var tracks: []
    property string playlistId: ""
    // how many rows the list shows; Select all picks them all, then turns into Select none
    property int total: 0
    readonly property bool everything: total > 0 && tracks.length >= total
    property var context: ({
            "type": "tracks"
        })
    readonly property var favorable: PlayerService.partition("favorite", tracks)
    readonly property bool allFavorite: favorable.ok.length > 0 && favorable.ok.every(t => CatalogService.withMarks(t).favorite)
    readonly property bool canRemove: playlistId !== "" && PlayerService.allows("playlistEdit", PlayerService.playlist(playlistId))

    signal removeRequested
    signal done
    signal selectAllRequested

    implicitWidth: row.implicitWidth + Theme.spacingM * 2
    implicitHeight: Theme.buttonHeightM
    radius: Theme.fullRadius(width, height)
    color: Theme.secondaryContainer
    z: Theme.zBar
    Accessible.role: Accessible.ToolBar
    Accessible.name: Phrases.selected(tracks.length)

    // keeps clicks from reaching the rows underneath
    MouseArea {
        anchors.fill: parent
    }

    Row {
        id: row
        anchors.centerIn: parent
        spacing: Theme.spacingXS

        DActionButton {
            anchors.verticalCenter: parent.verticalCenter
            iconName: "close"
            iconColor: Theme.onSecondaryContainer
            tooltipText: I18n.tr("Clear selection", "selection bar action")
            Accessible.name: tooltipText
            onClicked: root.done()
        }

        StyledText {
            anchors.verticalCenter: parent.verticalCenter
            text: Phrases.selected(root.tracks.length)
            font.pixelSize: Theme.fontSizeMedium
            font.weight: Theme.fontWeightMedium
            color: Theme.onSecondaryContainer
        }

        DActionButton {
            anchors.verticalCenter: parent.verticalCenter
            iconName: root.everything ? "deselect" : "select_all"
            iconColor: Theme.onSecondaryContainer
            tooltipText: root.everything ? I18n.tr("Select none", "selection bar action") : I18n.tr("Select all", "selection bar action")
            Accessible.name: tooltipText
            onClicked: root.everything ? root.done() : root.selectAllRequested()
        }

        DActionButton {
            anchors.verticalCenter: parent.verticalCenter
            iconName: "play_arrow"
            iconFilled: true
            iconColor: Theme.onSecondaryContainer
            tooltipText: I18n.tr("Play", "button that starts playback")
            Accessible.name: tooltipText
            onClicked: {
                PlayerService.playTracks(PlayerService.refsOf(root.tracks), 0, root.context);
                root.done();
            }
        }

        DActionButton {
            anchors.verticalCenter: parent.verticalCenter
            iconName: "queue_play_next"
            iconColor: Theme.onSecondaryContainer
            tooltipText: I18n.tr("Play next", "track row action")
            Accessible.name: tooltipText
            onClicked: {
                PlayerService.enqueue(root.tracks, true);
                root.done();
            }
        }

        DActionButton {
            anchors.verticalCenter: parent.verticalCenter
            iconName: "add_to_queue"
            iconColor: Theme.onSecondaryContainer
            tooltipText: I18n.tr("Add to queue", "track row action")
            Accessible.name: tooltipText
            onClicked: {
                PlayerService.enqueue(root.tracks, false);
                root.done();
            }
        }

        DActionButton {
            anchors.verticalCenter: parent.verticalCenter
            iconName: "playlist_add"
            iconColor: Theme.onSecondaryContainer
            tooltipText: I18n.tr("Add to playlist…", "track menu action")
            Accessible.name: tooltipText
            onClicked: Router.pickPlaylist(root.tracks)
        }

        // hidden when no selected track's library supports favorites
        DActionButton {
            anchors.verticalCenter: parent.verticalCenter
            visible: root.favorable.ok.length > 0
            iconName: "favorite"
            iconFilled: root.allFavorite
            iconColor: Theme.onSecondaryContainer
            tooltipText: root.allFavorite ? I18n.tr("Remove from favorites", "track menu action") : I18n.tr("Add to favorites", "track menu action")
            Accessible.name: tooltipText
            onClicked: {
                const value = !root.allFavorite;
                for (const t of root.favorable.ok)
                    PlayerService.setFavorite("track", t, value);
                PlayerService.reportSkipped(root.favorable.skipped);
            }
        }

        DActionButton {
            anchors.verticalCenter: parent.verticalCenter
            visible: root.canRemove
            iconName: "playlist_remove"
            iconColor: Theme.onSecondaryContainer
            tooltipText: I18n.tr("Remove from playlist", "playlist track action")
            Accessible.name: tooltipText
            onClicked: root.removeRequested()
        }
    }
}
