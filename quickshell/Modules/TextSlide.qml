import QtQuick
import qs.Common
import qs.Services

// Slides a now-playing text that changes with the track: a copy of the old text leaves sideways and fades while the
// owner is hidden, then the owner enters from the other side. Text that changes for the same track (a tag edit) fades
// in place. Bind the owner's opacity to fade and its x offset to shift.
Item {
    id: root

    // the item showing the text; the copy sits beside it
    required property Item owner
    required property string text
    property font font
    property color color
    property int horizontalAlignment: Text.AlignLeft
    property bool active: true

    property real fade: 1
    property real shift: 0

    readonly property real travel: Theme.spacingXL
    readonly property int queueIndex: PlayerService.queueIndex
    property string previous: text
    property string leaving: ""
    property real leaveFade: 0
    property real leaveShift: 0
    property int step: 1
    property double stepAt: 0
    property int lastIndex: queueIndex
    // the copy's parent: beside the owner, never a positioner or layout, which would place the copy as an item of its own
    readonly property Item host: {
        let p = owner.parent;
        while (p?.parent && ("spacing" in p || "rowSpacing" in p))
            p = p.parent;
        return p;
    }
    // the track the shown text belongs to
    property string textTrack

    function trackKey() {
        return PlayerService.current ? PlayerService.key(PlayerService.current) : "";
    }

    Component.onCompleted: textTrack = trackKey()
    onQueueIndexChanged: {
        step = queueIndex < lastIndex ? -1 : 1;
        lastIndex = queueIndex;
        stepAt = Date.now();
    }

    onTextChanged: {
        const old = previous;
        previous = text;
        const track = trackKey();
        const sameTrack = track === textTrack;
        textTrack = track;
        if (old === "" || text === "" || !active || !owner.visible || !Theme.animationsEnabled) {
            slide.stop();
            fade = 1;
            shift = 0;
            leaveFade = 0;
            return;
        }
        // the direction of a skip, known when the queue index moved just before; else forward
        const dir = sameTrack ? 0 : (Date.now() - stepAt < 1000 ? step : 1) * (LayoutMirroring.enabled ? -1 : 1);
        leaving = old;
        slide.travelSigned = dir * travel;
        slide.restart();
    }

    SequentialAnimation {
        id: slide
        property real travelSigned: 0

        ScriptAction {
            script: {
                const at = root.owner.mapToItem(root.host, 0, 0);
                copy.x = at.x;
                copy.y = at.y;
                root.fade = 0;
                root.shift = slide.travelSigned;
                root.leaveFade = 1;
                root.leaveShift = 0;
            }
        }
        ParallelAnimation {
            NumberAnimation {
                target: root
                property: "leaveFade"
                to: 0
                duration: Theme.expressiveDurations.expressiveFastEffects
                easing.type: Easing.BezierSpline
                easing.bezierCurve: Theme.expressiveCurves.expressiveFastEffects
            }
            NumberAnimation {
                target: root
                property: "leaveShift"
                to: -slide.travelSigned
                duration: Theme.expressiveDurations.expressiveFastEffects
                easing.type: Easing.BezierSpline
                easing.bezierCurve: Theme.expressiveCurves.expressiveFastEffects
            }
        }
        ParallelAnimation {
            NumberAnimation {
                target: root
                property: "fade"
                to: 1
                duration: Theme.expressiveDurations.expressiveFastEffects
                easing.type: Easing.BezierSpline
                easing.bezierCurve: Theme.expressiveCurves.expressiveFastEffects
            }
            NumberAnimation {
                target: root
                property: "shift"
                to: 0
                duration: Theme.expressiveDurations.expressiveFastEffects
                easing.type: Easing.BezierSpline
                easing.bezierCurve: Theme.expressiveCurves.expressiveFastEffects
            }
        }
    }

    // plain Text: StyledText fades its own opacity changes
    Text {
        id: copy
        parent: root.host
        width: root.owner.width
        height: root.owner.height
        visible: slide.running && root.leaveFade > 0
        opacity: root.leaveFade
        text: root.leaving
        font: root.font
        color: root.color
        textFormat: Text.PlainText
        wrapMode: Text.NoWrap
        elide: Text.ElideRight
        horizontalAlignment: root.horizontalAlignment
        verticalAlignment: Text.AlignVCenter
        Accessible.ignored: true
        transform: Translate {
            x: root.leaveShift
        }
    }
}
