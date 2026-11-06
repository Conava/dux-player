pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Common as DC
import qs.DCommon.Widgets
import "../Common/LyricsModel.js" as LyricsModel

// Synced lyrics with a progressive word fill, modelled on the shell's LyricsOverlay.
FocusScope {
    id: root

    property color backgroundColor: "transparent"
    property var artScheme: null
    property real radius: 0
    property bool following: true
    property real now: 0
    property int pushedItem: -1
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
    readonly property real leadFontSize: Math.round(textScale * Math.max(Theme.fontSizeXLarge, Math.min(Theme.fontSizeDisplay, transcript.height / PlayerMetrics.lyricsLeadHeightDivisor, transcript.width / PlayerMetrics.lyricsLeadWidthDivisor)))
    readonly property string message: {
        switch (controller.state) {
        case "instrumental":
            return I18n.tr("Instrumental", "track has no lyrics because it is instrumental");
        case "error":
            return I18n.tr("Lyrics unavailable", "lyrics lookup failed");
        case "none":
            return I18n.tr("No lyrics found", "lyrics lookup found nothing");
        }
        return "";
    }

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
        pushedItem = -1;
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
        settleRows();
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

    function settleRows() {
        for (const row of transcript.contentItem.children) {
            if (row.settle)
                row.settle();
        }
    }

    // the view snaps to the new centre and every row is pushed back by the same distance, so the rows catch up one
    // after another: the new line leads and the others follow outwards
    function followCurrent() {
        const animate = animateFollow && animationsEnabled && pushedItem >= 0;
        animateFollow = false;
        if (!ready || !controller.synced || !following || transcript.count === 0)
            return;
        const centre = Math.max(0, centreItem);
        const jump = Math.abs(centre - pushedItem) > 1;
        pushedItem = centre;
        // a scroll on its way retargets to the active line on every step
        if (scrollSpring.running)
            return;
        transcript.stopMomentum();
        transcript.forceLayout();
        heightCache = {};
        const target = Math.round(centreOffset(centre));
        const delta = target - transcript.contentY;
        if (delta === 0)
            return;
        // rows far off are only laid out by moving there, so a jump lets the view place them
        if (!animate && Math.abs(delta) > transcript.height) {
            settleRows();
            transcript.positionViewAtIndex(centre, ListView.Center);
            return;
        }
        // further than a view away the rows cannot catch up one by one, so the view scrolls there
        if (animate && Math.abs(delta) > transcript.height) {
            settleRows();
            scrollSpring.snapTo(transcript.contentY);
            scrollSpring.retarget(target);
            return;
        }
        transcript.contentY = target;
        if (!animate) {
            settleRows();
            return;
        }
        transcript.forceLayout();
        for (const row of transcript.contentItem.children) {
            if (!row.push || row.index < 0)
                continue;
            const lag = Math.abs(centre - row.index);
            row.push(delta, jump ? 0 : Math.min(lag, PlayerMetrics.lyricsTrackMaxPush) * row.moveDuration * PlayerMetrics.lyricsTrackPushShare);
        }
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
            transcript.contentY = Math.round(value);
            retarget(root.centreOffset(Math.max(0, root.centreItem)));
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
        color: root.backgroundColor
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
        fadeEdges: true
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

            width: transcript.width
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
