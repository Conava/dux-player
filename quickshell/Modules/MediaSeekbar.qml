pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Common as DC
import qs.DCommon.Widgets
import "../Common/SeekTrack.js" as SeekTrack

// The shell's wavy seekbar on the player's position clock: the wave moves only while shown and playing, and lies
// flat while paused. Expressive (Bento): a thicker wave and a round handle on an opaque rail. Flat (Material): the
// M3 slider, thin in the bar.
Item {
    id: root

    readonly property real length: PlayerService.duration
    // false: a progress bar only (no seeking, no focus)
    property bool interactive: true
    property bool expressive: false
    property bool flat: false
    property bool thin: false
    property color fillColor: ArtColors.accent
    property color trackColor: expressive ? ArtColors.accentRail : ArtColors.accentTrack
    // the handle keeps half its width from the ends, so the track maps onto the span between them
    readonly property real handleInset: flat ? Theme.sliderHandleWidth / 2 : expressive ? handleWidth / 2 : 0
    property real handleWidth: flat ? (isSeeking ? Theme.sliderHandleWidthPressed : Theme.sliderHandleWidth) : isSeeking ? PlayerMetrics.mediaSeekbarHandleDragWidth : PlayerMetrics.mediaSeekbarHandleSize
    property real handleHeight: flat ? (thin ? PlayerMetrics.mediaSeekbarHeight : Theme.sliderHandleHeight) : isSeeking ? PlayerMetrics.mediaSeekbarHandleDragHeight : PlayerMetrics.mediaSeekbarHandleSize
    readonly property bool usable: interactive && enabled && length > 0 && PlayerService.active
    readonly property bool canSeek: usable && PlayerService.seekable
    // a track that can't seek dims the bar and says why when it is used
    readonly property bool explains: usable && !PlayerService.seekable
    // a minimized window still counts as visible to its items
    readonly property bool shown: visible && Window.visibility !== Window.Minimized && Window.visibility !== Window.Hidden
    readonly property bool playing: PlayerService.playing && !PlayerService.loading
    // 1 while playing, easing to 0 on pause; the wave keeps moving until it is flat
    property real waveLevel: playing ? 1 : 0
    // M3WaveProgress's own default amplitude
    readonly property real flatBarAmplitude: 1.6
    property real playerValue: 0
    property real seekPreviewRatio: -1
    // 0–1, the preview while seeking
    readonly property real value: seekPreviewRatio >= 0 ? seekPreviewRatio : playerValue
    property bool isSeeking: false
    property real committedSeekRatio: -1
    property int settleChecks: 0

    function clampRatio(ratio) {
        return Math.max(0, Math.min(1, ratio));
    }

    // a 0–1 ratio as a position along the drawn track
    function trackValue(ratio) {
        return width > 0 ? SeekTrack.handleCenter(ratio, width, handleInset) / width : ratio;
    }

    function ratioOf(seconds) {
        return length > 0 ? clampRatio(seconds / length) : 0;
    }

    function seekTo(seconds) {
        if (explains)
            PlayerService.explainNoSeek();
        if (!canSeek)
            return;
        const clamped = Math.max(0, Math.min(seconds, length * 0.99));
        PlayerService.seek(clamped);
        seekPreviewRatio = ratioOf(clamped);
        committedSeekRatio = seekPreviewRatio;
        settleChecks = 15;
        settle.restart();
    }

    function clearPreview() {
        settle.stop();
        committedSeekRatio = -1;
        if (!isSeeking)
            seekPreviewRatio = -1;
    }

    implicitHeight: flat ? handleHeight : expressive ? PlayerMetrics.mediaSeekbarExpressiveHeight : PlayerMetrics.mediaSeekbarHeight
    activeFocusOnTab: usable
    LayoutMirroring.enabled: false
    LayoutMirroring.childrenInherit: true
    Accessible.role: interactive ? Accessible.Slider : Accessible.ProgressBar
    Accessible.name: I18n.tr("Position", "seekbar accessible name")
    Accessible.focusable: usable
    Accessible.onIncreaseAction: seekTo(value * length + 5)
    Accessible.onDecreaseAction: seekTo(value * length - 5)

    Keys.onPressed: event => {
        if (!usable)
            return;
        switch (event.key) {
        case Qt.Key_Left:
            seekTo(value * length - 5);
            break;
        case Qt.Key_Right:
            seekTo(value * length + 5);
            break;
        case Qt.Key_PageUp:
            seekTo(value * length + length / 10);
            break;
        case Qt.Key_PageDown:
            seekTo(value * length - length / 10);
            break;
        case Qt.Key_Home:
            seekTo(0);
            break;
        case Qt.Key_End:
            seekTo(length);
            break;
        default:
            return;
        }
        event.accepted = true;
    }

    Binding on playerValue {
        value: root.ratioOf(PlayerService.position)
        when: !frames.running
        restoreMode: Binding.RestoreNone
    }

    // follows the running position at the display's frame rate
    FrameAnimation {
        id: frames
        running: root.shown && PlayerService.positionRunning
        onTriggered: root.playerValue = root.ratioOf(PlayerService.livePosition())
    }

    PositionHold {
        active: root.shown
    }

    // after a seek the preview holds until the reported position catches up
    Timer {
        id: settle
        interval: 80
        repeat: true
        onTriggered: {
            if (root.isSeeking || root.committedSeekRatio < 0 || Math.abs(root.playerValue - root.committedSeekRatio) <= 0.0015 || root.settleChecks <= 0) {
                root.clearPreview();
                return;
            }
            root.settleChecks -= 1;
        }
    }

    FocusRing {
        radius: Theme.cornerRadiusXS + Theme.focusRingOffset
        visible: root.activeFocus
    }

    Behavior on waveLevel {
        enabled: Theme.animationsEnabled && root.shown
        DC.DAnim {
            duration: Theme.expressiveDurations.expressiveDefaultSpatial
            easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
        }
    }

    Behavior on handleWidth {
        enabled: Theme.animationsEnabled
        NumberAnimation {
            duration: Theme.expressiveDurations.expressiveEffects
            easing.type: Easing.BezierSpline
            easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
        }
    }

    Behavior on handleHeight {
        enabled: Theme.animationsEnabled
        NumberAnimation {
            duration: Theme.expressiveDurations.expressiveEffects
            easing.type: Easing.BezierSpline
            easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
        }
    }

    M3WaveProgress {
        anchors.fill: parent
        visible: !root.flat && root.length > 0
        opacity: root.explains ? Theme.disabledOpacity : 1
        value: root.trackValue(root.value)
        actualValue: root.trackValue(root.playerValue)
        // the handle carries the played wave along, so a drag shows no separate preview
        showActualPlaybackState: root.isSeeking && !root.expressive
        showHandle: root.expressive
        handleWidth: root.handleWidth
        handleHeight: root.handleHeight
        handleGap: PlayerMetrics.mediaSeekbarHandleGap
        handleColor: root.fillColor
        fillColor: root.fillColor
        playheadColor: root.fillColor
        trackColor: root.trackColor
        actualProgressColor: ArtColors.accentSubtle
        isPlaying: root.waveLevel > 0 && root.shown

        amp: (root.expressive ? PlayerMetrics.mediaSeekbarExpressiveAmplitude : root.flatBarAmplitude) * root.waveLevel

        Binding on lineWidth {
            when: root.expressive
            value: PlayerMetrics.mediaSeekbarExpressiveLineWidth
        }

        Binding on wavelength {
            when: root.expressive
            value: PlayerMetrics.mediaSeekbarExpressiveWavelength
        }
    }

    // the M3 slider: active track, handle, a gap either side, the inactive track with a stop at its end
    Item {
        id: flatTrack
        readonly property real trackHeight: root.thin ? Theme.sliderHandleWidth : Theme.sliderTrackHeight
        readonly property real outsideCorner: Theme.scaledRadius(DC.Style.sliderTrackCornerRadius, trackHeight / 2)
        readonly property real insideCorner: Theme.scaledRadius(DC.Style.sliderTrackInsideCornerRadius, trackHeight / 2)
        anchors.fill: parent
        visible: root.flat && root.length > 0
        opacity: root.explains ? Theme.disabledOpacity : 1

        Rectangle {
            anchors.verticalCenter: parent.verticalCenter
            width: Math.max(0, handle.x - Theme.sliderHandleGap)
            height: flatTrack.trackHeight
            // no hairline at the start
            visible: width >= flatTrack.outsideCorner
            topLeftRadius: flatTrack.outsideCorner
            bottomLeftRadius: flatTrack.outsideCorner
            topRightRadius: flatTrack.insideCorner
            bottomRightRadius: flatTrack.insideCorner
            color: root.fillColor
        }

        Rectangle {
            id: handle
            x: SeekTrack.handleCenter(root.value, root.width, root.handleInset) - width / 2
            anchors.verticalCenter: parent.verticalCenter
            width: root.handleWidth
            height: root.handleHeight
            radius: Theme.fullRadius(width, height)
            color: root.fillColor
        }

        Rectangle {
            x: handle.x + handle.width + Theme.sliderHandleGap
            anchors.verticalCenter: parent.verticalCenter
            width: Math.max(0, parent.width - x)
            height: flatTrack.trackHeight
            visible: width > 0
            topLeftRadius: flatTrack.insideCorner
            bottomLeftRadius: flatTrack.insideCorner
            topRightRadius: flatTrack.outsideCorner
            bottomRightRadius: flatTrack.outsideCorner
            color: root.trackColor

            Rectangle {
                anchors.right: parent.right
                anchors.rightMargin: (flatTrack.trackHeight - width) / 2
                anchors.verticalCenter: parent.verticalCenter
                width: Theme.sliderStopSize
                height: width
                radius: Theme.fullRadius(width, height)
                visible: parent.width > Theme.sliderHandleGap * 2 + width
                color: root.fillColor
            }
        }
    }

    MouseArea {
        id: mouse
        anchors.fill: parent
        cursorShape: root.canSeek ? Qt.PointingHandCursor : Qt.ArrowCursor
        enabled: root.usable

        function preview(x) {
            root.seekPreviewRatio = SeekTrack.ratioAt(x, root.width, root.handleInset);
        }

        onPressed: event => {
            if (root.explains) {
                PlayerService.explainNoSeek();
                return;
            }
            root.clearPreview();
            root.isSeeking = true;
            preview(event.x);
        }
        onPositionChanged: event => {
            if (pressed && root.isSeeking)
                preview(event.x);
        }
        onReleased: {
            if (!root.isSeeking)
                return;
            root.isSeeking = false;
            root.seekTo(root.seekPreviewRatio * root.length);
        }
        onCanceled: {
            root.isSeeking = false;
            root.clearPreview();
        }
    }
}
