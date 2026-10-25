import QtQuick
import Quickshell
import Quickshell.Io
import "../DCommon/Common/settings/SharedSettingsSpec.js" as Shared

// Reads Dux Shell's own settings and session (read-only, watched) so an app can take its style from the
// shell like a native shell window. value() falls back to the shell's defaults for keys the file omits.
QtObject {
    id: root

    property string configDir: {
        const xdg = Quickshell.env("XDG_CONFIG_HOME");
        return ((xdg && xdg !== "") ? xdg : Quickshell.env("HOME") + "/.config") + "/dms";
    }
    property string stateDir: {
        const xdg = Quickshell.env("XDG_STATE_HOME");
        return ((xdg && xdg !== "") ? xdg : Quickshell.env("HOME") + "/.local/state") + "/DankMaterialShell";
    }

    property var settings: null
    property var session: null
    readonly property bool found: settings !== null
    readonly property bool isLightMode: session?.isLightMode ?? false

    function value(key, fallback) {
        const stored = settings?.[key];
        if (stored !== undefined)
            return stored;
        return Shared.SPEC[key]?.def ?? fallback;
    }

    function parsed(text) {
        try {
            const data = JSON.parse(text);
            return data && typeof data === "object" && !Array.isArray(data) ? data : null;
        } catch (e) {
            return null;
        }
    }

    property FileView settingsFile: FileView {
        path: root.configDir + "/settings.json"
        watchChanges: true
        printErrors: false
        onFileChanged: reload()
        onLoaded: root.settings = root.parsed(text())
        onLoadFailed: root.settings = null
    }

    property FileView sessionFile: FileView {
        path: root.stateDir + "/session.json"
        watchChanges: true
        printErrors: false
        onFileChanged: reload()
        onLoaded: root.session = root.parsed(text())
        onLoadFailed: root.session = null
    }
}
