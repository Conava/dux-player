import QtQuick
import qs.Common
import qs.Services
import qs.Widgets

// The player's volume as a handle-less pill: the fill is the level, the speaker icon inside mutes. Bento fills
// with the accent, M3E is tonal with the level as text.
DPillSlider {
    id: root

    readonly property real volume: PlayerService.volume
    readonly property bool muted: PlayerService.muted || volume === 0
    readonly property bool tonal: UiConfig.nowPlaying.style === "m3e"
    property string accessibleName: I18n.tr("Volume", "audio volume: settings section, slider")

    implicitHeight: tonal ? Theme.buttonHeightXS : Theme.buttonHeightS
    value: muted ? 0 : Math.round(volume * 100)
    maximum: Math.max(1, Math.round(PlayerService.volumeMax * 100))
    iconName: muted ? "volume_off" : volume < 0.5 ? "volume_down" : "volume_up"
    iconClickable: true
    showValue: tonal
    unit: "%"
    fillColor: tonal ? ArtColors.accentContainer : ArtColors.accent
    onFillColor: tonal ? ArtColors.onAccentContainer : ArtColors.onAccent
    trackColor: tonal ? ArtColors.accentSecondaryContainer : ArtColors.accentTrack
    onTrackColor: tonal ? ArtColors.onAccentSecondaryContainer : ArtColors.accent
    Accessible.name: accessibleName
    onMoved: value => PlayerService.setVolume(value / 100)
    onIconClicked: PlayerService.setMuted(!PlayerService.muted)
}
