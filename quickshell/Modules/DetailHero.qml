import QtQuick
import qs.Common
import qs.DCommon.Widgets

// The top of a detail page: the cover, then an eyebrow, the title and the page's own lines below them.
Item {
    id: root

    property string eyebrow: ""
    property string title: ""
    // the art; it fills a square of coverSize
    property alias cover: coverBox.data
    default property alias details: detailColumn.data
    readonly property real spacing: Theme.spacingXL
    // the text column needs this much beside the cover (the Shuffle button is the widest line); under it the text goes below
    readonly property real minTextWidth: 220
    readonly property real besideCover: width - Math.min(PlayerMetrics.detailArtSize, width / 3) - spacing
    readonly property bool stacked: besideCover < minTextWidth
    readonly property real coverSize: Math.min(PlayerMetrics.detailArtSize, stacked ? width : width / 3)

    width: parent?.width ?? 0
    implicitHeight: stacked ? coverBox.height + spacing + detailColumn.height : Math.max(coverBox.height, detailColumn.height)

    Item {
        id: coverBox
        anchors.left: parent.left
        width: root.coverSize
        height: width
    }

    Column {
        id: detailColumn
        anchors.left: root.stacked ? parent.left : coverBox.right
        anchors.leftMargin: root.stacked ? 0 : root.spacing
        width: root.stacked ? root.width : root.width - coverBox.width - root.spacing
        // centered on the cover while it fits; a taller column grows downwards, never past the page's top
        y: root.stacked ? coverBox.height + root.spacing : Math.max(0, (coverBox.height - height) / 2)
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
