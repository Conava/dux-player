pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell

// "Open floating": the daemon keeps a compositor window rule for the player's windows through Dux Shell's
// `dms config windowrules`, or hands out the rule to add by hand when the shell isn't there.
Singleton {
    id: root

    readonly property var log: Log.scoped("WindowRuleService")

    readonly property string appId: "com.marlonkranz.player"
    // winrules.status: {compositor, mode: shell | manual | unsupported, active, snippet?, error?}
    property var status: ({})
    readonly property string compositor: status.compositor ?? ""
    readonly property bool supported: compositor !== "" && status.mode !== "unsupported"
    readonly property bool viaShell: status.mode === "shell"
    readonly property bool active: status.active === true

    function setFloating(enabled) {
        DaemonBridge.call("winrules.set", {
            "enabled": enabled
        }, result => {
            if (result.error) {
                root.log.warn("winrules.set", result.error);
                return;
            }
            root.status = result;
        });
    }

    function snippet() {
        return status.snippet ?? "";
    }

    function refresh() {
        DaemonBridge.call("winrules.status", null, result => {
            if (!result.error)
                root.status = result;
        });
    }

    Component.onCompleted: {
        DaemonBridge.subscribeTopic("winrules", s => root.status = s);
        if (DaemonBridge.connected)
            refresh();
    }

    Connections {
        target: DaemonBridge

        function onResubscribed() {
            root.refresh();
        }
    }
}
