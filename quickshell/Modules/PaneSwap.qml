pragma ComponentBehavior: Bound

import QtQuick
import qs.Common

// Shows one pane of a few and swaps them when `current` changes: the old pane fades out while the new one fades in
// and slides in from the side of its place in `order` (the end side for a later pane, mirrored in RTL). An empty
// `current` keeps what is shown, as the owner is closing the pane itself.
Item {
    id: root

    property string current
    // the panes in reading order
    property var order: []
    // pane name to Component
    property var sources: ({})

    readonly property real distance: Theme.spacingXL
    property int front: 0
    // the slots exist; the Repeater may build them after this item completes
    readonly property bool built: slots.count === 2
    // a new pane is loading and the old one is still shown
    property bool swapping: false

    function slotAt(i) {
        return slots.itemAt(i) as Slot;
    }

    // starts the new pane in once it is built, so a heavy pane never stalls the old one's fade
    function settle() {
        swapping = false;
        const shown = slotAt(front);
        const old = slotAt(1 - front);
        if (old.key !== "")
            old.leave();
        old.z = 0;
        shown.z = 1;
        shown.leaving = false;
        shown.enter();
    }

    function swapTo(name) {
        const old = slotAt(front);
        const next = slotAt(1 - front);
        if (!old || !next)
            return;
        if (old.key === name && !old.leaving)
            return;
        front = 1 - front;
        swapping = false;
        if (!Theme.animationsEnabled) {
            old.cut();
            next.asynchronous = false;
            next.key = name;
            next.opacity = 1;
            next.shift = 0;
            return;
        }
        let dir = 0;
        if (old.key !== "")
            dir = (order.indexOf(name) > order.indexOf(old.key) ? 1 : -1) * (I18n.isRtl ? -1 : 1);
        if (next.key !== name) {
            next.asynchronous = true;
            next.key = name;
            next.opacity = 0;
            next.shift = dir * distance;
        }
        swapping = true;
        if (next.status === Loader.Ready)
            settle();
    }

    onCurrentChanged: {
        if (built && current !== "")
            swapTo(current);
    }
    onBuiltChanged: {
        if (built && current !== "")
            swapTo(current);
    }

    component Slot: Loader {
        id: slot
        required property var owner

        required property int index
        property string key
        property real shift: 0
        property bool leaving: false

        function enter() {
            exit.stop();
            entry.restart();
        }

        function leave() {
            entry.stop();
            leaving = true;
            exit.restart();
        }

        function cut() {
            entry.stop();
            exit.stop();
            leaving = false;
            key = "";
        }

        Component.onCompleted: {
            if (index !== 0 || slot.owner.current === "")
                return;
            key = slot.owner.current;
        }

        onLoaded: {
            if (slot.owner.swapping && index === slot.owner.front)
                slot.owner.settle();
        }

        anchors.fill: parent
        active: key !== ""
        enabled: !leaving
        sourceComponent: slot.owner?.sources[key] ?? null
        transform: Translate {
            x: slot.shift
        }

        ParallelAnimation {
            id: entry

            NumberAnimation {
                target: slot
                property: "opacity"
                to: 1
                duration: Theme.expressiveDurations.normal
                easing.type: Easing.BezierSpline
                easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
            }

            NumberAnimation {
                target: slot
                property: "shift"
                to: 0
                duration: Theme.expressiveDurations.normal
                easing.type: Easing.BezierSpline
                easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
            }
        }

        NumberAnimation {
            id: exit
            target: slot
            property: "opacity"
            to: 0
            duration: Theme.expressiveDurations.expressiveEffects
            easing.type: Easing.BezierSpline
            easing.bezierCurve: Theme.expressiveCurves.emphasizedAccel
            onFinished: slot.cut()
        }
    }

    Repeater {
        id: slots
        model: 2

        Slot {
            owner: root
        }
    }
}
