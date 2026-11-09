import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// Press feedback for a button or row: the control shrinks while pressed and springs back on release. Enter on a
// control that never saw a press ripples from the centre; pointer presses ripple through the control's StateLayer.
// Declare it inside the control; set `cancelled` while a drag owns the press.
Item {
    id: root

    // any button type, or a row built on one; read for down, radius and clicked
    required property var control
    property color rippleColor: Style.onSurface
    property bool cancelled: false

    // wide rows shrink by at most spacingS per side, so a full-width row doesn't inset by tens of pixels
    readonly property real pressedScale: Math.max(0.96, 1 - 2 * Style.spacingS / Math.max(1, control?.width ?? 1))
    readonly property bool animates: !Style.reduceMotion && Style.currentAnimationSpeed !== Style.AnimationSpeed.None
    readonly property bool held: animates && !cancelled && (control?.down ?? false)
    property bool sawPress: false
    property real factor: held ? pressedScale : 1

    anchors.fill: parent

    onHeldChanged: if (held)
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
        function onClicked() {
            if (!root.sawPress)
                ripple.trigger(root.width / 2, root.height / 2);
            root.sawPress = false;
        }
        function onDownChanged() {
            if (!root.control.down && root.sawPress)
                Qt.callLater(() => root.sawPress = false);
        }
    }

    DRipple {
        id: ripple
        rippleColor: root.rippleColor
        cornerRadius: root.control?.radius ?? 0
        topLeftRadius: root.control?.topLeftRadius ?? cornerRadius
        topRightRadius: root.control?.topRightRadius ?? cornerRadius
        bottomLeftRadius: root.control?.bottomLeftRadius ?? cornerRadius
        bottomRightRadius: root.control?.bottomRightRadius ?? cornerRadius
    }
}
