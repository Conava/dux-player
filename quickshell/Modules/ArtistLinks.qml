pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.DCommon.Widgets

// The artists of a track or album, each its own link to that artist's page, on one line. An artist without a
// page (empty id) shows as text. Too long for the width, the line elides as one: names fill it in order and the one at the edge ends in an ellipsis.
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
    // the least a name needs to be worth showing at the edge
    readonly property real minName: ellipsisMetrics.advanceWidth * 4

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
        id: ellipsisMetrics
        font: metrics.font
        text: "…"
    }

    TextMetrics {
        id: separatorMetrics
        font: metrics.font
        text: root.separator
    }

    FontMetrics {
        id: nameMetrics
        font: metrics.font
    }

    // Per artist: the width its name may use, and whether the name, the separator after it, or a lone
    // ellipsis (the rest is hidden) shows. Computed from text widths so it holds in a mirrored layout.
    readonly property var slots: {
        const out = [];
        const sep = separatorMetrics.advanceWidth;
        const ell = ellipsisMetrics.advanceWidth;
        let start = 0;
        let hidden = false;
        for (let i = 0; i < artists.length; i++) {
            const w = nameMetrics.advanceWidth(artists[i].name);
            const room = fits ? Number.POSITIVE_INFINITY : Math.max(0, width - start);
            const last = i === artists.length - 1;
            hidden = hidden || (i > 0 && room < minName);
            const sepShown = !hidden && !last && w + sep + minName <= room;
            const trailing = !hidden && !last && !sepShown;
            out.push({
                "shown": !hidden,
                "width": Math.min(w, room - (trailing ? ell : 0)),
                "separator": sepShown,
                "ellipsis": trailing && w <= room - ell
            });
            start += w + sep;
        }
        return out;
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
                readonly property var slot: root.slots[index] ?? {
                    "shown": true,
                    "width": 0,
                    "separator": false,
                    "ellipsis": false
                }

                visible: slot.shown

                LinkText {
                    width: Math.min(implicitWidth, entry.slot.width)
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
                    visible: entry.slot.separator || entry.slot.ellipsis
                    text: entry.slot.separator ? root.separator : "…"
                    font.pixelSize: root.fontSize
                    font.weight: root.fontWeight
                    color: root.color
                }
            }
        }
    }
}
