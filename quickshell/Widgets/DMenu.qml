pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.DCommon.Common
import qs.DCommon.Widgets

// An action menu: icons, labels, a checked state and dividers, opened under a button or at the pointer.
// model: [{id, text, description, icon, iconFilled, checked, enabled, divider}]; a divider entry only needs
// `divider: true`. description is a second line, e.g. why a disabled entry is disabled. A toggle entry shows
// the state after its action as the icon (filled or outlined) instead of a check mark.
Item {
    id: root

    property var model: []
    property real minimumWidth: Style.fieldDefaultWidth
    property real maximumWidth: Style.dialogMaxWidth
    // opaque: menus open over busy content, and compositors don't reliably blur popups
    property color surfaceColor: Style.surfaceContainer
    property bool opened: false
    // off only where no input serial exists to grab with (scripted tests)
    property bool grabsFocus: true
    property int highlightIndex: -1

    readonly property real padding: Style.spacingS
    readonly property real itemRadius: Math.max(0, Style.windowRadius - padding)
    readonly property var actionIndexes: model.map((entry, i) => entry.divider || entry.enabled === false ? -1 : i).filter(i => i >= 0)

    signal triggered(string id)

    // x, y: a point in anchor's coordinates (the pointer for right-click); omitted, the menu opens below anchor
    function open(anchor, x, y) {
        const window = root.QsWindow.window;
        if (!window || !anchor || model.length === 0)
            return;
        const atPointer = x !== undefined;
        const pos = root.QsWindow.itemPosition(anchor);
        anchorPoint = Qt.point(pos.x + (atPointer ? x : 0), pos.y + (atPointer ? y : anchor.height));
        anchorHeight = atPointer ? 0 : anchor.height;
        highlightIndex = -1;
        opened = true;
    }

    // opens where another menu last opened, e.g. a submenu taking the place of its parent
    function openLike(other) {
        if (!root.QsWindow.window || model.length === 0)
            return;
        anchorPoint = other.anchorPoint;
        anchorHeight = other.anchorHeight;
        highlightIndex = -1;
        opened = true;
    }

    function coverWindow(popup) {
        coverWindowOf(popup, root.QsWindow.window);
    }

    function coverWindowOf(popup, window) {
        popup.anchor.window = window;
        popup.anchor.rect.x = 0;
        popup.anchor.rect.y = 0;
        popup.anchor.edges = Edges.Top | Edges.Left;
        popup.anchor.gravity = Edges.Bottom | Edges.Right;
        popup.implicitWidth = window.width;
        popup.implicitHeight = window.height;
        popup.anchor.updateAnchor();
    }

    function close() {
        opened = false;
    }

    function trigger(index) {
        const entry = model[index];
        if (!entry || entry.divider || entry.enabled === false)
            return;
        close();
        triggered(entry.id);
    }

    function step(delta) {
        if (actionIndexes.length === 0)
            return;
        const at = actionIndexes.indexOf(highlightIndex);
        const next = at < 0 ? (delta > 0 ? 0 : actionIndexes.length - 1) : (at + delta + actionIndexes.length) % actionIndexes.length;
        highlightIndex = actionIndexes[next];
    }

    property point anchorPoint: Qt.point(0, 0)
    property real anchorHeight: 0

    width: 0
    height: 0

    StyledText {
        id: labelProbe
        visible: false
        font.pixelSize: Style.fontSizeMedium
    }

    StyledText {
        id: descriptionProbe
        visible: false
        font.pixelSize: Style.fontSizeSmall
    }

    FontMetrics {
        id: labelMetrics
        font: labelProbe.font
    }

    FontMetrics {
        id: descriptionMetrics
        font: descriptionProbe.font
    }

    Loader {
        active: root.opened

        sourceComponent: PopupWindow {
            id: popup

            readonly property real menuWidth: Math.min(root.maximumWidth, Math.max(root.minimumWidth, items.widest + root.padding * 2))
            readonly property real menuHeight: Math.min(implicitHeight, items.implicitHeight + root.padding * 2)
            readonly property bool upwards: root.anchorPoint.y + menuHeight > height && root.anchorPoint.y - root.anchorHeight - menuHeight >= 0

            // covers the whole window so a click anywhere outside closes the menu
            Component.onCompleted: root.coverWindow(popup)
            color: "transparent"
            grabFocus: root.grabsFocus
            visible: true
            onVisibleChanged: {
                if (!visible)
                    root.close();
            }

            MouseArea {
                anchors.fill: parent
                acceptedButtons: Qt.AllButtons
                onPressed: root.close()
            }

            FocusScope {
                anchors.fill: parent
                focus: true
                Component.onCompleted: forceActiveFocus()

                Keys.onPressed: event => {
                    switch (event.key) {
                    case Qt.Key_Escape:
                        root.close();
                        break;
                    case Qt.Key_Down:
                    case Qt.Key_Tab:
                        root.step(1);
                        break;
                    case Qt.Key_Up:
                    case Qt.Key_Backtab:
                        root.step(-1);
                        break;
                    case Qt.Key_Home:
                        root.highlightIndex = root.actionIndexes[0] ?? -1;
                        break;
                    case Qt.Key_End:
                        root.highlightIndex = root.actionIndexes[root.actionIndexes.length - 1] ?? -1;
                        break;
                    case Qt.Key_Return:
                    case Qt.Key_Enter:
                    case Qt.Key_Space:
                        root.trigger(root.highlightIndex);
                        break;
                    default:
                        return;
                    }
                    event.accepted = true;
                }
            }

            Item {
                id: surface
                x: Math.max(0, Math.min(popup.width - width, I18n.isRtl ? root.anchorPoint.x - width : root.anchorPoint.x))
                y: Math.max(0, popup.upwards ? root.anchorPoint.y - root.anchorHeight - height : Math.min(popup.height - height, root.anchorPoint.y))
                width: popup.menuWidth
                height: popup.menuHeight
                LayoutMirroring.enabled: I18n.isRtl
                LayoutMirroring.childrenInherit: true
                Accessible.role: Accessible.PopupMenu

                // keeps clicks inside the menu from closing it
                MouseArea {
                    anchors.fill: parent
                    acceptedButtons: Qt.AllButtons
                }

                ElevationShadow {
                    anchors.fill: parent
                    level: Style.elevationLevel2
                    fallbackOffset: Style.spacingXS
                    targetRadius: Style.windowRadius
                    targetColor: root.surfaceColor
                    borderColor: Style.outlineMedium
                    borderWidth: Style.layerOutlineWidth
                    shadowEnabled: Style.elevationEnabled && Style.popoutElevationEnabled
                }

                DFlickable {
                    anchors.fill: parent
                    anchors.margins: root.padding
                    contentWidth: width
                    contentHeight: items.implicitHeight
                    fadeEdges: false
                    clip: true

                    Column {
                        id: items

                        // the longest label decides the menu's width
                        readonly property real widest: root.model.reduce((max, entry) => entry.divider ? max : Math.max(max, Math.max(labelMetrics.advanceWidth(entry.text ?? ""), descriptionMetrics.advanceWidth(entry.description ?? "")) + Style.iconSizeMedium * 2 + Style.spacingM * 4), 0)

                        width: parent.width

                        Repeater {
                            id: rows
                            model: root.model

                            Item {
                                id: row
                                required property var modelData
                                required property int index

                                width: items.width
                                height: modelData.divider ? Style.spacingS * 2 + Style.dividerWidth : (modelData.description ?? "") !== "" ? Style.listItemTwoLineHeight : Style.menuItemHeight

                                Rectangle {
                                    visible: row.modelData.divider === true
                                    anchors.centerIn: parent
                                    width: parent.width
                                    height: Style.dividerWidth
                                    color: Style.outlineVariant
                                }

                                StyledButton {
                                    id: action
                                    visible: row.modelData.divider !== true
                                    anchors.fill: parent
                                    enabled: row.modelData.enabled !== false
                                    focusPolicy: Qt.NoFocus
                                    radius: root.itemRadius
                                    color: root.highlightIndex === row.index ? Style.selectedContainer : "transparent"
                                    Accessible.role: Accessible.MenuItem
                                    Accessible.name: row.modelData.text ?? ""
                                    Accessible.description: row.modelData.description ?? ""
                                    Accessible.checkable: row.modelData.checked !== undefined
                                    Accessible.checked: row.modelData.checked === true
                                    onHoveredChanged: {
                                        if (hovered)
                                            root.highlightIndex = row.index;
                                    }
                                    onClicked: root.trigger(row.index)

                                    StateLayer {
                                        control: action
                                        stateColor: Style.onSurface
                                    }

                                    Row {
                                        id: content
                                        anchors.left: parent.left
                                        anchors.leftMargin: Style.spacingM
                                        anchors.verticalCenter: parent.verticalCenter
                                        spacing: Style.spacingM

                                        DIcon {
                                            anchors.verticalCenter: parent.verticalCenter
                                            visible: (row.modelData.icon ?? "") !== ""
                                            name: row.modelData.icon ?? ""
                                            filled: row.modelData.iconFilled === true
                                            size: Style.iconSizeMedium
                                            color: action.enabled ? Style.onSurfaceVariant : Style.onSurface_38
                                        }

                                        Column {
                                            anchors.verticalCenter: parent.verticalCenter
                                            spacing: Style.spacingXXS

                                            StyledText {
                                                text: row.modelData.text ?? ""
                                                font.pixelSize: Style.fontSizeMedium
                                                color: action.enabled ? Style.onSurface : Style.onSurface_38
                                            }

                                            StyledText {
                                                visible: text !== ""
                                                width: Math.min(implicitWidth, root.maximumWidth - Style.iconSizeMedium * 2 - Style.spacingM * 4)
                                                text: row.modelData.description ?? ""
                                                font.pixelSize: Style.fontSizeSmall
                                                color: Style.onSurfaceVariant
                                                elide: Text.ElideRight
                                                wrapMode: Text.NoWrap
                                            }
                                        }

                                        DIcon {
                                            anchors.verticalCenter: parent.verticalCenter
                                            visible: row.modelData.checked === true
                                            name: "check"
                                            size: Style.iconSizeMedium
                                            color: Style.primary
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
