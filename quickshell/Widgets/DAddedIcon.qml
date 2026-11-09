import QtQuick
import QtQuick.Shapes
import qs.DCommon.Common

// A list whose arrow curls in under its first line: something put into a list by hand. Material Symbols has no
// such glyph, so it is drawn on the 24-unit icon grid with the symbols' 2-unit rounded stroke.
Item {
    id: root

    property real size: Style.iconSizeMedium
    property color color: Style.surfaceText
    // Material Symbols' stroke on their 24-unit grid
    readonly property real stroke: 2

    implicitWidth: size
    implicitHeight: size
    Accessible.role: Accessible.Graphic

    Shape {
        width: 24
        height: 24
        scale: root.size / 24
        transformOrigin: Item.TopLeft
        preferredRendererType: Shape.CurveRenderer

        ShapePath {
            strokeColor: root.color
            strokeWidth: root.stroke
            fillColor: "transparent"
            capStyle: ShapePath.RoundCap
            joinStyle: ShapePath.RoundJoin

            PathSvg {
                path: "M3 3.5h10M3 14h18M3 20.5h18M15.5 1.5h2a4 4 0 0 1 0 8H11.5M13.5 7.5l-2 2 2 2"
            }
        }
    }
}
