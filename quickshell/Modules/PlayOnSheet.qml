pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Common as DC
import qs.DCommon.Widgets
import "../DCommon/Common/FocusNavigation.js" as FocusNavigation

// Bento's "Play on" sheet over the deck: the playback devices, the selected one with the player's volume. Material's
// shared axis: the deck's controls slide out and fade first, then the sheet slides in from the other side. Tab stays
// inside while it shows; Escape or Back closes it and focus returns to the button that opened it.
FocusScope {
    id: root

    readonly property bool opened: Router.playOnOpen
    readonly property real progress: motion.value
    readonly property real slideDistance: PlayerMetrics.mediaSheetSlideDistance * (I18n.isRtl ? -1 : 1)
    // the host's controls follow these
    readonly property real controlsOpacity: Math.max(0, 1 - progress / PlayerMetrics.mediaSheetFadeThreshold)
    readonly property real controlsShift: -slideDistance * progress
    readonly property var spring: Theme.springPreset("default", Theme.expressiveDurations.expressiveDefaultSpatial)
    readonly property Item focusItem: Window.activeFocusItem
    property Item lastInside: null
    property bool closedByKey: false

    function contains(item) {
        for (let p = item; p; p = p.parent) {
            if (p === root)
                return true;
        }
        return false;
    }

    // the last control in here along the tab chain
    function lastFocusable() {
        let last = backButton;
        let next = backButton.nextItemInFocusChain(true);
        for (let n = 0; next && next !== backButton && contains(next) && n < 200; n++) {
            last = next;
            next = next.nextItemInFocusChain(true);
        }
        return last;
    }

    function close(byKey) {
        closedByKey = byKey;
        Router.playOnOpen = false;
    }

    visible: progress > 0
    enabled: opened
    opacity: Math.max(0, Math.min(1, (progress - PlayerMetrics.mediaSheetFadeThreshold) / (1 - PlayerMetrics.mediaSheetFadeThreshold)))
    transform: Translate {
        x: root.slideDistance * (1 - root.progress)
    }
    Accessible.role: Accessible.Pane
    Accessible.name: title.text

    onOpenedChanged: {
        motion.retarget(opened ? 1 : 0);
        PlayerService.watchCastDevices(opened);
        if (opened) {
            closedByKey = false;
            list.reset();
            flickable.contentY = 0;
            // this sheet and the deck's controls swap enabled only once this change has gone through
            Qt.callLater(() => backButton.forceActiveFocus(Qt.OtherFocusReason));
            return;
        }
        if (lastInside === null)
            return;
        lastInside = null;
        const from = Router.playOnFrom;
        Qt.callLater(() => {
            // the view closed with it, and what closed it placed the focus
            if (!Router.nowPlayingOpen)
                return;
            if (root.closedByKey)
                FocusNavigation.focusItem(from, false);
            else
                from?.forceActiveFocus(Qt.OtherFocusReason);
        });
    }
    // Tab from the last control (or Backtab from the first) leaves the sheet: wrap around instead
    onFocusItemChanged: {
        if (!focusItem)
            return;
        if (contains(focusItem)) {
            lastInside = focusItem;
            return;
        }
        if (!opened || lastInside === null)
            return;
        const target = lastInside === backButton ? lastFocusable() : backButton;
        lastInside = null;
        target.forceActiveFocus(Qt.TabFocusReason);
    }
    Component.onDestruction: {
        if (opened) {
            PlayerService.watchCastDevices(false);
            Router.playOnOpen = false;
        }
    }

    readonly property EscapeLayer _escapeLayer: EscapeLayer {
        owner: root
        active: root.opened
        onEscaped: root.close(true)
    }

    DC.SpringMotion {
        id: motion
        enabled: !Theme.springMotionDisabled
        reducedMotion: !Theme.animationsEnabled
        stiffness: root.spring.stiffness
        damping: root.spring.damping
        positionEpsilon: PlayerMetrics.mediaSheetSettleProgress
    }

    Rectangle {
        anchors.fill: parent
        radius: Theme.cornerRadiusXL
        color: ArtColors.cardColor
    }

    Row {
        id: header
        x: Theme.spacingL
        y: Theme.spacingL
        width: parent.width - Theme.spacingL * 2
        spacing: Theme.spacingM

        DIconButton {
            id: backButton
            variant: "tonal"
            round: false
            iconName: I18n.isRtl ? "arrow_forward" : "arrow_back"
            containerColor: ArtColors.accentSecondaryContainer
            contentColor: ArtColors.onAccentSecondaryContainer
            tooltipText: I18n.tr("Back", "page header back button")
            Accessible.name: tooltipText
            onClicked: root.close(false)
        }

        StyledText {
            id: title
            anchors.verticalCenter: parent.verticalCenter
            width: parent.width - backButton.width - parent.spacing
            text: I18n.tr("Play on", "now playing sheet choosing the playback device")
            color: ArtColors.text
            font.pixelSize: Theme.fontSizeXLarge
            font.weight: Theme.fontWeightMedium
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
        }
    }

    DFlickable {
        id: flickable
        anchors.fill: parent
        anchors.margins: Theme.spacingL
        anchors.topMargin: header.y + header.height + Theme.spacingM
        contentHeight: list.implicitHeight
        clip: true

        DeviceList {
            id: list
            width: flickable.width
            showVolume: true
        }
    }
}
