import QtQuick
import qs.Common
import qs.DCommon.Widgets

// What a page shows when it has nothing to list: an icon, a title, a line of text and one optional action.
Column {
    id: root

    property string iconName: ""
    property string title: ""
    property string text: ""
    property string actionText: ""
    property string actionIcon: ""

    signal actionClicked

    spacing: Theme.spacingM
    topPadding: Theme.spacingXL

    DIcon {
        visible: root.iconName !== ""
        name: root.iconName
        size: Theme.iconSizeLarge
        color: Theme.surfaceVariantText
    }

    StyledText {
        width: parent.width
        visible: root.title !== ""
        text: root.title
        font.pixelSize: Theme.fontSizeLarge
        color: Theme.surfaceText
        wrapMode: Text.WordWrap
    }

    StyledText {
        width: parent.width
        visible: root.text !== ""
        text: root.text
        font.pixelSize: Theme.fontSizeMedium
        color: Theme.surfaceVariantText
        wrapMode: Text.WordWrap
    }

    DButton {
        visible: root.actionText !== ""
        text: root.actionText
        iconName: root.actionIcon
        onClicked: root.actionClicked()
    }
}
