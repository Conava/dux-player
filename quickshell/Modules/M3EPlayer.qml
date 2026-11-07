pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import "../Common/Format.js" as Format

// The Material player: one filled card in the cover's colour. Wide, the art sits at the end beside one vertically
// centred group (where it plays from, the title, the seekbar, the transport), the button zone above the art.
// Stacked (narrow, or a pane beside it), everything goes below the art; a pane in place takes the art's and title's
// place.
Item {
    id: root

    // the arrangement to show; Settings passes a draft for its live preview
    property var zones: UiConfig.surface("fullM3e").zones
    // shown in place of the art and title (lyrics or the queue in a narrow window), or null
    property Component inPlace: null
    // shown over the art when it stands in for the pane
    property string artNote: ""
    readonly property bool paneShown: inPlace !== null && (artNote === "" || !showArt)
    // from under the top row to the seekbar group; info.y would close a loop through the art's size
    readonly property real paneHeight: Math.max(0, height - margin * 2 - topHeight - info.implicitHeight - Theme.spacingL)
    // a pane is open beside the player: stacked, and level with the pane at the top
    property bool beside: false
    // the view draws one card behind the player and the pane
    property bool joined: false
    // a Settings preview: the art stays still
    property bool preview: false

    readonly property var track: PlayerService.current
    // the first shown cover sets the art's size; without one the player has no art
    readonly property var cover: zones.cover.find(i => i.type === "cover" && !i.hidden) ?? null
    // grows with a big window, so the card doesn't stay small on a wide screen
    readonly property real artMax: {
        const room = Math.min(width, height);
        switch (cover?.options?.size) {
        case "s":
            return Math.max(PlayerMetrics.expandedArtMaxSmall, room * PlayerMetrics.expandedArtRatioS);
        case "l":
            return Math.max(PlayerMetrics.expandedArtMaxLarge, room * PlayerMetrics.expandedArtRatioL);
        }
        return Math.max(PlayerMetrics.expandedArtMax, room * PlayerMetrics.expandedArtRatioM);
    }
    readonly property bool showArt: cover !== null
    readonly property real margin: PlayerMetrics.mediaCardMargin
    readonly property real gap: Theme.spacingXL
    readonly property real wideWidth: Math.min(width, artMax * 2 + gap + margin * 2)
    // the transport and the end buttons side by side, at a button size
    function rowNeed(size, minimal = false) {
        const both = playback.shown.length > 0 && modesRow.shown.length > 0;
        // the overflow check counts one more gap than the zone has
        return playback.naturalWidth(size, minimal) + modesRow.naturalWidth(size, minimal) + (both ? Theme.spacingM + modesRow.spacing : 0);
    }

    // stays stacked until the info column beside the art holds the transport and the kept buttons; the rest can go into the ⋮
    readonly property bool stacked: beside || inPlace !== null || !showArt || width < Theme.smallBreakpoint || (wideWidth - margin * 2 - gap) / 2 < Math.max(PlayerMetrics.barInfoWidth, rowNeed("s", true))
    readonly property real cardWidth: inPlace ? width : stacked ? Math.min(width, Math.max(artMax, PlayerMetrics.expandedArtMax) + margin * 2) : wideWidth
    readonly property real innerWidth: cardWidth - margin * 2
    readonly property real topHeight: topRow.visible ? topRow.height + Theme.spacingL : 0
    readonly property real artSize: {
        if (!showArt || paneShown)
            return 0;
        if (inPlace)
            return Math.round(Math.min(innerWidth, paneHeight));
        if (stacked)
            return Math.round(Math.max(0, Math.min(innerWidth, artMax, height - margin * 2 - topHeight - info.implicitHeight - Theme.spacingL)));
        return Math.round(Math.max(0, Math.min(artMax, (innerWidth - gap) / 2, height - margin * 2 - topHeight)));
    }
    readonly property real heroHeight: stacked ? (artSize > 0 ? artSize + Theme.spacingL : 0) : Math.max(artSize, info.implicitHeight)
    readonly property real infoWidth: stacked ? innerWidth : innerWidth - artSize - gap
    // smaller buttons when the row doesn't fit at "m"
    readonly property string buttonSize: innerWidth < Theme.smallBreakpoint || rowNeed("m") > infoWidth ? "s" : "m"

    readonly property var seekTypes: zones.seek.filter(i => !i.hidden).map(i => i.type)
    readonly property bool hasElapsed: seekTypes.indexOf("elapsed") >= 0
    readonly property bool hasDuration: seekTypes.indexOf("duration") >= 0
    readonly property string durationText: PlayerService.duration > 0 ? Format.duration(PlayerService.duration) : "--:--"

    // x of a child at the start or end of the card's content, mirrored for RTL
    function startX(itemWidth) {
        return I18n.isRtl ? card.width - margin - itemWidth : margin;
    }

    function endX(itemWidth) {
        return I18n.isRtl ? margin : card.width - margin - itemWidth;
    }

    Rectangle {
        id: card
        anchors.horizontalCenter: parent.horizontalCenter
        y: root.inPlace || root.beside && !root.joined ? 0 : Math.max(0, Math.round((root.height - height) / 2))
        width: root.cardWidth
        height: root.inPlace ? root.height : Math.min(root.height, root.margin * 2 + root.topHeight + (root.stacked ? root.heroHeight + info.implicitHeight : root.heroHeight))
        radius: Theme.cornerRadiusXL
        color: root.joined ? "transparent" : ArtColors.cardColor

        // the button zone at the end; stacked, where it plays from at the start
        Item {
            id: topRow
            x: root.margin
            y: root.margin
            width: root.innerWidth
            height: Math.max(buttons.implicitHeight, headerTop.visible ? headerTop.implicitHeight : 0)
            visible: buttons.populated || headerTop.visible

            NowPlayingZone {
                id: headerTop
                x: I18n.isRtl ? parent.width - width : 0
                width: Math.max(0, parent.width - (buttons.populated ? buttons.width + Theme.spacingM : 0))
                anchors.verticalCenter: parent.verticalCenter
                visible: root.stacked && populated
                instances: root.zones.header
                context: "view"
                textLook: "m3e"
            }

            NowPlayingZone {
                id: buttons
                x: I18n.isRtl ? 0 : parent.width - width
                anchors.verticalCenter: parent.verticalCenter
                instances: root.zones.buttons
                context: "view"
                look: "m3e"
                room: root.innerWidth - (headerTop.visible ? PlayerMetrics.barInfoMinWidth + Theme.spacingM : 0)
            }
        }

        PlayerArt {
            id: art
            x: root.stacked ? Math.round((card.width - width) / 2) : root.endX(width)
            y: root.margin + root.topHeight + (root.inPlace ? Math.round((root.paneHeight - height) / 2) : root.stacked ? 0 : Math.round((root.heroHeight - height) / 2))
            width: root.artSize
            height: width
            visible: width > 0
            radius: PlayerMetrics.mediaInnerRadius
            item: root.track
            shape: root.cover?.options?.shape ?? "rounded"
            rotate: root.cover?.options?.rotate !== false
            still: root.preview

            ArtNote {
                anchors.horizontalCenter: parent.horizontalCenter
                anchors.bottom: parent.bottom
                anchors.bottomMargin: Theme.spacingL
                text: root.inPlace && !root.paneShown ? root.artNote : ""
            }
        }

        // a pane in place: from under the top row down to the seekbar. It keeps running behind the art.
        Loader {
            x: root.margin
            y: root.margin + root.topHeight
            width: root.innerWidth
            height: root.paneHeight
            active: root.inPlace !== null
            enabled: root.paneShown
            opacity: root.paneShown ? 1 : 0
            sourceComponent: root.inPlace
        }

        Column {
            id: info
            x: root.stacked ? root.margin : root.startX(width)
            y: root.inPlace ? card.height - root.margin - implicitHeight : root.margin + root.topHeight + (root.stacked ? root.heroHeight : Math.round((root.heroHeight - implicitHeight) / 2))
            width: root.infoWidth
            spacing: Theme.spacingM

            NowPlayingZone {
                width: parent.width
                visible: !root.stacked && populated
                instances: root.zones.header
                context: "view"
                textLook: "m3e"
            }

            NowPlayingZone {
                width: parent.width
                visible: (!root.inPlace || !root.paneShown) && populated
                instances: root.zones.title
                context: "view"
                textLook: "m3e"
                column: true
            }

            // the seek zone: the seekbar, then "elapsed / length" in one readout, then anything else placed there
            Row {
                width: parent.width
                height: Math.max(seekbar.implicitHeight, readout.implicitHeight)
                visible: root.seekTypes.length > 0
                spacing: Theme.spacingM
                LayoutMirroring.enabled: false
                LayoutMirroring.childrenInherit: true

                MediaSeekbar {
                    id: seekbar
                    anchors.verticalCenter: parent.verticalCenter
                    visible: root.seekTypes.indexOf("seekbar") >= 0
                    width: Math.max(0, parent.width - (readout.visible ? readout.width + parent.spacing : 0) - (extra.visible ? extra.width + parent.spacing : 0))
                    flat: true
                }

                NumericText {
                    id: readout
                    anchors.verticalCenter: parent.verticalCenter
                    visible: root.hasElapsed || root.hasDuration
                    width: Math.ceil(reservedWidth)
                    text: [root.hasElapsed ? Format.duration(seekbar.value * PlayerService.duration) : "", root.hasDuration ? root.durationText : ""].filter(Boolean).join(" / ")
                    reserveText: text.replace(/\d/g, "8")
                    isMonospace: false
                    color: ArtColors.textVariant
                    font.pixelSize: Theme.fontSizeMedium
                    font.weight: Theme.fontWeightMedium
                }

                NowPlayingZone {
                    id: extra
                    anchors.verticalCenter: parent.verticalCenter
                    instances: root.zones.seek.filter(i => ["elapsed", "seekbar", "duration"].indexOf(i.type) < 0)
                    context: "view"
                }
            }

            // the transport at the start, the end zone at the far end; what doesn't fit beside the transport goes
            // into the end zone's ⋮ menu
            Item {
                width: parent.width
                height: Math.max(playback.implicitHeight, modesRow.implicitHeight)
                visible: playback.populated || modesRow.populated

                NowPlayingZone {
                    id: playback
                    x: I18n.isRtl ? parent.width - width : 0
                    anchors.verticalCenter: parent.verticalCenter
                    instances: root.zones.controls
                    context: "view"
                    size: root.buttonSize
                    look: "m3e"
                    room: root.infoWidth
                }

                NowPlayingZone {
                    id: modesRow
                    x: I18n.isRtl ? 0 : parent.width - width
                    anchors.verticalCenter: parent.verticalCenter
                    instances: root.zones.end
                    context: "view"
                    size: playback.size
                    look: "m3e"
                    room: Math.max(0, root.infoWidth - playback.implicitWidth - Theme.spacingM)
                    keepMoves: true
                }
            }
        }
    }
}
