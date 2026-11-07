import QtQuick

// Sits next to a page's scrolling view: makes it the one Router saves offsets from, starts a new route at the top
// and puts back the offset on back and forward.
QtObject {
    id: scroll

    required property Flickable view
    // where the view belongs, -1 for nowhere. Held until the user scrolls, so late rows and headers still land right.
    property real pending: -1
    property bool ready: false
    property bool placing: false

    readonly property string route: Router.route
    readonly property real viewY: view?.contentY ?? 0
    readonly property real viewOrigin: view?.originY ?? 0
    readonly property real viewContent: view?.contentHeight ?? 0
    readonly property real viewHeight: view?.height ?? 0

    function room() {
        return view.contentHeight + view.bottomMargin - view.height;
    }

    function apply() {
        // a page on its way out still sees the route change
        if (pending < 0 || Router.pageView !== view)
            return;
        placing = true;
        view.contentY = view.originY + Math.max(0, Math.min(pending, room()));
        placing = false;
    }

    onRouteChanged: {
        if (!ready)
            return;
        pending = Router.restoreOffset;
        if (pending < 0)
            view.contentY = view.originY;
        else
            Qt.callLater(apply);
    }
    // the wheel never sets `moving`, and a layout clamp lands on a bound, so only a move between the bounds is the user's
    onViewYChanged: {
        if (!ready || placing)
            return;
        const y = view.contentY - view.originY;
        if (y > 1 && y < room() - 1)
            pending = -1;
    }
    onViewOriginChanged: Qt.callLater(apply)
    onViewContentChanged: Qt.callLater(apply)
    onViewHeightChanged: Qt.callLater(apply)

    Component.onCompleted: {
        Router.pageView = view;
        pending = Router.restoreOffset;
        ready = true;
        Qt.callLater(apply);
    }
    Component.onDestruction: {
        if (Router.pageView === view)
            Router.pageView = null;
    }
}
