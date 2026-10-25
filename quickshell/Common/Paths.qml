pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Io
import QtCore
import qs.Services

Singleton {
    id: root

    readonly property url home: StandardPaths.standardLocations(StandardPaths.HomeLocation)[0]
    readonly property url xdgCache: StandardPaths.standardLocations(StandardPaths.GenericCacheLocation)[0]
    readonly property url cache: `${xdgCache}/dplayer`
    readonly property url imagecache: `${cache}/imagecache`
    readonly property string configDir: {
        const xdg = Quickshell.env("XDG_CONFIG_HOME");
        const base = (xdg && xdg !== "") ? xdg : Quickshell.env("HOME") + "/.config";
        return base + "/dplayer";
    }
    readonly property string stateDir: {
        const xdg = Quickshell.env("XDG_STATE_HOME");
        const base = (xdg && xdg !== "") ? xdg : Quickshell.env("HOME") + "/.local/state";
        return base + "/dplayer";
    }
    property bool ready: false

    function stringify(path: url): string {
        return path.toString().replace(/%20/g, " ");
    }

    function strip(path: url): string {
        return stringify(path).replace("file://", "");
    }

    function expandTilde(path: string): string {
        if (!path.startsWith("~/"))
            return path;
        return strip(root.home) + path.substring(1);
    }

    function resolveIconPath(iconName: string): string {
        if (!iconName)
            return "";
        return Quickshell.iconPath(iconName, true) || "";
    }

    function copyPathToClipboard(path: string): void {
        Quickshell.clipboardText = path;
    }

    Process {
        command: ["mkdir", "-p", root.strip(root.imagecache), root.configDir, root.stateDir]
        running: true
        onExited: code => {
            if (code !== 0)
                Log.warn(`mkdir of the app dirs exited with ${code}`);
            root.ready = true;
        }
    }
}
