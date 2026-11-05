import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

WindowDialog {
    id: root

    readonly property var speeds: PlayerMetrics.playbackSpeeds

    opened: Router.speedPickerOpen
    title: I18n.tr("Playback speed", "now playing button")
    supportingText: SettingsService.canChangeSpeed ? I18n.tr("Pitch is preserved.", "playback speed note") : I18n.tr("Requires mpv.", "why a playback setting is unavailable")
    acceptEnabled: false
    onRejected: Router.speedPickerOpen = false
    onOpenedChanged: {
        if (opened)
            forceActiveFocus();
    }

    DButtonGroup {
        model: root.speeds.map(s => I18n.tr("%1×", "playback speed option").arg(s))
        enabled: SettingsService.canChangeSpeed
        currentIndex: root.speeds.indexOf(PlayerService.effectiveSpeed)
        onSelectionChanged: (index, selected) => {
            if (selected)
                PlayerService.setSetting("speed", root.speeds[index]);
        }
    }

    actions: [
        DButton {
            text: I18n.tr("Close", "dialog dismiss button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: Router.speedPickerOpen = false
        }
    ]
}
