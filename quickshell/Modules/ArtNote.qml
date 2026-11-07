import QtQuick
import qs.Common
import qs.DCommon.Widgets

// A short note over the bottom of the art. Empty text hides it.
Rectangle {
    id: root

    property string text: ""

    visible: text !== ""
    width: Math.min(label.implicitWidth + Theme.spacingL * 2, parent?.width ?? 0)
    height: label.implicitHeight + Theme.spacingS * 2
    radius: Theme.fullRadius(width, height)
    // a theme surface with an edge, so it reads on any cover colour
    color: Theme.surfaceContainerHighest
    border.width: Theme.outlineWidth
    border.color: Theme.outline

    StyledText {
        id: label
        anchors.centerIn: parent
        width: Math.min(implicitWidth, root.width - Theme.spacingL * 2)
        text: root.text
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.surfaceText
        elide: Text.ElideRight
        wrapMode: Text.NoWrap
    }
}
