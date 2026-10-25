import QtQuick
import QtQuick.Controls
import qs.DCommon.Common
import qs.DCommon.Widgets
import "../Common/WheelInput.js" as WheelInput

// A horizontal list for rows inside a vertical scroller (carousels, shelves). DListView is vertical-only: its
// wheel handler scrolls contentY and accepts every event, so a horizontal DListView swallows the page's
// scrolling. Here sideways wheel and Shift+wheel scroll the row; vertical wheel goes on to the nearest
// scrolling ancestor.
ListView {
    id: root

    property real mouseWheelSpeed: Style.spacingXL * 3
    readonly property string forwardName: "forwardWheel"

    function wheelTarget() {
        for (let item = parent; item; item = item.parent) {
            // a DFlickable or DListView, found by duck typing
            if (typeof item[forwardName] === "function")
                return item;
        }
        return null;
    }

    orientation: ListView.Horizontal
    flickableDirection: Flickable.HorizontalFlick
    boundsBehavior: Flickable.StopAtBounds
    clip: true
    reuseItems: true

    WheelHandler {
        id: wheel
        acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad
        property var forwardedTo: null

        onWheel: event => {
            if (!WheelInput.isHorizontal(event)) {
                const target = root.wheelTarget();
                if (target) {
                    target[root.forwardName](event);
                    forwardedTo = target;
                }
                event.accepted = true;
                return;
            }
            const touchpad = WheelInput.isTouchpad(event);
            const delta = touchpad ? WheelInput.dominantDelta(event.pixelDelta) : WheelInput.dominantDelta(event.angleDelta) / 120 * root.mouseWheelSpeed;
            const maxX = Math.max(root.originX, root.contentWidth - root.width + root.originX);
            root.contentX = Math.max(root.originX, Math.min(maxX, root.contentX - delta));
            event.accepted = true;
        }

        onActiveChanged: {
            if (active || !forwardedTo)
                return;
            forwardedTo[root.forwardName + "End"]?.();
            forwardedTo = null;
        }
    }

    ScrollBar.horizontal: DHorizontalScrollbar {
        targetFlickable: root
    }
}
