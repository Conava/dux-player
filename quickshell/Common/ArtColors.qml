pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import qs.Services
import "Scheme.js" as Scheme

// Colors from the current album art (the cover's dominant color from the daemon), laid over whatever palette
// is active (shell or app).
Singleton {
    id: root

    readonly property string seed: SettingsData.artColors ? PlayerService.coverColor(PlayerService.current) : ""
    readonly property bool available: seed !== ""
    readonly property bool wholeApp: available && SettingsData.artColorsApp
    // a scheme just for the now-playing views, or null when they follow Theme
    readonly property var nowPlaying: available && !wholeApp ? Scheme.fromSeed(seed, Theme.isLightMode) : null

    // the color roles the shell's media chromes use, from the art scheme or Theme
    readonly property color accent: nowPlaying?.primary ?? Theme.primary
    readonly property color onAccent: nowPlaying?.primaryText ?? Theme.onPrimary
    readonly property color accentContainer: nowPlaying?.primaryContainer ?? Theme.primaryContainer
    readonly property color onAccentContainer: nowPlaying?.onPrimaryContainer ?? Theme.onPrimaryContainer
    readonly property color accentSecondaryContainer: nowPlaying?.secondaryContainer ?? Theme.secondaryContainer
    readonly property color onAccentSecondaryContainer: nowPlaying?.onSecondaryContainer ?? Theme.onSecondaryContainer
    readonly property color accentTrack: Theme.withAlpha(accent, PlayerMetrics.accentTrackAlpha)
    readonly property color accentSubtle: Theme.withAlpha(accent, PlayerMetrics.accentSubtleAlpha)
    readonly property color cardColor: nowPlaying ? Theme.withAlpha(nowPlaying.surfaceContainer, Theme.floatingWindowForegroundAlpha) : Theme.foregroundColor(Theme.cardSurface, true)
    // a card on a pane (the side panel)
    readonly property color chipColor: nowPlaying ? Theme.withAlpha(nowPlaying.surfaceContainerHigh, Theme.floatingWindowForegroundAlpha) : Theme.foregroundColor(Theme.chipSurface, true)
    readonly property color text: nowPlaying?.surfaceText ?? Theme.surfaceText
    readonly property color textVariant: nowPlaying?.surfaceVariantText ?? Theme.surfaceVariantText

    function appScheme(isLight) {
        return Scheme.fromSeed(seed, isLight);
    }
}
