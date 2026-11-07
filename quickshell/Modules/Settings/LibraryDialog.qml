pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets
import qs.DCommon.FileBrowser
import Quickshell

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
    // the reason Add is disabled shows only after the first edit or a submit attempt
    property bool touched: false
    property bool typingPath: false
    // "password" | "quickConnect"
    property string method: "password"
    // Add was pressed and waits for the connection test it started
    property bool savePending: false
    property int testTicket: 0

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
    readonly property var quickConnect: CatalogService.quickConnect
    readonly property string quickState: quickConnect.state
    // a library that signs in with a code has no password to ask for, until its sign-in expires
    readonly property bool codeLibrary: editing && library?.auth === "quickConnect"
    readonly property bool signInExpired: editing && library?.errorCode === "signInExpired"
    readonly property bool signedIn: codeLibrary && !signInExpired
    readonly property bool quickOffered: remote && (editing ? (library?.capabilities ?? []).indexOf("quickConnect") >= 0 && urlField.text.trim() === (library?.url ?? "") : CatalogService.quickConnectKinds.indexOf(kind) >= 0)
    readonly property bool showMethods: quickOffered && !signedIn
    readonly property bool quickMode: showMethods && method === "quickConnect"
    readonly property bool passwordForm: remote && !signedIn && !quickMode
    // a server login is tested before it is saved, unless the saved password is kept
    readonly property bool needsTest: remote && !signedIn && testState !== "ok" && !(editing && passwordField.text === "" && !noPassword.checked)
    readonly property bool quickBusy: quickMode && ["requesting", "waiting", "approved"].indexOf(quickState) >= 0
    readonly property bool valid: blocker === "" && !quickMode
    readonly property string blocker: {
        if (remote && signedIn)
            return "";
        if (remote) {
            if (editing && codeLibrary && passwordField.text === "" && !noPassword.checked)
                return I18n.tr("Enter the password.", "edit server dialog, why save is disabled");
            if (urlField.text.trim() === "" || userField.text.trim() === "")
                return I18n.tr("Enter the server address and user name.", "add server dialog, why add is disabled");
            if (editing && urlField.text.trim() !== (library?.url ?? "") && passwordField.text === "" && !noPassword.checked)
                return I18n.tr("Enter the password again for a new server address.", "edit server dialog, why save is disabled");
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
        noPassword.checked = false;
        method = "password";
        CatalogService.cancelQuickConnect();
        folders = [];
        invalidateTest();
        testMessage = "";
        typingPath = false;
    }

    function add() {
        reset();
        editingId = "";
        kind = "local";
        touched = false;
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
        touched = false;
        opened = true;
        if (signInExpired && codeLibrary && quickOffered)
            selectMethod("quickConnect");
        nameField.forceActiveFocus();
    }

    function startQuickConnect() {
        if (urlField.text.trim() === "")
            return;
        CatalogService.startQuickConnect(editing ? editingId : "", kind, nameField.text.trim(), urlField.text.trim());
    }

    function selectMethod(next) {
        method = next;
        invalidateTest();
        if (next === "quickConnect")
            startQuickConnect();
        else
            CatalogService.cancelQuickConnect();
    }

    function addFolder(typed) {
        const path = Paths.expandTilde(typed.trim()).replace(/(.)\/+$/, "$1");
        if (path === "" || folders.indexOf(path) >= 0)
            return;
        folders = folders.concat([path]);
        touched = true;
        folderField.text = "";
    }

    // A field edit or closing makes a running test's reply stale.
    function invalidateTest() {
        testTicket++;
        testState = "";
        savePending = false;
    }

    function runTest() {
        testState = "testing";
        PlayerService.testServer(kind, urlField.text.trim(), userField.text.trim(), passwordField.text, ++testTicket);
    }

    function save() {
        touched = true;
        if (!valid || testState === "testing")
            return;
        if (needsTest) {
            savePending = true;
            runTest();
            return;
        }
        commit();
    }

    function commit() {
        const name = nameField.text.trim();
        if (editing) {
            const patch = remote && signedIn ? {
                "name": name
            } : remote ? {
                "name": name,
                "url": urlField.text.trim(),
                "user": userField.text.trim(),
                "password": noPassword.checked ? "" : passwordField.text || undefined,
                // back to a password, from a library that signed in with a code
                "auth": codeLibrary ? "" : undefined
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
    acceptEnabled: valid && testState !== "testing"
    onAccepted: save()
    onRejected: {
        passwordField.text = "";
        opened = false;
    }
    onOpenedChanged: {
        if (!opened) {
            invalidateTest();
            CatalogService.cancelQuickConnect();
        }
    }
    Component.onDestruction: CatalogService.cancelQuickConnect()
    onQuickStateChanged: {
        if (opened && quickMode && quickState === "done")
            opened = false;
    }

    Connections {
        target: PlayerService
        function onServerTestFinished(ok, message, ticket) {
            if (ticket !== root.testTicket || root.testState !== "testing")
                return;
            root.testState = ok ? "ok" : "failed";
            root.testMessage = message;
            const pending = root.savePending;
            root.savePending = false;
            if (ok && pending)
                root.commit();
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
                root.invalidateTest();
                root.method = "password";
                CatalogService.cancelQuickConnect();
            }
        }
    }

    StyledText {
        width: parent.width
        visible: root.signInExpired
        text: Phrases.reason("signInExpired")
        font.pixelSize: Theme.fontSizeMedium
        color: Theme.error
        wrapMode: Text.WordWrap
    }

    DTextField {
        id: nameField
        width: parent.width
        enabled: !root.quickBusy
        outlined: true
        labelText: root.remote ? I18n.tr("Name (optional)", "library dialog field") : I18n.tr("Name", "a name: field label, sort key")
        placeholderText: root.remote ? I18n.tr("Uses the server address", "library dialog name placeholder for a server") : ""
        onTextChanged: root.touched = true
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
                lastInGroup: index === root.folders.length - 1

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

        Row {
            topPadding: root.folders.length > 0 ? Theme.spacingS : 0
            spacing: Theme.spacingS

            DButton {
                text: I18n.tr("Add folder", "library dialog folder action")
                iconName: "add"
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                onClicked: folderPicker.open()
            }

            DButton {
                visible: !root.typingPath
                text: I18n.tr("Enter a path", "library dialog action that shows a folder path field")
                backgroundColor: "transparent"
                textColor: Theme.primary
                onClicked: {
                    root.typingPath = true;
                    folderField.forceActiveFocus();
                }
            }
        }

        DTextField {
            id: folderField
            visible: root.typingPath
            width: parent.width
            outlined: true
            labelText: I18n.tr("Folder", "library dialog folder field")
            placeholderText: I18n.tr("~/Music", "example folder path in the library dialog")
            supportingText: I18n.tr("Press Enter to add the folder.", "library dialog folder field hint")
            leftIconName: "create_new_folder"
            onAccepted: root.addFolder(text)
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
        enabled: !root.quickBusy && !root.signedIn
        onTextChanged: {
            root.invalidateTest();
            root.touched = true;
            if (["off", "failed", "expired"].indexOf(root.quickState) >= 0)
                CatalogService.cancelQuickConnect();
        }
    }

    DButtonGroup {
        visible: root.remote && root.showMethods
        model: [
            {
                "text": I18n.tr("Password", "add server dialog field"),
                "icon": "key"
            },
            {
                "text": I18n.tr("Quick Connect", "add server dialog sign-in method"),
                "icon": "qr_code"
            }
        ]
        currentIndex: root.method === "quickConnect" ? 1 : 0
        onSelectionChanged: (index, selected) => {
            if (selected && root.method !== (index === 1 ? "quickConnect" : "password"))
                root.selectMethod(index === 1 ? "quickConnect" : "password");
        }
    }

    SettingsGroup {
        visible: root.quickMode
        width: parent.width

        SettingsRow {
            visible: root.quickState === "requesting" || root.quickState === "approved"
            title: root.quickState === "approved" ? I18n.tr("Approved. Signing in…", "Quick Connect state") : I18n.tr("Getting a code…", "Quick Connect state")
            searchable: false

            DLoadingIndicator {
                anchors.verticalCenter: parent.verticalCenter
                size: Theme.iconSizeMedium
            }
        }

        SettingsRow {
            visible: root.quickState === "" && urlField.text.trim() === ""
            title: I18n.tr("Enter the server address.", "Quick Connect state before a code can be asked for")
            iconName: "dns"
            searchable: false
        }

        SettingsRow {
            visible: root.quickState === "waiting"
            title: root.quickConnect.code
            titleColor: Theme.primary
            subtitle: I18n.tr("On a signed-in device, open Settings, then Quick Connect, and enter this code.", "Quick Connect instructions")
            searchable: false

            DActionButton {
                anchors.verticalCenter: parent.verticalCenter
                iconName: "content_copy"
                tooltipText: I18n.tr("Copy", "copy a config snippet")
                Accessible.name: tooltipText
                onClicked: {
                    Quickshell.clipboardText = root.quickConnect.code;
                    Router.notify(I18n.tr("Copied", "toast after text was copied to the clipboard"));
                }
            }
        }

        SettingsRow {
            visible: root.quickState === "waiting"
            title: I18n.tr("Waiting for approval", "Quick Connect state")
            subtitle: I18n.tr("The code expires after 10 minutes.", "Quick Connect hint")
            searchable: false

            DLoadingIndicator {
                anchors.verticalCenter: parent.verticalCenter
                size: Theme.iconSizeMedium
            }
        }

        SettingsRow {
            visible: ["failed", "expired", "off"].indexOf(root.quickState) >= 0
            title: root.quickState === "expired" ? I18n.tr("The code expired", "Quick Connect state") : root.quickState === "off" ? Phrases.reason("quickConnectOff") : I18n.tr("Couldn't sign in with Quick Connect", "Quick Connect state")
            subtitle: root.quickState === "expired" ? I18n.tr("It wasn't approved in time.", "Quick Connect state") : root.quickState === "off" ? I18n.tr("Sign in with a password instead.", "Quick Connect state") : Phrases.reason(root.quickConnect.errorCode, "")
            iconName: "error"
            iconColor: Theme.error
            searchable: false
        }
    }

    DButton {
        visible: root.quickMode && (root.quickState === "failed" || root.quickState === "expired" || (root.quickState === "" && urlField.text.trim() !== ""))
        text: root.quickState === "expired" ? I18n.tr("Get a new code", "Quick Connect action") : root.quickState === "" ? I18n.tr("Get a code", "Quick Connect action") : I18n.tr("Try again", "library action that checks an unavailable library again")
        backgroundColor: Theme.secondaryContainer
        textColor: Theme.onSecondaryContainer
        enabled: urlField.text.trim() !== ""
        onClicked: root.startQuickConnect()
    }

    DTextField {
        id: userField
        visible: root.passwordForm
        width: parent.width
        outlined: true
        labelText: I18n.tr("User name", "add server dialog field")
        leftIconName: "person"
        onTextChanged: {
            root.invalidateTest();
            root.touched = true;
        }
    }

    DTextField {
        id: passwordField
        visible: root.passwordForm
        width: parent.width
        outlined: true
        labelText: I18n.tr("Password", "add server dialog field")
        supportingText: root.editing ? noPassword.checked ? "" : I18n.tr("Leave empty to keep the saved password. Stored in the system keyring.", "edit server dialog password hint") : I18n.tr("Leave empty if the account has none. Stored in the system keyring.", "add server dialog password hint")
        enabled: !noPassword.checked
        echoMode: TextInput.Password
        showPasswordToggle: true
        leftIconName: "key"
        onTextChanged: {
            root.invalidateTest();
            root.touched = true;
        }
    }

    DToggle {
        id: noPassword
        visible: root.passwordForm && root.editing
        width: parent.width
        text: I18n.tr("Account has no password", "edit server dialog toggle")
        onToggled: value => {
            checked = value;
            passwordField.text = "";
            root.invalidateTest();
            root.touched = true;
        }
    }

    Row {
        visible: root.passwordForm
        spacing: Theme.spacingM

        DButton {
            text: I18n.tr("Test connection", "add server dialog action")
            backgroundColor: Theme.secondaryContainer
            textColor: Theme.onSecondaryContainer
            busy: root.testState === "testing"
            enabled: root.testState !== "testing" && urlField.text.trim() !== ""
            onClicked: root.runTest()
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
        visible: root.passwordForm && root.testState === "failed" && root.testMessage !== ""
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
            description: I18n.tr("Stores ratings and favorites in the tags of the music files.", "local library setting description")
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
        visible: root.touched && root.blocker !== "" && !root.quickMode
        text: root.blocker
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.surfaceVariantText
        wrapMode: Text.WordWrap
    }

    FileBrowserModal {
        id: folderPicker
        parentModal: root.QsWindow.window
        browserTitle: I18n.tr("Choose folder", "library dialog folder action")
        bucket: "library"
        folderMode: true
        onFileSelected: path => root.addFolder(path)
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
            visible: !root.quickMode
            enabled: root.valid && root.testState !== "testing"
            busy: root.savePending
            onClicked: root.save()
        }
    ]
}
