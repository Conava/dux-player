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

    readonly property var items: PlayerService.queue
    readonly property real remaining: Math.max(0, PlayerService.queueLeft - PlayerService.position)
    // the qids in the rows' order; during a drag the rows move ahead of the daemon's queue
    property var shown: []
    property int dragFrom: -1
    property int dragAt: -1
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
        if (apply && from !== to)
            PlayerService.queueMove(from, to, () => root.sync());
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
            }
        ]
        onTriggered: root.editing = true
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
        active: root.editing

        sourceComponent: PageEditor {
            sheetHost: root
            kind: "queue"
            type: "tracks"
            itemType: "queue"
        }
    }

    DListView {
        id: queueList
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: editor.bottom
        anchors.bottom: parent.bottom
        anchors.topMargin: Theme.spacingM
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
        displaced: Transition {
            NumberAnimation {
                property: "y"
                duration: Theme.shortDuration
            }
        }
        move: Transition {
            NumberAnimation {
                property: "y"
                duration: Theme.shortDuration
            }
        }

        delegate: DListItem {
            id: entry
            required property int index
            required property var qid
            readonly property var track: PlayerService.track(PlayerService.queueItem(qid))
            readonly property bool current: PlayerService.queue[PlayerService.queueIndex]?.qid === qid
            // put in by hand (play next, add to queue), not part of what the queue was started from
            readonly property bool added: PlayerService.queueItem(qid)?.added === true
            readonly property bool selected: root.selectedSet[qid] === true
            // a selected row acts on the whole selection
            readonly property var actionIndexes: selected && root.selectedIndexes.length > 1 ? root.selectedIndexes : [index]
            readonly property bool hovering: hovered || leadHover.hovered || rowEnd.pointerInside || menu.opened
            readonly property string info: UiConfig.infoText(track ?? {}, root.lines)
            property int pressModifiers: Qt.NoModifier
            width: queueList.width
            implicitHeight: info !== "" ? Theme.listItemTwoLineHeight : Theme.listItemHeight
            isSelected: root.selecting ? selected : current
            isHovered: hovered || menu.opened
            firstInGroup: index === 0
            lastInGroup: index === queueList.count - 1
            z: handle.dragging ? 1 : 0
            Accessible.name: Phrases.named(track?.title ?? "", track?.artist ?? "")
            Accessible.description: added ? I18n.tr("Added by you", "queue row: a track put in with play next or add to queue") : ""
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
                onStarted: {
                    root.dragFrom = entry.index;
                    root.dragAt = entry.index;
                }
                onMoved: position => {
                    // near an edge the list scrolls along
                    const edge = Theme.listItemTwoLineHeight;
                    if (position.y - queueList.contentY < edge)
                        queueList.contentY = Math.max(queueList.originY, queueList.contentY - edge / 4);
                    else if (queueList.contentY + queueList.height - position.y < edge)
                        queueList.contentY = Math.min(queueList.maximumContentY, queueList.contentY + edge / 4);
                    root.previewMove(queueList.indexAt(queueList.width / 2, position.y));
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
                added: entry.added
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
    }

    TrackDropArea {
        radius: PlayerMetrics.paneRadius
        onDropped: PlayerService.enqueue(Router.dragTracks, false)
    }
}
