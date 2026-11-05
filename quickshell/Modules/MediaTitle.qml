import QtQuick
import qs.Common
import qs.Widgets
import qs.DCommon.Common as DC

// A track title in the Bento title face, shrunk to fit down to XX-large, then scrolling (the shell's media title).
// Expressive: Google Sans Flex, condensed and rounded, in the art's accent; else Notable (the Display style).
DScrollingText {
    id: root

    property bool expressive: true
    property real fullPixelSize: Theme.fontSizeDisplay
    property real minimumPixelSize: Theme.fontSizeXXLarge

    // at the full size: the shrunken width depends on the room given, so it can't decide that room
    readonly property real fullTextWidth: fullSize.advanceWidth
    readonly property real fittedPixelSize: {
        if (fullSize.advanceWidth <= width)
            return fullPixelSize;
        return Math.max(minimumPixelSize, Math.floor(fullPixelSize * width / fullSize.advanceWidth));
    }

    fontToken: expressive ? DC.Fonts.sans : DC.Fonts.notable
    height: Math.ceil(fullSizeFont.height)
    color: expressive ? ArtColors.highlight : ArtColors.text
    font.pixelSize: fittedPixelSize
    // the bundled static weights share the family name, so the regular weight picks the variable face
    font.weight: Font.Normal
    font.variableAxes: expressive ? PlayerMetrics.mediaTitleFontAxes : ({})
    loop: true
    fadeEdges: true
    pxPerMs: PlayerMetrics.mediaTextScrollSpeed / 1000
    stepInterval: PlayerMetrics.mediaTextScrollStep

    TextMetrics {
        id: fullSize
        text: root.text
        font: Qt.font({
            "family": root.font.family,
            "weight": root.font.weight,
            "variableAxes": root.font.variableAxes,
            "pixelSize": root.fullPixelSize
        })
    }

    FontMetrics {
        id: fullSizeFont
        font: fullSize.font
    }
}
