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
    // the seed scheme's primaryContainer, which stays grey for a grey cover
    readonly property color tint: shown ? Scheme.primaryContainer(seed, Theme.isLightMode, true) : "transparent"

    anchors.left: parent.left
    anchors.right: parent.right
    anchors.top: parent.top
    height: Math.max(0, PlayerMetrics.pageTintHeight - scroll)
    clip: true
    visible: shown && height > 0
    enabled: false
    Accessible.ignored: true

    Rectangle {
        y: -root.scroll
        width: parent.width
        height: PlayerMetrics.pageTintHeight
        radius: PlayerMetrics.paneRadius
        gradient: Gradient {
            GradientStop {
                position: 0
                color: Qt.alpha(root.tint, PlayerMetrics.pageTintAlpha)
                Behavior on color {
                    enabled: Theme.animationsEnabled
                    ColorAnimation {
                        duration: Theme.expressiveDurations.expressiveEffects
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
