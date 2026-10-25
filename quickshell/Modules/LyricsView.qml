pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// Synced lyrics with word highlight, modelled on the shell's LyricsOverlay.
FocusScope {
    id: root

    property color backgroundColor: "transparent"
    property var artScheme: null
    property real radius: 0
    property bool following: true
    property bool followSnap: false
    property Item followedItem: null
    property real followedY: 0

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
    readonly property int activeIndex: controller.activeIndex
    readonly property int firstFocusedIndex: controller.focusedGroups[0] ?? -1
    readonly property string trackKey: controller.trackKey
    readonly property string shownResult: controller.shownResult
    readonly property bool userScrolling: transcript.isUserScrolling || transcript.dragging
    readonly property real currentLineY: transcript.currentItem?.y ?? 0
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
    onActiveIndexChanged: followTimer.restart()
    onFirstFocusedIndexChanged: followTimer.restart()
    onReadyChanged: snapToCurrent()
    onShownResultChanged: snapToCurrent()
    onWidthChanged: snapToCurrent()
    onHeightChanged: snapToCurrent()
    onTrackKeyChanged: {
        following = true;
        followMotion.stop();
        transcript.stopMomentum();
        transcript.positionViewAtBeginning();
    }
    onUserScrollingChanged: {
        if (userScrolling)
            browse();
    }
    onCurrentLineYChanged: {
        if (!followedItem || transcript.currentItem !== followedItem || transcript.currentIndex !== Math.max(0, activeIndex))
            return;
        const shift = currentLineY - followedY;
        followedY = currentLineY;
        if (shift === 0 || !following)
            return;
        if (followMotion.running) {
            followTimer.restart();
            return;
        }
        transcript.contentY = Math.max(transcript.originY, Math.min(transcript.maximumContentY, transcript.contentY + shift));
    }

    function browse() {
        following = false;
        followMotion.stop();
    }

    function snapToCurrent() {
        followSnap = true;
        followTimer.restart();
    }

    function followCurrent() {
        const snap = followSnap;
        followSnap = false;
        if (!ready || !controller.synced || !following || transcript.count === 0)
            return;
        followMotion.stop();
        transcript.stopMomentum();
        const previous = transcript.contentY;
        transcript.currentIndex = Math.max(0, activeIndex);
        transcript.forceLayout();
        transcript.positionViewAtIndex(transcript.currentIndex, ListView.Center);
        if (transcript.currentItem && transcript.currentItem.height > transcript.height)
            transcript.positionViewAtIndex(transcript.currentIndex, ListView.Beginning);
        let target = transcript.contentY;
        const first = transcript.itemAtIndex(controller.focusedGroups[0] ?? transcript.currentIndex);
        const last = transcript.currentItem;
        if (first && last && first !== last && first.y < target && last.y + last.height - first.y <= transcript.height)
            target = Math.max(transcript.originY, Math.min(transcript.maximumContentY, first.y));
        target = Math.round(target);
        transcript.contentY = target;
        followedItem = last;
        followedY = last?.y ?? 0;
        if (snap || !animationsEnabled || Math.abs(target - previous) > transcript.height)
            return;
        transcript.contentY = previous;
        followMotion.from = previous;
        followMotion.to = target;
        followMotion.restart();
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

    Timer {
        id: followTimer
        interval: 0
        onTriggered: root.followCurrent()
    }

    NumberAnimation {
        id: followMotion
        target: transcript
        property: "contentY"
        duration: Theme.expressiveDurations.expressiveDefaultSpatial
        easing.type: Easing.BezierSpline
        easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
    }

    Rectangle {
        anchors.fill: parent
        radius: root.radius
        color: root.backgroundColor
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
        anchors.bottom: followButton.top
        anchors.margins: Theme.spacingL
        clip: true
        visible: root.ready
        reuseItems: true
        spacing: Theme.spacingXXS
        model: root.controller.synced ? root.controller.lines : root.controller.plainLines
        add: null
        remove: null
        displaced: null
        move: null
        header: Item {
            height: root.controller.synced ? transcript.height / 3 : Theme.spacingS
        }
        footer: Item {
            width: transcript.width
            height: Math.max(root.controller.synced ? transcript.height / 3 : Theme.spacingS, credit.implicitHeight + Theme.spacingM * 2)

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
        onCountChanged: followTimer.restart()

        delegate: Item {
            id: lyric
            required property int index
            required property var modelData
            width: transcript.width
            height: vocals.implicitHeight

            // Positioners relayout on polish, so the list would measure reused and rescaled rows at their old height.
            Column {
                id: vocals
                width: parent.width

                Repeater {
                    onItemAdded: vocals.forceLayout()
                    model: root.controller.synced ? lyric.modelData.parts.filter(part => UiConfig.lyricsBackgroundVocals || !part.background) : [
                        {
                            x: lyric.modelData,
                            w: [],
                            cues: [],
                            side: 0,
                            voiceName: "",
                            background: false
                        }
                    ]

                    delegate: LyricsVocal {
                        required property var modelData
                        width: vocals.width
                        controller: root.controller
                        part: modelData
                        accent: modelData.chorus ? root.accents[0] : root.accents[(modelData.voiceIndex ?? 0) % root.accents.length]
                        synced: root.controller.synced
                        following: root.following
                        leadFontSize: root.leadFontSize
                        dimColor: root.artScheme?.surfaceVariantText ?? Theme.onSurfaceVariant
                        distance: lyric ? Math.abs(lyric.index - root.activeIndex) : 0
                        animationsEnabled: root.animationsEnabled
                        onImplicitHeightChanged: vocals.forceLayout()
                        inViewport: {
                            if (!lyric)
                                return false;
                            const top = lyric.y + y;
                            return top + height > transcript.contentY && top < transcript.contentY + transcript.height;
                        }
                    }
                }
            }

            StateLayer {
                anchors.fill: parent
                enabled: root.controller.canSeekLines
                stateColor: Theme.onSurfaceVariant
                cornerRadius: Theme.cornerRadius
                onClicked: {
                    root.following = true;
                    root.forceActiveFocus(Qt.MouseFocusReason);
                    root.controller.seekToLine(lyric.index);
                }
            }
        }
    }

    DButton {
        id: followButton
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.bottom
        anchors.bottomMargin: Theme.spacingM
        buttonHeight: Theme.buttonHeightXS
        height: root.showFollow ? implicitHeight : 0
        visible: root.showFollow
        text: I18n.tr("Follow playback", "resume automatic scrolling of lyrics")
        iconName: "my_location"
        backgroundColor: Theme.secondaryContainer
        textColor: Theme.onSecondaryContainer
        onClicked: {
            root.following = true;
            root.forceActiveFocus(Qt.OtherFocusReason);
            root.followCurrent();
        }
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
