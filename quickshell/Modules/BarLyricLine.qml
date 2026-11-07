pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets
import "../Common/LyricsModel.js" as LyricsModel

// The bar's one-line lyrics, modelled on the shell's bar lyric line: the sung line with each word filled as it is
// sung, paged when the line is wider than the bar. Without a synced line it shows the title and artist.
Item {
    id: root

    property color color: ArtColors.text
    // fills each word across its sung time instead of at once
    property bool sweep: false
    property real fontSize: Theme.fontSizeMedium

    readonly property LyricsController controller: LyricsController {
        // held only while the line is on screen; a minimized window still counts as visible to its items
        enabled: root.visible && root.Window.visibility !== Window.Minimized && root.Window.visibility !== Window.Hidden
    }
    readonly property bool wordFill: UiConfig.lyricsWordSync
    readonly property real clock: controller.sampleTime
    readonly property int lineIndex: controller.activeIndex
    readonly property string fontKey: measure.font.family + "/" + measure.font.pixelSize + "/" + measure.font.weight
    readonly property bool active: current.text !== ""
    property string pagedKey: ""
    property string pageKey: ""
    property var pages: []
    property bool snapping: false
    property real shift: 0
    property real turnedWidth: 0

    implicitHeight: Math.ceil(measure.implicitHeight)
    clip: true
    LayoutMirroring.enabled: false
    LayoutMirroring.childrenInherit: true

    onClockChanged: place()
    onLineIndexChanged: place()
    onWidthChanged: place()
    onFontKeyChanged: place()
    onSweepChanged: place()
    onWordFillChanged: place()
    onVisibleChanged: place()
    Component.onCompleted: place()

    function flat(value) {
        return value.replace(/\s*\n\s*/g, " ");
    }

    function widthOf(value) {
        measure.text = value.replace(/\s+$/, "");
        return measure.implicitWidth;
    }

    function wordsOf(part) {
        return wordFill ? part.w : [];
    }

    function place() {
        sweepMotion.stop();
        const parts = visible && controller.synced ? (controller.lines[controller.activeIndex]?.parts ?? []) : [];
        const at = controller.currentTime();
        const next = parts.find(candidate => !candidate.background && at >= candidate.t && at < candidate.e) ?? parts.find(candidate => !candidate.background) ?? parts[0] ?? null;
        if (!next || width <= 0) {
            clearLine();
            return;
        }
        const words = wordsOf(next);
        const key = controller.activeIndex + "/" + next.t + "/" + next.voice;
        const paged = key + "/" + width + "/" + fontKey + "/" + words.length;
        if (paged !== pagedKey) {
            pagedKey = paged;
            pages = paginate(words.length > 0 ? words.map(word => word.x) : (flat(next.x).match(/\S+\s*/g) ?? [flat(next.x)]));
        }
        if (words.length > 0) {
            placeWords(key, next, words, at);
            return;
        }
        placeLine(key, next, at);
    }

    function placeWords(key, line, words, at) {
        nextPage.stop();
        const index = lastStarted(words, at);
        const page = pages.find(candidate => index <= candidate.last) ?? pages[pages.length - 1];
        const sung = index < page.first ? 0 : widthOf(flat(words.slice(page.first, index + 1).map(word => word.x).join("")));
        if (!sweep || !Theme.animationsEnabled || index < page.first) {
            turnTo(key + "/" + page.first, page.text, sung);
            return;
        }
        const before = index === page.first ? 0 : widthOf(flat(words.slice(page.first, index).map(word => word.x).join("")));
        const span = LyricsModel.cueSpans(line)[index];
        const duration = span.e - span.t;
        const progress = Number.isFinite(duration) && duration > 0 ? Math.max(0, Math.min(1, (at - span.t) / duration)) : 1;
        turnTo(key + "/" + page.first, page.text, before + (sung - before) * progress);
        if (progress >= 1 || !controller.playing || controller.rate <= 0)
            return;
        // one linear run per word: the controller wakes this at each cue, nothing ticks per frame in between
        sweepMotion.to = sung;
        sweepMotion.duration = Math.max(1, Math.min(2147483647, (span.e - at) * 1000 / controller.rate));
        sweepMotion.start();
    }

    function placeLine(key, line, at) {
        nextPage.stop();
        const span = line.e - line.t;
        const timed = pages.length > 1 && Number.isFinite(span) && span > 0;
        const pageIndex = timed ? Math.max(0, Math.min(pages.length - 1, Math.floor((at - line.t) / span * pages.length))) : 0;
        const page = pages[pageIndex];
        turnTo(key + "/" + page.first, page.text, widthOf(page.text));
        if (!timed || !controller.playing || controller.rate <= 0 || pageIndex >= pages.length - 1)
            return;
        const boundary = line.t + span * (pageIndex + 1) / pages.length;
        nextPage.interval = Math.max(1, Math.min(2147483647, (boundary - at) * 1000 / controller.rate));
        nextPage.start();
    }

    // cues in time order reorder some providers' space cues, which share the next word's start; w keeps text order
    function lastStarted(words, at) {
        let index = -1;
        for (let i = 0; i < words.length; i++) {
            if (words[i].t <= at)
                index = i;
        }
        return index;
    }

    function paginate(chunks) {
        const words = [];
        for (let i = 0; i < chunks.length; i++) {
            const text = flat(chunks[i]);
            const open = words.length > 0 && !words[words.length - 1].closed;
            if (open) {
                words[words.length - 1].last = i;
                words[words.length - 1].text += text;
            } else {
                words.push({
                    first: i,
                    last: i,
                    text
                });
            }
            words[words.length - 1].closed = /\s$/.test(text);
        }
        const result = [];
        let page = null;
        for (const word of words) {
            if (page && widthOf(page.text + word.text) > width) {
                result.push(page);
                page = null;
            }
            if (!page) {
                page = {
                    first: word.first,
                    last: word.last,
                    text: word.text
                };
                continue;
            }
            page.last = word.last;
            page.text += word.text;
        }
        if (page)
            result.push(page);
        return result;
    }

    function turnTo(key, text, sung) {
        // a page boundary that moved because the bar was resized is no turn
        const resized = width !== turnedWidth;
        turnedWidth = width;
        if (key === pageKey) {
            current.fill = sung;
            return;
        }
        pageKey = key;
        slide.stop();
        const animate = Theme.animationsEnabled && current.text !== "" && !resized;
        outgoing.text = animate ? current.text : "";
        outgoing.fill = current.fill;
        snap(text, sung);
        shift = animate ? 1 : 0;
        if (animate)
            slide.start();
    }

    function clearLine() {
        nextPage.stop();
        sweepMotion.stop();
        slide.stop();
        pagedKey = "";
        pageKey = "";
        pages = [];
        outgoing.text = "";
        snap("", 0);
        shift = 0;
    }

    function snap(text, sung) {
        snapping = true;
        current.text = text;
        current.fill = sung;
        snapping = false;
    }

    Timer {
        id: nextPage
        onTriggered: root.place()
    }

    NumberAnimation {
        id: slide
        target: root
        property: "shift"
        to: 0
        duration: Theme.mediumDuration
        easing.type: Easing.BezierSpline
        easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
        onFinished: outgoing.text = ""
    }

    NumberAnimation {
        id: sweepMotion
        target: current
        property: "fill"
    }

    StyledText {
        id: measure
        visible: false
        font.pixelSize: root.fontSize
        font.weight: Theme.fontWeightMedium
        wrapMode: Text.NoWrap
        elide: Text.ElideNone
    }

    // no synced line: the title and artist
    DScrollingText {
        width: parent.width
        height: parent.height
        visible: !root.active
        text: [PlayerService.current?.title, PlayerService.current?.artist].filter(part => part).join(" • ")
        font.pixelSize: root.fontSize
        font.weight: Theme.fontWeightMedium
        color: root.color
        active: PlayerService.playing
        fadeEdges: true
        loop: true
        pxPerMs: PlayerMetrics.mediaTextScrollSpeed / 1000
        stepInterval: PlayerMetrics.mediaTextScrollStep
    }

    component Face: Item {
        id: face

        property string text: ""
        property real fill: 0
        // the first strong letter decides which side the fill starts from
        readonly property bool rtl: /^[^A-Za-z\u00C0-\u024F\u0590-\u08FF\uFB1D-\uFDFF\uFE70-\uFEFF]*[\u0590-\u08FF\uFB1D-\uFDFF\uFE70-\uFEFF]/.test(text)

        width: root.width
        height: root.height

        Behavior on fill {
            enabled: !root.snapping && !root.sweep && Theme.animationsEnabled
            NumberAnimation {
                duration: Theme.shortDuration
                easing.type: Easing.BezierSpline
                easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
            }
        }

        StyledText {
            id: base
            anchors.verticalCenter: parent.verticalCenter
            text: face.text
            font: measure.font
            color: Theme.withAlpha(root.color, Theme.pendingOpacity)
            wrapMode: Text.NoWrap
            elide: Text.ElideNone
            Accessible.name: face.text
        }

        Item {
            id: filled
            x: face.rtl ? base.implicitWidth - face.fill : 0
            width: face.fill
            height: parent.height
            clip: true

            StyledText {
                x: -filled.x
                anchors.verticalCenter: parent.verticalCenter
                text: face.text
                font: measure.font
                color: root.color
                wrapMode: Text.NoWrap
                elide: Text.ElideNone
                Accessible.ignored: true
            }
        }
    }

    Face {
        id: outgoing
        y: (root.shift - 1) * root.height
        opacity: root.shift
        visible: text !== ""
    }

    Face {
        id: current
        y: root.shift * root.height
        opacity: 1 - root.shift
    }
}
