pragma ComponentBehavior: Bound

import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// Press feedback for a button or row: the control shrinks while pressed and springs back on release. Enter on a
// control that never saw a press ripples from the centre; pointer presses ripple through the control's StateLayer.
// Declare it inside the control; set `cancelled` while a drag owns the press. A second one declared on the same control
// (a row built on a button that already has one) takes over from the first.
Item {
    id: root

    // any button type, or a row built on one; read for down, radius and clicked
    required property var control
    property color rippleColor: Style.onSurface
    property bool cancelled: false

    // wide rows shrink by at most spacingS per side, so a full-width row doesn't inset by tens of pixels
    readonly property real pressedScale: Math.max(0.96, 1 - 2 * Style.spacingS / Math.max(1, control?.width ?? 1))
    readonly property bool animates: !Style.reduceMotion && Style.currentAnimationSpeed !== Style.AnimationSpeed.None
    // superseded() last, so it only runs while the control is down
    readonly property bool down: (control?.enabled ?? false) && (control?.down ?? false) && !superseded()
    readonly property bool held: animates && !cancelled && down
    property bool sawPress: false
    property real factor: held ? pressedScale : 1

    function superseded() {
        let seen = false;
        for (const kid of control?.children ?? []) {
            if (kid === root)
                seen = true;
            else if (seen && kid.control === control && kid.pressedScale !== undefined)
                return true;
        }
        return false;
    }

    anchors.fill: parent

    onDownChanged: if (down)
        sawPress = true

    Behavior on factor {
        enabled: root.animates && !root.cancelled
        DAnim {
            duration: root.held ? Style.expressiveDurations.expressiveFastEffects : Style.expressiveDurations.expressiveFastSpatial
            easing.bezierCurve: root.held ? Style.expressiveCurves.expressiveFastEffects : Style.expressiveCurves.expressiveFastSpatial
        }
    }

    Binding {
        target: root.control
        property: "scale"
        value: root.factor
        when: root.control !== null && root.factor !== 1
    }

    Connections {
        target: root.control
        // list rows have no canceled signal
        ignoreUnknownSignals: true
        function onClicked() {
            if (!root.sawPress && !root.superseded()) {
                ripple.active = true;
                (ripple.item as DRipple).trigger(root.width / 2, root.height / 2);
            }
            root.sawPress = false;
        }
        // a Qt.callLater on release kept a menu that this click opens from taking the keyboard
        function onCanceled() {
            root.sawPress = false;
        }
    }

    // built on the first keyboard click: every button carries this, most never need it
    Loader {
        id: ripple
        anchors.fill: parent
        active: false
        sourceComponent: DRipple {
            rippleColor: root.rippleColor
            cornerRadius: root.control?.radius ?? 0
            topLeftRadius: root.control?.topLeftRadius ?? cornerRadius
            topRightRadius: root.control?.topRightRadius ?? cornerRadius
            bottomLeftRadius: root.control?.bottomLeftRadius ?? cornerRadius
            bottomRightRadius: root.control?.bottomRightRadius ?? cornerRadius
        }
    }
}
