pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.DCommon.Widgets

// A DReorderList delegate: a drag handle at the start (keys and the a11y actions move the row too); anchor the
// row's content after `handle`.
DListItem {
    id: root

    required property DReorderList view
    required property int index
    property string label: ""
    property real handleMargin: Theme.spacingS
    readonly property alias handle: dragHandle

    function focusHandle(reason) {
        dragHandle.forceActiveFocus(reason);
    }

    width: view.width
    surfaceColor: SettingsMetrics.rowColor
    firstInGroup: view.order.indexOf(index) === 0
    lastInGroup: view.order.indexOf(index) === view.count - 1
    z: dragHandle.dragging ? 1 : 0
    Accessible.name: label

    DDragHandle {
        id: dragHandle
        anchors.left: parent.left
        anchors.leftMargin: root.handleMargin
        anchors.verticalCenter: parent.verticalCenter
        coordinateItem: root.view
        label: root.label
        dragging: root.view.draggingIndex === root.index
        canMoveUp: root.view.order.indexOf(root.index) > 0
        canMoveDown: root.view.order.indexOf(root.index) < root.view.count - 1
        onStarted: position => root.view.begin(root.index, position)
        onMoved: position => root.view.dragTo(position)
        onFinished: root.view.finish()
        onDragCanceled: root.view.cancel()
        onMoveRequested: delta => root.view.move(root.index, delta)
    }
}
