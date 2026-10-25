pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Io
import qs.DCommon.Common as DC

// The socket link to the dplayer daemon: requests and replies on one connection, the event stream on a
// second one, both redialed with backoff when the daemon goes away.
Singleton {
    id: root

    readonly property var log: Log.scoped("DaemonBridge")

    readonly property string socketPath: Quickshell.env("DPLAYER_SOCKET")

    property bool connected: false
    property bool subscribed: false
    property int apiVersion: 0

    property var pendingRequests: ({})
    property var topicHandlers: ({})
    // topics outside the default subscription, by how many views want them right now
    property var watchers: ({})
    property int requestCounter: 0
    readonly property int requestTimeout: 30000

    // the event connection came up; state that only arrives as events must be fetched again
    signal resubscribed
    // a result worth a snackbar: a request failed, or a service has news
    signal notice(string text)

    function start() {
        if (socketPath === "") {
            log.warn("DPLAYER_SOCKET is not set; start the window with `dplayer show`");
            return;
        }
        requestSocket.connected = true;
    }

    DC.DSocket {
        id: requestSocket
        path: root.socketPath
        connected: false

        onConnectionStateChanged: {
            root.connected = linkUp;
            subscribeSocket.connected = linkUp;
            if (!linkUp)
                root._failPendingRequests("disconnected from the dplayer daemon", "daemonDisconnected");
        }

        parser: SplitParser {
            onRead: line => {
                if (!line)
                    return;
                let response;
                try {
                    response = JSON.parse(line);
                } catch (e) {
                    root.log.warn("bad response", line.substring(0, 200));
                    return;
                }
                root._handleResponse(response);
            }
        }
    }

    DC.DSocket {
        id: subscribeSocket
        path: root.socketPath
        connected: false

        onConnectionStateChanged: {
            root.subscribed = linkUp;
            if (!linkUp)
                return;
            const topics = Object.keys(root.topicHandlers).filter(t => (root.watchers[t] ?? 1) > 0);
            if (topics.length > 0)
                root._subscription("subscribe", topics);
            root.resubscribed();
        }

        parser: SplitParser {
            onRead: line => {
                if (!line)
                    return;
                let event;
                try {
                    event = JSON.parse(line);
                } catch (e) {
                    return;
                }
                const handlers = root.topicHandlers[event.event];
                if (!handlers)
                    return;
                for (const handler of handlers.slice()) {
                    try {
                        handler(event.data ?? {});
                    } catch (e) {
                        root.log.warn("handler for", event.event, "failed:", e);
                    }
                }
            }
        }
    }

    // Topics registered before start() go out with the subscribe request when the link comes up. A watched
    // topic is only subscribed while watchTopic holds it.
    function subscribeTopic(topic, handler, watched) {
        if (watched && watchers[topic] === undefined)
            watchers[topic] = 0;
        const handlers = topicHandlers[topic] ?? [];
        handlers.push(handler);
        topicHandlers[topic] = handlers;
        // a singleton created after the link came up still gets its events
        if (subscribed && handlers.length === 1 && (watchers[topic] ?? 1) > 0)
            _subscription("subscribe", [topic]);
    }

    // For topics the daemon only works for while someone listens (lyrics lookups, cast discovery): each
    // visible view adds one while it shows and removes it when it hides.
    function watchTopic(topic, on) {
        const before = watchers[topic] ?? 0;
        const count = Math.max(0, before + (on ? 1 : -1));
        watchers[topic] = count;
        if (subscribed && (before === 0) !== (count === 0))
            _subscription(count > 0 ? "subscribe" : "unsubscribe", [topic]);
    }

    // Drops a topic's handlers and its subscription (per-watch topics like files:<watchId>).
    function unsubscribeTopic(topic) {
        if (!topicHandlers[topic])
            return;
        delete topicHandlers[topic];
        delete watchers[topic];
        if (subscribed)
            _subscription("unsubscribe", [topic]);
    }

    function _subscription(method, topics) {
        subscribeSocket.send({
            "id": _nextId(),
            "method": method,
            "params": {
                "topics": topics
            }
        });
    }

    function _nextId() {
        return ++requestCounter;
    }

    function _handleResponse(response) {
        if (!response.id) {
            if (response.apiVersion !== undefined)
                apiVersion = response.apiVersion;
            return;
        }
        const pending = pendingRequests[response.id];
        if (!pending)
            return;
        delete pendingRequests[response.id];
        pending.done(response);
    }

    function _failPendingRequests(reason, code) {
        const callbacks = pendingRequests;
        pendingRequests = ({});
        for (const id in callbacks)
            callbacks[id].done({
                "error": reason,
                "code": code
            });
    }

    // A daemon that stops answering (a stuck mpv) must not leave busy flags set forever.
    Timer {
        id: timeoutSweep
        interval: 5000
        repeat: true
        onTriggered: {
            const now = Date.now();
            if (Object.keys(root.pendingRequests).length === 0)
                stop();
            for (const id in root.pendingRequests) {
                const pending = root.pendingRequests[id];
                if (now - pending.sent < root.requestTimeout)
                    continue;
                delete root.pendingRequests[id];
                root.log.warn(pending.method, "timed out");
                pending.done({
                    "error": "the dplayer daemon didn't answer in time",
                    "code": "daemonTimeout"
                });
            }
        }
    }

    // callback(result) gets the result, or {error, code} with a plain-English message
    function call(method, params, callback) {
        const done = response => {
            if (!callback)
                return;
            callback(response.error ? {
                "error": response.error,
                "code": response.code ?? ""
            } : response.result ?? {});
        };
        if (!connected) {
            done({
                "error": "not connected to the dplayer daemon",
                "code": "daemonOffline"
            });
            return;
        }
        const id = _nextId();
        const request = {
            "id": id,
            "method": method
        };
        if (params)
            request.params = params;
        pendingRequests[id] = {
            "done": done,
            "method": method,
            "sent": Date.now()
        };
        timeoutSweep.running = true;
        requestSocket.send(request);
    }

    // call() for commands: a failure is logged and, with failText, shown; then(result) runs on success and
    // failed(error) on failure, e.g. to undo an optimistic change.
    function request(method, params, failText, then, failed) {
        call(method, params, result => {
            if (result.error) {
                root.log.warn(method, result.error);
                if (failText)
                    root.notice(failText);
                failed?.(result.error);
                return;
            }
            then?.(result);
        });
    }
}
