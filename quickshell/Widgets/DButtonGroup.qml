pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.DCommon.Common
import qs.DCommon.Widgets as Shared

// The shared DButtonGroup plus the press squish: the pressed segment widens and its neighbours give the width back, so
// the group keeps its width. Delete this file once the shared group has the squish. The segments are the group's children
// with baseWidth and pressed; without them (the shared group changed) nothing happens.
Shared.DButtonGroup {
    id: root

    // the shared transport press: a pressed segment grows by this factor
    readonly property real pressScale: 1.15
    readonly property bool squishes: !Style.reduceMotion && Style.currentAnimationSpeed !== Style.AnimationSpeed.None
    // set once per batch of added segments: a binding would track every segment's width and rebuild the grips on each
    property var segments: []
    onChildrenChanged: Qt.callLater(() => segments = children.filter(kid => kid.baseWidth !== undefined && kid.pressed !== undefined))
    // the segment that is or was last held, and how far its squish has come
    property int heldIndex: -1
    property real squish: 0

    Behavior on squish {
        enabled: root.squishes
        DAnim {
            duration: Style.expressiveDurations.expressiveFastSpatial
            easing.bezierCurve: Style.expressiveCurves.expressiveFastSpatial
        }
    }

    // a segment's width while squished; the pressed one takes the growth from its one or two neighbours
    function squished(index) {
        const held = segments[heldIndex];
        if (!held || heldIndex === index)
            return held ? held.baseWidth * (1 + (pressScale - 1) * squish) : 0;
        const next = segments[index];
        if (Math.abs(heldIndex - index) !== 1)
            return next.baseWidth;
        const neighbours = (segments[heldIndex - 1] ? 1 : 0) + (segments[heldIndex + 1] ? 1 : 0);
        return next.baseWidth - held.baseWidth * (pressScale - 1) * squish / neighbours;
    }

    // the press watchers exist only for a group the pointer is over, so pages with many groups build cheaply
    HoverHandler {
        id: hover
    }

    Instantiator {
        model: hover.hovered ? root.segments : []
        delegate: Connections {
            required property var modelData
            required property int index
            target: modelData
            function onPressedChanged() {
                if (!modelData.pressed) {
                    root.squish = 0;
                    return;
                }
                root.heldIndex = index;
                root.squish = 1;
            }
        }
    }

    Instantiator {
        model: root.squish > 0 ? root.segments : []
        delegate: Binding {
            required property var modelData
            required property int index
            target: modelData
            property: "width"
            value: root.squished(index)
            when: root.squishes && root.heldIndex >= 0 && root.heldIndex < root.segments.length
        }
    }
}
