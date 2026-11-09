import QtQuick
import QtQuick.Controls
import qs.Services

// Sits next to a page's scrolling view: makes it the one Router saves offsets from, starts a new route at the top
// and puts back the offset on back and forward. An Item because a pointer handler under a QtObject crashes on setting its parent.
Item {
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
    readonly property bool userMoving: (view?.moving ?? false) || (view?.ScrollBar.vertical?.pressed ?? false)
    // the header's field only sets the query
    readonly property string filter: PlayerService.searchQuery + "\n" + ((Router.pageSearch as ScopedSearch)?.text ?? "")

    // the wheel never sets `moving`
    readonly property WheelHandler _wheel: WheelHandler {
        parent: scroll.view
        acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad
        blocking: false
        onWheel: scroll.pending = -1
    }

    // Home stays built while hidden: it only follows the route while it shows, and registers again on return
    function isKeptHome() {
        for (let p = view; p; p = p.parent) {
            if (p.objectName === "keptHome")
                return true;
        }
        return false;
    }

    function room() {
        return view.contentHeight + view.bottomMargin - view.height;
    }

    function apply() {
        // a page on its way out still sees the route change
        if (pending < 0 || Router.pageScroll !== scroll)
            return;
        placing = true;
        view.contentY = view.originY + Math.max(0, Math.min(pending, room()));
        placing = false;
    }

    onRouteChanged: {
        if (!ready)
            return;
        if (isKeptHome()) {
            if (Router.page !== "home")
                return;
            Router.pageScroll = scroll;
        }
        pending = Router.restoreOffset;
        if (pending < 0)
            view.contentY = view.originY;
        else
            Qt.callLater(apply);
    }
    onUserMovingChanged: {
        if (userMoving)
            pending = -1;
    }
    // a route change resets the field too, and that isn't the user's
    onFilterChanged: {
        if (ready && Router.restoreOffset < 0)
            pending = -1;
    }
    // a layout clamp lands on a bound, so only a move between the bounds counts here
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
        Router.pageScroll = scroll;
        pending = Router.restoreOffset;
        ready = true;
        Qt.callLater(apply);
    }
    Component.onDestruction: {
        if (Router.pageScroll === scroll)
            Router.pageScroll = null;
    }
}
