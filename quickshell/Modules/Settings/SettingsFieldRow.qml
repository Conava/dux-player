import QtQuick
import qs.Common
import qs.DCommon.Widgets

// A form field as a contained row: the row title labels it, the hint goes in the subtitle.
SettingsRow {
    id: root

    readonly property alias field: input

    signal accepted

    body: DTextField {
        id: input
        width: parent.width
        outlined: true
        onAccepted: root.accepted()
    }
}
