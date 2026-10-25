pragma ComponentBehavior: Bound

import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// A slider without a handle: a pill whose fill is the value, an optional icon inside at the start and the value
// at the end. Click or drag anywhere sets it, the wheel and arrow keys step it.
Item {
    id: root

    property real value: 0
    property real minimum: 0
    property real maximum: 100
    property real step: 5
    property string iconName: ""
    // the icon is a button of its own (e.g. mute) instead of part of the track
    property bool iconClickable: false
    property bool showValue: false
    property string unit: ""
    property color fillColor: Style.primary
    property color trackColor: Style.secondaryContainer
    property color onFillColor: Style.onPrimary
    property color onTrackColor: Style.onSecondaryContainer
    readonly property bool dragging: mouse.pressed && !mouse.onIcon
    readonly property real ratio: maximum > minimum ? Math.max(0, Math.min(1, (value - minimum) / (maximum - minimum))) : 0
    readonly property real fillWidth: ratio > 0 ? Math.max(height, width * ratio) : 0
    readonly property real iconBox: iconName !== "" ? height : 0

    signal moved(real value)
    signal iconClicked

    function setFromX(x) {
        const inner = mirrored ? width - x : x;
        const raw = minimum + Math.max(0, Math.min(1, inner / width)) * (maximum - minimum);
        moved(Math.round(raw));
    }

    function stepBy(direction) {
        moved(Math.max(minimum, Math.min(maximum, Math.round(value) + direction * step)));
    }

    readonly property bool mirrored: LayoutMirroring.enabled

    implicitWidth: Style.buttonHeightS * 4
    implicitHeight: Style.buttonHeightXS
    activeFocusOnTab: true
    Accessible.role: Accessible.Slider
    Accessible.description: Math.round(value) + unit
    Keys.onPressed: event => {
        const forward = mirrored ? Qt.Key_Left : Qt.Key_Right;
        const back = mirrored ? Qt.Key_Right : Qt.Key_Left;
        if (event.key === forward || event.key === Qt.Key_Up)
            stepBy(1);
        else if (event.key === back || event.key === Qt.Key_Down)
            stepBy(-1);
        else
            return;
        event.accepted = true;
    }

    Rectangle {
        id: track
        anchors.fill: parent
        radius: Style.fullRadius(width, height)
        color: root.trackColor

        Rectangle {
            x: root.mirrored ? parent.width - width : 0
            width: root.fillWidth
            height: parent.height
            radius: parent.radius
            color: root.fillColor

            Behavior on width {
                enabled: Style.currentAnimationSpeed !== Style.AnimationSpeed.None && !root.dragging
                NumberAnimation {
                    duration: Style.shortDuration
                }
            }
        }

        Rectangle {
            anchors.fill: parent
            radius: parent.radius
            color: root.onTrackColor
            opacity: mouse.pressed ? Style.stateLayerPressed : mouse.containsMouse ? Style.stateLayerHover : 0
        }
    }

    DIcon {
        id: icon
        visible: root.iconName !== ""
        x: root.mirrored ? root.width - root.iconBox + (root.iconBox - width) / 2 : (root.iconBox - width) / 2
        anchors.verticalCenter: parent.verticalCenter
        name: root.iconName
        size: Math.min(Style.iconSize, root.height - Style.spacingXS * 2)
        color: root.fillWidth >= root.iconBox * 0.75 ? root.onFillColor : root.onTrackColor
    }

    StyledText {
        visible: root.showValue
        x: root.mirrored ? Style.spacingM : root.width - width - Style.spacingM
        anchors.verticalCenter: parent.verticalCenter
        text: Math.round(root.value) + root.unit
        font.pixelSize: Style.fontSizeSmall
        font.weight: Style.fontWeightMedium
        color: root.fillWidth >= root.width - Style.spacingM ? root.onFillColor : root.onTrackColor
    }

    MouseArea {
        id: mouse
        property bool onIcon: false
        anchors.fill: parent
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onPressed: event => {
            const fromStart = root.mirrored ? root.width - event.x : event.x;
            onIcon = root.iconClickable && fromStart < root.iconBox;
            root.forceActiveFocus(Qt.MouseFocusReason);
            if (!onIcon)
                root.setFromX(event.x);
        }
        onPositionChanged: event => {
            if (pressed && !onIcon)
                root.setFromX(event.x);
        }
        onClicked: {
            if (onIcon)
                root.iconClicked();
        }
        onWheel: wheel => {
            if (wheel.angleDelta.y === 0) {
                wheel.accepted = false;
                return;
            }
            root.stepBy(wheel.angleDelta.y > 0 ? 1 : -1);
        }
    }

    FocusRing {
        radius: track.radius
    }
}
