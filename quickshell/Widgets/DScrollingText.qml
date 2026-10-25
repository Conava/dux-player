pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Effects
import qs.DCommon.Common
import qs.DCommon.Widgets

// One line of text that scrolls (marquee) while it doesn't fit and `active` is set; elides otherwise.
Item {
    id: root

    property alias text: label.text
    property alias font: label.font
    property alias fontToken: label.fontToken
    property alias color: label.color
    property bool active: true
    property real holdStartMs: 2000
    property real holdEndMs: 2000
    property real pxPerMs: 1 / 60
    property int stepInterval: 60
    property real overscroll: 5
    property bool loop: false
    property bool fadeEdges: false
    property real loopGap: Style.spacingXL * 2
    property int horizontalAlignment: Text.AlignLeft
    readonly property alias implicitTextWidth: label.implicitWidth
    readonly property bool needsScrolling: label.implicitWidth > width
    readonly property bool rightToLeft: label.effectiveHorizontalAlignment === Text.AlignRight
    readonly property bool scrollActive: needsScrolling && visible && (Window.window?.visible ?? false) && active && !Style.reduceMotion
    readonly property real maxScrollOffset: loop ? label.implicitWidth + loopGap : Math.max(0, label.implicitWidth - width + overscroll)

    property real scrollOffset: 0
    property int scrollDirection: 1
    property real scrollHoldMs: holdStartMs
    property real lastStepMs: 0

    function resetScroll() {
        scrollOffset = 0;
        scrollDirection = 1;
        scrollHoldMs = holdStartMs;
    }

    function stepScroll(deltaMs) {
        if (scrollHoldMs > 0) {
            scrollHoldMs -= deltaMs;
            return;
        }
        const next = scrollOffset + scrollDirection * deltaMs * pxPerMs;
        if (loop) {
            scrollOffset = next % maxScrollOffset;
            return;
        }
        if (next >= maxScrollOffset) {
            scrollOffset = maxScrollOffset;
            scrollDirection = -1;
            scrollHoldMs = holdEndMs;
            return;
        }
        if (next <= 0) {
            resetScroll();
            return;
        }
        scrollOffset = next;
    }

    onScrollActiveChanged: {
        if (!scrollActive) {
            resetScroll();
            return;
        }
        lastStepMs = Date.now();
    }

    clip: true
    implicitHeight: label.implicitHeight
    Accessible.role: Accessible.StaticText
    Accessible.name: text
    layer.enabled: fadeEdges && needsScrolling
    layer.effect: MultiEffect {
        maskEnabled: true
        maskSource: edgeMask.item as Item
    }

    StyledText {
        id: label

        anchors.verticalCenter: parent.verticalCenter
        wrapMode: Text.NoWrap
        width: root.needsScrolling ? implicitWidth : Math.min(implicitWidth, root.width)
        elide: Text.ElideNone
        // whole-pixel x plus native glyph snapping turns even motion into a stutter
        renderType: root.needsScrolling ? Text.QtRendering : resolvedRenderType
        Accessible.ignored: true
        x: {
            if (root.needsScrolling) {
                const offset = Math.min(root.scrollOffset, root.maxScrollOffset);
                return root.rightToLeft ? root.width - width + offset : -offset;
            }
            switch (root.horizontalAlignment) {
            case Text.AlignHCenter:
                return (root.width - width) / 2;
            case Text.AlignRight:
                return root.width - width;
            }
            return 0;
        }
        onTextChanged: root.resetScroll()
    }

    Loader {
        anchors.verticalCenter: parent.verticalCenter
        x: label.x + (root.rightToLeft ? -1 : 1) * root.maxScrollOffset
        active: root.loop && root.needsScrolling

        sourceComponent: StyledText {
            text: label.text
            font: label.font
            color: label.color
            wrapMode: Text.NoWrap
            renderType: Text.QtRendering
            Accessible.ignored: true
        }
    }

    Loader {
        id: edgeMask
        anchors.fill: parent
        active: root.fadeEdges && root.needsScrolling

        sourceComponent: Rectangle {
            id: mask
            readonly property real edge: Math.min(0.5, Style.spacingL / Math.max(1, root.width))
            visible: false
            layer.enabled: true
            gradient: Gradient {
                orientation: Gradient.Horizontal
                GradientStop {
                    position: 0
                    color: "transparent"
                }
                GradientStop {
                    position: mask.edge
                    color: "white"
                }
                GradientStop {
                    position: 1 - mask.edge
                    color: "white"
                }
                GradientStop {
                    position: 1
                    color: "transparent"
                }
            }
        }
    }

    // a timer instead of an animation: a running animation commits a frame every vsync
    Timer {
        interval: root.stepInterval
        repeat: true
        running: root.scrollActive
        onTriggered: {
            const now = Date.now();
            const elapsed = Math.min(now - root.lastStepMs, root.stepInterval * 4);
            root.lastStepMs = now;
            root.stepScroll(elapsed);
        }
    }
}
