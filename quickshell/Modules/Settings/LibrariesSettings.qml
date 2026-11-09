pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets

Item {
    id: root

    // "" for the list, a library id for its page
    property string subPage: ""
    readonly property var subViews: PlayerService.libraries.map(l => l.id)
    readonly property bool showingLibrary: subPage !== "" && PlayerService.library(subPage) !== null
    readonly property string subTitle: showingLibrary ? PlayerService.library(subPage).name : ""
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

    // quiet is the settings search reading a library's page; it fetches nothing either way
    function openSubView(id, quiet) {
        subPage = id;
    }

    function goBack() {
        subPage = "";
    }

    function describe(library) {
        if (PlayerService.isRemote(library.id))
            return Phrases.join(PlayerService.kindLabel(library.kind), I18n.tr("%1 at %2", "server account: user at address").arg(library.user).arg(library.url.replace(/^https?:\/\//, "")));
        // a turned-off library's music isn't loaded, so there is nothing to count
        return Phrases.joinAll([library.folders.join(", "), library.enabled ? Phrases.tracks(Phrases.libraryTrackCount(library.id)) : ""]);
    }

    // what needs attention; the library's page has the rest
    function status(library) {
        if (!library.enabled)
            return "";
        if (library.error)
            return Phrases.libraryError(library);
        return Phrases.libraryBusy(library);
    }

    SettingsPage {
        id: page
        anchors.fill: parent

        bottomPadding: SettingsMetrics.pagePaddingV + (fab.visible ? fab.height + Theme.spacingL : 0)

        Column {
            id: groupColumn
            width: parent.width
            visible: !root.showingLibrary
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
                        showChevron: true
                        clickable: true
                        onClicked: root.openSubView(modelData.id, false)

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

                        body: StyledText {
                            width: parent.width
                            visible: text !== ""
                            text: root.status(row.modelData)
                            font.pixelSize: Theme.fontSizeSmall
                            color: row.modelData.scanning ? Theme.primary : Theme.error
                            wrapMode: Text.WordWrap
                            horizontalAlignment: Text.AlignLeft
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

        LibraryPage {
            visible: root.showingLibrary
            libraryId: root.showingLibrary ? root.subPage : ""
        }
    }

    DFab {
        id: fab
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.bottom
        anchors.bottomMargin: Theme.spacingL
        visible: !root.showingLibrary
        text: I18n.tr("Add library", "libraries settings primary action")
        iconName: "add"
        onClicked: dialog.add()
    }

    LibraryDialog {
        id: dialog
        host: root
    }
}
