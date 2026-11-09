import QtQuick
import qs.Common
import qs.Widgets
import qs.DCommon.Widgets
import qs.DCommon.Common as DC

// A track title in the Bento title face, shrunk to fit down to XX-large, then scrolling (the shell's media title).
// Expressive: Google Sans Flex, condensed and rounded, in the art's accent; else Notable (the Display style).
DScrollingText {
    id: root

    property bool expressive: true
    property real fullPixelSize: Theme.fontSizeDisplay
    property real minimumPixelSize: Theme.fontSizeXXLarge
    // a changed title slides out and in; only now-playing shows titles with this item
    property bool slide: true

    // at the full size: the shrunken width depends on the room given, so it can't decide that room
    readonly property real fullTextWidth: fullSize.implicitWidth
    readonly property real fittedPixelSize: {
        if (fullTextWidth <= width)
            return fullPixelSize;
        return Math.max(minimumPixelSize, Math.floor(fullPixelSize * width / fullTextWidth));
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
    opacity: change.fade
    transform: Translate {
        x: change.shift
    }

    TextSlide {
        id: change
        owner: root
        text: root.text
        font: root.font
        color: root.color
        horizontalAlignment: root.horizontalAlignment
        active: root.slide
    }

    // not TextMetrics: its width can fall a pixel short of the drawn text, which then scrolls
    StyledText {
        id: fullSize
        visible: false
        text: root.text
        fontToken: root.fontToken
        font.pixelSize: root.fullPixelSize
        font.weight: root.font.weight
        font.variableAxes: root.font.variableAxes
        wrapMode: Text.NoWrap
        elide: Text.ElideNone
    }

    FontMetrics {
        id: fullSizeFont
        font: fullSize.font
    }
}
