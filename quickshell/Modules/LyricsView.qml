pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Common as DC
import qs.DCommon.Widgets
import qs.Widgets
import "../Common/LyricsModel.js" as LyricsModel

// Synced lyrics with a progressive word fill, modelled on the shell's LyricsOverlay.
FocusScope {
    id: root

    property color cardColor: "transparent"
    property var artScheme: null
    property real radius: 0
    property bool following: true
    property real now: 0
    property bool animateFollow: false
    property var heightCache: ({})
    property int centreItem: -1
    property real centreTime: -Infinity
    // a text measures 0 until its component is complete, so rows created with the view would stay 0 wide
    property bool canMeasure: false

    readonly property var accents: artScheme ? [artScheme.primary, artScheme.tertiary, artScheme.secondary] : [Theme.primary, Theme.tertiary, Theme.secondary]
    readonly property real textScale: UiConfig.lyricsTextSize === "s" ? PlayerMetrics.lyricsScaleS : UiConfig.lyricsTextSize === "l" ? PlayerMetrics.lyricsScaleL : 1
    readonly property LyricsController controller: LyricsController {
        // a minimized window still counts as visible to its items
        enabled: root.visible && root.Window.visibility !== Window.Minimized && root.Window.visibility !== Window.Hidden
    }
    readonly property bool ready: controller.state === "ready"
    readonly property bool unsynced: ready && !controller.synced
    readonly property bool showFollow: ready && controller.synced && !following
    readonly property bool animationsEnabled: Theme.animationsEnabled
    readonly property bool playing: controller.playing
    readonly property real sampleTime: controller.sampleTime
    readonly property string trackKey: controller.trackKey
    readonly property var items: controller.synced ? LyricsModel.buildItems(controller.lines, PlayerMetrics.lyricsBreakGap, UiConfig.lyricsBackgroundVocals) : LyricsModel.plainItems(controller.plainLines)
    readonly property int trackedCentre: controller.synced ? LyricsModel.centreItem(items, sampleTime, controller.followIndex) : -1
    // only a word sweep or a break moves between the controller's cue ticks
    readonly property bool needsClock: {
        const item = items[centreItem];
        if (!item || item.kind === "plain")
            return false;
        if (item.kind === "break")
            return true;
        return animationsEnabled && UiConfig.lyricsWordSync && item.parts.some(part => part.w.length > 0);
    }
    readonly property bool userScrolling: transcript.isUserScrolling || transcript.dragging
    readonly property bool compact: width < PlayerMetrics.lyricsFollowLabelMinWidth
    readonly property real breakSize: Math.round(leadFontSize * PlayerMetrics.lyricsBreakIndicatorScale)
    // the ends leave room for half a single row line, so scrolling stops with the first or last line centred
    readonly property real endPadding: controller.synced ? Math.max(0, transcript.height / 2 - (leadFontMetrics.lineHeight + Theme.spacingS * 2) / 2) : Theme.spacingS
    readonly property real edgeFade: Math.min(transcript.height * PlayerMetrics.lyricsEdgeFadeRatio, PlayerMetrics.lyricsEdgeFadeMax)
    readonly property real leadFontSize: Math.round(textScale * Math.max(Theme.fontSizeXLarge, Math.min(Theme.fontSizeDisplay, transcript.height / PlayerMetrics.lyricsLeadHeightDivisor, transcript.width / PlayerMetrics.lyricsLeadWidthDivisor)))
    readonly property string message: PlayerService.lyricsMessage

    activeFocusOnTab: true
    Accessible.role: Accessible.Pane
    Accessible.name: I18n.tr("Lyrics", "song lyrics: page, toggle, settings tab")
    // a position correction can set the clock back a moment just after the view moved on, which would scroll
    // back to the previous row and forward again
    onTrackedCentreChanged: {
        const backstep = controller.synced && trackedCentre >= 0 && trackedCentre === centreItem - 1;
        if (backstep && sampleTime < centreTime && centreTime - sampleTime < PlayerMetrics.lyricsFollowBackstepSlack)
            return;
        centreTime = sampleTime;
        centreItem = trackedCentre;
    }
    onCentreItemChanged: requestFollow(true)
    Component.onCompleted: {
        canMeasure = true;
        centreItem = trackedCentre;
    }
    onItemsChanged: {
        centreTime = -Infinity;
        centreItem = trackedCentre;
        requestFollow(false);
    }
    onReadyChanged: requestFollow(false)
    onWidthChanged: requestFollow(false)
    onHeightChanged: requestFollow(false)
    onLeadFontSizeChanged: requestFollow(false)
    onSampleTimeChanged: {
        if (!clock.running)
            now = sampleTime;
    }
    onTrackKeyChanged: {
        following = true;
        scrollSpring.snapTo(transcript.contentY);
        transcript.stopMomentum();
        transcript.positionViewAtBeginning();
        requestFollow(false);
    }
    onUserScrollingChanged: {
        if (userScrolling)
            browse();
    }

    function browse() {
        following = false;
        scrollSpring.snapTo(transcript.contentY);
    }

    function resume() {
        following = true;
        forceActiveFocus(Qt.OtherFocusReason);
        requestFollow(true);
    }

    function requestFollow(animate) {
        animateFollow = animate && (followTimer.running ? animateFollow : true);
        followTimer.restart();
    }

    function followCurrent() {
        const animate = animateFollow && animationsEnabled;
        animateFollow = false;
        if (!ready || !controller.synced || !following || transcript.count === 0)
            return;
        const centre = Math.max(0, centreItem);
        transcript.stopMomentum();
        transcript.forceLayout();
        heightCache = {};
        const target = Math.round(centreOffset(centre));
        if (animate) {
            if (!scrollSpring.running)
                scrollSpring.snapTo(transcript.contentY);
            scrollSpring.retarget(target);
            // a disabled spring takes the target at once and leaves the view where it is
            if (scrollSpring.running)
                return;
        }
        scrollSpring.snapTo(transcript.contentY);
        // rows far off are only laid out by moving there, so a jump lets the view place them
        if (Math.abs(target - transcript.contentY) > transcript.height)
            transcript.positionViewAtIndex(centre, ListView.Center);
        else
            transcript.contentY = target;
    }

    // the same sum a row makes of its height, so a line without a row is placed exactly
    function itemHeight(index) {
        const item = items[index];
        if (!item)
            return 0;
        const layouts = item.kind === "line" ? LyricsModel.partLayouts(item.parts, transcript.width, leadFontMetrics, backgroundFontMetrics, Theme.spacingXS) : [];
        return LyricsModel.rowHeight(item, layouts, breakSize, Theme.spacingS, Theme.spacingXS);
    }

    // counted from the row in the middle of the view, which is always laid out for real: the view keeps far off rows
    // such as the first one at a guessed y, and lines without a row have none
    function centreOffset(centre) {
        let anchorIndex = transcript.indexAt(transcript.width / 2, transcript.contentY + transcript.height / 2);
        let anchor = transcript.itemAtIndex(anchorIndex);
        if (!anchor) {
            anchorIndex = Math.max(0, centreItem);
            anchor = transcript.itemAtIndex(anchorIndex);
        }
        if (!anchor)
            return transcript.contentY;
        let y = anchor.y;
        if (centre > anchorIndex) {
            for (let i = anchorIndex; i < centre; i++)
                y += cachedHeight(i);
        } else {
            for (let i = centre; i < anchorIndex; i++)
                y -= cachedHeight(i);
        }
        return y + cachedHeight(centre) / 2 - transcript.height / 2;
    }

    function cachedHeight(index) {
        if (!(index in heightCache))
            heightCache[index] = itemHeight(index);
        return heightCache[index];
    }

    function scrollBy(amount) {
        browse();
        transcript.stopMomentum();
        transcript.contentY = Math.max(transcript.originY, Math.min(transcript.maximumContentY, transcript.contentY + amount));
    }

    Keys.onPressed: event => {
        switch (event.key) {
        case Qt.Key_Up:
            scrollBy(-Theme.listItemHeight);
            break;
        case Qt.Key_Down:
            scrollBy(Theme.listItemHeight);
            break;
        case Qt.Key_PageUp:
            scrollBy(-transcript.height);
            break;
        case Qt.Key_PageDown:
            scrollBy(transcript.height);
            break;
        default:
            return;
        }
        event.accepted = true;
    }

    // a spring keeps its speed when the active line changes on the way, so the scroll bends towards the new line
    DC.SpringMotion {
        id: scrollSpring
        enabled: !Theme.springMotionDisabled
        stiffness: PlayerMetrics.lyricsScrollStiffness
        damping: PlayerMetrics.lyricsScrollDamping
        positionEpsilon: PlayerMetrics.lyricsScrollSettleDistance
        velocityEpsilon: PlayerMetrics.lyricsScrollSettleSpeed
        onValueChanged: {
            if (!running)
                return;
            transcript.contentY = value;
            retarget(Math.round(root.centreOffset(Math.max(0, root.centreItem))));
        }
    }

    Timer {
        id: followTimer
        interval: 0
        onTriggered: root.followCurrent()
    }

    FrameAnimation {
        id: clock
        running: root.controller.enabled && root.ready && root.controller.synced && root.playing && root.needsClock
        onTriggered: root.now = root.controller.currentTime()
        onRunningChanged: root.now = root.controller.currentTime()
    }

    Rectangle {
        anchors.fill: parent
        radius: root.radius
        color: root.cardColor
    }

    LineMeasure {
        id: leadFontMetrics
        font.pixelSize: root.leadFontSize
        font.weight: Theme.fontWeightBold
    }

    LineMeasure {
        id: backgroundFontMetrics
        font.pixelSize: Math.round(root.leadFontSize * Theme.fontSizeLarge / Theme.fontSizeXLarge)
        font.weight: Theme.fontWeightMedium
    }

    // rows are measured by the same text type that draws them, so a row never comes out wider than it was laid out
    component LineMeasure: Item {
        property alias font: probe.font
        readonly property real lineHeight: metrics.height

        visible: false

        function advanceWidth(text) {
            probe.text = text;
            return probe.implicitWidth;
        }

        StyledText {
            id: probe
            wrapMode: Text.NoWrap
            elide: Text.ElideNone
        }

        FontMetrics {
            id: metrics
            font: probe.font
        }
    }

    StyledText {
        id: heading
        anchors.top: parent.top
        anchors.topMargin: Theme.spacingM
        anchors.horizontalCenter: parent.horizontalCenter
        width: parent.width - Theme.spacingM * 2
        text: I18n.tr("Unsynced", "lyrics have no timestamps")
        color: Theme.onSurfaceVariant
        font.pixelSize: Theme.fontSizeSmall
        horizontalAlignment: Text.AlignHCenter
        visible: root.unsynced
    }

    DListView {
        id: transcript
        // a fade painted over the rows lets them show through a translucent window, cut off at the list's edge
        fadeEdges: false
        showScrollBar: false
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: heading.visible ? heading.bottom : parent.top
        anchors.bottom: parent.bottom
        anchors.margins: Theme.spacingL
        clip: true
        visible: root.ready
        reuseItems: true
        cacheBuffer: Math.max(0, Math.round(height / 2))
        model: root.canMeasure ? root.items : []
        add: null
        remove: null
        displaced: null
        move: null
        header: Item {
            width: transcript.width
            height: root.endPadding
        }
        footer: Item {
            width: transcript.width
            height: Math.max(root.endPadding, credit.implicitHeight + Theme.spacingM * 2)

            StyledText {
                id: credit
                anchors.top: parent.top
                anchors.topMargin: Theme.spacingM
                anchors.horizontalCenter: parent.horizontalCenter
                width: Math.min(implicitWidth, parent.width)
                text: root.controller.attributionName
                color: Theme.outline
                font.pixelSize: Theme.fontSizeSmall
                elide: Text.ElideRight
                wrapMode: Text.NoWrap
            }

            StateLayer {
                anchors.fill: credit
                anchors.margins: -Theme.spacingXS
                cornerRadius: Theme.cornerRadius
                stateColor: Theme.outline
                visible: root.controller.attributionUrl !== ""
                tooltipText: root.controller.attributionUrl
                onClicked: Qt.openUrlExternally(root.controller.attributionUrl)
            }
        }
        onCountChanged: root.requestFollow(false)

        delegate: LyricsLine {
            id: line

            required property var modelData
            readonly property bool live: Math.abs(index - root.centreItem) <= 1
            // from the row's outer edge, so it is gone before the list's clip can cut it
            readonly property real edgeDistance: Math.min(y - transcript.contentY, transcript.contentY + transcript.height - y - height)

            width: transcript.width
            opacity: root.edgeFade > 0 ? Math.max(0, Math.min(1, edgeDistance / root.edgeFade)) : 1
            item: modelData
            controller: root.controller
            accents: root.accents
            clock: {
                if (!root.controller.synced)
                    return -1;
                if (line.live)
                    return root.now;
                return line.index < root.centreItem ? Infinity : -1;
            }
            promoted: root.controller.synced && index === root.centreItem
            synced: root.controller.synced
            following: root.following
            playing: root.playing
            animationsEnabled: root.animationsEnabled
            leadFontSize: root.leadFontSize
            leadMetrics: leadFontMetrics
            backgroundMetrics: backgroundFontMetrics
            breakSize: root.breakSize
            dimColor: root.artScheme?.surfaceVariantText ?? Theme.onSurfaceVariant
            onActivated: {
                root.following = true;
                root.forceActiveFocus(Qt.MouseFocusReason);
                if (line.item.kind === "break")
                    root.controller.seekToTime(line.item.t);
                else
                    root.controller.seekToLine(line.item.line);
            }
        }
    }

    DIconButton {
        id: sourceButton
        anchors.top: parent.top
        anchors.right: parent.right
        anchors.margins: Theme.spacingS
        visible: root.ready
        iconName: "more_vert"
        Accessible.name: I18n.tr("Lyrics source", "button choosing which provider's lyrics a track uses")
        // asking every provider can take a while, so the menu opens at once and the sources join it when they answer
        onClicked: {
            const track = sourceMenu.track;
            sourceMenu.open(sourceButton);
            if (sourceMenu.choicesFor === track)
                return;
            PlayerService.lyricsChoices(list => {
                if (sourceMenu.track !== track)
                    return;
                sourceMenu.choices = list;
                sourceMenu.choicesFor = track;
            });
        }
    }

    DMenu {
        id: sourceMenu

        readonly property string track: PlayerService.current ? PlayerService.current.library + ":" + PlayerService.current.id : ""
        property var choices: []
        property string choicesFor: ""
        readonly property bool auto: !PlayerService.lyricsChosen
        readonly property string shown: PlayerService.lyrics?.source ?? ""
        readonly property var sources: [
            {
                "id": "",
                "text": I18n.tr("Automatic", "Automatic"),
                "checked": auto
            }
        ].concat((choicesFor === track ? choices : []).map(c => ({
                    "id": c.source,
                    "text": I18n.tr("%1 · %2", "lyrics source and its timing").arg(nameOf(c.source)).arg(c.timing === "word" ? I18n.tr("Word by word", "Word by word") : c.timing === "line" ? I18n.tr("Line by line", "lyrics timing of a source") : I18n.tr("Not synced", "lyrics timing of a source")),
                    "info": statusText(c.status),
                    "checked": !auto && c.source === shown
                })))
        function nameOf(source) {
            if (source === "sidecar")
                return I18n.tr("Lyrics file", "lyrics source: a file next to the track");
            return choices.find(c => c.source === source)?.name ?? source;
        }

        function signed(seconds) {
            const rounded = Math.round(seconds * 10) / 10;
            return (rounded < 0 ? "\u2212" : "+") + Math.abs(rounded).toFixed(1);
        }

        // what the last automatic pick measured and did with a source's lyrics
        function statusText(status) {
            const seconds = status?.seconds ?? 0;
            switch (status?.kind) {
            case "shown-on-time":
                return I18n.tr("Shown · on time (%1 s)", "lyrics source status: shown, its timing measured on time by this shift").arg(signed(seconds));
            case "moved":
                return I18n.tr("Shown · auto-moved %1 s", "lyrics source status: shown, timing moved automatically by this shift").arg(signed(seconds));
            case "aligned":
                return I18n.tr("Shown · timing aligned to %1", "lyrics source status: shown, its words placed on the line timing of the named source").arg(nameOf(status.source ?? ""));
            case "shown":
                return I18n.tr("Shown · not changed", "lyrics source status: shown as the source sent it");
            case "on-time":
                return I18n.tr("On time (%1 s)", "lyrics source status: timing measured on time by this shift").arg(signed(seconds));
            case "late":
                return I18n.tr("%1 s late", "lyrics source status: timing measured this many seconds late").arg(Math.abs(seconds).toFixed(1));
            case "early":
                return I18n.tr("%1 s early", "lyrics source status: timing measured this many seconds early").arg(Math.abs(seconds).toFixed(1));
            case "file":
                return I18n.tr("Lyrics file · not changed", "lyrics source status: the user's lyrics file, never moved");
            }
            return I18n.tr("Not measured", "lyrics source status: timing not measured against the audio");
        }

        // what the user set until the daemon answers it, so an older answer doesn't move the slider back
        property real requested: NaN

        function send() {
            if (isNaN(requested))
                return;
            const track = sourceMenu.track;
            const sent = requested;
            // a newer value is still on its way; it clears itself when answered
            PlayerService.setLyricsOffset(sent, () => {
                if (sourceMenu.track === track && sourceMenu.requested === sent)
                    sourceMenu.requested = NaN;
            });
        }

        // the slider comes first, so sources answering after the menu opened join below it
        model: [
            {
                "id": "timing",
                "slider": true,
                "text": I18n.tr("Lyrics timing", "lyrics menu slider showing a track's lyrics earlier or later"),
                "value": isNaN(requested) ? PlayerService.lyricsOffset : requested,
                "from": -PlayerMetrics.lyricsNudgeMax,
                "to": PlayerMetrics.lyricsNudgeMax,
                "stepSize": PlayerMetrics.lyricsNudgeStep,
                "unit": " " + I18n.tr("s", "unit suffix for seconds"),
                "resetValue": 0
            },
            {
                "divider": true
            }
        ].concat(sources)
        onOpenedChanged: {
            nudgeThrottle.stop();
            requested = NaN;
        }
        onTrackChanged: {
            nudgeThrottle.stop();
            requested = NaN;
        }
        // a drag sends at most every lyricsNudgeInterval, so the lyrics move along without flooding the daemon
        onValueMoved: (id, value) => {
            requested = value;
            if (!nudgeThrottle.running)
                nudgeThrottle.start();
        }
        onValueSet: (id, value) => {
            requested = value;
            nudgeThrottle.stop();
            send();
        }
        onTriggered: id => PlayerService.chooseLyrics(id)

        Timer {
            id: nudgeThrottle
            interval: PlayerMetrics.lyricsNudgeInterval
            onTriggered: sourceMenu.send()
        }
    }

    DButton {
        id: followButton
        readonly property string label: I18n.tr("Follow playback", "resume automatic scrolling of lyrics")

        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.bottom
        anchors.bottomMargin: Theme.spacingL
        buttonHeight: Theme.buttonHeightXS
        visible: root.showFollow
        text: root.compact ? "" : label
        tooltipText: root.compact ? label : ""
        Accessible.name: label
        iconName: "my_location"
        backgroundColor: ArtColors.accentContainer
        textColor: ArtColors.onAccentContainer
        onClicked: root.resume()
    }

    DLoadingIndicator {
        anchors.centerIn: parent
        visible: root.controller.pending
        contained: true
        Accessible.role: Accessible.ProgressBar
        Accessible.name: I18n.tr("Loading lyrics", "lyrics loading indicator")
    }

    StyledText {
        anchors.centerIn: parent
        width: parent.width - Theme.spacingL * 2
        visible: !root.ready && root.message !== ""
        text: root.message
        color: Theme.onSurfaceVariant
        font.pixelSize: Theme.fontSizeLarge
        horizontalAlignment: Text.AlignHCenter
        wrapMode: Text.WordWrap
    }
}
