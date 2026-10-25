pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets

// Adds a library or edits one. Local libraries take folders; a server library is one account on a
// (Open)Subsonic or Jellyfin server.
WindowDialog {
    id: root

    property string editingId: ""
    property string kind: "local"
    property var folders: []
    // "" | "testing" | "ok" | "failed"
    property string testState: ""
    property string testMessage: ""

    readonly property bool editing: editingId !== ""
    readonly property var library: editing ? PlayerService.library(editingId) : null
    readonly property var marksJob: SettingsService.markFiles.job?.library === editingId ? SettingsService.markFiles.job : null
    readonly property var marksDone: SettingsService.markFilesDone?.library === editingId ? SettingsService.markFilesDone : null
    readonly property var marksFailed: SettingsService.markFilesFailed?.library === editingId ? SettingsService.markFilesFailed : null
    // the daemon keeps the last problems of every library; these are the ones in this library's folders
    readonly property var marksProblems: (SettingsService.markFiles.problems ?? []).filter(pr => (library?.folders ?? []).some(f => pr.path.startsWith(f.replace(/\/?$/, "/"))))
    readonly property bool remote: kind !== "local"
    // the kinds the daemon can add, in the order offered
    readonly property var kinds: ["local", "subsonic", "jellyfin"].filter(k => PlayerService.libraryKinds.indexOf(k) >= 0)
    readonly property bool valid: blocker === ""
    readonly property string blocker: {
        if (remote) {
            if (urlField.text.trim() === "" || userField.text.trim() === "")
                return I18n.tr("Enter the server address and user name.", "add server dialog, why add is disabled");
            if (testState !== "ok" && !(editing && passwordField.text === ""))
                return I18n.tr("Test the connection first.", "add server dialog, why add is disabled");
            return "";
        }
        if (nameField.text.trim() === "")
            return I18n.tr("Enter a name.", "add local library dialog, why add is disabled");
        if (folders.length === 0)
            return I18n.tr("Add at least one folder.", "add local library dialog, why add is disabled");
        return "";
    }

    function reset() {
        nameField.text = "";
        folderField.text = "";
        urlField.text = "";
        userField.text = "";
        passwordField.text = "";
        folders = [];
        testState = "";
        testMessage = "";
    }

    function add() {
        reset();
        editingId = "";
        kind = "local";
        opened = true;
        nameField.forceActiveFocus();
    }

    function edit(library) {
        reset();
        editingId = library.id;
        kind = library.kind;
        nameField.text = library.name;
        folders = (library.folders ?? []).slice();
        urlField.text = library.url ?? "";
        userField.text = library.user ?? "";
        opened = true;
        nameField.forceActiveFocus();
    }

    function addFolder() {
        const path = folderField.text.trim();
        if (path === "" || folders.indexOf(path) >= 0)
            return;
        folders = folders.concat([path]);
        folderField.text = "";
    }

    function save() {
        if (!valid)
            return;
        const name = nameField.text.trim();
        if (editing) {
            const patch = remote ? {
                "name": name,
                "url": urlField.text.trim(),
                "user": userField.text.trim(),
                "password": passwordField.text
            } : {
                "name": name
            };
            // new folders need a rescan, a rename doesn't
            if (!remote && JSON.stringify(folders) !== JSON.stringify(PlayerService.library(editingId)?.folders ?? []))
                patch.folders = folders;
            PlayerService.updateLibrary(editingId, patch);
        } else if (remote) {
            PlayerService.addServerLibrary(kind, name, urlField.text.trim(), userField.text.trim(), passwordField.text);
        } else {
            PlayerService.addLocalLibrary(name, folders);
        }
        passwordField.text = "";
        opened = false;
    }

    maximumHeight: parent ? parent.height - Theme.spacingXL * 2 : Infinity
    title: editing ? I18n.tr("Edit library", "library dialog title") : I18n.tr("Add library", "libraries settings primary action")
    acceptEnabled: valid
    onAccepted: save()
    onRejected: {
        passwordField.text = "";
        opened = false;
    }

    Connections {
        target: PlayerService
        function onServerTestFinished(ok, message) {
            if (root.testState !== "testing")
                return;
            root.testState = ok ? "ok" : "failed";
            root.testMessage = message;
        }
    }

    DButtonGroup {
        visible: !root.editing
        model: root.kinds.map(k => ({
                    "text": PlayerService.kindLabel(k),
                    "icon": k === "local" ? "folder" : "cloud"
                }))
        currentIndex: root.kinds.indexOf(root.kind)
        onSelectionChanged: (index, selected) => {
            if (selected) {
                root.kind = root.kinds[index];
                root.testState = "";
            }
        }
    }

    DTextField {
        id: nameField
        width: parent.width
        outlined: true
        labelText: root.remote ? I18n.tr("Name (optional)", "library dialog field") : I18n.tr("Name", "a name: field label, sort key")
        placeholderText: root.remote ? I18n.tr("Uses the server address", "library dialog name placeholder for a server") : ""
    }

    Column {
        width: parent.width
        spacing: Theme.groupedListGap
        visible: !root.remote

        Repeater {
            model: root.folders

            DListRow {
                id: folderRow
                required property string modelData
                required property int index
                width: parent.width
                firstInGroup: index === 0
                lastInGroup: false

                contentItem: Item {
                    DIcon {
                        id: folderIcon
                        anchors.left: parent.left
                        anchors.leftMargin: Theme.spacingL
                        anchors.verticalCenter: parent.verticalCenter
                        name: "folder"
                        size: Theme.iconSize
                        color: folderRow.supportingContentColor
                    }

                    StyledText {
                        anchors.left: folderIcon.right
                        anchors.leftMargin: Theme.spacingM
                        anchors.right: removeFolder.left
                        anchors.rightMargin: Theme.spacingS
                        anchors.verticalCenter: parent.verticalCenter
                        text: folderRow.modelData
                        font.pixelSize: Theme.fontSizeMedium
                        color: folderRow.contentColor
                        elide: Text.ElideMiddle
                        wrapMode: Text.NoWrap
                        horizontalAlignment: Text.AlignLeft
                    }

                    DActionButton {
                        id: removeFolder
                        anchors.right: parent.right
                        anchors.rightMargin: Theme.spacingS
                        anchors.verticalCenter: parent.verticalCenter
                        iconName: "close"
                        tooltipText: I18n.tr("Remove folder", "library dialog folder action")
                        Accessible.name: tooltipText
                        onClicked: root.folders = root.folders.filter((_, i) => i !== folderRow.index)
                    }
                }
            }
        }

        DListRow {
            width: parent.width
            implicitHeight: folderField.height + Theme.spacingS * 2
            firstInGroup: root.folders.length === 0
            lastInGroup: true

            contentItem: Item {
                DTextField {
                    id: folderField
                    anchors.left: parent.left
                    anchors.right: addFolderButton.left
                    anchors.leftMargin: Theme.spacingS
                    anchors.rightMargin: Theme.spacingS
                    anchors.verticalCenter: parent.verticalCenter
                    outlined: true
                    labelText: I18n.tr("Folder", "library dialog folder field")
                    placeholderText: I18n.tr("~/Music", "example folder path in the library dialog")
                    leftIconName: "create_new_folder"
                    onAccepted: root.addFolder()
                }

                DButton {
                    id: addFolderButton
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.spacingS
                    anchors.verticalCenter: parent.verticalCenter
                    text: I18n.tr("Add folder", "library dialog folder action")
                    buttonHeight: Theme.buttonHeightXS
                    backgroundColor: Theme.secondaryContainer
                    textColor: Theme.onSecondaryContainer
                    enabled: folderField.text.trim() !== ""
                    onClicked: root.addFolder()
                }
            }
        }

        StyledText {
            width: parent.width
            topPadding: Theme.spacingXS
            text: I18n.tr("Includes subfolders. Changes are detected automatically.", "library dialog folders hint")
            font.pixelSize: Theme.fontSizeSmall
            color: Theme.surfaceVariantText
            wrapMode: Text.WordWrap
        }
    }

    DTextField {
        id: urlField
        visible: root.remote
        width: parent.width
        outlined: true
        labelText: I18n.tr("Server address", "add server dialog field")
        placeholderText: I18n.tr("https://music.example.net", "example server address in the add server dialog")
        leftIconName: "dns"
        onTextChanged: root.testState = ""
    }

    DTextField {
        id: userField
        visible: root.remote
        width: parent.width
        outlined: true
        labelText: I18n.tr("User name", "add server dialog field")
        leftIconName: "person"
        onTextChanged: root.testState = ""
    }

    DTextField {
        id: passwordField
        visible: root.remote
        width: parent.width
        outlined: true
        labelText: I18n.tr("Password", "add server dialog field")
        supportingText: root.editing ? I18n.tr("Leave empty to keep the saved password. Stored in the system keyring.", "edit server dialog password hint") : I18n.tr("Stored in the system keyring.", "add server dialog password hint")
        echoMode: TextInput.Password
        showPasswordToggle: true
        leftIconName: "key"
        onTextChanged: root.testState = ""
    }

    Row {
        visible: root.remote
        spacing: Theme.spacingM

        DButton {
            text: I18n.tr("Test connection", "add server dialog action")
            backgroundColor: Theme.secondaryContainer
            textColor: Theme.onSecondaryContainer
            busy: root.testState === "testing"
            enabled: root.testState !== "testing" && urlField.text.trim() !== ""
            onClicked: {
                root.testState = "testing";
                PlayerService.testServer(root.kind, urlField.text.trim(), userField.text.trim(), passwordField.text);
            }
        }

        StyledText {
            anchors.verticalCenter: parent.verticalCenter
            visible: root.testState === "ok" || root.testState === "failed"
            text: root.testState === "ok" ? I18n.tr("Connected", "add server dialog, connection test passed") : I18n.tr("Couldn't connect to server", "add server dialog, connection test failed title")
            font.pixelSize: Theme.fontSizeMedium
            color: root.testState === "ok" ? Theme.primary : Theme.error
        }
    }

    StyledText {
        width: parent.width
        visible: root.remote && root.testState === "failed" && root.testMessage !== ""
        text: root.testMessage
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.surfaceVariantText
        wrapMode: Text.WordWrap
    }

    // ratings and favorites in the files' tags; applies at once, not on Save
    SettingsGroup {
        visible: root.editing && (root.library?.capabilities ?? []).indexOf("writeMarks") >= 0
        width: parent.width

        SettingsToggleRow {
            text: I18n.tr("Write ratings and favorites to files", "local library setting")
            description: I18n.tr("Stores ratings and favorites in the tags of your music files.", "local library setting description")
            checked: root.library?.writeMarks ?? false
            onToggled: value => SettingsService.setWriteMarks(root.editingId, value)
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
                return I18n.tr("Writes ratings and favorites set before this was turned on.", "local library action description");
            }
            subtitleColor: root.marksFailed && !root.marksJob ? Theme.error : Theme.surfaceVariantText

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                text: I18n.tr("Write", "button that writes ratings to files")
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                busy: root.marksJob !== null
                enabled: !SettingsService.markFiles.job
                onClicked: SettingsService.writeMarksNow(root.editingId)
            }
        }

        Repeater {
            model: (root.library?.writeMarks ?? false) ? root.marksProblems : []

            SettingsRow {
                required property var modelData
                title: modelData.path.substring(modelData.path.lastIndexOf("/") + 1)
                subtitle: Phrases.join(modelData.skipped ? I18n.tr("Skipped", "a file left alone when writing ratings") : I18n.tr("Failed", "a file that couldn't be written"), Phrases.reason(modelData.code, modelData.reason))
                iconName: modelData.skipped ? "block" : "error"
                iconColor: modelData.skipped ? Theme.surfaceVariantText : Theme.error
                Accessible.description: Phrases.join(modelData.path, subtitle)
            }
        }
    }

    StyledText {
        width: parent.width
        visible: root.blocker !== ""
        text: root.blocker
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.surfaceVariantText
        wrapMode: Text.WordWrap
    }

    actions: [
        DButton {
            text: I18n.tr("Cancel", "dialog dismiss button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: root.rejected()
        },
        DButton {
            text: root.editing ? I18n.tr("Save", "dialog confirm button") : I18n.tr("Add", "dialog confirm button")
            enabled: root.valid
            onClicked: root.save()
        }
    ]
}
