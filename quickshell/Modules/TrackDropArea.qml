import QtQuick
import qs.Common

// Takes tracks dragged from a track list; outlined while a drag hovers it.
DropArea {
    id: root

    property real radius: Theme.cornerRadiusL

    anchors.fill: parent
    keys: ["dplayer/tracks"]

    Rectangle {
        anchors.fill: parent
        radius: root.radius
        color: "transparent"
        border.width: Theme.outlineWidthFocused
        border.color: Theme.primary
        visible: root.containsDrag
    }
}
