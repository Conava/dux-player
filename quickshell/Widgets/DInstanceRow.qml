pragma ComponentBehavior: Bound

import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// One element in a DReorderList: drag handle, icon, title and subtitle, a chevron when it has options, the eye
// (shown/hidden) and remove. Ctrl+Up/Down move it, Delete/Backspace remove it, Menu or Shift+F10 ask for a menu.
DListItem {
    id: root

    required property DReorderList view
    required property int index
    property string iconName: ""
    property string title: ""
    property string subtitle: ""
    // replaces the subtitle, in error color
    property string warning: ""
    property bool hidden: false
    // false for elements that can't be hidden (a spacer); the eye's space is kept so columns line up
    property bool hideable: true
    // false keeps the eye but disables it; say why in subtitle
    property bool hideEnabled: true
    property bool removable: true
    property bool hasOptions: false
    // extra controls before the chevron (buttons that act on the element)
    property alias actions: extra.data
    // the title and icon in the supporting color: hidden, or not available in this version
    property bool dimmed: hidden

    readonly property bool dragging: (view?.draggingIndex ?? -1) === index
    readonly property int position: view?.order.indexOf(index) ?? -1
    readonly property string shownSubtitle: warning !== "" ? warning : hidden ? I18n.tr("Hidden", "element in an editor, hidden but kept in its place") : subtitle
    readonly property color titleColor: dimmed && enabled && !isSelected ? supportingContentColor : contentColor

    signal optionsRequested
    signal hiddenToggled(bool hidden)
    signal removeRequested

    // DReorderList calls it to restore focus after a keyboard move
    function focusHandle(reason) {
        root.forceActiveFocus(reason);
    }

    width: view?.width ?? 0
    height: implicitHeight
    focusPolicy: Qt.StrongFocus
    isSelected: dragging
    firstInGroup: dragging || position === 0
    lastInGroup: dragging || position === (view?.count ?? 0) - 1
    z: dragging ? 1 : 0
    Accessible.name: title
    Accessible.description: shownSubtitle
    onClicked: {
        if (hasOptions)
            optionsRequested();
    }

    Keys.onPressed: event => {
        const ctrl = event.modifiers & Qt.ControlModifier;
        switch (event.key) {
        case Qt.Key_Delete:
        case Qt.Key_Backspace:
            if (root.removable)
                root.removeRequested();
            event.accepted = true;
            return;
        case Qt.Key_Up:
        case Qt.Key_Down:
            if (!ctrl)
                return;
            root.view.move(root.index, event.key === Qt.Key_Up ? -1 : 1);
            event.accepted = true;
            return;
        case Qt.Key_Menu:
            root.contextMenuRequested(root.width / 2, root.height / 2);
            event.accepted = true;
            return;
        case Qt.Key_F10:
            if (!(event.modifiers & Qt.ShiftModifier))
                return;
            root.contextMenuRequested(root.width / 2, root.height / 2);
            event.accepted = true;
            return;
        }
    }

    Behavior on y {
        enabled: (root.view?.animateLayout ?? false) && !root.dragging && !Style.reduceMotion && Style.currentAnimationSpeed !== Style.AnimationSpeed.None
        DAnim {
            duration: Style.expressiveDurations.expressiveDefaultSpatial
            easing.bezierCurve: Style.expressiveCurves.expressiveFastSpatial
        }
    }

    DDragHandle {
        id: handle
        anchors.left: parent.left
        anchors.leftMargin: Style.spacingS
        anchors.verticalCenter: parent.verticalCenter
        focusPolicy: Qt.NoFocus
        coordinateItem: root.view
        label: root.title
        enabled: (root.view?.externalDrag ?? false) || (root.view?.count ?? 0) > 1
        dragging: root.dragging
        canMoveUp: root.position > 0
        canMoveDown: root.position < (root.view?.count ?? 0) - 1
        onStarted: position => root.view.begin(root.index, position)
        onMoved: position => root.view.dragTo(position)
        onFinished: root.view.finish()
        onDragCanceled: root.view.cancel()
        onMoveRequested: delta => root.view.move(root.index, delta)
    }

    DIcon {
        id: icon
        anchors.left: handle.right
        anchors.leftMargin: Style.spacingS
        anchors.verticalCenter: parent.verticalCenter
        visible: root.iconName !== ""
        name: root.iconName
        size: Style.iconSize
        color: !root.enabled ? Style.onSurface_38 : root.dimmed && !root.isSelected ? root.supportingContentColor : Style.primary
    }

    Column {
        anchors.left: icon.visible ? icon.right : handle.right
        anchors.leftMargin: Style.spacingM
        anchors.right: trailing.left
        anchors.rightMargin: Style.spacingS
        anchors.verticalCenter: parent.verticalCenter

        StyledText {
            width: parent.width
            text: root.title
            font.pixelSize: Style.fontSizeMedium
            color: root.titleColor
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
            horizontalAlignment: Text.AlignLeft
        }

        StyledText {
            width: parent.width
            visible: text !== ""
            text: root.shownSubtitle
            font.pixelSize: Style.fontSizeSmall
            color: root.warning !== "" && root.enabled ? Style.error : root.supportingContentColor
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
            horizontalAlignment: Text.AlignLeft
        }
    }

    Row {
        id: trailing
        anchors.right: parent.right
        anchors.rightMargin: Style.spacingS
        anchors.verticalCenter: parent.verticalCenter
        spacing: Style.spacingXS

        Row {
            id: extra
            anchors.verticalCenter: parent.verticalCenter
            spacing: Style.spacingXS
        }

        DIcon {
            anchors.verticalCenter: parent.verticalCenter
            visible: root.hasOptions
            name: "chevron_right"
            size: Style.iconSize
            color: root.supportingContentColor
            rotation: I18n.isRtl ? 180 : 0
        }

        Item {
            anchors.verticalCenter: parent.verticalCenter
            width: eye.width
            height: eye.height

            DActionButton {
                id: eye
                visible: root.hideable
                checkable: true
                checked: !root.hidden
                enabled: root.hideEnabled
                iconName: checked ? "visibility" : "visibility_off"
                iconColor: checked ? Style.primary : Style.onSurfaceVariant
                tooltipText: checked ? I18n.tr("Hide", "eye button on an editor row") : I18n.tr("Show", "eye button on an editor row")
                Accessible.role: Accessible.CheckBox
                Accessible.name: I18n.tr("Show %1", "eye button on an editor row, %1 is the element").arg(root.title)
                Accessible.checked: checked
                onClicked: root.hiddenToggled(!root.hidden)
            }
        }

        // kept empty when the row can't be removed, so the eyes of a list line up
        Item {
            anchors.verticalCenter: parent.verticalCenter
            width: remove.width
            height: remove.height

            DActionButton {
                id: remove
                visible: root.removable
                iconName: "close"
                iconColor: Style.error
                tooltipText: I18n.tr("Remove", "button that removes an item")
                Accessible.name: I18n.tr("Remove %1", "button on an editor row, %1 is the element").arg(root.title)
                onClicked: root.removeRequested()
            }
        }
    }
}
