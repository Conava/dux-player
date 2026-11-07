import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// A clickable column header for a sortable list. The cell is the click target; the active sort shows in the label's
// color and its arrow.
StyledButton {
    id: root

    property string label: ""
    property bool active: false
    property bool descending: false
    property int align: Text.AlignLeft
    readonly property color labelColor: active ? Style.primary : hovered ? Style.onSurface : Style.surfaceVariantText

    signal sortRequested

    implicitHeight: Style.buttonHeightXXS
    implicitWidth: content.implicitWidth
    radius: Style.cornerRadiusS
    color: "transparent"
    Accessible.role: Accessible.ColumnHeader
    Accessible.name: label
    Accessible.description: active ? (descending ? I18n.tr("Sorted descending", "State of a sortable column header") : I18n.tr("Sorted ascending", "State of a sortable column header")) : ""
    onClicked: sortRequested()

    FocusRing {
        visible: root.visualFocus
    }

    Row {
        id: content
        anchors.verticalCenter: parent.verticalCenter
        anchors.left: root.align === Text.AlignRight ? undefined : parent.left
        anchors.right: root.align === Text.AlignRight ? parent.right : undefined
        spacing: Style.spacingXS
        layoutDirection: root.align === Text.AlignRight ? Qt.RightToLeft : Qt.LeftToRight

        StyledText {
            anchors.verticalCenter: parent.verticalCenter
            text: root.label
            width: Math.min(implicitWidth, Math.max(0, root.width - Style.iconSizeSmall - Style.spacingXS))
            elide: Text.ElideRight
            font.pixelSize: Style.fontSizeSmall
            font.weight: Style.fontWeightMedium
            color: root.labelColor
        }

        // always takes its room, so the label doesn't move when the arrow appears
        DIcon {
            anchors.verticalCenter: parent.verticalCenter
            name: "arrow_upward"
            size: Style.iconSizeSmall
            color: root.labelColor
            opacity: root.active ? 1 : root.hovered || root.visualFocus ? Style.pendingOpacity : 0
            rotation: root.active && root.descending ? 180 : 0

            Behavior on opacity {
                NumberAnimation {
                    duration: Style.reduceMotion ? 0 : Style.shortDuration
                }
            }

            Behavior on rotation {
                NumberAnimation {
                    duration: Style.reduceMotion ? 0 : Style.shortDuration
                    easing.type: Style.standardEasing
                }
            }
        }
    }
}
