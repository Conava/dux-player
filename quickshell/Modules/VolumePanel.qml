pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Common as DC
import qs.DCommon.Widgets

// The volume panel a volume button opens: the output's name over the media volume slider, laid over the window
// (not a popup window, so it follows the window's scheme and stacking).
Item {
    id: overlay

    required property Item anchorItem

    function show() {
        const p = anchorItem.mapToItem(overlay, 0, 0);
        const above = p.y - card.height - Theme.spacingS >= 0;
        card.x = Math.max(Theme.spacingS, Math.min(overlay.width - card.width - Theme.spacingS, p.x + (anchorItem.width - card.width) / 2));
        card.y = above ? p.y - card.height - Theme.spacingS : p.y + anchorItem.height + Theme.spacingS;
        visible = true;
        slider.forceActiveFocus();
    }

    objectName: "volumePanel"
    parent: anchorItem.Window.window?.contentItem ?? anchorItem
    anchors.fill: parent
    visible: false
    z: Theme.zDialog

    readonly property EscapeLayer _escapeLayer: EscapeLayer {
        owner: overlay
        active: overlay.visible
        onEscaped: overlay.visible = false
    }

    MouseArea {
        anchors.fill: parent
        acceptedButtons: Qt.AllButtons
        onPressed: overlay.visible = false
        onWheel: wheel => wheel.accepted = true
    }

    DC.ElevationShadow {
        anchors.fill: card
        level: Theme.elevationLevel2
        fallbackOffset: Theme.spacingXS
        targetRadius: card.radius
        targetColor: card.color
        shadowEnabled: Theme.elevationEnabled
    }

    Rectangle {
        id: card
        width: PlayerMetrics.volumePanelWidth
        height: column.implicitHeight + PlayerMetrics.mediaCardMargin * 2
        radius: Theme.cornerRadiusXL
        color: ArtColors.nowPlaying?.surfaceContainerHigh ?? Theme.surfaceContainerHigh
        Accessible.role: Accessible.Dialog
        Accessible.name: I18n.tr("Volume", "audio volume: settings section, slider")

        MouseArea {
            anchors.fill: parent
        }

        Column {
            id: column
            x: PlayerMetrics.mediaCardMargin
            y: PlayerMetrics.mediaCardMargin
            width: parent.width - PlayerMetrics.mediaCardMargin * 2
            spacing: Theme.spacingS

            StyledText {
                width: parent.width
                text: PlayerService.casting ? PlayerService.castDeviceName : PlayerService.currentOutput?.name ?? ""
                font.pixelSize: Theme.fontSizeSmall
                color: ArtColors.textVariant
                elide: Text.ElideRight
                wrapMode: Text.NoWrap
                horizontalAlignment: Text.AlignLeft
            }

            MediaVolumeSlider {
                id: slider
                width: parent.width
            }
        }
    }
}
