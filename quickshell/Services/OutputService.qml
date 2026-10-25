pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common

// Playback devices: local outputs and, with cast-helper, Chromecast devices (ids cast:…), several outputs
// at once, and the equalizer's state.
Singleton {
    id: root

    readonly property var log: Log.scoped("OutputService")

    // {id, name, kind, available, onDisconnect}
    property var outputDevices: [_defaultDevice()]
    // one id, or several local outputs played in sync
    property var selectedOutputs: ["default"]
    // the user's "delay this output" per output id, in ms
    property var outputDelays: ({})
    property bool resetOutputOnStart: true
    // pause when the output played on goes away; off, the default output takes over
    property bool pauseOnDisconnect: true
    property bool canMultiOutput: false
    property bool testTonePlaying: false
    readonly property var currentOutput: outputDevices.find(d => d.id === selectedOutputs[0]) ?? outputDevices[0]
    readonly property bool multiOutput: selectedOutputs.length > 1
    // "off" | "on" | "casting" | "unavailable" (devicesError says why, when it knows)
    property string equalizerState: "off"
    property string devicesError: ""

    // cast.status: {available, updateNeeded, casting, session}
    property var castStatus: ({})
    readonly property bool castAvailable: castStatus.available === true
    readonly property bool casting: castStatus.casting === true
    readonly property string castDeviceName: casting ? (castStatus.session?.name ?? "") : ""
    // the last cast failure in words, "" when there is none
    readonly property string castErrorText: castStatus.error ? _castErrorText(castStatus.error) : ""

    function output(id) {
        return outputDevices.find(d => d.id === id) ?? null;
    }

    function selectOutput(id) {
        if (!output(id)?.available)
            return;
        _selectOutputs([id]);
    }

    // adds or removes a local output from a multi-output set; the default and cast devices play alone
    function toggleOutput(id) {
        const device = output(id);
        if (!device?.available || device.kind === "default" || device.kind === "cast")
            return;
        const locals = selectedOutputs.filter(o => output(o)?.kind !== "default" && output(o)?.kind !== "cast");
        const next = locals.indexOf(id) >= 0 ? locals.filter(o => o !== id) : locals.concat([id]);
        _selectOutputs(next.length > 0 ? next : ["default"]);
    }

    // applied on slider release: a change is a short skip on that output
    function setOutputDelay(id, ms) {
        DaemonBridge.request("devices.setDelay", {
            "id": id,
            "ms": Math.max(0, Math.min(PlayerMetrics.outputDelayMax, Math.round(ms)))
        }, I18n.tr("Couldn't save the delay", "toast when an output's sync delay is refused"));
    }

    function setPauseOnDisconnect(enabled) {
        pauseOnDisconnect = enabled;
        DaemonBridge.request("devices.setPauseOnDisconnect", {
            "enabled": enabled
        }, I18n.tr("Couldn't save the setting", "toast when the daemon refuses a settings change"), null, () => root._refresh());
    }

    // one of several outputs going away; mode: "pause" | "continue"
    function setOnDisconnect(id, mode) {
        DaemonBridge.request("devices.setOnDisconnect", {
            "id": id,
            "mode": mode
        }, I18n.tr("Couldn't save the setting", "toast when the daemon refuses a settings change"));
    }

    function setResetOutputOnStart(enabled) {
        DaemonBridge.request("devices.setResetOnStart", {
            "enabled": enabled
        }, I18n.tr("Couldn't save the setting", "toast when the daemon refuses a settings change"));
    }

    function playTestTone() {
        DaemonBridge.request("devices.testTone", null, I18n.tr("Couldn't play the test tone", "toast when the multi-output test tone fails"));
    }

    // the device picker holds this while it shows; cast discovery runs only then
    function watchCastDevices(on) {
        DaemonBridge.watchTopic("castDevices", on);
    }

    // error: {code, detail, missing}; the helper's detail is English and stays in the log
    function _castErrorText(error) {
        switch (error.code) {
        case "missingDependency":
            return I18n.tr("Casting requires: %1", "cast error, %1 lists package names").arg((error.missing ?? []).join(", "));
        case "deviceLost":
            return I18n.tr("Lost the connection to the cast device", "cast error");
        case "deviceNotFound":
            return I18n.tr("The cast device is no longer on the network", "cast error");
        case "launchRefused":
            return I18n.tr("The cast device refused the cast", "cast error");
        case "unsupportedFile":
        case "loadFailed":
            return I18n.tr("The cast device couldn't play this track", "cast error");
        case "receiverStopped":
            return I18n.tr("Casting was stopped on the device", "cast error");
        case "serverUnreachable":
            return I18n.tr("The cast device can't reach this computer", "cast error");
        case "portInUse":
            return I18n.tr("The port for casting files is already in use", "cast error");
        case "busy":
            return I18n.tr("Another app is already casting", "cast error");
        case "avahi":
            return I18n.tr("Can't search for cast devices: discovery is unavailable", "cast error");
        case "insecureServer":
            return I18n.tr("This server's tracks can only be cast over https", "cast error");
        }
        return I18n.tr("Casting failed", "cast error");
    }

    function _applyCast(status) {
        const before = JSON.stringify(castStatus.error ?? null);
        castStatus = status;
        if (status.error && JSON.stringify(status.error) !== before) {
            log.warn("cast:", status.error.code, status.error.detail ?? "");
            DaemonBridge.notice(castErrorText);
        }
    }

    function _defaultDevice() {
        return {
            "id": "default",
            "name": I18n.tr("System default", "playback device name"),
            "kind": "default",
            "available": true
        };
    }

    // shown at once; a refused switch (no PipeWire tools, a cast that fails) puts the daemon's choice back
    function _selectOutputs(ids) {
        selectedOutputs = ids;
        DaemonBridge.request("devices.select", {
            "outputs": ids
        }, I18n.tr("Couldn't switch the playback device", "toast when switching the playback device fails"), state => root._applyDevices(state), () => root._refresh());
    }

    function _refresh() {
        DaemonBridge.request("devices.list", null, null, d => root._applyDevices(d));
        DaemonBridge.request("cast.status", null, null, s => root._applyCast(s));
    }

    function _applyDevices(d) {
        const devices = (d.devices ?? []).map(dev => dev.id === "default" ? Object.assign({}, dev, {
                "name": I18n.tr("System default", "playback device name")
            }) : dev);
        outputDevices = devices.length > 0 ? devices : [_defaultDevice()];
        selectedOutputs = (d.selected ?? []).length > 0 ? d.selected : ["default"];
        outputDelays = d.delays ?? {};
        resetOutputOnStart = d.resetOnStart !== false;
        pauseOnDisconnect = d.pauseOnDisconnect !== false;
        canMultiOutput = d.canMulti === true;
        testTonePlaying = d.testTone === true;
        equalizerState = d.equalizer ?? "off";
        devicesError = (d.errors ?? []).length > 0 ? Phrases.joinAll(d.errors.map(e => Phrases.reason(e.code, e.message))) : d.error ?? "";
        if (d.error)
            log.warn("devices:", d.error);
    }

    Component.onCompleted: {
        DaemonBridge.subscribeTopic("devices", d => root._applyDevices(d));
        DaemonBridge.subscribeTopic("cast", s => root._applyCast(s));
        // the devices event carries the cast devices; this subscription only keeps discovery running
        DaemonBridge.subscribeTopic("castDevices", () => {}, true);
        if (DaemonBridge.subscribed)
            _refresh();
    }

    Connections {
        target: DaemonBridge

        function onResubscribed() {
            root._refresh();
        }
    }
}
