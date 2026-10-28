import QtQuick
import qs.Common
import qs.Modules

Column {
    id: root

    property string title: ""
    // for the settings search index (scripts/settings_index.py): synonyms the title doesn't contain
    property string keywords: ""
    // why the card is hidden in this setup, for the settings search
    property string unavailableReason: ""
    readonly property bool isSettingsCard: true
    default property alias content: contentGroup.data
    property alias headerActions: sectionLabel.actions
    readonly property bool hasHeader: title !== ""

    width: parent?.width ?? 0
    spacing: 0

    SectionLabel {
        id: sectionLabel
        width: parent.width
        text: root.title
        visible: root.hasHeader
    }

    SettingsGroup {
        id: contentGroup
        width: parent.width
    }
}
