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

    // the shader's shape as a hit test: a control over the art sets containmentMask to this
    readonly property QtObject mask: QtObject {
        function contains(point: point): bool {
            if (root.width <= 0)
                return false;
            const x = point.x / root.width - 0.5;
            const y = point.y / root.height - 0.5;
            const len = Math.hypot(x, y);
            const circle = len - 0.5;
            let shape = circle;
            if (root.shape === "cookie") {
                const a = (Math.atan2(y, x) - root.spin) * 12;
                const t = root.cookieTerms;
                shape = len - (t.x + t.y * Math.cos(a) + t.z * Math.cos(2 * a) + t.w * Math.cos(3 * a));
            } else if (root.shape === "rounded" || root.shape === "square") {
                const c = Math.cos(root.spin);
                const s = Math.sin(root.spin);
                const r = root.shape === "square" ? 0 : Math.max(0, Math.min(0.5, root.radius / root.width));
                const qx = Math.abs(c * x + s * y) - (0.5 - r);
                const qy = Math.abs(c * y - s * x) - (0.5 - r);
                shape = Math.hypot(Math.max(qx, 0), Math.max(qy, 0)) + Math.min(Math.max(qx, qy), 0) - r;
            }
            const morph = root.shape === "circle" ? 0 : Math.max(0, Math.min(1, root.morph));
            return circle + (shape - circle) * morph <= 0;
        }
    }

    layer.enabled: true
    layer.smooth: true
    layer.effect: ShaderEffect {
        // the layer sets this before the shader's own sampler property exists
        property var source

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
