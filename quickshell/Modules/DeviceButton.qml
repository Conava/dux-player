import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// Shows where audio goes and opens the playback device picker.
DIconButton {
    readonly property bool away: PlayerService.selectedOutputs[0] !== "default" || PlayerService.multiOutput

    iconName: PlayerService.multiOutput ? "speaker_group" : UiConfig.deviceIcon(PlayerService.currentOutput?.kind ?? "default")
    checkable: true
    checked: away
    Accessible.name: I18n.tr("Playback device", "playback device: settings tab, button, dialog")
    tooltipText: PlayerService.multiOutput ? I18n.tr("Playing on %1 outputs", "playback device button tooltip, %1 is 2 or more").arg(PlayerService.selectedOutputs.length) : I18n.tr("Playing on %1", "playback device button tooltip").arg(PlayerService.castDeviceName || (PlayerService.currentOutput?.name ?? ""))
    onClicked: Router.devicePickerOpen = true
}
