pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell

Singleton {
    id: root

    enum Level {
        Debug,
        Info,
        Warn,
        Error,
        Fatal
    }

    readonly property int level: _parseLevel(Quickshell.env("DPLAYER_LOG_LEVEL"))

    function scoped(module) {
        return {
            debug: function () {
                root._emit(Log.Level.Debug, module, arguments);
            },
            info: function () {
                root._emit(Log.Level.Info, module, arguments);
            },
            warn: function () {
                root._emit(Log.Level.Warn, module, arguments);
            },
            error: function () {
                root._emit(Log.Level.Error, module, arguments);
            },
            fatal: function () {
                root._emit(Log.Level.Fatal, module, arguments);
            }
        };
    }

    function debug() {
        _emit(Log.Level.Debug, "", arguments);
    }
    function info() {
        _emit(Log.Level.Info, "", arguments);
    }
    function warn() {
        _emit(Log.Level.Warn, "", arguments);
    }
    function error() {
        _emit(Log.Level.Error, "", arguments);
    }
    function fatal() {
        _emit(Log.Level.Fatal, "", arguments);
    }

    function _parseLevel(name) {
        switch ((name || "").toLowerCase()) {
        case "debug":
            return Log.Level.Debug;
        case "warn":
        case "warning":
            return Log.Level.Warn;
        case "error":
            return Log.Level.Error;
        case "fatal":
            return Log.Level.Fatal;
        default:
            return Log.Level.Info;
        }
    }

    function _stringify(v) {
        if (v === null)
            return "null";
        if (v === undefined)
            return "undefined";
        if (typeof v === "string")
            return v;
        if (v instanceof Error)
            return v.toString();
        try {
            return JSON.stringify(v);
        } catch (e) {
            return String(v);
        }
    }

    function _captureStack(skip) {
        try {
            throw new Error();
        } catch (e) {
            const lines = (e.stack || "").split("\n");
            return lines.slice(1 + (skip || 0)).join("\n");
        }
    }

    function _callerLocation() {
        const stack = _captureStack(2);
        const lines = stack.split("\n");
        for (const line of lines) {
            const m = line.match(/([^/@\s]+\.qml):(\d+)/);
            if (!m)
                continue;
            if (m[1] === "Log.qml")
                continue;
            return {
                file: m[1],
                line: m[2]
            };
        }
        return null;
    }

    function _emit(lvl, module, args) {
        if (lvl < root.level)
            return;

        const argList = Array.from(args);
        const loc = _callerLocation();
        const msg = argList.map(_stringify).join(" ");

        let tag;
        if (module && loc && loc.file === module + ".qml")
            tag = "[" + module + ":" + loc.line + "] ";
        else if (module && loc)
            tag = "[" + module + "] (" + loc.file + ":" + loc.line + ") ";
        else if (module)
            tag = "[" + module + "] ";
        else if (loc)
            tag = "(" + loc.file + ":" + loc.line + ") ";
        else
            tag = "";

        const body = tag + msg;

        switch (lvl) {
        case Log.Level.Debug:
            console.debug(body);
            break;
        case Log.Level.Info:
            console.info(body);
            break;
        case Log.Level.Warn:
            console.warn(body);
            break;
        case Log.Level.Error:
        case Log.Level.Fatal:
            console.error(body);
            break;
        }

        if (lvl === Log.Level.Fatal)
            Qt.callLater(() => Qt.exit(1));
    }
}
