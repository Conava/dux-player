import QtQuick
import qs.Common
import "../Common/Scheme.js" as Scheme

// A fade from a cover's colour into the page at its top. `scroll` is how far the page's list has moved up, so
// the tint leaves with the header. Never takes input.
Item {
    id: root

    // a colour string; "" for none
    property string seed: ""
    property real scroll: 0

    readonly property bool shown: seed !== "" && UiConfig.pageTinted
    // the seed scheme's primaryContainer, which stays grey for a grey cover; kept while the tint fades out
    readonly property color tint: seed !== "" ? Scheme.primaryContainer(seed, Theme.isLightMode, true) : held
    property color held: "transparent"

    onTintChanged: {
        if (seed !== "")
            held = tint;
    }

    anchors.left: parent.left
    anchors.right: parent.right
    anchors.top: parent.top
    height: Math.max(0, PlayerMetrics.pageTintHeight - scroll)
    clip: true
    visible: fill.opacity > 0 && height > 0
    enabled: false
    Accessible.ignored: true

    // a colour known when the page is built shows at once; one that arrives later fades in
    Rectangle {
        id: fill
        y: -root.scroll
        width: parent.width
        height: PlayerMetrics.pageTintHeight
        radius: PlayerMetrics.paneRadius
        opacity: root.shown ? 1 : 0
        Behavior on opacity {
            enabled: Theme.animationsEnabled
            NumberAnimation {
                duration: Theme.expressiveDurations.expressiveEffects
                easing.type: Easing.BezierSpline
                easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
            }
        }
        gradient: Gradient {
            GradientStop {
                position: 0
                color: Qt.alpha(root.tint, PlayerMetrics.pageTintAlpha)
                // only a colour already showing cross-fades to the next
                Behavior on color {
                    enabled: Theme.animationsEnabled && fill.opacity > 0
                    ColorAnimation {
                        duration: Theme.expressiveDurations.expressiveEffects
                        easing.type: Easing.BezierSpline
                        easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
                    }
                }
            }
            GradientStop {
                position: 1
                color: Qt.alpha(root.tint, 0)
            }
        }
    }
}
