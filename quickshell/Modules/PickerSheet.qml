pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.DCommon.Widgets

// A bottom sheet of grouped choices. sections: [{title, items: [{label, icon, value, done}]}]; "done" items
// are already in place and say so through doneFormat (%1 is the label). With multiple set, rows are checkboxes
// over `chosen` and the sheet stays open.
DBottomSheet {
    id: root

    property var sections: []
    property string doneFormat: "%1"
    property bool multiple: false
    property var chosen: []

    signal picked(var value)

    z: Theme.zSheet
    onDismissRequested: opened = false

    readonly property EscapeLayer _escapeLayer: EscapeLayer {
        owner: root
        active: root.opened
        handlesKey: true
    }

    Repeater {
        model: root.sections

        Column {
            id: section
            required property var modelData
            width: parent.width
            spacing: Theme.groupedListGap
            visible: modelData.items.length > 0

            StyledText {
                width: parent.width
                topPadding: Theme.spacingS
                bottomPadding: Theme.spacingS
                text: section.modelData.title
                font.pixelSize: Theme.fontSizeMedium
                font.weight: Theme.fontWeightMedium
                color: Theme.primary
                horizontalAlignment: Text.AlignLeft
            }

            Repeater {
                model: section.modelData.items

                DListItem {
                    id: row
                    required property var modelData
                    required property int index
                    readonly property bool ticked: root.chosen.indexOf(modelData.value) >= 0
                    width: section.width
                    enabled: !modelData.done
                    surfaceColor: SettingsMetrics.rowColor
                    firstInGroup: index === 0
                    lastInGroup: index === section.modelData.items.length - 1
                    Accessible.name: modelData.label
                    Accessible.role: root.multiple ? Accessible.CheckBox : Accessible.ListItem
                    Accessible.checkable: root.multiple
                    Accessible.checked: ticked
                    onClicked: {
                        root.picked(modelData.value);
                        if (!root.multiple)
                            root.opened = false;
                    }

                    DIcon {
                        id: icon
                        anchors.left: parent.left
                        anchors.leftMargin: Theme.spacingL
                        anchors.verticalCenter: parent.verticalCenter
                        name: root.multiple ? row.ticked ? "check_box" : "check_box_outline_blank" : row.modelData.icon ?? "add"
                        filled: row.ticked
                        size: Theme.iconSize
                        color: row.ticked ? Theme.primary : row.contentColor
                    }

                    StyledText {
                        anchors.left: icon.right
                        anchors.leftMargin: Theme.spacingL
                        anchors.right: parent.right
                        anchors.rightMargin: Theme.spacingL
                        anchors.verticalCenter: parent.verticalCenter
                        text: row.modelData.done ? root.doneFormat.arg(row.modelData.label) : row.modelData.label
                        font.pixelSize: Theme.fontSizeMedium
                        color: row.contentColor
                        elide: Text.ElideRight
                        wrapMode: Text.NoWrap
                        horizontalAlignment: Text.AlignLeft
                    }
                }
            }
        }
    }
}
