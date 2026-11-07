import QtQuick
import qs.Common
import qs.DCommon.Widgets

// Asks for the name and type of a custom page.
WindowDialog {
    id: root

    readonly property var types: ["tracks", "albums", "artists"]
    readonly property bool valid: nameField.text.trim() !== ""
    readonly property alias field: nameField

    signal created(string name, string type)

    function show() {
        nameField.text = "";
        typeGroup.currentIndex = 0;
        opened = true;
        nameField.forceActiveFocus();
    }

    function create() {
        if (!valid)
            return;
        opened = false;
        created(nameField.text.trim(), types[typeGroup.currentIndex]);
    }

    title: I18n.tr("New custom page", "action and dialog that create a custom page")
    acceptEnabled: valid
    onAccepted: create()
    onRejected: opened = false

    DTextField {
        id: nameField
        width: parent.width
        outlined: true
        labelText: I18n.tr("Name", "a name: field label, sort key")
        onAccepted: root.create()
    }

    StyledText {
        width: parent.width
        text: I18n.tr("Type", "custom page dialog: what the page lists")
        font.pixelSize: Theme.fontSizeMedium
        font.weight: Theme.fontWeightMedium
        color: Theme.surfaceText
    }

    DButtonGroup {
        id: typeGroup
        model: root.types.map(t => ({
                    "text": UiConfig.typeLabel(t)
                }))
        currentIndex: 0
        onSelectionChanged: (index, selected) => {
            if (selected)
                currentIndex = index;
        }
    }

    actions: [
        DButton {
            text: I18n.tr("Cancel", "dialog dismiss button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: root.opened = false
        },
        DButton {
            text: I18n.tr("Create", "new playlist dialog confirm button")
            enabled: root.valid
            onClicked: root.create()
        }
    ]
}
