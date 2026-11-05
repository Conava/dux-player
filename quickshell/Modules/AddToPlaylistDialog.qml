pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

WindowDialog {
    id: root

    readonly property var tracks: Router.playlistPick
    readonly property var choices: PlaylistService.playlists.filter(p => !PlaylistService.hiddenIds[p.id] && (p.kind === "local" || CatalogService.enabledLibrary(p.library)))
    // a string, so a playlist's tracks changing doesn't rebuild the rows and drop the focus
    readonly property string choiceKey: choices.map(p => p.id).join("\n")
    // playlist id -> "all" | "none", what an edit waits for: the row shows it at once, as a server takes a moment
    property var pending: ({})
    // playlist id -> true: its tracks couldn't be read
    property var unreadable: ({})

    function close() {
        Router.pickPlaylist([]);
    }

    function setPending(id, state) {
        const next = Object.assign({}, pending);
        if (state)
            next[id] = state;
        else
            delete next[id];
        pending = next;
    }

    function load(id) {
        PlaylistService.loadPlaylist(id, null, () => {
            root.unreadable = Object.assign({}, root.unreadable, {
                [id]: true
            });
        });
    }

    opened: tracks.length > 0
    title: tracks.length === 1 ? I18n.tr("Playlists with “%1”", "add to playlist dialog title for one track").arg(tracks[0].title) : I18n.tr("Playlists with these %1 tracks", "add to playlist dialog title for several tracks").arg(tracks.length)
    acceptEnabled: false
    // the tab chain would leave for the window's controls before reaching the dialog's own
    Keys.onTabPressed: event => {
        if (activeFocus)
            newRow.forceActiveFocus(Qt.TabFocusReason);
        else
            event.accepted = false;
    }
    onRejected: close()
    // later: taking focus inside the binding update loops through DDialog's focus tracking
    onOpenedChanged: {
        if (!opened)
            return;
        pending = {};
        unreadable = {};
        for (const p of choices) {
            if (p.loaded !== true)
                load(p.id);
        }
        Qt.callLater(forceActiveFocus);
    }

    Column {
        width: parent.width
        spacing: Theme.groupedListGap

        DListItem {
            id: newRow
            width: parent.width
            focusPolicy: Qt.StrongFocus
            Keys.onBacktabPressed: doneButton.forceActiveFocus(Qt.BacktabFocusReason)
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
            model: root.choiceKey === "" ? [] : root.choiceKey.split("\n")

            DListItem {
                id: row
                required property string modelData
                required property int index
                readonly property var playlist: PlaylistService.playlist(modelData)
                readonly property bool loaded: playlist?.loaded === true
                readonly property var missing: root.tracks.filter(t => PlaylistService.trackPositions(modelData, [t]).length === 0)
                readonly property bool holdsAll: loaded && missing.length === 0
                readonly property bool holdsSome: loaded && missing.length < root.tracks.length
                readonly property string waitingFor: root.pending[modelData] ?? ""
                readonly property bool settled: waitingFor === "all" ? holdsAll : waitingFor === "none" && !holdsSome
                readonly property bool showsAll: waitingFor !== "" ? waitingFor === "all" : holdsAll
                readonly property bool showsSome: waitingFor !== "" ? waitingFor === "all" : holdsSome
                // a server playlist takes the picked tracks from its own server and skips the rest
                readonly property string blocker: playlist ? PlayerService.whyNot("playlistEdit", playlist) : ""
                readonly property bool fits: blocker === "" && (holdsAll || PlayerService.partition("playlistAdd", missing, playlist).ok.length > 0)
                width: parent.width
                implicitHeight: Theme.listItemTwoLineHeight
                firstInGroup: false
                lastInGroup: index === root.choiceKey.split("\n").length - 1
                enabled: loaded && fits
                focusPolicy: Qt.StrongFocus
                Accessible.role: Accessible.CheckBox
                Accessible.name: playlist?.name ?? ""
                Accessible.checked: showsAll
                onSettledChanged: {
                    if (settled)
                        root.setPending(modelData, "");
                }
                onClicked: {
                    const id = modelData;
                    if (waitingFor !== "")
                        return;
                    const free = () => root.setPending(id, "");
                    if (holdsAll) {
                        root.setPending(id, "none");
                        PlayerService.removeFromPlaylist(id, PlaylistService.trackPositions(id, root.tracks), free);
                        return;
                    }
                    root.setPending(id, "all");
                    const skipped = PlayerService.addToPlaylist(id, missing, free);
                    PlayerService.reportSkipped(skipped);
                    // part of the selection can't go here, so the row won't reach "all"
                    if (skipped > 0)
                        free();
                }

                DIcon {
                    id: icon
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.spacingL
                    anchors.verticalCenter: parent.verticalCenter
                    name: row.playlist?.kind === "server" ? "cloud" : "queue_music"
                    size: Theme.iconSize
                    color: row.contentColor
                }

                Column {
                    anchors.left: icon.right
                    anchors.leftMargin: Theme.spacingL
                    anchors.right: check.left
                    anchors.rightMargin: Theme.spacingM
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: Theme.spacingXXS

                    StyledText {
                        width: parent.width
                        text: row.playlist?.name ?? ""
                        font.pixelSize: Theme.fontSizeMedium
                        color: row.contentColor
                        elide: Text.ElideRight
                        wrapMode: Text.NoWrap
                        horizontalAlignment: Text.AlignLeft
                    }

                    StyledText {
                        width: parent.width
                        text: {
                            if (root.unreadable[row.modelData])
                                return I18n.tr("Couldn't load the playlist", "toast when a playlist's tracks can't be loaded");
                            if (!row.loaded)
                                return I18n.tr("Loading", "something still waiting for data: the play button, a list");
                            if (row.blocker !== "")
                                return row.blocker;
                            if (row.playlist?.kind === "local")
                                return I18n.tr("Local playlist", "playlist kind label");
                            if (!row.fits)
                                return I18n.tr("Only tracks from %1 can be added", "why a server playlist cannot take the picked tracks, %1 is the server library").arg(PlayerService.libraryName(row.playlist?.library));
                            return I18n.tr("Playlist on %1", "playlist kind label for a server playlist, %1 is the library").arg(PlayerService.libraryName(row.playlist?.library));
                        }
                        font.pixelSize: Theme.fontSizeSmall
                        color: row.supportingContentColor
                        elide: Text.ElideRight
                        wrapMode: Text.NoWrap
                        horizontalAlignment: Text.AlignLeft
                    }
                }

                DIcon {
                    id: check
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.spacingL
                    anchors.verticalCenter: parent.verticalCenter
                    visible: row.loaded
                    name: row.showsAll ? "check_box" : row.showsSome ? "indeterminate_check_box" : "check_box_outline_blank"
                    filled: row.showsSome
                    size: Theme.iconSize
                    color: row.showsSome ? Theme.primary : row.contentColor
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
            id: doneButton
            Keys.onTabPressed: newRow.forceActiveFocus(Qt.TabFocusReason)
            text: I18n.tr("Done", "edit mode finish button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: root.close()
        }
    ]
}
