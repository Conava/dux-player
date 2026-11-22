pragma ComponentBehavior: Bound

import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// A column of letters that jumps to the first item of a letter in a list sorted by name. `model` is a list of
// {letter, first}; letters without items (first < 0) are dimmed and do nothing. When the height can't hold every
// letter, an even subset shows with a dot between groups; a dot picks the nearest letter that has items.
// The buttons take no focus: the list stays the keyboard target. The letter of the row at `position` is bold and
// zoomed; letters near the pointer grow like a dock's icons.
Item {
    id: root
    objectName: "letterIndex"

    property var model: []
    // the list row at the top of the view
    property int position: -1

    signal picked(int first)

    readonly property real itemWidth: Style.buttonHeightXS
    readonly property real itemHeight: Style.iconSize
    readonly property real itemGap: Style.spacingXXS
    readonly property int slots: Math.max(1, Math.floor((height + itemGap) / (itemHeight + itemGap)))
    readonly property var entries: layout(model, slots)
    readonly property bool animates: !Style.reduceMotion && Style.currentAnimationSpeed !== Style.AnimationSpeed.None
    // the entry whose letter holds `position`: the last one starting at or before it
    readonly property int currentAt: {
        let found = -1;
        for (let i = 0; i < model.length; i++) {
            const first = model[i].first;
            if (first < 0)
                continue;
            if (first > position)
                break;
            found = i;
        }
        return found;
    }
    readonly property string currentLetter: model[currentAt]?.letter ?? ""
    readonly property real currentScale: 1.3
    readonly property real hoverScale: 0.5
    // how far from the pointer, in cells, a letter still grows
    readonly property real hoverReach: 2
    readonly property real pointerY: hover.point.position.y
    // eases the zoom in and out as the pointer arrives and leaves; while over the column it follows the pointer
    property real hoverAmount: hover.hovered ? 1 : 0
    Behavior on hoverAmount {
        enabled: root.animates
        NumberAnimation {
            duration: Style.expressiveDurations.expressiveFastSpatial
            easing.type: Easing.BezierSpline
            easing.bezierCurve: Style.expressiveCurves.expressiveFastSpatial
        }
    }

    implicitWidth: itemWidth
    implicitHeight: Math.max(0, model.length * (itemHeight + itemGap) - itemGap)

    // [{dot, at}]: every letter when they fit, else k letters spread over the model with a dot (at the middle of
    // the span it stands for) between neighbours.
    function layout(letters, count) {
        const total = letters.length;
        const out = [];
        if (total <= count) {
            for (let i = 0; i < total; i++)
                out.push({
                    dot: false,
                    at: i
                });
            return out;
        }
        const shown = Math.max(1, Math.floor((count + 1) / 2));
        let previous = -1;
        for (let i = 0; i < shown; i++) {
            const at = shown === 1 ? 0 : Math.round(i * (total - 1) / (shown - 1));
            if (previous >= 0)
                out.push({
                    dot: true,
                    at: Math.floor((previous + at) / 2)
                });
            out.push({
                dot: false,
                at: at
            });
            previous = at;
        }
        return out;
    }

    // the first item of the letter nearest `at` that has items; nothing when no letter does
    function pickNear(at) {
        for (let d = 0; d < model.length; d++) {
            for (const i of [at - d, at + d]) {
                if (i >= 0 && i < model.length && model[i].first >= 0) {
                    picked(model[i].first);
                    return;
                }
            }
        }
    }

    HoverHandler {
        id: hover
    }

    Column {
        id: column
        anchors.horizontalCenter: parent.horizontalCenter
        spacing: root.itemGap

        Repeater {
            model: root.entries

            StyledButton {
                id: cell
                required property var modelData
                readonly property var item: root.model[cell.modelData.at]
                readonly property bool dot: cell.modelData.dot
                readonly property bool current: !cell.dot && cell.modelData.at === root.currentAt
                readonly property real nearness: Math.max(0, 1 - Math.abs(root.pointerY - cell.y - cell.height / 2) / (cell.height * root.hoverReach))
                property real currentZoom: cell.current ? root.currentScale : 1
                Behavior on currentZoom {
                    enabled: root.animates
                    NumberAnimation {
                        duration: Style.expressiveDurations.expressiveFastSpatial
                        easing.type: Easing.BezierSpline
                        easing.bezierCurve: Style.expressiveCurves.expressiveFastSpatial
                    }
                }

                width: root.itemWidth
                height: root.itemHeight
                radius: Style.fullRadius(cell.width, cell.height)
                color: "transparent"
                focusPolicy: Qt.NoFocus
                enabled: cell.dot || (cell.item?.first ?? -1) >= 0
                Accessible.role: Accessible.Button
                Accessible.ignored: cell.dot
                Accessible.name: cell.dot ? "" : I18n.tr("Jump to %1", "letter index button, %1 is a letter").arg(cell.item?.letter ?? "")
                onClicked: dot ? root.pickNear(cell.modelData.at) : root.picked(cell.item.first)

                StateLayer {
                    control: cell
                    stateColor: Style.primary
                }

                DPressFeedback {
                    control: cell
                }

                StyledText {
                    visible: !cell.dot
                    anchors.centerIn: parent
                    text: cell.item?.letter ?? ""
                    font.pixelSize: Style.fontSizeSmall
                    font.weight: cell.current ? Style.fontWeightBold : Style.fontWeightMedium
                    color: !cell.enabled ? Style.onSurface_38 : cell.current ? Style.primary : Style.onSurfaceVariant
                    scale: Math.max(cell.currentZoom, 1 + root.hoverScale * root.hoverAmount * cell.nearness * (cell.enabled ? 1 : 0))
                }

                Rectangle {
                    visible: cell.dot
                    anchors.centerIn: parent
                    width: Style.spacingXS
                    height: width
                    radius: width / 2
                    color: Style.onSurface_38
                }
            }
        }
    }
}
