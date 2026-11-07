pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services

// The Bento player, after the shell's Bento media chrome: the art column and beside it the deck (where it plays
// from, the title, the seek block, the transport and a button group), no cards. Lyrics and the queue take the art's
// place on a card, so the deck stays put. Narrow, the deck goes below the art.
Item {
    id: root

    // the arrangement to show; Settings passes a draft for its live preview
    property var zones: UiConfig.surface("fullBento").zones
    // shown in the art's place (lyrics or the queue), or null
    property Component inPlace: null
    // shown over the art when it stands in for the pane
    property string artNote: ""
    readonly property bool paneShown: inPlace !== null && (artNote === "" || cover === null)
    // a Settings preview: nothing moves and the Play on sheet stays shut
    property bool preview: false

    readonly property var track: PlayerService.current
    // the first shown cover sets the art column; without one the player has no art
    readonly property var cover: zones.cover.find(i => i.type === "cover" && !i.hidden) ?? null
    readonly property real artRatio: {
        switch (cover?.options?.size) {
        case "s":
            return PlayerMetrics.mediaArtColumnRatioS;
        case "l":
            return PlayerMetrics.mediaArtColumnRatioL;
        }
        return PlayerMetrics.mediaArtColumnRatioM;
    }
    readonly property bool showArt: cover !== null || inPlace !== null
    readonly property real gap: Theme.spacingXL
    // the deck's own blocks, which the lines above and below the seekbar don't count in: they go by what these leave
    readonly property var ownBlocks: [header, titleZone, topZone, seekBlock, bottomZone]
    // at the tight gaps, with the art at its least when it can't move beside the deck
    readonly property real lineSpare: height - ownBlocks.reduce((sum, block) => sum + (block.populated ? block.height + Theme.spacingS : 0), 0) - Theme.buttonHeightM - Theme.spacingS - gap - (showArt && width < Theme.smallBreakpoint ? PlayerMetrics.mediaArtMinSize : 0)
    // the deck's blocks without the transport and the gaps between them
    readonly property real blocksHeight: ownBlocks.concat([lineAbove, lineBelow]).reduce((sum, block) => sum + (block.populated ? block.height : 0), 0)
    // a short window tightens the deck's gaps before anything is cut off
    readonly property bool roomy: height >= blocksHeight + Theme.buttonHeightM + Theme.spacingXL * 2 + Theme.spacingL * 3
    readonly property real outerSpacing: roomy ? Theme.spacingXL : Theme.spacingS
    readonly property real innerSpacing: roomy ? Theme.spacingL : Theme.spacingS
    // narrow, the deck goes below the art while that leaves the art some room; else the columns get narrow
    readonly property bool compact: showArt && (width < Theme.smallBreakpoint || (width - gap) * (1 - artRatio) < PlayerMetrics.mediaDeckMinWidth && height - blocksHeight - Theme.buttonHeightM - outerSpacing * 2 - innerSpacing * 3 - gap >= PlayerMetrics.mediaArtMinSize)
    // whole pixels: the art and the lyrics draw through layers, which resample when placed between pixels
    readonly property real artColumnWidth: !showArt ? 0 : compact ? width : Math.round((width - gap) * artRatio)
    readonly property real deckArea: !showArt || compact ? width : width - artColumnWidth - gap
    readonly property real deckWidth: Math.round(Math.min(deckArea, Math.max(PlayerMetrics.mediaDeckMaxWidth, width * PlayerMetrics.mediaDeckMaxRatio)))
    readonly property real artworkSize: {
        if (!showArt)
            return 0;
        if (compact)
            return Math.round(Math.max(0, Math.min(width, height - gap - deck.height)));
        return Math.round(Math.min(artColumnWidth, height));
    }
    readonly property real paneHeight: compact ? artworkSize : Math.round(Math.min(height, Math.max(artworkSize, deck.height)))
    // the transport takes what the deck's other blocks leave of the art's height, so both columns end about level
    readonly property real deckFixedHeight: blocksHeight + [header, titleZone].filter(b => b.populated).length * outerSpacing + [topZone, lineAbove, seekBlock, lineBelow, bottomZone].filter(b => b.populated).length * innerSpacing
    readonly property real transportSize: compact ? Theme.buttonHeightM : Math.round(Math.max(Theme.buttonHeightM, Math.min(PlayerMetrics.mediaTransportMaxHeight, artworkSize - deckFixedHeight)))
    readonly property real stackHeight: compact ? artworkSize + gap + deck.height : Math.max(artworkSize, deck.height)
    readonly property real stackY: Math.round((height - stackHeight) / 2)

    readonly property PlayOnSheet sheet: sheetLoader.item as PlayOnSheet

    // the pane last shown, kept while it fades out
    property Component shownPane: null
    onInPlaceChanged: {
        if (inPlace)
            shownPane = inPlace;
    }

    Item {
        id: artColumn
        visible: root.showArt
        x: root.compact || !I18n.isRtl ? 0 : parent.width - width
        y: root.compact ? root.stackY : 0
        width: root.artColumnWidth
        height: root.compact ? root.artworkSize : parent.height

        PlayerArt {
            anchors.centerIn: parent
            width: root.artworkSize
            height: width
            visible: root.cover !== null && opacity > 0
            opacity: root.paneShown ? 0 : 1
            scale: root.paneShown ? PlayerMetrics.mediaArtLyricsScale : 1
            item: root.track
            shape: root.cover?.options?.shape ?? "cookie"
            rotate: root.cover?.options?.rotate !== false
            still: root.preview || root.paneShown

            Behavior on opacity {
                enabled: Theme.animationsEnabled
                NumberAnimation {
                    duration: Theme.expressiveDurations.expressiveEffects
                    easing.type: Easing.BezierSpline
                    easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
                }
            }

            Behavior on scale {
                enabled: Theme.animationsEnabled
                NumberAnimation {
                    duration: Theme.expressiveDurations.expressiveDefaultSpatial
                    easing.type: Easing.BezierSpline
                    easing.bezierCurve: Theme.expressiveCurves.expressiveDefaultSpatial
                }
            }
        }

        Loader {
            anchors.centerIn: parent
            width: parent.width
            height: root.paneHeight
            active: opacity > 0 || root.inPlace !== null
            enabled: root.paneShown
            opacity: root.paneShown ? 1 : 0
            sourceComponent: root.shownPane

            Behavior on opacity {
                enabled: Theme.animationsEnabled
                NumberAnimation {
                    duration: Theme.expressiveDurations.expressiveEffects
                    easing.type: Easing.BezierSpline
                    easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
                }
            }
        }
    }

    ArtNote {
        parent: artColumn
        anchors.horizontalCenter: parent.horizontalCenter
        y: Math.round((parent.height + root.artworkSize) / 2) - height - Theme.spacingL
        text: root.inPlace && !root.paneShown ? root.artNote : ""
    }

    Item {
        id: deck
        x: {
            if (root.compact || !root.showArt)
                return Math.round((parent.width - width) / 2);
            const start = I18n.isRtl ? 0 : root.artColumnWidth + root.gap;
            return Math.round(start + (root.deckArea - width) / 2);
        }
        y: root.compact ? root.stackY + root.artworkSize + root.gap : Math.round((parent.height - height) / 2)
        width: root.deckWidth
        height: controls.implicitHeight

        Column {
            id: controls
            width: parent.width
            spacing: root.outerSpacing
            opacity: root.sheet?.controlsOpacity ?? 1
            visible: opacity > 0
            enabled: !Router.playOnOpen || root.preview
            transform: Translate {
                x: root.sheet?.controlsShift ?? 0
            }

            NowPlayingZone {
                id: header
                width: parent.width
                height: Math.max(implicitHeight, Theme.buttonHeightXS)
                instances: root.zones.header
                context: "view"
                textLook: "bento"
                centered: true
                room: width
            }

            NowPlayingZone {
                id: titleZone
                width: parent.width
                instances: root.zones.title
                context: "view"
                textLook: "bento"
                centered: UiConfig.titleCentered("fullBento")
                column: true
            }

            Column {
                width: parent.width
                spacing: root.innerSpacing

                NowPlayingZone {
                    id: topZone
                    anchors.horizontalCenter: parent.horizontalCenter
                    width: fitWidth
                    instances: root.zones.top
                    context: "view"
                    look: "group"
                    room: parent.width
                }

                NowPlayingLine {
                    id: lineAbove
                    anchors.horizontalCenter: parent.horizontalCenter
                    width: lineWidth
                    available: parent.width
                    spare: root.lineSpare
                    gap: Theme.spacingS
                    other: lineBelow
                    instances: root.zones.seekAbove
                    context: "view"
                    look: "group"
                    centered: true
                }

                SeekBlock {
                    id: seekBlock
                    width: parent.width
                    instances: root.zones.seek
                }

                NowPlayingLine {
                    id: lineBelow
                    anchors.horizontalCenter: parent.horizontalCenter
                    width: lineWidth
                    available: parent.width
                    spare: root.lineSpare
                    gap: Theme.spacingS
                    other: lineAbove
                    lower: true
                    instances: root.zones.seekBelow
                    context: "view"
                    look: "group"
                    centered: true
                }

                NowPlayingZone {
                    id: controlsZone
                    width: parent.width
                    instances: root.zones.controls
                    context: "view"
                    size: "m"
                    look: "transport"
                    fill: true
                    buttonHeight: root.transportSize
                    room: width
                }

                NowPlayingZone {
                    id: bottomZone
                    anchors.horizontalCenter: parent.horizontalCenter
                    width: fitWidth
                    instances: root.zones.bottom
                    context: "view"
                    look: "group"
                    room: parent.width
                }
            }
        }

        // takes the controls' place, so opening it never moves the deck or the art
        Loader {
            id: sheetLoader
            anchors.fill: parent
            active: !root.preview
            sourceComponent: PlayOnSheet {}
        }
    }
}
