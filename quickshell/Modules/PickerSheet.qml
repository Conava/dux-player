pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.DCommon.Widgets

// A bottom sheet of grouped choices. sections: [{title, items: [{label, icon, value, done}]}]; "done" items
// are already in place and say so through doneFormat (%1 is the label).
DBottomSheet {
    id: root

    property var sections: []
    property string doneFormat: "%1"

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
            }

            Repeater {
                model: section.modelData.items

                DListItem {
                    id: row
                    required property var modelData
                    required property int index
                    width: section.width
                    enabled: !modelData.done
                    surfaceColor: SettingsMetrics.rowColor
                    firstInGroup: index === 0
                    lastInGroup: index === section.modelData.items.length - 1
                    Accessible.name: modelData.label
                    onClicked: {
                        root.picked(modelData.value);
                        root.opened = false;
                    }

                    DIcon {
                        id: icon
                        anchors.left: parent.left
                        anchors.leftMargin: Theme.spacingL
                        anchors.verticalCenter: parent.verticalCenter
                        name: row.modelData.icon ?? "add"
                        size: Theme.iconSize
                        color: row.contentColor
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
