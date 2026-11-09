import QtQuick
import qs.Common
import qs.DCommon.Widgets
import qs.Services

// The player's volume: a filled track with the speaker icon inset at the start of the fill and a handle bar at its
// edge. The icon mutes. Bento fills with the accent, M3E is tonal with the level as text.
DSlider {
    id: root

    readonly property real volume: PlayerService.volume
    readonly property bool muted: PlayerService.muted || volume === 0
    readonly property bool tonal: UiConfig.nowPlaying.style === "m3e"
    property string accessibleName: I18n.tr("Volume", "audio volume: settings section, slider")

    size: "s"
    value: muted ? 0 : Math.round(volume * 100)
    maximum: Math.max(1, Math.round(PlayerService.volumeMax * 100))
    insetIcon: muted ? "volume_off" : volume < 0.5 ? "volume_down" : "volume_up"
    insetIconClickable: true
    showValue: tonal
    unit: "%"
    fillColor: tonal ? ArtColors.accentContainer : ArtColors.accent
    fillTextColor: tonal ? ArtColors.onAccentContainer : ArtColors.onAccent
    trackColor: tonal ? ArtColors.accentSecondaryContainer : ArtColors.accentTrack
    trackTextColor: tonal ? ArtColors.onAccentSecondaryContainer : ArtColors.accent
    Accessible.name: accessibleName
    onSliderValueChanged: v => PlayerService.setVolume(v / 100)
    onInsetIconClicked: PlayerService.setMuted(!PlayerService.muted)
}
