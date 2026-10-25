pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

WindowDialog {
    id: root

    readonly property var tracks: Router.playlistPick
    readonly property var choices: PlaylistService.playlists.filter(p => !PlaylistService.hiddenIds[p.id] && (p.kind === "local" || CatalogService.enabledLibrary(p.library)))

    function close() {
        Router.pickPlaylist([]);
    }

    opened: tracks.length > 0
    title: tracks.length === 1 ? I18n.tr("Add \"%1\" to playlist", "add to playlist dialog title for one track").arg(tracks[0].title) : I18n.tr("Add %1 tracks to playlist", "add to playlist dialog title for several tracks").arg(tracks.length)
    acceptEnabled: false
    onRejected: close()
    // later: taking focus inside the binding update loops through DDialog's focus tracking
    onOpenedChanged: {
        if (opened)
            Qt.callLater(forceActiveFocus);
    }

    Column {
        width: parent.width
        spacing: Theme.groupedListGap

        DListItem {
            id: newRow
            width: parent.width
            lastInGroup: root.choices.length === 0
            Accessible.name: newLabel.text
            onClicked: {
                const picked = root.tracks;
                root.close();
                newPlaylist.show(picked);
            }

            DIcon {
                id: newIcon
                anchors.left: parent.left
                anchors.leftMargin: Theme.spacingL
                anchors.verticalCenter: parent.verticalCenter
                name: "add"
                size: Theme.iconSize
                color: newRow.contentColor
            }

            StyledText {
                id: newLabel
                anchors.left: newIcon.right
                anchors.leftMargin: Theme.spacingL
                anchors.right: parent.right
                anchors.rightMargin: Theme.spacingL
                anchors.verticalCenter: parent.verticalCenter
                text: I18n.tr("New playlist", "action and dialog that create a playlist")
                font.pixelSize: Theme.fontSizeMedium
                color: newRow.contentColor
                elide: Text.ElideRight
                horizontalAlignment: Text.AlignLeft
            }
        }

        Repeater {
            model: root.choices

            DListItem {
                id: row
                required property var modelData
                required property int index
                // a server playlist takes the picked tracks from its own server and skips the rest
                readonly property string blocker: PlayerService.whyNot("playlistEdit", modelData)
                readonly property bool fits: blocker === "" && PlayerService.partition("playlistAdd", root.tracks, modelData).ok.length > 0
                width: parent.width
                implicitHeight: Theme.listItemTwoLineHeight
                firstInGroup: false
                lastInGroup: index === root.choices.length - 1
                enabled: fits
                Accessible.name: modelData.name
                onClicked: {
                    PlayerService.reportSkipped(PlayerService.addToPlaylist(modelData.id, root.tracks));
                    root.close();
                }

                DIcon {
                    id: icon
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.spacingL
                    anchors.verticalCenter: parent.verticalCenter
                    name: row.modelData.kind === "server" ? "cloud" : "queue_music"
                    size: Theme.iconSize
                    color: row.contentColor
                }

                Column {
                    anchors.left: icon.right
                    anchors.leftMargin: Theme.spacingL
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.spacingL
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: Theme.spacingXXS

                    StyledText {
                        width: parent.width
                        text: row.modelData.name
                        font.pixelSize: Theme.fontSizeMedium
                        color: row.contentColor
                        elide: Text.ElideRight
                        wrapMode: Text.NoWrap
                        horizontalAlignment: Text.AlignLeft
                    }

                    StyledText {
                        width: parent.width
                        text: {
                            if (row.blocker !== "")
                                return row.blocker;
                            if (row.modelData.kind === "local")
                                return I18n.tr("Local playlist", "playlist kind label");
                            if (!row.fits)
                                return I18n.tr("Only tracks from %1 can be added", "why a server playlist cannot take the picked tracks, %1 is the server library").arg(PlayerService.libraryName(row.modelData.library));
                            return I18n.tr("Playlist on %1", "playlist kind label for a server playlist, %1 is the library").arg(PlayerService.libraryName(row.modelData.library));
                        }
                        font.pixelSize: Theme.fontSizeSmall
                        color: row.supportingContentColor
                        elide: Text.ElideRight
                        wrapMode: Text.NoWrap
                        horizontalAlignment: Text.AlignLeft
                    }
                }
            }
        }
    }

    NewPlaylistDialog {
        id: newPlaylist
        host: root
    }

    actions: [
        DButton {
            text: I18n.tr("Cancel", "dialog dismiss button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: root.close()
        }
    ]
}
