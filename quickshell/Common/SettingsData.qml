pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Io
import qs.Common
import "Layouts.js" as Layouts

// UI-only settings. Playback, libraries and lyrics settings belong to the daemon (PlayerService.settings).
// Runtime state that should survive a restart goes to ui-state.json in the state dir, never in here.
Singleton {
    id: root

    readonly property string settingsPath: Paths.configDir + "/ui-settings.json"
    readonly property string statePath: Paths.stateDir + "/ui-state.json"

    property alias themeMode: adapter.themeMode
    property alias presetTheme: adapter.presetTheme
    // "" (unset: shell colors when detected, else the stock preset) | "stock" | "seed" | "custom"
    property alias colorScheme: adapter.colorScheme
    property alias seedColor: adapter.seedColor
    // a validated theme from CustomTheme.parse: {name, dark, light}
    property alias customTheme: adapter.customTheme
    // per style group: "shell" | "custom" (used while Follow Dux Shell is off)
    property alias styleGroups: adapter.styleGroups
    property alias windowTransparency: adapter.windowTransparency
    property alias elevationEnabled: adapter.elevationEnabled
    property alias blurEnabled: adapter.blurEnabled
    property alias radiusStrength: adapter.radiusStrength
    property alias density: adapter.density
    property alias animationDuration: adapter.animationDuration
    property alias reduceMotion: adapter.reduceMotion
    property alias springBounce: adapter.springBounce
    property alias enableRippleEffects: adapter.enableRippleEffects
    property alias fontSource: adapter.fontSource
    property alias fontFamily: adapter.fontFamily
    property alias monoFontFamily: adapter.monoFontFamily
    property alias displayFontFamily: adapter.displayFontFamily
    property alias fontWeight: adapter.fontWeight
    property alias fontScale: adapter.fontScale
    property alias focusRingEnabled: adapter.focusRingEnabled
    property alias focusRingWidth: adapter.focusRingWidth
    property alias focusRingColor: adapter.focusRingColor
    property alias windowTitleBars: adapter.windowTitleBars
    property alias language: adapter.language
    // take every style value from Dux Shell when it is detected
    property alias followShell: adapter.followShell
    // Customization: surfaces, row actions, list options, navigation and saved views, in one versioned object
    // (Layouts.js) whose contents round-trip untouched. Read it through UiConfig, which applies the defaults.
    property alias layout: adapter.layout
    property alias lyricsDisplay: adapter.lyricsDisplay
    property alias undoToast: adapter.undoToast
    // "auto" (rail when wide, bar when narrow) | "rail" | "railLabels" | "bar"
    property alias navStyle: adapter.navStyle
    // {searchPage, searchAll}: key sequences in Qt's portable text ("Ctrl+Shift+F"); missing ones use the defaults
    property alias shortcuts: adapter.shortcuts
    // colors from the current album art override the palette: in now playing, or in the whole app
    property alias artColors: adapter.artColors
    property alias artColorsApp: adapter.artColorsApp
    // ui-state.json: the full view's last pane ("lyrics" | "queue" | "") and the side panel's last tab
    property alias lastPane: stateAdapter.lastPane
    property alias sideTab: stateAdapter.sideTab

    function flush() {
        if (stateDebounce.running) {
            stateDebounce.stop();
            stateFile.writeAdapter();
        }
        if (!writeDebounce.running)
            return;
        writeDebounce.stop();
        settingsFile.blockWrites = true;
        settingsFile._write();
    }

    // Files from before the layout object: build it from the old keys, which the next write drops
    function migrate() {
        let raw;
        try {
            raw = JSON.parse(settingsFile.text());
        } catch (e) {
            return;
        }
        if (Layouts.needsMigration(raw))
            root.layout = Layouts.migrate(raw);
    }

    Component.onDestruction: flush()

    Timer {
        id: writeDebounce

        interval: 400
        onTriggered: {
            if (!Paths.ready) {
                restart();
                return;
            }
            settingsFile._write();
        }
    }

    FileView {
        id: settingsFile
        path: root.settingsPath
        blockLoading: true
        atomicWrites: true
        watchChanges: true
        printErrors: false

        property bool _loaded: false
        property bool _selfWrite: false

        onLoaded: {
            _loaded = true;
            root.migrate();
        }
        onLoadFailed: _loaded = true
        onFileChanged: {
            if (_selfWrite) {
                _selfWrite = false;
                return;
            }
            reload();
        }
        onAdapterUpdated: {
            if (!_loaded)
                return;
            writeDebounce.restart();
        }

        function _write() {
            _selfWrite = true;
            writeAdapter();
        }

        JsonAdapter {
            id: adapter
            property string themeMode: "auto"
            property string colorScheme: ""
            property string seedColor: "#6750a4"
            property var customTheme: null
            property var styleGroups: ({})
            property real windowTransparency: 1.0
            property bool elevationEnabled: true
            property bool blurEnabled: false
            property string presetTheme: "purple"
            property int radiusStrength: 50
            property string density: "comfortable"
            property int animationDuration: 250
            property bool reduceMotion: false
            property int springBounce: 1
            property bool enableRippleEffects: true
            property string fontSource: "auto"
            property string fontFamily: ""
            property string monoFontFamily: ""
            property string displayFontFamily: ""
            property int fontWeight: 400
            property real fontScale: 1.0
            property bool focusRingEnabled: true
            property real focusRingWidth: 1.5
            property string focusRingColor: "primary"
            property bool windowTitleBars: true
            property string language: ""
            property var layout: ({})
            property var lyricsDisplay: ({})
            property var undoToast: ({})
            property string navStyle: "auto"
            property var shortcuts: ({})
            property bool artColors: false
            property bool artColorsApp: false
            property bool followShell: true
        }
    }

    Timer {
        id: stateDebounce

        interval: 400
        onTriggered: {
            if (!Paths.ready) {
                restart();
                return;
            }
            stateFile.writeAdapter();
        }
    }

    FileView {
        id: stateFile
        path: root.statePath
        blockLoading: true
        atomicWrites: true
        printErrors: false

        property bool _loaded: false

        onLoaded: _loaded = true
        onLoadFailed: _loaded = true
        onAdapterUpdated: {
            if (_loaded)
                stateDebounce.restart();
        }

        JsonAdapter {
            id: stateAdapter
            property string lastPane: ""
            property string sideTab: "cover"
        }
    }
}
