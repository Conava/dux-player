pragma ComponentBehavior: Bound

import QtQuick
import qs.DCommon.Common
import "../DCommon/Widgets/MaterialShapes.js" as Shapes

// Masks its children (artwork) to a shape. morph 0 is a circle, 1 the shape; spin turns it (radians).
// Size it square. It never animates by itself: the caller drives morph and spin and stops them when hidden.
Item {
    id: root

    property string shape: "cookie" // "cookie" | "circle" | "rounded" | "square"
    property real morph: 1
    property real spin: 0
    property real radius: Style.cornerRadiusXL

    // The 12-fold cosine terms of the cookie's polar radius. The off-centre terms of the Material
    // path are left out, so the shape spins about the artwork's centre without wobbling.
    readonly property vector4d cookieTerms: {
        const radii = Shapes.polarProfile("cookie12", 256);
        const terms = [radii.reduce((sum, r) => sum + r, 0) / radii.length];
        for (let harmonic = 1; harmonic <= 3; harmonic++) {
            let sum = 0;
            for (let i = 0; i < radii.length; i++)
                sum += radii[i] * Math.cos(12 * harmonic * (-Math.PI + 2 * Math.PI * i / radii.length));
            terms.push(2 * sum / radii.length);
        }
        return Qt.vector4d(terms[0], terms[1], terms[2], terms[3]);
    }

    layer.enabled: true
    layer.smooth: true
    layer.effect: ShaderEffect {
        readonly property real morph: root.shape === "circle" ? 0 : Math.max(0, Math.min(1, root.morph))
        readonly property real spinAngle: root.spin
        readonly property real sizePx: root.width
        readonly property real lobes: 12
        readonly property vector4d terms: root.cookieTerms
        readonly property real shapeKind: ["circle", "cookie", "rounded", "square"].indexOf(root.shape)
        readonly property real cornerRadius: root.width > 0 ? root.radius / root.width : 0

        fragmentShader: Qt.resolvedUrl("../Shaders/qsb/art_shape.frag.qsb")
    }
}
