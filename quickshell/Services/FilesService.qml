pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell

// DCommon's file browser backend (DC.Host.files) over the daemon's files.* group. Browsing only: the daemon
// offers no mkdir, rename or trash, so the picker hides them.
Singleton {
    id: root

    readonly property bool connected: DaemonBridge.connected && DaemonBridge.subscribed
    readonly property var capabilities: ({})
    property var userDirs: []
    // watchId -> topic
    property var _topics: ({})

    signal watchEvent(var data)

    onConnectedChanged: {
        _topics = {};
        if (connected)
            _send("files.userDirs", null, result => root.userDirs = result.dirs ?? []);
    }

    function watch(path, options, callback) {
        _send("files.watch", Object.assign({
            "path": path
        }, _options(options)), result => {
            if (result.topic) {
                root._topics[result.watchId] = result.topic;
                DaemonBridge.subscribeTopic(result.topic, data => root.watchEvent(data));
            }
            callback?.(result);
        });
    }

    function unwatch(watchId) {
        const topic = _topics[watchId];
        if (!topic)
            return;
        delete _topics[watchId];
        DaemonBridge.unsubscribeTopic(topic);
        _send("files.unwatch", {
            "watchId": watchId
        }, null);
    }

    function page(watchId, options, callback) {
        _send("files.list", Object.assign({
            "watchId": watchId
        }, _options(options)), callback);
    }

    function list(path, options, callback) {
        _send("files.list", Object.assign({
            "path": path
        }, _options(options)), callback);
    }

    function stat(path, callback) {
        _send("files.stat", {
            "path": path
        }, callback);
    }

    function count(paths, includeHidden, callback) {
        _send("files.count", {
            "paths": paths,
            "includeHidden": includeHidden === true
        }, callback);
    }

    function thumbnails(paths, size, watchId, callback) {
        const params = {
            "paths": paths,
            "size": size || "normal"
        };
        if (watchId)
            params.watchId = watchId;
        _send("files.thumbnail", params, callback);
    }

    function _options(options) {
        const o = options ?? {};
        const params = {};
        if (o.includeHidden !== undefined)
            params.includeHidden = o.includeHidden;
        if (o.filters !== undefined)
            params.filters = o.filters;
        if (o.sortKey)
            params.sort = o.sortKey;
        if (o.sortDesc !== undefined)
            params.desc = o.sortDesc;
        if (o.dirsFirst !== undefined)
            params.dirsFirst = o.dirsFirst;
        if (o.limit)
            params.limit = o.limit;
        if (o.cursor)
            params.cursor = o.cursor;
        return params;
    }

    function _send(method, params, callback) {
        DaemonBridge.call(method, params, result => callback?.(result));
    }
}
