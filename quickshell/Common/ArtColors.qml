pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import qs.Services
import "Scheme.js" as Scheme
import "../DCommon/Common/Hct.js" as Hct

// Colors from the current album art (the cover's dominant color from the daemon), laid over whatever palette
// is active (shell or app).
Singleton {
    id: root

    // The track whose colours show. A new cover takes over once the old one has blurred out, so the colours
    // change with the new cover's blur-in.
    property var track: null
    readonly property string seed: SettingsData.artColorMode === "nowPlaying" || SettingsData.artColorMode === "app" ? PlayerService.coverColor(track) : ""
    readonly property bool available: seed !== ""
    readonly property bool wholeApp: available && SettingsData.artColorMode === "app"
    // a scheme just for the now-playing views, or null when they follow Theme
    readonly property var nowPlaying: available && !wholeApp ? schemeOf(Theme.isLightMode) : null
    // A grey, black or white cover gives a near-grey scheme: under this seed chroma every role's chroma shrinks
    // with the seed's, which puts the primary at twice the seed's chroma like the page tint.
    readonly property real fullChromaAt: 24
    // the first cover's colours land at once
    property bool instant: false

    component Fade: Behavior {
        enabled: Theme.animationsEnabled && !root.instant
        ColorAnimation {
            duration: Theme.expressiveDurations.expressiveDefaultSpatial
            easing.type: Easing.BezierSpline
            easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
        }
    }

    function follow() {
        handoffWait.stop();
        instant = seed === "";
        track = PlayerService.current;
        instant = false;
    }

    function schemeOf(isLight) {
        const scheme = Scheme.fromSeed(seed, isLight);
        const k = Math.min(1, Hct.toHct(Qt.color(seed)).chroma / fullChromaAt);
        if (k >= 1)
            return scheme;
        const out = {};
        for (const role in scheme) {
            const c = Hct.toHct(Qt.color(scheme[role]));
            out[role] = Hct.fromHct(c.hue, c.chroma * k, c.tone).toString();
        }
        return out;
    }

    Component.onCompleted: track = PlayerService.current

    Connections {
        target: PlayerService
        function onCurrentChanged() {
            const from = PlayerService.coverUrl(root.track);
            const to = PlayerService.coverUrl(PlayerService.current);
            if (from !== "" && to !== "" && from !== to && Theme.animationsEnabled)
                handoffWait.restart();
            else
                root.follow();
        }
    }

    // as long as the now-playing art's blur-out
    Timer {
        id: handoffWait
        interval: Theme.expressiveDurations.expressiveEffects
        onTriggered: root.follow()
    }

    // the color roles the shell's media chromes use, from the art scheme or Theme
    property color accent: nowPlaying?.primary ?? Theme.primary
    // QML reads "onAccent: …" as the change handler of accent and drops it, so Binding sets the on* roles
    property color onAccent // qmllint disable signal-handler-name-clash
    property color accentContainer: nowPlaying?.primaryContainer ?? Theme.primaryContainer
    property color onAccentContainer // qmllint disable signal-handler-name-clash
    property color accentSecondaryContainer: nowPlaying?.secondaryContainer ?? Theme.secondaryContainer
    property color onAccentSecondaryContainer // qmllint disable signal-handler-name-clash
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
        return schemeOf(isLight);
    }
}
