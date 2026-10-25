pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.DCommon.Widgets

// The artists of a track or album, each its own link to that artist's page, on one line. An artist without a
// page (empty id) shows as text. Too long for the width, each name gets an equal share and elides.
Item {
    id: root

    // [{id, name}]
    property var artists: []
    property real fontSize: Theme.fontSizeSmall
    property int fontWeight: Font.Normal
    property color color: Theme.surfaceVariantText
    property int horizontalAlignment: Text.AlignLeft

    readonly property string separator: I18n.tr(", ", "between artist names")
    readonly property bool fits: metrics.advanceWidth <= width
    readonly property real share: Math.max(0, width / Math.max(1, artists.length) - separatorMetrics.advanceWidth)

    // after a link was followed, e.g. to close the dialog it sits in
    signal activated

    implicitWidth: metrics.advanceWidth
    implicitHeight: metrics.height
    Accessible.role: Accessible.Grouping
    Accessible.name: artists.map(a => a.name).join(separator)

    // StyledText's font family, which TextMetrics alone wouldn't use
    StyledText {
        id: fontProbe
        visible: false
        font.pixelSize: root.fontSize
        font.weight: root.fontWeight
    }

    TextMetrics {
        id: metrics
        font: fontProbe.font
        text: root.Accessible.name
    }

    TextMetrics {
        id: separatorMetrics
        font: metrics.font
        text: root.separator
    }

    Row {
        anchors.left: root.horizontalAlignment === Text.AlignHCenter ? undefined : parent.left
        anchors.horizontalCenter: root.horizontalAlignment === Text.AlignHCenter ? parent.horizontalCenter : undefined
        anchors.verticalCenter: parent.verticalCenter

        Repeater {
            model: root.artists

            Row {
                id: entry
                required property var modelData
                required property int index

                LinkText {
                    width: root.fits ? implicitWidth : Math.min(implicitWidth, root.share)
                    text: entry.modelData.name
                    linked: entry.modelData.id !== ""
                    font.pixelSize: root.fontSize
                    font.weight: root.fontWeight
                    color: root.color
                    onActivated: {
                        Router.go("artist", entry.modelData.id);
                        root.activated();
                    }
                }

                StyledText {
                    visible: entry.index < root.artists.length - 1
                    text: root.separator
                    font.pixelSize: root.fontSize
                    font.weight: root.fontWeight
                    color: root.color
                }
            }
        }
    }
}
