pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets
import "../Common/Format.js" as Format
import "../Services/Catalog.js" as Catalog

// The play queue: drag the handle to reorder, click to jump; Ctrl/Shift-click or the art's checkbox selects.
// Rows and menus are the queue type's, edited in place from the header's ⋮ ("Edit queue rows").
Item {
    id: root

    // one frame at 60 Hz
    readonly property int autoScrollInterval: 16
    readonly property var items: PlayerService.queue
    readonly property real remaining: Math.max(0, PlayerService.queueLeft - PlayerService.position)
    // the qids in the rows' order; during a drag the rows move ahead of the daemon's queue
    property var shown: []
    property int dragFrom: -1
    property int dragAt: -1
    // where the pointer holds the dragged row, in the list's content
    property real dragY: 0
    property real dragGrab: 0
    // folding the marker moved the rows below it up, while the pointer kept its place
    property real dragShift: 0
    // the marker is folded away during a drag: its extra height would make rows swap back and forth under the pointer
    readonly property real stopMarkerHeight: Theme.spacingXL + Theme.spacingS
    property bool editing: false
    // by qid, so the selection survives reorders
    property var selectedQids: []
    property var anchorQid: null
    readonly property var selectedSet: {
        const set = {};
        for (const q of selectedQids)
            set[q] = true;
        return set;
    }
    // in queue order; entries that left the queue drop out
    readonly property var selectedIndexes: items.map((e, i) => selectedSet[e.qid] ? i : -1).filter(i => i >= 0)
    readonly property bool selecting: selectedIndexes.length > 0
    // the entry playback stops after, 0 for none
    readonly property var stopQid: PlayerService.sleepMode === "afterEntry" ? PlayerService.sleepEntry : PlayerService.sleepMode === "endOfTrack" ? PlayerService.queue[PlayerService.queueIndex]?.qid ?? 0 : 0
    readonly property var lines: UiConfig.pageOptions("queue", "").lines.filter(l => l !== "duration" && l !== "library")

    onItemsChanged: sync()
    onVisibleChanged: {
        if (visible)
            Qt.callLater(showCurrent);
    }
    Component.onCompleted: {
        sync();
        Qt.callLater(showCurrent);
    }

    function showCurrent() {
        if (PlayerService.queueIndex >= 0 && PlayerService.queueIndex < queueList.count)
            queueList.positionViewAtIndex(PlayerService.queueIndex, ListView.Center);
    }

    // Applies the queue's change to the rows as one move, insert or remove, so the list keeps its scroll
    // position and only touched rows are rebuilt.
    function sync() {
        if (dragFrom >= 0)
            return;
        const want = items.map(i => i.qid);
        const edit = Catalog.diff(shown, want);
        // a queue that arrives after the view opened is shown at its current track too
        const first = shown.length === 0;
        shown = want;
        if (first)
            Qt.callLater(showCurrent);
        if (!edit)
            return;
        if (edit.move) {
            rows.move(edit.move[0], edit.move[1], 1);
            return;
        }
        if (edit.remove > 0)
            rows.remove(edit.at, edit.remove);
        if (edit.insert.length > 0)
            rows.insert(edit.at, edit.insert.map(qid => ({
                        "qid": qid
                    })));
    }

    function clearSelection() {
        selectedQids = [];
        anchorQid = null;
    }

    function selectAll() {
        Router.selectionOwner = root;
        selectedQids = items.map(e => e.qid);
        selectionLayer.raise();
    }

    function stopsBefore(index) {
        return dragFrom < 0 && index > 0 && shown[index - 1] === stopQid;
    }

    // modifiers: Qt.ShiftModifier extends from the last picked entry, anything else toggles
    function select(index, modifiers) {
        Router.selectionOwner = root;
        selectionLayer.raise();
        const qid = items[index]?.qid;
        if (qid === undefined)
            return;
        const anchor = items.findIndex(e => e.qid === anchorQid);
        if (modifiers & Qt.ShiftModifier && anchor >= 0) {
            const range = items.slice(Math.min(anchor, index), Math.max(anchor, index) + 1).map(e => e.qid);
            selectedQids = selectedQids.filter(q => range.indexOf(q) < 0).concat(range);
            return;
        }
        anchorQid = qid;
        selectedQids = selectedSet[qid] ? selectedQids.filter(q => q !== qid) : selectedQids.concat([qid]);
    }

    // Several entries go as one removal with one Undo, which puts each back at its place.
    function removeEntries(indexes) {
        const gone = indexes.map(i => items[i]?.qid);
        Router.removeFromQueue(indexes, () => root.selectedQids = root.selectedQids.filter(q => gone.indexOf(q) < 0));
    }

    function edgeDirection() {
        const edge = PlayerMetrics.listRowTwoLineHeight;
        if (dragY - queueList.contentY < edge && queueList.contentY > queueList.originY)
            return -1;
        if (queueList.contentY + queueList.height - dragY < edge && queueList.contentY < queueList.maximumContentY)
            return 1;
        return 0;
    }

    // the row under the pointer; a pointer beyond the list counts as at its edge, so the dragged row stays built
    function dragIndex() {
        const y = Math.max(queueList.contentY, Math.min(queueList.contentY + queueList.height - 1, dragY - dragShift));
        return queueList.indexAt(queueList.width / 2, y);
    }

    function previewMove(to) {
        if (to < 0 || to === dragAt)
            return;
        rows.move(dragAt, to, 1);
        const next = shown.slice();
        next.splice(to, 0, next.splice(dragAt, 1)[0]);
        shown = next;
        dragAt = to;
    }

    function endDrag(apply) {
        const from = dragFrom;
        const to = dragAt;
        if (!apply)
            previewMove(from);
        dragFrom = -1;
        dragAt = -1;
        // sync() skipped queue changes during the drag; the order may also have changed under the dragged row
        const now = items.findIndex(e => e.qid === shown[to]);
        if (!apply || from === to || now < 0) {
            sync();
            return;
        }
        const target = shown.slice(0, to).filter(q => items.some(e => e.qid === q)).length;
        PlayerService.queueMove(now, Math.min(target, items.length - 1), () => root.sync());
    }

    ListModel {
        id: rows
    }

    PositionHold {
        active: root.visible && root.Window.visibility !== Window.Minimized
    }

    Shortcut {
        sequence: StandardKey.SelectAll
        enabled: root.visible && root.selecting && Router.ownsSelectionKeys(root)
        onActivated: root.selectAll()
    }

    readonly property EscapeLayer _selectionLayer: EscapeLayer {
        id: selectionLayer
        owner: root
        active: root.selecting
        onEscaped: root.clearSelection()
    }

    readonly property EscapeLayer _editLayer: EscapeLayer {
        owner: root
        active: root.editing
        onEscaped: root.editing = false
    }

    DMenu {
        id: queueMenu
        model: [
            {
                "id": "edit",
                "text": I18n.tr("Edit queue rows", "queue menu action that opens the queue's edit mode"),
                "icon": "edit"
            },
            {
                "id": "add",
                "text": I18n.tr("Add to playlist…", "track menu action"),
                "icon": "playlist_add",
                "enabled": root.items.length > 0
            }
        ]
        onTriggered: id => {
            if (id === "edit")
                root.editing = true;
            else if (id === "add")
                Router.pickPlaylist(root.items.map(i => PlayerService.track(i)).filter(t => t));
        }
    }

    Item {
        id: heading
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: Theme.spacingL
        height: Math.max(titleColumn.implicitHeight, actions.implicitHeight)

        Column {
            id: titleColumn
            anchors.left: parent.left
            anchors.right: actions.left
            anchors.verticalCenter: parent.verticalCenter
            spacing: Theme.spacingXXS

            StyledText {
                width: parent.width
                text: root.selecting ? Phrases.selected(root.selectedIndexes.length) : I18n.tr("Queue", "the play queue: panel, toggle")
                font.pixelSize: Theme.fontSizeXLarge
                color: Theme.surfaceText
                elide: Text.ElideRight
                horizontalAlignment: Text.AlignLeft
            }

            StyledText {
                width: parent.width
                text: Phrases.join(Phrases.tracks(root.items.length), I18n.tr("%1 left", "time left in the queue, %1 is a duration").arg(Format.duration(root.remaining)))
                font.pixelSize: Theme.fontSizeSmall
                color: Theme.surfaceVariantText
                elide: Text.ElideRight
                horizontalAlignment: Text.AlignLeft
            }
        }

        Row {
            id: actions
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            spacing: Theme.spacingXS

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                visible: !root.selecting && !root.editing
                text: I18n.tr("Clear", "button that empties a list")
                buttonHeight: Theme.buttonHeightXS
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                enabled: root.items.length > 1
                onClicked: PlayerService.queueClear()
            }

            DActionButton {
                id: queueMenuButton
                anchors.verticalCenter: parent.verticalCenter
                visible: !root.selecting && !root.editing
                iconName: "more_vert"
                tooltipText: I18n.tr("More", "track row menu button")
                Accessible.name: tooltipText
                onClicked: queueMenu.open(queueMenuButton)
            }

            DActionButton {
                readonly property bool everything: root.selectedIndexes.length >= root.items.length
                anchors.verticalCenter: parent.verticalCenter
                visible: root.selecting
                iconName: everything ? "deselect" : "select_all"
                tooltipText: everything ? I18n.tr("Select none", "selection bar action") : I18n.tr("Select all", "selection bar action")
                Accessible.name: tooltipText
                onClicked: everything ? root.clearSelection() : root.selectAll()
            }

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                visible: root.selecting
                text: I18n.tr("Remove", "button that removes an item")
                buttonHeight: Theme.buttonHeightXS
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                onClicked: root.removeEntries(root.selectedIndexes)
            }

            DActionButton {
                anchors.verticalCenter: parent.verticalCenter
                visible: root.selecting
                iconName: "close"
                tooltipText: I18n.tr("Clear selection", "selection bar action")
                Accessible.name: tooltipText
                onClicked: root.clearSelection()
            }

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                visible: root.editing && !root.selecting
                text: I18n.tr("Reset", "button that restores the defaults")
                buttonHeight: Theme.buttonHeightXS
                backgroundColor: "transparent"
                textColor: Theme.primary
                onClicked: (editor.item as PageEditor)?.reset()
            }

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                visible: root.editing && !root.selecting
                text: I18n.tr("Done", "edit mode finish button")
                iconName: "check"
                buttonHeight: Theme.buttonHeightXS
                onClicked: root.editing = false
            }
        }
    }

    // Edit queue rows: the queue's info lines and its rows and menus
    Loader {
        id: editor
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: heading.bottom
        anchors.topMargin: Theme.spacingM
        anchors.leftMargin: Theme.spacingM
        anchors.rightMargin: Theme.spacingM
        // a Loader keeps its last size after unloading, which would leave the list pushed down
        height: (item as PageEditor)?.implicitHeight ?? 0
        active: root.editing

        sourceComponent: PageEditor {
            sheetHost: root
            kind: "queue"
            type: "tracks"
            itemType: "queue"
        }
    }

    // near an edge during a drag the list keeps scrolling while the pointer rests
    Timer {
        interval: root.autoScrollInterval
        repeat: true
        running: root.dragFrom >= 0 && root.edgeDirection() !== 0
        onTriggered: {
            const before = queueList.contentY;
            const step = PlayerMetrics.listRowTwoLineHeight / 8 * root.edgeDirection();
            queueList.contentY = Math.max(queueList.originY, Math.min(queueList.maximumContentY, before + step));
            // the pointer stays where it is on screen, so it moves along in the content
            root.dragY += queueList.contentY - before;
            root.previewMove(root.dragIndex());
        }
    }

    DListView {
        id: queueList
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: editor.bottom
        anchors.bottom: parent.bottom
        anchors.topMargin: Theme.spacingM
        // rows are square, so they stop above the card's rounded corners
        anchors.bottomMargin: PlayerMetrics.paneRadius
        anchors.leftMargin: Theme.spacingM
        anchors.rightMargin: Theme.spacingM
        bottomMargin: Theme.spacingL
        clip: true
        spacing: Theme.groupedListGap
        model: rows
        footer: Item {
            width: queueList.width
            height: continueButton.visible ? continueButton.height + Theme.spacingL : 0

            DButton {
                id: continueButton
                anchors.horizontalCenter: parent.horizontalCenter
                anchors.bottom: parent.bottom
                visible: PlayerService.continueKnown && !PlayerService.continuing && root.items.length > 0
                text: I18n.tr("Continue with similar tracks", "queue end button that keeps filling the queue")
                iconName: "all_inclusive"
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                onClicked: PlayerService.continueQueue()
            }
        }
        // reduce motion and speed None make every duration 0, so rows land at once
        readonly property int moveDuration: Theme.animationsEnabled ? Theme.expressiveDurations.expressiveFastSpatial : 0
        readonly property int fadeDuration: Theme.animationsEnabled ? Theme.expressiveDurations.expressiveEffects : 0
        // an add interrupted mid-fade can leave rows displaced at partial opacity, so they always end at 1
        displaced: Transition {
            NumberAnimation {
                property: "y"
                duration: queueList.moveDuration
                easing.type: Easing.BezierSpline
                easing.bezierCurve: Theme.expressiveCurves.expressiveFastSpatial
            }
            NumberAnimation {
                properties: "opacity,scale"
                to: 1
                duration: 0
            }
        }
        move: Transition {
            NumberAnimation {
                property: "y"
                duration: queueList.moveDuration
                easing.type: Easing.BezierSpline
                easing.bezierCurve: Theme.expressiveCurves.expressiveFastSpatial
            }
            NumberAnimation {
                properties: "opacity,scale"
                to: 1
                duration: 0
            }
        }
        add: Transition {
            NumberAnimation {
                property: "opacity"
                from: 0
                to: 1
                duration: queueList.fadeDuration
                easing.type: Easing.BezierSpline
                easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
            }
            NumberAnimation {
                property: "y"
                from: ViewTransition.destination.y + Theme.spacingM
                duration: queueList.fadeDuration
                easing.type: Easing.BezierSpline
                easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
            }
        }
        remove: Transition {
            ParallelAnimation {
                NumberAnimation {
                    property: "opacity"
                    to: 0
                    duration: queueList.fadeDuration
                    easing.type: Easing.BezierSpline
                    easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
                }
                NumberAnimation {
                    property: "x"
                    to: Theme.spacingXL
                    duration: queueList.fadeDuration
                    easing.type: Easing.BezierSpline
                    easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
                }
            }
        }

        delegate: Item {
            id: slot
            required property int index
            required property var qid
            readonly property bool stops: qid === root.stopQid && root.dragFrom < 0
            readonly property bool dragged: handle.dragging
            width: queueList.width
            height: entry.height + (stops ? root.stopMarkerHeight : 0)
            z: dragged ? 1 : 0
            // the view can release a far scrolled delegate, which would strand the drag
            Component.onDestruction: {
                if (dragged)
                    root.endDrag(false);
            }
            // the model only moves a row between slots; this keeps it under the pointer on the way
            transform: Translate {
                y: slot.dragged ? root.dragY - root.dragGrab - slot.y : 0
            }

            DListItem {
                id: entry
                readonly property int index: slot.index
                readonly property var qid: slot.qid
                readonly property var qitem: PlayerService.queueItem(qid)
                readonly property var track: PlayerService.track(qitem)
                readonly property bool current: PlayerService.queue[PlayerService.queueIndex]?.qid === qid
                // "hand" (play next, add to queue), "auto" (the queue continued by itself) or "" for what it was started from
                readonly property string origin: qitem?.byHand === true ? "hand" : qitem?.auto === true ? "auto" : ""
                readonly property bool selected: root.selectedSet[qid] === true
                // a selected row acts on the whole selection
                readonly property var actionIndexes: selected && root.selectedIndexes.length > 1 ? root.selectedIndexes : [index]
                readonly property bool hovering: hovered || leadHover.hovered || rowEnd.pointerInside || menu.opened
                readonly property string info: UiConfig.infoText(track ?? {}, root.lines, UiConfig.pageOptions("queue", "").lineOptions)
                property int pressModifiers: Qt.NoModifier
                width: queueList.width
                implicitHeight: info !== "" ? PlayerMetrics.listRowTwoLineHeight : PlayerMetrics.listRowHeight
                listView: queueList
                surfaceColor: PlayerMetrics.listRowColor
                isSelected: root.selecting ? selected : current
                isHovered: hovered || menu.opened
                firstInGroup: index === 0 || root.stopsBefore(index)
                lastInGroup: index === queueList.count - 1 || slot.stops
                Accessible.name: Phrases.named(track?.title ?? "", track?.artist ?? "")
                Accessible.description: origin === "hand" ? I18n.tr("Added by you", "queue row: a track put in with play next or add to queue") : origin === "auto" ? I18n.tr("Added automatically", "queue row: a track the queue continued with") : ""
                onClicked: {
                    const modifiers = pressModifiers & (Qt.ControlModifier | Qt.ShiftModifier);
                    if (modifiers || root.selecting)
                        root.select(index, modifiers || Qt.ControlModifier);
                    else
                        PlayerService.queueJump(index);
                }
                onContextMenuRequested: (x, y) => menu.open(entry, x, y)

                TapHandler {
                    acceptedButtons: Qt.LeftButton
                    onPressedChanged: {
                        if (pressed)
                            entry.pressModifiers = point.modifiers;
                    }
                }

                TrackMenu {
                    id: menu
                    type: "queue"
                    tracks: entry.actionIndexes.map(i => PlayerService.track(root.items[i])).filter(t => t)
                    queueIndexes: entry.actionIndexes
                    onQueueRemoveRequested: root.removeEntries(entry.actionIndexes)
                }

                DDragHandle {
                    id: handle
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.spacingXS
                    anchors.verticalCenter: parent.verticalCenter
                    coordinateItem: queueList.contentItem
                    label: entry.track?.title ?? ""
                    dragging: root.dragFrom >= 0 && root.dragAt === entry.index
                    canMoveUp: entry.index > 0
                    canMoveDown: entry.index < queueList.count - 1
                    onStarted: position => {
                        root.dragY = position.y;
                        root.dragGrab = position.y - slot.y;
                        const before = slot.y;
                        root.dragFrom = entry.index;
                        root.dragAt = entry.index;
                        // the marker folds away on drag start; its row may not be built, so measure the shift
                        queueList.forceLayout();
                        root.dragShift = before - slot.y;
                    }
                    onMoved: position => {
                        root.dragY = position.y;
                        root.previewMove(root.dragIndex());
                    }
                    onFinished: root.endDrag(true)
                    onDragCanceled: root.endDrag(false)
                    onMoveRequested: delta => PlayerService.queueMove(entry.index, entry.index + delta)
                }

                // the art turns into the selection checkbox on hover and while selecting
                Item {
                    id: art
                    readonly property bool checkbox: root.selecting || leadHover.hovered
                    anchors.left: handle.right
                    anchors.leftMargin: Theme.spacingXS
                    anchors.verticalCenter: parent.verticalCenter
                    width: PlayerMetrics.rowArtSize
                    height: width

                    HoverHandler {
                        id: leadHover
                        cursorShape: Qt.PointingHandCursor
                    }

                    TapHandler {
                        onTapped: root.select(entry.index, Qt.NoModifier)
                    }

                    AlbumArt {
                        anchors.fill: parent
                        visible: !art.checkbox
                        radius: Theme.cornerRadiusS
                        item: entry.track
                    }

                    DIcon {
                        anchors.centerIn: parent
                        visible: art.checkbox
                        name: entry.selected ? "check_box" : "check_box_outline_blank"
                        filled: entry.selected
                        size: Theme.iconSize
                        color: entry.selected ? Theme.primary : entry.supportingContentColor
                        Accessible.role: Accessible.CheckBox
                        Accessible.name: I18n.tr("Select", "track row checkbox")
                        Accessible.checked: entry.selected
                    }
                }

                Column {
                    anchors.left: art.right
                    anchors.leftMargin: Theme.spacingM
                    anchors.right: rowEnd.left
                    anchors.rightMargin: Theme.spacingS
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: Theme.spacingXXS

                    StyledText {
                        width: parent.width
                        text: entry.track?.title ?? ""
                        font.pixelSize: Theme.fontSizeMedium
                        font.weight: Theme.fontWeightMedium
                        color: entry.current ? Theme.primary : entry.contentColor
                        elide: Text.ElideRight
                        wrapMode: Text.NoWrap
                        horizontalAlignment: Text.AlignLeft
                    }

                    StyledText {
                        width: parent.width
                        visible: entry.info !== ""
                        text: entry.info
                        font.pixelSize: Theme.fontSizeSmall
                        color: entry.supportingContentColor
                        elide: Text.ElideRight
                        wrapMode: Text.NoWrap
                        horizontalAlignment: Text.AlignLeft
                    }
                }

                RowEnd {
                    id: rowEnd
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.spacingS
                    anchors.verticalCenter: parent.verticalCenter
                    actions: menu
                    item: entry.track
                    hovering: entry.hovering
                    origin: entry.origin
                    textColor: entry.supportingContentColor
                }

                Keys.onDeletePressed: root.removeEntries(entry.actionIndexes)
                Keys.onPressed: event => {
                    switch (event.key) {
                    case Qt.Key_Backspace:
                        root.removeEntries(entry.actionIndexes);
                        break;
                    case Qt.Key_Menu:
                        menu.open(rowEnd.moreButton ?? entry);
                        break;
                    default:
                        return;
                    }
                    event.accepted = true;
                }
            }

            Item {
                id: stopMarker
                anchors.bottom: parent.bottom
                width: parent.width
                height: root.stopMarkerHeight
                visible: slot.stops

                Rectangle {
                    anchors.left: parent.left
                    anchors.right: stopLabel.left
                    anchors.rightMargin: Theme.spacingS
                    anchors.verticalCenter: parent.verticalCenter
                    height: Theme.dividerWidth
                    color: Theme.primary
                }

                StyledText {
                    id: stopLabel
                    anchors.centerIn: parent
                    text: I18n.tr("Stops here", "queue divider after the track playback stops at")
                    font.pixelSize: Theme.fontSizeSmall
                    color: Theme.primary
                    wrapMode: Text.NoWrap
                }

                Rectangle {
                    anchors.left: stopLabel.right
                    anchors.right: stopClear.left
                    anchors.leftMargin: Theme.spacingS
                    anchors.rightMargin: Theme.spacingXS
                    anchors.verticalCenter: parent.verticalCenter
                    height: Theme.dividerWidth
                    color: Theme.primary
                }

                DActionButton {
                    id: stopClear
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    iconName: "close"
                    tooltipText: I18n.tr("Don't stop", "queue row action that clears stop after this track")
                    Accessible.name: tooltipText
                    onClicked: PlayerService.cancelSleepTimer()
                }
            }
        }
    }

    DButton {
        readonly property int current: PlayerService.queueIndex
        // contentHeight also moves when rows are created, which itemAtIndex can't report
        readonly property bool onScreen: {
            const top = queueList.contentY;
            const bottom = top + Math.min(queueList.height, queueList.contentHeight);
            const row = current >= 0 ? queueList.itemAtIndex(current) : null;
            return row !== null && row.y + row.height > top && row.y < bottom;
        }

        anchors.horizontalCenter: queueList.horizontalCenter
        anchors.bottom: queueList.bottom
        // lifts clear of the footer button when the list's end is in view
        anchors.bottomMargin: Theme.spacingL + (queueList.footerItem ? Math.max(0, Math.min(queueList.footerItem.height, queueList.contentY + queueList.height - queueList.footerItem.y)) : 0)
        buttonHeight: Theme.buttonHeightXS
        visible: current >= 0 && current < queueList.count && !onScreen && root.dragFrom < 0
        text: I18n.tr("Now playing", "Now playing")
        iconName: "my_location"
        backgroundColor: ArtColors.accentContainer
        textColor: ArtColors.onAccentContainer
        onClicked: root.showCurrent()
    }

    TrackDropArea {
        radius: PlayerMetrics.paneRadius
        onDropped: PlayerService.enqueue(Router.dragTracks, false)
    }
}
