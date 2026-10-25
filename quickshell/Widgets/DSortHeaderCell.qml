import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// A clickable column header for a sortable list: the label, an arrow on the active column.
StyledButton {
    id: root

    property string label: ""
    property bool active: false
    property bool descending: false
    property int align: Text.AlignLeft

    signal sortRequested

    implicitHeight: Style.buttonHeightXXS
    radius: Style.cornerRadiusS
    color: "transparent"
    Accessible.role: Accessible.ColumnHeader
    Accessible.name: label
    Accessible.description: active ? (descending ? I18n.tr("Sorted descending", "State of a sortable column header") : I18n.tr("Sorted ascending", "State of a sortable column header")) : ""
    onClicked: sortRequested()

    StateLayer {
        control: root
        stateColor: Style.onSurface
    }

    FocusRing {
        visible: root.visualFocus
    }

    Row {
        anchors.verticalCenter: parent.verticalCenter
        anchors.left: root.align === Text.AlignRight ? undefined : parent.left
        anchors.right: root.align === Text.AlignRight ? parent.right : undefined
        spacing: Style.spacingXS
        layoutDirection: root.align === Text.AlignRight ? Qt.RightToLeft : Qt.LeftToRight

        StyledText {
            anchors.verticalCenter: parent.verticalCenter
            text: root.label
            font.pixelSize: Style.fontSizeSmall
            font.weight: root.active ? Style.fontWeightBold : Style.fontWeightMedium
            color: root.active ? Style.primary : Style.surfaceVariantText
        }

        DIcon {
            anchors.verticalCenter: parent.verticalCenter
            visible: root.active
            name: root.descending ? "arrow_downward" : "arrow_upward"
            size: Style.iconSizeSmall
            color: Style.primary
        }
    }
}
