pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// The shell's wavy seekbar on the player's position clock: the wave moves only while shown and playing.
Item {
    id: root

    readonly property real length: PlayerService.duration
    // false: a progress bar only (no seeking, no focus)
    property bool interactive: true
    property color fillColor: ArtColors.accent
    property color trackColor: ArtColors.accentTrack
    readonly property bool canSeek: interactive && enabled && length > 0 && PlayerService.current !== null
    // a minimized window still counts as visible to its items
    readonly property bool shown: visible && Window.visibility !== Window.Minimized && Window.visibility !== Window.Hidden
    readonly property bool playing: PlayerService.playing && !PlayerService.loading
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

    function ratioOf(seconds) {
        return length > 0 ? clampRatio(seconds / length) : 0;
    }

    function seekTo(seconds) {
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

    implicitHeight: PlayerMetrics.mediaSeekbarHeight
    activeFocusOnTab: canSeek
    LayoutMirroring.enabled: false
    LayoutMirroring.childrenInherit: true
    Accessible.role: interactive ? Accessible.Slider : Accessible.ProgressBar
    Accessible.name: I18n.tr("Position", "seekbar accessible name")
    Accessible.focusable: canSeek
    Accessible.onIncreaseAction: seekTo(value * length + 5)
    Accessible.onDecreaseAction: seekTo(value * length - 5)

    Keys.onPressed: event => {
        if (!canSeek)
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

    M3WaveProgress {
        anchors.fill: parent
        visible: root.length > 0
        value: root.value
        actualValue: root.playerValue
        showActualPlaybackState: root.isSeeking
        fillColor: root.fillColor
        playheadColor: root.fillColor
        trackColor: root.trackColor
        actualProgressColor: ArtColors.accentSubtle
        isPlaying: root.playing && root.shown

        MouseArea {
            id: mouse
            anchors.fill: parent
            cursorShape: root.canSeek ? Qt.PointingHandCursor : Qt.ArrowCursor
            enabled: root.canSeek

            function preview(x) {
                root.seekPreviewRatio = root.clampRatio(x / width);
            }

            onPressed: event => {
                root.clearPreview();
                root.isSeeking = true;
                preview(event.x);
            }
            onPositionChanged: event => {
                if (pressed)
                    preview(event.x);
            }
            onReleased: {
                root.isSeeking = false;
                root.seekTo(root.seekPreviewRatio * root.length);
            }
            onCanceled: {
                root.isSeeking = false;
                root.clearPreview();
            }
        }
    }
}
