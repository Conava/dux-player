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

    property alias themeMode: settingsAdapter.themeMode
    property alias presetTheme: settingsAdapter.presetTheme
    // "" (unset: shell colors when detected, else the stock preset) | "stock" | "seed" | "custom"
    property alias colorScheme: settingsAdapter.colorScheme
    property alias seedColor: settingsAdapter.seedColor
    // a validated theme from CustomTheme.parse: {name, dark, light}
    property alias customTheme: settingsAdapter.customTheme
    // per style group: "shell" | "custom" (used while Follow Dux Shell is off)
    property alias styleGroups: settingsAdapter.styleGroups
    property alias windowTransparency: settingsAdapter.windowTransparency
    property alias elevationEnabled: settingsAdapter.elevationEnabled
    property alias blurEnabled: settingsAdapter.blurEnabled
    property alias radiusStrength: settingsAdapter.radiusStrength
    property alias density: settingsAdapter.density
    property alias animationDuration: settingsAdapter.animationDuration
    property alias reduceMotion: settingsAdapter.reduceMotion
    property alias springBounce: settingsAdapter.springBounce
    property alias enableRippleEffects: settingsAdapter.enableRippleEffects
    property alias fontSource: settingsAdapter.fontSource
    property alias fontFamily: settingsAdapter.fontFamily
    property alias monoFontFamily: settingsAdapter.monoFontFamily
    property alias displayFontFamily: settingsAdapter.displayFontFamily
    property alias fontWeight: settingsAdapter.fontWeight
    property alias fontScale: settingsAdapter.fontScale
    property alias focusRingEnabled: settingsAdapter.focusRingEnabled
    property alias focusRingWidth: settingsAdapter.focusRingWidth
    property alias focusRingColor: settingsAdapter.focusRingColor
    property alias windowTitleBars: settingsAdapter.windowTitleBars
    // "button" | "field": the header search as a button or a full field
    property alias headerSearch: settingsAdapter.headerSearch
    property alias language: settingsAdapter.language
    // take every style value from Dux Shell when it is detected
    property alias followShell: settingsAdapter.followShell
    // Customization: surfaces, row actions, list options, navigation and saved views, in one versioned object
    // (Layouts.js) whose contents round-trip untouched. Read it through UiConfig, which applies the defaults.
    property alias layout: settingsAdapter.layout
    property alias lyricsDisplay: settingsAdapter.lyricsDisplay
    property alias undoToast: settingsAdapter.undoToast
    // "auto" (rail when wide, bar when narrow) | "rail" | "railLabels" | "bar"
    property alias navStyle: settingsAdapter.navStyle
    // {searchPage, searchAll}: key sequences in Qt's portable text ("Ctrl+Shift+F"); missing ones use the defaults
    property alias shortcuts: settingsAdapter.shortcuts
    // colors from the current album art override the palette: in now playing, or in the whole app
    property alias artColorMode: settingsAdapter.artColorMode
    property alias trackTileOpensAlbum: settingsAdapter.trackTileOpensAlbum
    // ui-state.json: the full view's last pane ("lyrics" | "queue" | "") and the side panel's last tab
    property alias lastPane: stateAdapter.lastPane
    property alias sideTab: stateAdapter.sideTab

    function flush() {
        if (stateDebounce.running) {
            stateDebounce.stop();
            stateFile.writeAdapter();
        }
        // Quickshell blanks the pending data when a blocking write has to wait for a running one, and saves an empty file
        settingsWriter.waitForJob();
        if (!writeDebounce.running)
            return;
        writeDebounce.stop();
        settingsWriter.blockWrites = true;
        settingsFile._write();
        settingsWriter.blockWrites = false;
    }

    // Files from before the layout object: build it from the old keys, which the next write drops
    function migrate() {
        const text = settingsFile.text();
        let raw = null;
        try {
            raw = JSON.parse(text);
        } catch (e) {}
        if (!Layouts.isObject(raw)) {
            // saving would replace the file with the defaults, so nothing is saved until it is valid or gone
            if (text.trim() === "")
                return;
            settingsFile._loaded = false;
            console.warn("settings file is invalid, not saving over it; a copy has the suffix .bad");
            brokenCopy.path = root.settingsPath + ".bad";
            brokenCopy.setText(text);
            return;
        }
        settingsFile._extra = Layouts.extraTop(raw);
        // the old artColors keys stay in the file as unknown keys, so an older version still reads them
        const mode = Layouts.artColorMode(raw);
        if (raw.artColorMode !== mode)
            root.artColorMode = mode;
        if (Layouts.needsMigration(raw)) {
            root.layout = Layouts.migrate(raw);
            return;
        }
        // JsonAdapter loads nested lists as QML sequences, which Array.isArray rejects, so Layouts would read every
        // stored list as unset and the next edit would save the defaults. This is the file's content: no write.
        settingsFile._loaded = false;
        root.layout = raw.layout;
        settingsFile._loaded = true;
    }

    property alias stored: settingsAdapter

    // writeAdapter would drop the keys the adapter doesn't declare
    function fileText(extra: var): string {
        const props = JSON.parse(JSON.stringify(root.stored));
        delete props.objectName;
        return JSON.stringify(Object.assign({}, extra, props), null, 4);
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
        path: _rereading ? "" : root.settingsPath
        blockLoading: true
        watchChanges: true
        printErrors: false

        property bool _loaded: false
        // the file's content as far as this process knows, to tell our own writes from outside changes
        property string _known: ""
        property bool _rereading: false
        // top-level keys the adapter doesn't declare, written back so a newer or older version's keys survive
        property var _extra: ({})

        onLoaded: {
            _known = text();
            _loaded = true;
            root.migrate();
        }
        // an existing file that won't load stays untouched: writing would replace it with the defaults
        onLoadFailed: error => {
            _loaded = error === FileViewError.FileNotFound;
            if (!_loaded)
                console.warn("settings file won't load, not saving over it:", FileViewError.toString(error));
        }
        // Our own writes arrive here too, some of them twice. Passing through an empty path makes the adapter read
        // the file even when it went back to what this view read last, which reload() would skip.
        onFileChanged: {
            settingsWriter.reload();
            if (settingsWriter.text() === _known)
                return;
            _rereading = true;
            _rereading = false;
        }
        onAdapterUpdated: {
            if (!_loaded)
                return;
            writeDebounce.restart();
        }

        function _write() {
            if (!_loaded)
                return;
            _known = root.fileText(_extra);
            settingsWriter.setText(_known);
        }

        JsonAdapter {
            id: settingsAdapter
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
            property string headerSearch: "button"
            property string language: ""
            property var layout: ({})
            property var lyricsDisplay: ({})
            property var undoToast: ({})
            property string navStyle: "auto"
            property var shortcuts: ({})
            property bool trackTileOpensAlbum: false
            property string artColorMode: "off"
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

    // Writes go through their own view: a write on settingsFile would load its text back into the adapter, undoing
    // an edit made meanwhile and turning the layout's lists into sequences that Layouts reads as unset.
    FileView {
        id: settingsWriter
        path: root.settingsPath
        blockAllReads: true
        atomicWrites: true
        printErrors: false

        onSaveFailed: error => console.warn("settings file couldn't be saved:", FileViewError.toString(error))
    }

    FileView {
        id: brokenCopy
        atomicWrites: true
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
        onSaveFailed: error => console.warn("window state couldn't be saved:", FileViewError.toString(error))
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
