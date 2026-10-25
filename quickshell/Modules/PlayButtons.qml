import QtQuick
import qs.Common
import qs.DCommon.Widgets

// Play and Shuffle for everything on a page; extra buttons go after them. Not a Row itself: a Row's
// topPadding would push extras centered on it below the buttons' center.
Item {
    id: root

    property real buttonHeight: Theme.buttonHeightM
    property real topPadding: 0
    default property alias extras: row.data

    signal play(bool shuffle)

    implicitWidth: row.width
    implicitHeight: row.height + topPadding

    Row {
        id: row
        y: root.topPadding
        spacing: Theme.spacingS

        DButton {
            text: I18n.tr("Play", "button that starts playback")
            iconName: "play_arrow"
            buttonHeight: root.buttonHeight
            onClicked: root.play(false)
        }

        DButton {
            text: I18n.tr("Shuffle", "button or mode that plays in random order")
            iconName: "shuffle"
            buttonHeight: root.buttonHeight
            backgroundColor: Theme.secondaryContainer
            textColor: Theme.onSecondaryContainer
            onClicked: root.play(true)
        }
    }
}
