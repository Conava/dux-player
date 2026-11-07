pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.DCommon.Widgets
import "../Common/LyricsModel.js" as LyricsModel

// One row of the lyrics: a line of parts, an instrumental break or a line of unsynced text.
Item {
    id: root

    required property var item
    required property int index
    required property var controller
    required property var accents
    // the playback time for the rows around the centre, -1 ahead of it and Infinity behind it
    property real clock: -1
    property bool promoted: false
    property bool synced: true
    property bool following: true
    property bool playing: false
    property bool animationsEnabled: true
    property real leadFontSize: Theme.fontSizeXLarge
    property var leadMetrics: null
    property var backgroundMetrics: null
    property real breakSize: Theme.iconSizeLarge
    property color dimColor: Theme.onSurfaceVariant
    // a line that playback moves on from keeps its highlight a moment before it dims
    property bool lingering: false
    // a line left by a seek turns inactive at once and stays unsung until the dimming hides its real fill
    property bool holding: false
    property real ringProgress: 0

    readonly property bool isBreak: item.kind === "break"
    readonly property bool isPlain: item.kind === "plain"
    readonly property int moveDuration: Math.round((item.move ?? PlayerMetrics.lyricsMoveMin) * 1000)
    readonly property bool lit: promoted || lingering
    readonly property real shownClock: holding ? -1 : clock
    readonly property real breakProgress: isBreak ? Math.max(0, Math.min(1, (clock - item.t) / (item.e - item.t))) : 0
    // lines that wait or are done rest in one translucent colour; while browsing every line stays fully visible
    readonly property color restingColor: Theme.withAlpha(dimColor, synced && following ? PlayerMetrics.lyricsLineInactiveAlpha : 1)
    readonly property bool shrunk: synced && following && !isPlain && !promoted
    readonly property int leadAlignment: UiConfig.lyricsAlignment === "start" ? (I18n.isRtl ? Text.AlignRight : Text.AlignLeft) : Text.AlignHCenter
    // every part is wrapped here, so the height is known when the row is created: a height that settles a frame
    // later moves the rows around it while the view scrolls past. Measuring writes the probe's text, so this is
    // rebuilt from handlers rather than bound.
    property var layouts: []
    readonly property font leadFont: leadMetrics?.font ?? Qt.font({})
    readonly property font backgroundFont: backgroundMetrics?.font ?? Qt.font({})
    // FontMetrics takes a new font after the rows see it, so a row laid out on the font alone keeps the old height
    readonly property real leadLineHeight: leadMetrics?.lineHeight ?? 0
    readonly property real backgroundLineHeight: backgroundMetrics?.lineHeight ?? 0

    signal activated

    function relayout() {
        if (item.kind !== "line" || width <= 0 || !leadMetrics || !backgroundMetrics) {
            layouts = [];
            return;
        }
        layouts = LyricsModel.partLayouts(item.parts, width, leadMetrics, backgroundMetrics, Theme.spacingXS);
    }

    function alignedX(alignment, childWidth) {
        return Math.round(alignment === Text.AlignLeft ? 0 : alignment === Text.AlignRight ? width - childWidth : (width - childWidth) / 2);
    }

    function originFor(alignment) {
        return alignment === Text.AlignLeft ? Item.Left : alignment === Text.AlignRight ? Item.Right : Item.Center;
    }

    onWidthChanged: relayout()
    onItemChanged: relayout()
    onLeadFontChanged: relayout()
    onBackgroundFontChanged: relayout()
    onLeadLineHeightChanged: relayout()
    onBackgroundLineHeightChanged: relayout()
    Component.onCompleted: relayout()
    onPromotedChanged: {
        lingerTimer.stop();
        fadeTimer.stop();
        lingering = !promoted && !controller.jumping;
        holding = !promoted && controller.jumping;
        if (holding) {
            ringProgress = 0;
            fadeTimer.start();
        } else if (lingering) {
            lingerTimer.start();
        }
    }

    implicitHeight: isPlain ? plainLoader.height + Theme.spacingS * 2 : LyricsModel.rowHeight(item, layouts, breakSize, Theme.spacingS, Theme.spacingXS)
    height: implicitHeight
    Accessible.role: Accessible.StaticText
    Accessible.name: isBreak ? I18n.tr("Instrumental", "track has no lyrics because it is instrumental") : item.text
    ListView.onReused: {
        lingerTimer.stop();
        fadeTimer.stop();
        lingering = false;
        holding = false;
        ringProgress = 0;
    }

    // only the active break shows its progress; one that playback moves on from keeps its arc while it fades
    Binding {
        target: root
        property: "ringProgress"
        value: root.breakProgress
        when: root.isBreak && root.lit
        restoreMode: Binding.RestoreNone
    }

    Timer {
        id: lingerTimer
        interval: PlayerMetrics.lyricsLineLinger
        onTriggered: {
            root.lingering = false;
            fadeTimer.start();
        }
    }

    Timer {
        id: fadeTimer
        interval: root.moveDuration
        onTriggered: {
            root.holding = false;
            root.ringProgress = 0;
        }
    }

    Repeater {
        model: root.layouts

        LyricsPart {
            required property var modelData

            y: Theme.spacingS + modelData.y
            width: root.width
            layout: modelData
        }
    }

    Loader {
        x: root.alignedX(root.leadAlignment, width)
        y: Theme.spacingS
        width: root.breakSize
        height: root.breakSize
        active: root.isBreak
        scale: root.shrunk ? PlayerMetrics.lyricsLineInactiveScale : 1
        transformOrigin: root.originFor(root.leadAlignment)

        Behavior on scale {
            enabled: root.animationsEnabled
            NumberAnimation {
                duration: root.moveDuration
                easing.type: Easing.BezierSpline
                easing.bezierCurve: PlayerMetrics.lyricsTrackCurve
            }
        }

        sourceComponent: DCircularWavyProgress {
            readonly property color lead: root.accents[0]

            progress: root.ringProgress
            color: root.lit ? lead : root.restingColor
            trackColor: root.lit && root.following ? Theme.withAlpha(lead, PlayerMetrics.lyricsRestAlpha) : root.restingColor
            waveSpeed: root.promoted && root.playing && root.animationsEnabled ? wavelength : 0

            Behavior on color {
                enabled: root.animationsEnabled
                ColorAnimation {
                    duration: root.moveDuration
                }
            }

            Behavior on trackColor {
                enabled: root.animationsEnabled
                ColorAnimation {
                    duration: root.moveDuration
                }
            }
        }
    }

    Loader {
        id: plainLoader

        y: Theme.spacingS
        width: parent.width
        active: root.isPlain

        sourceComponent: StyledText {
            text: root.item.text || " "
            color: root.dimColor
            font.pixelSize: root.leadFontSize
            font.weight: Theme.fontWeightMedium
            horizontalAlignment: root.leadAlignment
            wrapMode: Text.WrapAtWordBoundaryOrAnywhere
            Accessible.ignored: true
        }
    }

    StateLayer {
        anchors.fill: parent
        enabled: root.controller.canSeekLines
        stateColor: Theme.onSurfaceVariant
        cornerRadius: Theme.cornerRadius
        onClicked: root.activated()
    }

    component LyricsPart: Item {
        id: lyricsPart

        required property var layout

        readonly property var part: layout.part
        readonly property bool timed: UiConfig.lyricsWordSync && part.w.length > 0
        readonly property bool rtl: LyricsModel.isRtl(part.x)
        readonly property int alignment: part.side === 0 ? root.leadAlignment : (part.side < 0) !== rtl ? Text.AlignLeft : Text.AlignRight
        readonly property bool sung: root.shownClock >= part.t
        // a background part can still be sung after its line left the centre
        readonly property bool lit: root.lit || (sung && root.shownClock < part.e)
        readonly property color accent: part.chorus ? root.accents[0] : root.accents[(part.voiceIndex ?? 0) % root.accents.length]
        // while browsing every line rests in the same colour, so the playing one reads by its fill alone
        readonly property color restColor: lit && root.following ? Theme.withAlpha(accent, PlayerMetrics.lyricsRestAlpha) : root.restingColor
        readonly property color fillColor: lit ? accent : root.restingColor

        height: layout.rows.length * layout.rowHeight
        // a background part still being sung keeps its size while the rest of its line shrinks
        scale: root.shrunk && !(part.background && root.clock >= part.t && root.clock < part.e) ? PlayerMetrics.lyricsLineInactiveScale : 1
        transformOrigin: root.originFor(alignment)

        Behavior on scale {
            enabled: root.animationsEnabled
            NumberAnimation {
                duration: root.moveDuration
                easing.type: Easing.BezierSpline
                easing.bezierCurve: PlayerMetrics.lyricsTrackCurve
            }
        }

        Repeater {
            model: lyricsPart.layout.rows

            Item {
                id: row

                required property var modelData
                required property int index
                // smoothstep is undefined for an edge with no width, so reduced motion keeps one pixel of it
                readonly property real lead: root.animationsEnabled ? lyricsPart.layout.rowHeight * PlayerMetrics.lyricsSweepLead : 1
                readonly property real feather: width > 0 ? lead / width : 0
                // line-timed parts have no cues, so the whole line changes colour when it starts
                readonly property real edge: {
                    if (!lyricsPart.timed || width <= 0)
                        return -feather;
                    const reached = root.animationsEnabled ? LyricsModel.smoothEdge(modelData.segments, root.shownClock, lead, PlayerMetrics.lyricsSweepWindow) : LyricsModel.fillEdge(modelData.segments, root.shownClock, false, lead);
                    return Math.min(1, reached / width);
                }
                readonly property color unfilledColor: !lyricsPart.timed && lyricsPart.sung ? lyricsPart.fillColor : lyricsPart.restColor

                x: root.alignedX(lyricsPart.alignment, width)
                y: index * lyricsPart.layout.rowHeight
                width: modelData.width
                height: lyricsPart.layout.rowHeight

                StyledText {
                    id: glyphs
                    width: row.width
                    height: row.height
                    text: row.modelData.text
                    font: lyricsPart.layout.font
                    // only the coverage reaches the shader, which applies the colour
                    color: Theme.surfaceText
                    wrapMode: Text.NoWrap
                    elide: Text.ElideNone
                    Accessible.ignored: true
                }

                ShaderEffectSource {
                    id: glyphSource
                    sourceItem: glyphs
                    hideSource: true
                    smooth: true
                }

                ShaderEffect {
                    property var source: glyphSource
                    property color restColor: row.unfilledColor
                    property color fillColor: lyricsPart.fillColor
                    property real edge: row.edge
                    property real feather: row.feather
                    property real rtl: lyricsPart.rtl ? 1 : 0

                    anchors.fill: parent
                    fragmentShader: Qt.resolvedUrl("../Shaders/qsb/lyrics_fill.frag.qsb")

                    Behavior on restColor {
                        enabled: root.animationsEnabled
                        ColorAnimation {
                            duration: root.moveDuration
                        }
                    }

                    Behavior on fillColor {
                        enabled: root.animationsEnabled
                        ColorAnimation {
                            duration: root.moveDuration
                        }
                    }
                }
            }
        }
    }
}
