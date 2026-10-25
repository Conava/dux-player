import QtQuick
import qs.Common
import qs.DCommon.Widgets

// Asks for one line of text: a name for a new or renamed item.
WindowDialog {
    id: root

    property string label: ""
    property string confirmText: I18n.tr("Save", "dialog confirm button")
    property var callback: null
    readonly property bool valid: field.text.trim() !== ""

    function ask(dialogTitle, initial, confirm, done, fieldLabel) {
        title = dialogTitle;
        label = fieldLabel || I18n.tr("Name", "a name: field label, sort key");
        confirmText = confirm;
        field.text = initial;
        root.callback = done;
        opened = true;
        field.forceActiveFocus();
        field.selectAll();
    }

    function finish() {
        if (!valid)
            return;
        opened = false;
        callback?.(field.text.trim());
    }

    acceptEnabled: valid
    onAccepted: finish()
    onRejected: opened = false

    DTextField {
        id: field
        width: parent.width
        outlined: true
        labelText: root.label
        onAccepted: root.finish()
    }

    actions: [
        DButton {
            text: I18n.tr("Cancel", "dialog dismiss button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: root.opened = false
        },
        DButton {
            text: root.confirmText
            enabled: root.valid
            onClicked: root.finish()
        }
    ]
}
