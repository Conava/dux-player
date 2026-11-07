import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// A clickable column header for a sortable list. The cell is the click target; the active sort and the hover
// show as a pill around the label and its arrow.
StyledButton {
    id: root

    property string label: ""
    property bool active: false
    property bool descending: false
    property int align: Text.AlignLeft
    // how far the pill reaches past the label, so the label stays in line with the rows below
    property real highlightPadding: Style.spacingS

    signal sortRequested

    implicitHeight: Style.buttonHeightXXS
    // the pill hugs the label, but never reaches past the cell's own width
    implicitWidth: content.implicitWidth + highlightPadding * 2
    radius: Style.cornerRadiusS
    color: "transparent"
    Accessible.role: Accessible.ColumnHeader
    Accessible.name: label
    Accessible.description: active ? (descending ? I18n.tr("Sorted descending", "State of a sortable column header") : I18n.tr("Sorted ascending", "State of a sortable column header")) : ""
    onClicked: sortRequested()

    Rectangle {
        id: pill
        readonly property bool rightAligned: root.align === Text.AlignRight
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        anchors.left: rightAligned ? undefined : parent.left
        anchors.right: rightAligned ? parent.right : undefined
        anchors.leftMargin: -root.highlightPadding
        anchors.rightMargin: -root.highlightPadding
        width: Math.min(content.implicitWidth + root.highlightPadding * 2, root.width + root.highlightPadding * 2)
        radius: Style.fullRadius(width, height)
        color: root.active ? Style.secondaryContainer : "transparent"

        Behavior on color {
            ColorAnimation {
                duration: Style.reduceMotion ? 0 : Style.shortDuration
            }
        }

        StateLayer {
            control: root
            cornerRadius: pill.radius
            stateColor: root.active ? Style.onSecondaryContainer : Style.onSurface
        }

        FocusRing {
            visible: root.visualFocus
        }
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
            color: root.active ? Style.onSecondaryContainer : Style.surfaceVariantText
        }

        // always takes its room, so the pill doesn't change width when the arrow appears
        DIcon {
            anchors.verticalCenter: parent.verticalCenter
            name: "arrow_upward"
            size: Style.iconSizeSmall
            color: root.active ? Style.onSecondaryContainer : Style.surfaceVariantText
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
