import QtQuick
import qs.Common
import qs.DCommon.Widgets

// The top of a detail page: the cover, then an eyebrow, the title and the page's own lines below them.
Row {
    id: root

    property string eyebrow: ""
    property string title: ""
    // the art; it fills a square of coverSize
    property alias cover: coverBox.data
    default property alias details: detailColumn.data
    readonly property real coverSize: Math.min(PlayerMetrics.detailArtSize, width / 3)

    width: parent?.width ?? 0
    spacing: Theme.spacingXL

    Item {
        id: coverBox
        width: root.coverSize
        height: width
    }

    Column {
        id: detailColumn
        width: root.width - coverBox.width - root.spacing
        anchors.verticalCenter: coverBox.verticalCenter
        spacing: Theme.spacingS

        StyledText {
            text: root.eyebrow
            font.pixelSize: Theme.fontSizeSmall
            font.weight: Theme.fontWeightMedium
            color: Theme.primary
        }

        StyledText {
            width: parent.width
            text: root.title
            font.pixelSize: Theme.fontSizeDisplay
            font.weight: Theme.fontWeightMedium
            color: Theme.surfaceText
            wrapMode: Text.WordWrap
            horizontalAlignment: Text.AlignLeft
        }
    }
}
