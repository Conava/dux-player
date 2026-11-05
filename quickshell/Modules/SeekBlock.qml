pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import "../Common/Format.js" as Format

// A seek zone in Bento's look: the expressive seekbar across, elapsed and the length below it at either end, then
// whatever else the zone holds. A stream without a length keeps the space, so nothing around it moves.
Column {
    id: root

    property var instances: []
    property string context: "view"

    readonly property var types: instances.filter(i => !i.hidden).map(i => i.type)
    readonly property bool hasLength: PlayerService.duration > 0
    readonly property bool populated: types.length > 0

    visible: populated
    spacing: Theme.spacingXS
    LayoutMirroring.enabled: false
    LayoutMirroring.childrenInherit: true

    MediaSeekbar {
        id: seekbar
        width: parent.width
        visible: root.types.indexOf("seekbar") >= 0
        expressive: true
    }

    Item {
        width: parent.width
        height: elapsed.implicitHeight
        visible: root.types.indexOf("elapsed") >= 0 || root.types.indexOf("duration") >= 0
        opacity: root.hasLength ? 1 : 0

        NumericText {
            id: elapsed
            visible: root.types.indexOf("elapsed") >= 0
            width: Math.ceil(reservedWidth)
            text: Format.duration(seekbar.value * PlayerService.duration)
            reserveText: duration.text.replace(/\d/g, "8")
            isMonospace: false
            color: ArtColors.textVariant
            font.pixelSize: Theme.fontSizeSmall
            font.weight: Theme.fontWeightMedium

            PositionHold {
                active: elapsed.visible && !seekbar.visible && elapsed.Window.visibility !== Window.Minimized
            }
        }

        NumericText {
            id: duration
            anchors.right: parent.right
            visible: root.types.indexOf("duration") >= 0
            width: Math.ceil(reservedWidth)
            text: Format.duration(PlayerService.duration)
            reserveText: text.replace(/\d/g, "8")
            isMonospace: false
            horizontalAlignment: Text.AlignRight
            color: ArtColors.textVariant
            font.pixelSize: Theme.fontSizeSmall
            font.weight: Theme.fontWeightMedium
        }
    }

    NowPlayingZone {
        anchors.horizontalCenter: parent.horizontalCenter
        instances: root.instances.filter(i => ["elapsed", "seekbar", "duration"].indexOf(i.type) < 0)
        context: root.context
        room: parent.width
    }
}
