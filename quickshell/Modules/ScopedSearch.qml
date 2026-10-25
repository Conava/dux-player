import QtQuick
import qs.Common
import qs.DCommon.Widgets

// A page's search field in the launcher's look, header-sized unless the page says otherwise. Escape clears it,
// then takes the focus away; the "Search this page" shortcut focuses the one showing; a press anywhere else takes the focus away.
DSearchField {
    id: root

    objectName: "scopedSearch"
    width: PlayerMetrics.scopedSearchWidth
    height: Theme.buttonHeightS
    textColor: Theme.onSurface
    font.pixelSize: Theme.fontSizeMedium
    backgroundColor: Theme.chipSurface
    normalBorderColor: Theme.outlineVariant
    onVisibleChanged: claim()
    Component.onCompleted: claim()
    Component.onDestruction: {
        if (Router.pageSearch === root)
            Router.pageSearch = null;
    }

    readonly property EscapeLayer _escapeLayer: EscapeLayer {
        owner: root
        active: root.inputFocused
        onEscaped: {
            if (root.text !== "")
                root.text = "";
            else
                root.setFocus(false);
        }
    }

    function claim() {
        if (visible)
            Router.pageSearch = root;
        else if (Router.pageSearch === root)
            Router.pageSearch = null;
    }

    // over the whole window while the field has focus; a press outside it drops the focus and goes on to
    // whatever is under it
    MouseArea {
        parent: root.Window.window?.contentItem ?? root
        anchors.fill: parent
        z: Theme.zToast + 1
        enabled: root.inputFocused
        acceptedButtons: Qt.AllButtons
        onPressed: mouse => {
            mouse.accepted = false;
            const p = mapToItem(root, mouse.x, mouse.y);
            if (p.x < 0 || p.y < 0 || p.x > root.width || p.y > root.height)
                root.setFocus(false);
        }
    }
}
