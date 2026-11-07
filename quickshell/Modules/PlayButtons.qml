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

    // the buttons in one line; the Flow wraps them when the parent is narrower
    implicitWidth: {
        let w = 0, n = 0;
        for (const c of row.children) {
            if (!c.visible)
                continue;
            w += c.implicitWidth;
            n++;
        }
        return w + Math.max(0, n - 1) * row.spacing;
    }
    implicitHeight: row.height + topPadding
    width: Math.min(implicitWidth, parent?.width ?? implicitWidth)

    Flow {
        id: row
        y: root.topPadding
        width: root.width
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
