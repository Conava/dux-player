pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets
import qs.DCommon.FileBrowser
import Quickshell

// Adds a library: folders, or one account on an (Open)Subsonic or Jellyfin server.
WindowDialog {
    id: root

    property string kind: "local"
    property var folders: []
    // the reason Add is disabled shows only after the first edit or a submit attempt
    property bool touched: false
    property bool typingPath: false

    readonly property bool remote: kind !== "local"
    // the kinds the daemon can add, in the order offered
    readonly property var kinds: ["local", "subsonic", "jellyfin"].filter(k => PlayerService.libraryKinds.indexOf(k) >= 0)
    readonly property string quickState: signIn.quickState
    readonly property bool valid: blocker === "" && !(remote && signIn.quickMode)
    readonly property string blocker: {
        if (remote)
            return signIn.blocker;
        if (nameField.text.trim() === "")
            return I18n.tr("Enter a name.", "add local library dialog, why add is disabled");
        if (folders.length === 0)
            return I18n.tr("Add at least one folder.", "add local library dialog, why add is disabled");
        return "";
    }

    function add() {
        nameField.text = "";
        folderField.text = "";
        folders = [];
        typingPath = false;
        kind = "local";
        signIn.reset("", "");
        touched = false;
        opened = true;
        nameField.forceActiveFocus();
    }

    function addFolder(typed) {
        const path = Paths.expandTilde(typed.trim()).replace(/(.)\/+$/, "$1");
        if (path === "" || folders.indexOf(path) >= 0)
            return;
        folders = folders.concat([path]);
        touched = true;
        folderField.text = "";
    }

    function save() {
        touched = true;
        if (!valid)
            return;
        if (remote) {
            signIn.submit();
            return;
        }
        commit();
    }

    function commit() {
        const name = nameField.text.trim();
        if (remote)
            PlayerService.addServerLibrary(kind, name, signIn.url, signIn.user, signIn.password);
        else
            PlayerService.addLocalLibrary(name, folders);
        signIn.clear();
        opened = false;
    }

    maximumHeight: parent ? parent.height - Theme.spacingXL * 2 : Infinity
    title: I18n.tr("Add library", "libraries settings primary action")
    acceptEnabled: valid && signIn.testState !== "testing"
    onAccepted: save()
    onRejected: opened = false
    onOpenedChanged: {
        if (!opened)
            signIn.clear();
    }
    onQuickStateChanged: {
        if (opened && signIn.quickMode && quickState === "done")
            opened = false;
    }

    DButtonGroup {
        model: root.kinds.map(k => ({
                    "text": PlayerService.kindLabel(k),
                    "icon": k === "local" ? "folder" : "cloud"
                }))
        currentIndex: root.kinds.indexOf(root.kind)
        onSelectionChanged: (index, selected) => {
            if (selected)
                root.kind = root.kinds[index];
        }
    }

    DTextField {
        id: nameField
        width: parent.width
        enabled: !signIn.quickBusy
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

    ServerSignIn {
        id: signIn
        visible: root.remote
        spacing: root.contentSpacing
        kind: root.kind
        name: nameField.text.trim()
        onEdited: root.touched = true
        onReady: root.commit()
    }

    StyledText {
        width: parent.width
        visible: root.touched && root.blocker !== "" && !(root.remote && signIn.quickMode)
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
            text: I18n.tr("Add", "dialog confirm button")
            visible: !(root.remote && signIn.quickMode)
            enabled: root.valid && signIn.testState !== "testing"
            busy: signIn.savePending
            onClicked: root.save()
        }
    ]
}
