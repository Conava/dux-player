import QtQuick
import qs.Common
import qs.DCommon.Widgets

// Asks before a destructive action: Cancel, and the action in the error color.
WindowDialog {
    id: root

    property string confirmText: ""

    signal confirmed

    function ask() {
        opened = true;
    }

    onRejected: opened = false
    onAccepted: {
        opened = false;
        confirmed();
    }

    actions: [
        DButton {
            text: I18n.tr("Cancel", "dialog dismiss button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: root.rejected()
        },
        DButton {
            text: root.confirmText
            backgroundColor: Theme.errorContainer
            textColor: Theme.onErrorContainer
            onClicked: root.accepted()
        }
    ]
}
