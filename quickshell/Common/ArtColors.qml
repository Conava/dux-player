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

    readonly property string seed: SettingsData.artColorMode === "nowPlaying" || SettingsData.artColorMode === "app" ? PlayerService.coverColor(PlayerService.current) : ""
    readonly property bool available: seed !== ""
    readonly property bool wholeApp: available && SettingsData.artColorMode === "app"
    // a scheme just for the now-playing views, or null when they follow Theme
    readonly property var nowPlaying: available && !wholeApp ? Scheme.fromSeed(seed, Theme.isLightMode) : null

    component Fade: Behavior {
        enabled: Theme.animationsEnabled
        ColorAnimation {
            duration: Theme.expressiveDurations.large
        }
    }

    // the color roles the shell's media chromes use, from the art scheme or Theme
    property color accent: nowPlaying?.primary ?? Theme.primary
    // QML reads "onAccent: …" as the change handler of accent and drops it, so Binding sets the on* roles
    property color onAccent
    property color accentContainer: nowPlaying?.primaryContainer ?? Theme.primaryContainer
    property color onAccentContainer
    property color accentSecondaryContainer: nowPlaying?.secondaryContainer ?? Theme.secondaryContainer
    property color onAccentSecondaryContainer
    readonly property list<QtObject> roleBindings: [
        Binding {
            target: root
            property: "onAccent"
            value: root.nowPlaying?.primaryText ?? Theme.onPrimary
        },
        Binding {
            target: root
            property: "onAccentContainer"
            value: root.nowPlaying?.onPrimaryContainer ?? Theme.onPrimaryContainer
        },
        Binding {
            target: root
            property: "onAccentSecondaryContainer"
            value: root.nowPlaying?.onSecondaryContainer ?? Theme.onSecondaryContainer
        }
    ]
    readonly property color accentTrack: Theme.withAlpha(accent, PlayerMetrics.accentTrackAlpha)
    // an opaque rail that reads on any surface, unlike the faint accentTrack
    readonly property color accentRail: Theme.blend(Theme.outline, accent, PlayerMetrics.accentRailMix)
    // the title's colour: the art's accent, or tertiary without art colours
    property color highlight: nowPlaying?.primary ?? (available ? Theme.primary : Theme.tertiary)
    readonly property color accentSubtle: Theme.withAlpha(accent, PlayerMetrics.accentSubtleAlpha)
    property color cardColor: nowPlaying ? Theme.withAlpha(nowPlaying.surfaceContainer, Theme.floatingWindowForegroundAlpha) : Theme.foregroundColor(Theme.cardSurface, true)
    // a card on a pane (the side panel)
    property color chipColor: nowPlaying ? Theme.withAlpha(nowPlaying.surfaceContainerHigh, Theme.floatingWindowForegroundAlpha) : Theme.foregroundColor(Theme.chipSurface, true)
    property color text: nowPlaying?.surfaceText ?? Theme.surfaceText
    property color textVariant: nowPlaying?.surfaceVariantText ?? Theme.surfaceVariantText

    property color surface: nowPlaying?.surface ?? Theme.surface

    Fade on accent {}
    Fade on onAccent {}
    Fade on accentContainer {}
    Fade on onAccentContainer {}
    Fade on accentSecondaryContainer {}
    Fade on onAccentSecondaryContainer {}
    Fade on highlight {}
    Fade on cardColor {}
    Fade on chipColor {}
    Fade on text {}
    Fade on textVariant {}
    Fade on surface {}

    function appScheme(isLight) {
        return Scheme.fromSeed(seed, isLight);
    }
}
