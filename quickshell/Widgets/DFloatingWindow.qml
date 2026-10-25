import QtQuick
import Quickshell
import qs.DCommon.Common
import qs.DCommon.Widgets

// A FloatingWindow whose background follows the window transparency. Quickshell picks the surface format once,
// from the window color at creation: an opaque color there gives a buffer without alpha, and a later translucent
// color only darkens toward black. So the window stays transparent and a rectangle paints the surface.
FloatingWindow {
    id: root

    property color surfaceColor: Style.floatingWindowSurface

    color: "transparent"

    Rectangle {
        anchors.fill: parent
        color: root.surfaceColor
    }

    WindowBlur {
        targetWindow: root
        blurWidth: root.visible ? root.width : 0
        blurHeight: root.visible ? root.height : 0
    }
}
