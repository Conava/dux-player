import QtQuick
import qs.Common
import qs.Widgets
import qs.DCommon.Common as DC

// A track title in the Bento title face (Notable), shrunk to fit down to XX-large, then scrolling (the shell's media title).
DScrollingText {
    id: root

    property real fullPixelSize: Theme.fontSizeDisplay
    property real minimumPixelSize: Theme.fontSizeXXLarge

    readonly property real fittedPixelSize: {
        if (fullSize.advanceWidth <= width)
            return fullPixelSize;
        return Math.max(minimumPixelSize, Math.floor(fullPixelSize * width / fullSize.advanceWidth));
    }

    fontToken: DC.Fonts.notable
    height: Math.ceil(fullSizeFont.height)
    color: ArtColors.text
    font.pixelSize: fittedPixelSize
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
            "pixelSize": root.fullPixelSize
        })
    }

    FontMetrics {
        id: fullSizeFont
        font: fullSize.font
    }
}
