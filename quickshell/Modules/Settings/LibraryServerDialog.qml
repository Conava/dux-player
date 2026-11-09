import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets

// Changes a server library's address or signs in to it again.
WindowDialog {
    id: root

    property string libraryId: ""
    // the reason Save is disabled shows only after the first edit or a submit attempt
    property bool touched: false
    readonly property string quickState: signIn.quickState

    function open(id) {
        const library = PlayerService.library(id);
        libraryId = id;
        signIn.reset(library?.url ?? "", library?.user ?? "");
        touched = false;
        opened = true;
        signIn.focusFirst();
    }

    function save() {
        touched = true;
        signIn.submit();
    }

    maximumHeight: parent ? parent.height - Theme.spacingXL * 2 : Infinity
    title: I18n.tr("Server", "details field: the server address")
    acceptEnabled: signIn.valid && signIn.testState !== "testing"
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

    ServerSignIn {
        id: signIn
        spacing: root.contentSpacing
        libraryId: root.libraryId
        kind: PlayerService.library(root.libraryId)?.kind ?? ""
        onEdited: root.touched = true
        onReady: {
            PlayerService.updateLibrary(root.libraryId, patch());
            clear();
            root.opened = false;
        }
    }

    StyledText {
        width: parent.width
        visible: root.touched && signIn.blocker !== "" && !signIn.quickMode
        text: signIn.blocker
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
            text: I18n.tr("Save", "dialog confirm button")
            visible: !signIn.quickMode
            enabled: signIn.valid && signIn.testState !== "testing"
            busy: signIn.savePending
            onClicked: root.save()
        }
    ]
}
