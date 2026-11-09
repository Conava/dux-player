pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets
import qs.DCommon.FileBrowser
import Quickshell

// One library's settings, shown inside the Libraries page. Everything applies at once.
Column {
    id: root

    property string libraryId: ""
    // held in properties: as Column children the Column would position them and drop their anchors
    readonly property TextPromptDialog namePrompt: TextPromptDialog {
        host: root
    }
    readonly property LibraryServerDialog serverDialog: LibraryServerDialog {
        host: root
    }
    readonly property ConfirmDialog removeDialog: ConfirmDialog {
        host: root
        title: I18n.tr("Remove \"%1\"?", "remove library confirmation title").arg(root.library?.name ?? "")
        supportingText: root.server ? I18n.tr("The server and its saved password are removed from Dux Player. Nothing is deleted on the server.", "remove server library confirmation") : I18n.tr("The folders are removed from Dux Player. No files are deleted.", "remove local library confirmation")
        confirmText: I18n.tr("Remove", "button that removes an item")
        onConfirmed: PlayerService.removeLibrary(root.libraryId)
    }

    readonly property var library: PlayerService.library(libraryId)
    readonly property bool server: (library?.kind ?? "local") !== "local"
    readonly property var folders: library?.folders ?? []
    readonly property var capabilities: library?.capabilities ?? []
    // a library signed in with a code can't change its address until the sign-in expires
    readonly property bool signedInWithCode: library?.auth === "quickConnect" && library?.errorCode !== "signInExpired"
    readonly property var usage: SettingsService.cacheLibrary(libraryId)
    property bool typingPath: false

    readonly property var marksJob: SettingsService.markFiles.job?.library === libraryId ? SettingsService.markFiles.job : null
    readonly property var marksDone: SettingsService.markFilesDone?.library === libraryId ? SettingsService.markFilesDone : null
    readonly property var marksFailed: SettingsService.markFilesFailed?.library === libraryId ? SettingsService.markFilesFailed : null
    // the daemon keeps the last problems of every library; these are the ones in this library's folders
    readonly property var marksProblems: (SettingsService.markFiles.problems ?? []).filter(pr => folders.some(f => pr.path.startsWith(f.replace(/\/?$/, "/"))))
    readonly property bool toServer: capabilities.indexOf("scrobble") >= 0 && (PlayerService.scrobble.noServerLibraries ?? []).indexOf(libraryId) < 0
    // the accounts that take plays: signed in and switched on
    readonly property bool listenbrainz: PlayerService.scrobble.listenbrainz?.connected === true && PlayerService.scrobble.listenbrainz?.enabled === true
    readonly property bool lastfm: PlayerService.scrobble.lastfm?.connected === true && PlayerService.scrobble.lastfm?.enabled === true
    // signed in to an account, switched on or not
    readonly property bool signedInAccount: PlayerService.scrobble.listenbrainz?.connected === true || PlayerService.scrobble.lastfm?.connected === true

    function status() {
        if (!library)
            return "";
        if (library.error)
            return Phrases.libraryError(library);
        if (library.scanning)
            return Phrases.libraryBusy(library);
        if (!library.lastScan)
            return "";
        return I18n.tr("Last scanned %1", "library row status, %1 is a date and time").arg(new Date(library.lastScan * 1000).toLocaleString(Qt.locale(), Locale.ShortFormat));
    }

    function address() {
        return (library?.url ?? "").replace(/^https?:\/\//, "");
    }

    function setFolders(next) {
        PlayerService.updateLibrary(libraryId, {
            "folders": next
        });
    }

    function addFolder(typed) {
        const path = Paths.expandTilde(typed.trim()).replace(/(.)\/+$/, "$1");
        if (path === "" || folders.indexOf(path) >= 0)
            return;
        setFolders(folders.concat([path]));
        folderRow.field.text = "";
    }

    onLibraryIdChanged: typingPath = false
    // the settings search's section for this page's rows: the page header shows the name
    readonly property string searchSection: library?.name ?? ""
    width: parent?.width ?? 0
    spacing: Theme.spacingL

    SettingsCard {
        SettingsToggleRow {
            text: I18n.tr("Enabled", "library row switch")
            description: checked ? "" : I18n.tr("Off. Its tracks are hidden and skipped in playlists.", "disabled library status")
            checked: root.library?.enabled ?? false
            onToggled: value => PlayerService.setLibraryEnabled(root.libraryId, value)
        }

        SettingsRow {
            title: I18n.tr("Name", "a name: field label, sort key")
            trailingBadge: root.library?.name ?? ""
            showChevron: true
            clickable: true
            onClicked: root.namePrompt.ask(I18n.tr("Rename library", "library page: the rename dialog title"), root.library?.name ?? "", I18n.tr("Save", "dialog confirm button"), name => PlayerService.updateLibrary(root.libraryId, {
                    "name": name
                }))
        }

        SettingsRow {
            visible: root.server
            title: I18n.tr("Server", "details field: the server address")
            subtitle: root.library?.errorCode === "signInExpired" ? Phrases.reason("signInExpired") : root.library?.user ? I18n.tr("Signed in as %1", "scrobbling account state, %1 is the user name").arg(root.library.user) : ""
            subtitleColor: root.library?.errorCode === "signInExpired" ? Theme.error : Theme.surfaceVariantText
            trailingBadge: root.address()
            showChevron: !root.signedInWithCode
            clickable: !root.signedInWithCode
            onClicked: root.serverDialog.open(root.libraryId)

            body: StyledText {
                width: parent.width
                visible: text !== ""
                text: PlayerService.featureSummary(root.libraryId)
                font.pixelSize: Theme.fontSizeSmall
                color: Theme.surfaceVariantText
                wrapMode: Text.WordWrap
                horizontalAlignment: Text.AlignLeft
            }
        }

        SettingsRow {
            visible: !root.server
            title: I18n.tr("Folders", "library kind option")
            subtitle: I18n.tr("Includes subfolders. Changes are detected automatically.", "library dialog folders hint")
        }

        Repeater {
            model: root.server ? [] : root.folders

            SettingsRow {
                id: folder
                required property string modelData
                title: modelData
                searchable: false
                iconName: "folder"

                // a library keeps at least one folder
                DActionButton {
                    anchors.verticalCenter: parent.verticalCenter
                    visible: root.folders.length > 1
                    iconName: "close"
                    tooltipText: I18n.tr("Remove folder", "library dialog folder action")
                    Accessible.name: Phrases.named(tooltipText, folder.modelData)
                    onClicked: root.setFolders(root.folders.filter(f => f !== folder.modelData))
                }
            }
        }

        SettingsRow {
            visible: !root.server
            title: I18n.tr("Add folder", "library dialog folder action")
            iconName: "add"
            clickable: true
            onClicked: folderPicker.open()

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                visible: !root.typingPath
                text: I18n.tr("Enter a path", "library dialog action that shows a folder path field")
                backgroundColor: "transparent"
                textColor: Theme.primary
                onClicked: {
                    root.typingPath = true;
                    folderRow.field.forceActiveFocus();
                }
            }
        }

        SettingsFieldRow {
            id: folderRow
            visible: !root.server && root.typingPath
            title: I18n.tr("Folder", "library dialog folder field")
            searchable: false
            subtitle: I18n.tr("Press Enter to add the folder.", "library dialog folder field hint")
            field.placeholderText: I18n.tr("~/Music", "example folder path in the library dialog")
            field.leftIconName: "create_new_folder"
            onAccepted: root.addFolder(field.text)
        }

        Repeater {
            model: root.library?.noticeCodes ?? []

            SettingsRow {
                // {code, message, detail}
                required property var modelData
                title: Phrases.reason(modelData.code, modelData.message, modelData.detail)
                searchable: false
                iconName: "info"
            }
        }
    }

    // ratings and favorites in the files' tags
    SettingsCard {
        visible: root.capabilities.indexOf("writeMarks") >= 0
        title: I18n.tr("Ratings in files", "library page section")

        SettingsToggleRow {
            text: I18n.tr("Write ratings and favorites to files", "local library setting")
            description: I18n.tr("Stores ratings and favorites in the tags of the music files.", "local library setting description")
            checked: root.library?.writeMarks ?? false
            onToggled: value => SettingsService.setWriteMarks(root.libraryId, value)
        }

        SettingsRow {
            visible: root.library?.writeMarks ?? false
            title: I18n.tr("Write existing ratings now", "local library action")
            subtitle: {
                if (root.marksJob) {
                    const progress = I18n.tr("Writing… %1 of %2", "progress of writing ratings to files").arg(root.marksJob.done).arg(root.marksJob.total);
                    return SettingsService.markFiles.waiting ? Phrases.join(progress, I18n.tr("The current track is written when the next one starts", "why writing ratings to files waits")) : progress;
                }
                if (root.marksFailed)
                    return Phrases.reason(root.marksFailed.code, root.marksFailed.reason);
                if (root.marksDone)
                    return root.marksDone.failed > 0 ? I18n.tr("%1 files written, %2 failed", "ratings written to files").arg(root.marksDone.written).arg(root.marksDone.failed) : I18n.tr("%1 files written", "ratings written to files").arg(root.marksDone.written);
                return I18n.tr("Writes ratings and favorites set earlier.", "local library action description");
            }
            subtitleColor: root.marksFailed && !root.marksJob ? Theme.error : Theme.surfaceVariantText

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                text: I18n.tr("Write", "button that writes ratings to files")
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                busy: root.marksJob !== null
                enabled: !SettingsService.markFiles.job
                onClicked: SettingsService.writeMarksNow(root.libraryId)
            }
        }

        Repeater {
            model: (root.library?.writeMarks ?? false) ? root.marksProblems : []

            SettingsRow {
                required property var modelData
                title: modelData.path.substring(modelData.path.lastIndexOf("/") + 1)
                searchable: false
                subtitle: Phrases.join(modelData.skipped ? I18n.tr("Skipped", "a file left alone when writing ratings") : I18n.tr("Failed", "a file that couldn't be written"), Phrases.reason(modelData.code, modelData.reason))
                iconName: modelData.skipped ? "block" : "error"
                iconColor: modelData.skipped ? Theme.surfaceVariantText : Theme.error
                Accessible.description: Phrases.join(modelData.path, subtitle)
            }
        }
    }

    // where the library's plays are reported
    SettingsCard {
        title: I18n.tr("Scrobbling", "scrobbling: feature and settings tab")

        SettingsToggleRow {
            visible: root.capabilities.indexOf("scrobble") >= 0
            text: I18n.tr("Report plays to %1", "library setting, %1 is the server library").arg(root.library?.name ?? "")
            searchable: false
            checked: root.toServer
            onToggled: value => PlayerService.setScrobbleLibraryServer(root.libraryId, value)
        }

        SettingsToggleRow {
            // brand names stay untranslated
            text: root.listenbrainz === root.lastfm ? I18n.tr("Scrobble to ListenBrainz and Last.fm", "library setting") : root.listenbrainz ? I18n.tr("Scrobble to ListenBrainz", "scrobbling setting") : I18n.tr("Scrobble to Last.fm", "scrobbling setting")
            searchable: false
            description: !enabled ? (root.signedInAccount ? I18n.tr("Turn on an account on the Scrobbling page first.", "why a library's scrobbling setting is unavailable") : I18n.tr("Sign in on the Scrobbling page first.", "why a library's scrobbling setting is unavailable")) : root.toServer ? I18n.tr("Plays may count twice if the server also scrobbles.", "scrobbling server library row") : ""
            enabled: root.listenbrainz || root.lastfm
            checked: enabled && (PlayerService.scrobble.noAccountsLibraries ?? []).indexOf(root.libraryId) < 0
            onToggled: value => PlayerService.setScrobbleLibraryAccounts(root.libraryId, value)
        }
    }

    // servers only, and only with a working offline cache: its status then has a limit
    SettingsGroup {
        visible: PlayerService.isRemote(root.libraryId) && SettingsService.cache.limit !== undefined

        SettingsRow {
            title: I18n.tr("Downloads", "the offline downloads: page, settings tab")
            iconName: "download"
            trailingBadge: Phrases.join(Phrases.tracks(root.usage.count ?? 0), Phrases.size(root.usage.used ?? 0))
            showChevron: true
            clickable: true
            onClicked: Router.openSettings("downloads/library:" + root.libraryId)
        }
    }

    SettingsGroup {
        SettingsRow {
            title: root.server ? I18n.tr("Sync now", "library row action") : I18n.tr("Rescan", "library row action")
            subtitle: root.status()
            subtitleColor: root.library?.scanning ? Theme.primary : root.library?.error && root.library?.enabled ? Theme.error : Theme.surfaceVariantText
            iconName: "refresh"

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                visible: (root.library?.enabled ?? false) && !(root.library?.available ?? true)
                text: I18n.tr("Try again", "library action that checks an unavailable library again")
                backgroundColor: "transparent"
                textColor: Theme.primary
                onClicked: PlayerService.retryLibrary(root.libraryId)
            }

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                text: root.server ? I18n.tr("Sync now", "library row action") : I18n.tr("Rescan", "library row action")
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                busy: root.library?.scanning ?? false
                enabled: (root.library?.enabled ?? false) && !(root.library?.scanning ?? false)
                onClicked: PlayerService.rescanLibrary(root.libraryId)
            }
        }

        SettingsRow {
            title: I18n.tr("Remove library", "library page action")
            iconName: "delete"
            iconColor: Theme.error

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                text: I18n.tr("Remove", "button that removes an item")
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                onClicked: root.removeDialog.ask()
            }
        }
    }

    FileBrowserModal {
        id: folderPicker
        parentModal: root.QsWindow.window
        browserTitle: I18n.tr("Choose folder", "library dialog folder action")
        bucket: "library"
        folderMode: true
        onFileSelected: path => root.addFolder(path)
    }
}
