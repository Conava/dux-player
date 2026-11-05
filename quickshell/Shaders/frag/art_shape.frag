#version 450

// The cookie's polar radius is a short cosine series, so rotating or morphing it only changes uniforms.

layout(location = 0) in vec2 qt_TexCoord0;
layout(location = 0) out vec4 fragColor;

layout(std140, binding = 0) uniform buf {
    mat4 qt_Matrix;
    float qt_Opacity;
    float morph;
    float spinAngle;
    float sizePx;
    float lobes;
    vec4 terms;
    float shapeKind; // 0 circle, 1 cookie, 2 rounded square (cornerRadius), 3 square
    float cornerRadius; // as a fraction of the side
} ubuf;

layout(binding = 1) uniform sampler2D source;

float sdRoundBox(vec2 p, float halfSide, float r) {
    vec2 q = abs(p) - vec2(halfSide - r);
    return length(max(q, 0.0)) + min(max(q.x, q.y), 0.0) - r;
}

void main() {
    vec2 p = qt_TexCoord0 - 0.5;
    float circle = length(p) - 0.5;
    float shape = circle;
    if (ubuf.shapeKind > 0.5 && ubuf.shapeKind < 1.5) {
        float a = (atan(p.y, p.x) - ubuf.spinAngle) * ubuf.lobes;
        float cookie = ubuf.terms.x + ubuf.terms.y * cos(a) + ubuf.terms.z * cos(2.0 * a) + ubuf.terms.w * cos(3.0 * a);
        shape = length(p) - cookie;
    } else if (ubuf.shapeKind > 1.5) {
        float c = cos(ubuf.spinAngle);
        float s = sin(ubuf.spinAngle);
        vec2 q = vec2(c * p.x + s * p.y, c * p.y - s * p.x);
        shape = sdRoundBox(q, 0.5, ubuf.shapeKind > 2.5 ? 0.0 : clamp(ubuf.cornerRadius, 0.0, 0.5));
    }
    float mask = clamp(0.5 - mix(circle, shape, ubuf.morph) * ubuf.sizePx, 0.0, 1.0);
    fragColor = texture(source, qt_TexCoord0) * mask * ubuf.qt_Opacity;
}
