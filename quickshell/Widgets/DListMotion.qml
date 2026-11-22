import QtQuick
import qs.DCommon.Common

// A list's or grid's edit transitions: `displaced: DListMotion.Displaced {}`, `move: DListMotion.Move {}`,
// `add: DListMotion.Add {}`, `remove: DListMotion.Remove {}`. Each is off with reduce motion or speed None, and while
// `active` is false (a view that replaces its whole model sets it for in-place edits only). Never a populate: a
// populate transition parks the positions the view writes while it runs.
QtObject {
    // an add interrupted mid-fade can leave a row displaced at partial opacity or scale, so moved rows end at 1
    component Displaced: Transition {
        id: displaced
        property bool active: true
        // a list's rows keep a bound x (mirrored insets) that an x animation would drop; a grid sets "x,y"
        property string axes: "y"
        enabled: active && !Style.reduceMotion && Style.currentAnimationSpeed !== Style.AnimationSpeed.None

        NumberAnimation {
            properties: displaced.axes
            duration: Style.expressiveDurations.expressiveFastSpatial
            easing.type: Easing.BezierSpline
            easing.bezierCurve: Style.expressiveCurves.expressiveFastSpatial
        }
        NumberAnimation {
            properties: "opacity,scale"
            to: 1
            duration: 0
        }
    }

    component Move: Displaced {}

    component Add: Transition {
        id: add
        property bool active: true
        // how far a new row rises into place
        property real rise: Style.spacingM
        enabled: active && !Style.reduceMotion && Style.currentAnimationSpeed !== Style.AnimationSpeed.None

        NumberAnimation {
            property: "opacity"
            from: 0
            to: 1
            duration: Style.expressiveDurations.expressiveEffects
            easing.type: Easing.BezierSpline
            easing.bezierCurve: Style.expressiveCurves.emphasizedDecel
        }
        NumberAnimation {
            property: "y"
            from: ViewTransition.destination.y + add.rise
            duration: Style.expressiveDurations.expressiveEffects
            easing.type: Easing.BezierSpline
            easing.bezierCurve: Style.expressiveCurves.emphasizedDecel
        }
    }

    // fades and slides toward the end edge, the left one in RTL
    component Remove: Transition {
        property bool active: true
        enabled: active && !Style.reduceMotion && Style.currentAnimationSpeed !== Style.AnimationSpeed.None

        NumberAnimation {
            property: "opacity"
            to: 0
            duration: Style.expressiveDurations.expressiveEffects
            easing.type: Easing.BezierSpline
            easing.bezierCurve: Style.expressiveCurves.emphasizedAccel
        }
        NumberAnimation {
            property: "x"
            to: ViewTransition.destination.x + (I18n.isRtl ? -Style.spacingXL : Style.spacingXL)
            duration: Style.expressiveDurations.expressiveEffects
            easing.type: Easing.BezierSpline
            easing.bezierCurve: Style.expressiveCurves.emphasizedAccel
        }
    }
}
