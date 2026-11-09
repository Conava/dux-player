pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets

Item {
    id: root

    property var pendingRemove: null
    // Home's empty state asks for the add dialog
    readonly property bool addWanted: Router.wantAddLibrary

    onAddWantedChanged: {
        if (!addWanted)
            return;
        // later: clearing the flag from its own change handler is a binding loop
        Qt.callLater(() => {
            Router.wantAddLibrary = false;
            dialog.add();
        });
    }

    function describe(library) {
        if (PlayerService.isRemote(library.id))
            return Phrases.join(PlayerService.kindLabel(library.kind), I18n.tr("%1 at %2", "server account: user at address").arg(library.user).arg(library.url.replace(/^https?:\/\//, "")));
        return Phrases.join(library.folders.join(", "), Phrases.tracks(Phrases.libraryTrackCount(library.id)));
    }

    function status(library) {
        if (!library.enabled)
            return I18n.tr("Off. Its tracks are hidden and skipped in playlists.", "disabled library status");
        if (library.error)
            return Phrases.libraryError(library);
        if (library.scanning)
            return Phrases.libraryBusy(library);
        if (!library.lastScan)
            return "";
        return I18n.tr("Last scanned %1", "library row status, %1 is a date and time").arg(new Date(library.lastScan * 1000).toLocaleString(Qt.locale(), Locale.ShortFormat));
    }

    SettingsPage {
        id: page
        anchors.fill: parent

        bottomPadding: SettingsMetrics.pagePaddingV + fab.height + Theme.spacingL

        Column {
            id: groupColumn
            width: parent.width
            spacing: Theme.spacingL

            SettingsPageIntro {
                text: I18n.tr("Turning a library off hides its music without removing it.", "libraries settings page intro")
            }

            SettingsGroup {
                Repeater {
                    model: PlayerService.libraries

                    SettingsRow {
                        id: row
                        required property var modelData
                        readonly property bool remote: PlayerService.isRemote(modelData.id)

                        title: modelData.name
                        subtitle: root.describe(modelData)
                        // its settings hold the scrobbling switches
                        keywords: "scrobble scrobbling listenbrainz last.fm lastfm"
                        clickable: true
                        onClicked: dialog.edit(modelData)

                        leading: Rectangle {
                            width: Theme.iconButtonSize
                            height: Theme.iconButtonSize
                            radius: Theme.fullRadius(width, height)
                            anchors.verticalCenter: parent.verticalCenter
                            color: row.remote ? Theme.tertiaryContainer : Theme.secondaryContainer

                            DIcon {
                                anchors.centerIn: parent
                                name: UiConfig.entryIcon({
                                    "kind": "library",
                                    "target": row.modelData.id
                                })
                                size: Theme.iconSizeMedium
                                color: row.remote ? Theme.onTertiaryContainer : Theme.onSecondaryContainer
                            }
                        }

                        body: Column {
                            width: parent.width

                            StyledText {
                                width: parent.width
                                visible: text !== ""
                                text: PlayerService.featureSummary(row.modelData.id)
                                font.pixelSize: Theme.fontSizeSmall
                                color: Theme.surfaceVariantText
                                wrapMode: Text.WordWrap
                                horizontalAlignment: Text.AlignLeft
                            }

                            StyledText {
                                width: parent.width
                                visible: text !== ""
                                text: root.status(row.modelData)
                                font.pixelSize: Theme.fontSizeSmall
                                color: row.modelData.scanning ? Theme.primary : row.modelData.error && row.modelData.enabled ? Theme.error : Theme.surfaceVariantText
                                wrapMode: Text.WordWrap
                                horizontalAlignment: Text.AlignLeft
                            }

                            Repeater {
                                model: row.modelData.noticeCodes

                                StyledText {
                                    // {code, message, detail}
                                    required property var modelData
                                    width: parent.width
                                    text: Phrases.reason(modelData.code, modelData.message, modelData.detail)
                                    font.pixelSize: Theme.fontSizeSmall
                                    color: Theme.surfaceVariantText
                                    wrapMode: Text.WordWrap
                                    horizontalAlignment: Text.AlignLeft
                                }
                            }
                        }

                        DActionButton {
                            anchors.verticalCenter: parent.verticalCenter
                            visible: row.modelData.enabled && !row.modelData.available
                            iconName: "sync_problem"
                            tooltipText: I18n.tr("Try again", "library action that checks an unavailable library again")
                            Accessible.name: tooltipText
                            onClicked: PlayerService.retryLibrary(row.modelData.id)
                        }

                        DActionButton {
                            anchors.verticalCenter: parent.verticalCenter
                            iconName: "refresh"
                            enabled: row.modelData.enabled && !row.modelData.scanning
                            tooltipText: row.remote ? I18n.tr("Sync now", "library row action") : I18n.tr("Rescan", "library row action")
                            Accessible.name: tooltipText
                            onClicked: PlayerService.rescanLibrary(row.modelData.id)
                        }

                        DActionButton {
                            anchors.verticalCenter: parent.verticalCenter
                            iconName: "delete"
                            iconColor: Theme.error
                            tooltipText: I18n.tr("Remove", "button that removes an item")
                            Accessible.name: tooltipText
                            onClicked: {
                                root.pendingRemove = row.modelData;
                                removeDialog.ask();
                            }
                        }

                        DToggle {
                            anchors.verticalCenter: parent.verticalCenter
                            hideText: true
                            text: I18n.tr("Enabled", "library row switch")
                            checked: row.modelData.enabled
                            onToggled: value => PlayerService.setLibraryEnabled(row.modelData.id, value)
                        }
                    }
                }
            }

            StyledText {
                width: parent.width
                visible: PlayerService.libraries.length === 0
                text: I18n.tr("No libraries", "libraries settings empty state")
                font.pixelSize: Theme.fontSizeMedium
                color: Theme.surfaceVariantText
                horizontalAlignment: Text.AlignLeft
            }
        }
    }

    DFab {
        id: fab
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.bottom
        anchors.bottomMargin: Theme.spacingL
        text: I18n.tr("Add library", "libraries settings primary action")
        iconName: "add"
        onClicked: dialog.add()
    }

    LibraryDialog {
        id: dialog
        host: root
    }

    ConfirmDialog {
        id: removeDialog
        host: root
        title: I18n.tr("Remove \"%1\"?", "remove library confirmation title").arg(root.pendingRemove?.name ?? "")
        supportingText: PlayerService.isRemote(root.pendingRemove?.id ?? "") ? I18n.tr("The server and its saved password are removed from Dux Player. Nothing is deleted on the server.", "remove server library confirmation") : I18n.tr("The folders are removed from Dux Player. No files are deleted.", "remove local library confirmation")
        confirmText: I18n.tr("Remove", "button that removes an item")
        onConfirmed: PlayerService.removeLibrary(root.pendingRemove.id)
    }
}
