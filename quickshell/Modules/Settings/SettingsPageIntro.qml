import QtQuick
import qs.Common
import qs.DCommon.Widgets

// The line under a settings page's title that says what the page is for.
StyledText {
    width: parent?.width ?? 0
    font.pixelSize: Theme.fontSizeMedium
    color: Theme.surfaceVariantText
    wrapMode: Text.WordWrap
    horizontalAlignment: Text.AlignLeft
}
