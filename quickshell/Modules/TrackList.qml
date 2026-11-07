pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets
import "../Common/Layouts.js" as Layouts
import "../Common/ColumnDrag.js" as ColumnDrag

// The track list every view shares: rows, multi-select with a selection bar, drag to queue or playlist,
// per-track menu, and on wide windows sortable columns.
Item {
    id: root

    property var tracks: []
    property var lines: ["artist", "album"]
    // each custom line's title, template, sort and width, and the other columns' sort and width
    property var lineOptions: ({})
    // "list" | "compact"
    property string layout: "list"
    property bool showArt: true
    property bool numbered: false
    // columns need a sortable view; album and playlist pages keep their own order
    property bool sortable: false
    property string sortKey: ""
    property bool descending: false
    // the sort key of the number column ("#"), for lists with an order of their own; "" leaves it unsortable
    property string orderKey: ""
    // tracks carry `position` (their place in the playlist) so a sorted list still edits the right entries
    property string playlistId: ""
    // what playing from this list starts, for Continue listening
    property var context: playlistId !== "" ? {
        "type": "playlist",
        "id": playlistId
    } : {
        "type": "tracks"
    }
    property Component header: null
    // a ListView section property such as "day", or ""
    property string sectionProperty: ""
    // or one section name per track
    property var sectionNames: []
    readonly property bool sectioned: sectionProperty !== "" || sectionNames.length > 0
    // rank numbers (1, 2, 3 in list order) in place of covers
    property bool ranked: false
    // a click plays that track alone, not the list from there
    property bool playsOne: false
    // a page sharing this list with others; a new one starts at the top
    property string route: ""
    property real bottomInset: 0

    property var selectedIndexes: []
    property int anchorIndex: -1

    readonly property bool columns: sortable && layout === "list" && width >= PlayerMetrics.compactBreakpoint
    // widths of the columns being dragged, by line; only written to the settings on release
    property var draft: ({})
    readonly property var columnOptions: ({
            "lines": lines,
            "lineOptions": sizedLineOptions
        })
    readonly property var sizedLineOptions: {
        const out = Object.assign({}, lineOptions);
        for (const line of Object.keys(draft))
            out[line] = Object.assign({}, out[line], {
                "size": draft[line]
            });
        return out;
    }
    readonly property var baseColumns: columns ? UiConfig.trackColumns(columnOptions) : []
    readonly property real titleWeight: columns ? UiConfig.titleWeight(columnOptions) : PlayerMetrics.titleColumnWeight
    // The width the title and the fill columns share: the row less everything of fixed width. The layout's own
    // stretch factors don't split it by weight, so each fill column gets its pixels here and the title takes the rest.
    property real endWidth: 0
    readonly property real fillTotal: view.width - Theme.spacingL - (orderKey === "" && showArt ? PlayerMetrics.rowArtSize : PlayerMetrics.columnNumberWidth) - Theme.spacingS - endWidth - Theme.spacingM * (baseColumns.length + 2) - baseColumns.reduce((n, c) => n + (c.width ?? 0), 0)
    readonly property real weightTotal: baseColumns.reduce((n, c) => n + (c.fill ?? 0), titleWeight)
    // pixels per unit of weight; the title never goes under its minimum
    readonly property real pixelsPerWeight: Math.max(fillTotal, PlayerMetrics.columnTitleMinWidth * weightTotal / titleWeight) / weightTotal
    // fill columns carry their weight and the pixels it comes to, as a width
    readonly property var columnList: baseColumns.map(c => Object.assign({}, c, {
            "weight": c.fill,
            "fill": undefined,
            "width": c.fill !== undefined ? c.fill * pixelsPerWeight : c.width
        }))
    readonly property real timeWidth: columns ? UiConfig.timeWidth(columnOptions) : PlayerMetrics.columnTimeWidth
    readonly property string titleSort: Layouts.columnSort(columnOptions, "title")
    // the sort has no header cell (Plays, Last played, ...), so it gets one of its own to show and flip it
    readonly property bool sortHidden: columns && sortKey !== "" && sortKey !== titleSort && sortKey !== "duration" && sortKey !== orderKey && !columnList.some(c => c.sort === sortKey)
    readonly property var selectedSet: {
        const set = {};
        for (const i of selectedIndexes)
            set[i] = true;
        return set;
    }
    readonly property var selectedTracks: selectedIndexes.slice().sort((a, b) => a - b).map(i => tracks[i]).filter(t => t !== undefined)
    readonly property var selectedPositions: positionsOf(selectedIndexes)
    readonly property bool selecting: selectedIndexes.length > 0
    readonly property var vanishing: ({
            "library": "",
            "id": "",
            "title": ""
        })
    readonly property alias view: list

    signal sortRequested(string key)
    // a drag or reset changed these columns' widths: line key to the stored size (1 is Medium)
    signal columnsResized(var sizes)
    // scrolled to the last row, for lists that load more on demand
    signal endReached

    function positionsOf(indexes) {
        return indexes.map(i => root.tracks[i]?.position ?? i);
    }

    function dragTo(cols, index, dx) {
        const r = ColumnDrag.resize(cols, index, dx);
        const next = {};
        next[cols[index].line] = r.left;
        next[cols[index + 1].line] = r.right;
        draft = next;
    }

    function endDrag() {
        const sizes = {};
        for (const line of Object.keys(draft))
            sizes[line] = ColumnDrag.stored(draft[line]);
        if (Object.keys(sizes).length > 0)
            columnsResized(sizes);
        draft = {};
    }

    function clearSelection() {
        selectedIndexes = [];
        anchorIndex = -1;
    }

    function select(index, modifiers) {
        Router.selectionOwner = root;
        selectionLayer.raise();
        if (modifiers & Qt.ShiftModifier && anchorIndex >= 0) {
            const low = Math.min(anchorIndex, index);
            const high = Math.max(anchorIndex, index);
            const next = selectedIndexes.filter(i => i < low || i > high);
            for (let i = low; i <= high; i++)
                next.push(i);
            selectedIndexes = next;
            return;
        }
        anchorIndex = index;
        selectedIndexes = selectedSet[index] ? selectedIndexes.filter(i => i !== index) : selectedIndexes.concat([index]);
    }

    function selectAll() {
        Router.selectionOwner = root;
        selectedIndexes = tracks.map((_, i) => i);
        selectionLayer.raise();
    }

    // a reloaded list keeps the selection; a different one (other length or ends) clears it
    readonly property string signature: tracks.length === 0 ? "" : tracks.length + "|" + CatalogService.key(tracks[0]) + "|" + CatalogService.key(tracks[tracks.length - 1])
    onSignatureChanged: clearSelection()

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

    // the header's row end mirrors the rows' (same hover buttons and slots)
    TrackMenu {
        id: headerActions
        tracks: root.tracks.length > 0 ? [root.tracks[0]] : []
        playlistId: root.playlistId
    }

    component SortCell: DSortHeaderCell {
        property string key: ""
        active: root.sortKey === key
        descending: root.descending
        onSortRequested: root.sortRequested(key)
    }

    // the grip between a header cell and the next one; its parent is the cell
    component ColumnHandle: MouseArea {
        id: handle
        // the header row, which knows the cells
        required property var strip
        // index of the column on its start side
        property int column: 0
        property real startX: 0
        property var cols: []
        readonly property real dir: I18n.isRtl ? -1 : 1

        anchors.left: parent.right
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        width: Theme.spacingM
        hoverEnabled: true
        preventStealing: true
        cursorShape: Qt.SplitHCursor
        Accessible.role: Accessible.Grip
        Accessible.name: I18n.tr("Resize column", "track list column border handle")
        onPressed: mouse => {
            cols = strip.measure();
            startX = mapToItem(root.view, mouse.x, 0).x;
        }
        onPositionChanged: mouse => {
            if (pressed)
                root.dragTo(cols, column, (mapToItem(root.view, mouse.x, 0).x - startX) * dir);
        }
        onReleased: root.endDrag()
        onCanceled: root.endDrag()
        onDoubleClicked: {
            root.draft = {};
            const sizes = {};
            sizes[strip.lineAt(column)] = 1;
            sizes[strip.lineAt(column + 1)] = 1;
            root.columnsResized(sizes);
        }

        Rectangle {
            anchors.centerIn: parent
            width: Theme.groupedListGap
            height: parent.height
            radius: Theme.fullRadius(width, height)
            color: Theme.primary
            visible: handle.pressed || handle.containsMouse
        }
    }

    DListView {
        id: list
        anchors.fill: parent
        clip: true
        spacing: Theme.groupedListGap
        bottomMargin: PlayerMetrics.pagePadding + root.bottomInset + (root.selecting ? selectionBar.height + Theme.spacingL : 0)
        // not the array: Qt copies an array model item by item (a quarter second at 100k tracks), and a re-sorted
        // list of the same length keeps its rows
        model: RowModel {
            rows: root.tracks.length
            groups: root.sectionNames.length > 0 ? root.sectionNames : root.sectionProperty === "" ? [] : root.tracks.map(t => String(t[root.sectionProperty] ?? ""))
        }
        // unset, the view makes the first row current as rows arrive, and that row takes the focus
        currentIndex: -1
        readonly property string route: root.route
        onRouteChanged: contentY = originY
        onAtYEndChanged: {
            if (atYEnd && count > 0)
                root.endReached();
        }
        header: Column {
            width: list.width

            Loader {
                width: parent.width
                sourceComponent: root.header
            }

            // mirrors TrackRow's wide layout
            RowLayout {
                id: header
                width: parent?.width ?? 0
                visible: root.columns && root.tracks.length > 0
                spacing: Theme.spacingM

                Binding {
                    target: root
                    property: "endWidth"
                    value: headerEnd.implicitWidth
                }

                // the resizable columns, title first, as the drag math wants them
                function lineAt(k) {
                    return k === 0 ? "title" : root.columnList[k - 1].line;
                }

                function measure() {
                    const cells = [titleCell].concat(root.columnList.map((_, i) => columnRepeater.itemAt(i)));
                    return cells.map((cell, k) => {
                        const line = lineAt(k);
                        const spec = k === 0 ? null : root.columnList[k - 1];
                        const size = Layouts.columnSize(root.columnOptions, line);
                        return {
                            "line": line,
                            "fill": k === 0 || spec.weight !== undefined,
                            "size": size,
                            "base": (k === 0 ? root.titleWeight : spec.weight ?? spec.width) / size,
                            "px": cell.width,
                            "min": k === 0 ? PlayerMetrics.columnTitleMinWidth : 0
                        };
                    });
                }

                Item {
                    visible: root.orderKey === ""
                    Layout.leftMargin: Theme.spacingL
                    Layout.preferredWidth: root.showArt ? PlayerMetrics.rowArtSize : PlayerMetrics.columnNumberWidth
                }

                SortCell {
                    visible: root.orderKey !== ""
                    Layout.leftMargin: Theme.spacingL - Theme.spacingS
                    Layout.preferredWidth: PlayerMetrics.columnNumberWidth + Theme.spacingS
                    label: I18n.tr("#", "track list column: the position in a playlist")
                    key: root.orderKey
                }

                SortCell {
                    id: titleCell
                    Layout.fillWidth: true
                    Layout.preferredWidth: root.titleWeight
                    Layout.minimumWidth: PlayerMetrics.columnTitleMinWidth
                    label: UiConfig.sortLabel("title")
                    key: root.titleSort

                    ColumnHandle {
                        strip: header
                        visible: root.columnList.length > 0
                        column: 0
                    }
                }

                Repeater {
                    id: columnRepeater
                    // a count, so a drag that changes the widths keeps the cells (and the handle being dragged)
                    model: root.columnList.length

                    SortCell {
                        required property int index
                        readonly property var modelData: root.columnList[index] ?? ({})
                        Layout.preferredWidth: modelData.width
                        label: modelData.label
                        key: modelData.sort

                        // the last column borders the length slot, which has no handle
                        ColumnHandle {
                            strip: header
                            visible: parent.index < root.columnList.length - 1
                            column: parent.index + 1
                        }
                    }
                }

                RowEnd {
                    id: headerEnd
                    Layout.rightMargin: Theme.spacingS
                    actions: headerActions
                    item: root.tracks[0] ?? null
                    fixedLibrary: true
                    timeWidth: root.timeWidth
                    timeHeader: Component {
                        SortCell {
                            width: root.timeWidth
                            label: UiConfig.sortLabel("duration")
                            key: "duration"
                            align: Text.AlignRight
                        }
                    }
                }
            }

            Item {
                width: parent.width
                height: root.sortHidden ? hiddenSort.height + Theme.spacingXS : 0
                visible: root.sortHidden

                SortCell {
                    id: hiddenSort
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.spacingL
                    label: UiConfig.sortLabel(root.sortKey, root.lineOptions)
                    key: root.sortKey
                }
            }

            // keeps the first row off the header
            Item {
                width: 1
                height: root.columns && root.tracks.length > 0 ? Theme.spacingXS : 0
            }
        }

        section.property: root.sectioned ? "group" : ""
        // without a section property a count model would still get a header per row
        section.delegate: root.sectioned ? sectionHeader : null

        Component {
            id: sectionHeader

            StyledText {
                required property string section
                width: list.width
                topPadding: Theme.spacingL
                bottomPadding: Theme.spacingS
                text: section
                font.pixelSize: Theme.fontSizeMedium
                font.weight: Theme.fontWeightMedium
                color: Theme.primary
            }
        }

        delegate: TrackRow {
            required property int index
            // a shorter list reaches the rows a moment before the count drops and the extra rows go
            readonly property var modelData: root.tracks[index] ?? root.vanishing
            readonly property bool inSelection: root.selectedSet[index] === true
            width: list.width
            track: CatalogService.withMarks(modelData)
            number: root.ranked ? index + 1 : root.numbered ? (modelData.position !== undefined ? modelData.position + 1 : modelData.number ?? index + 1) : 0
            showArt: root.showArt
            columns: root.columns
            compact: root.layout === "compact"
            lines: root.lines
            lineOptions: root.lineOptions
            columnSpecs: root.columnList
            titleWeight: root.titleWeight
            timeWidth: root.timeWidth
            selecting: root.selecting
            selected: inSelection
            actionTracks: inSelection && root.selectedIndexes.length > 1 ? root.selectedTracks : [modelData]
            playlistId: root.playlistId
            playlistIndexes: inSelection && root.selectedIndexes.length > 1 ? root.selectedPositions : root.positionsOf([index])
            // sections (discs, days) each start a group of their own
            firstInGroup: index === 0 || ListView.section !== ListView.previousSection
            lastInGroup: index === root.tracks.length - 1 || ListView.section !== ListView.nextSection
            onActivated: root.playsOne ? PlayerService.playTracks([modelData], 0, root.context) : PlayerService.playTracks(root.tracks, index, root.context)
            onSelectRequested: modifiers => root.select(index, modifiers)
        }
    }

    SelectionBar {
        id: selectionBar
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.bottom
        anchors.bottomMargin: Theme.spacingL + root.bottomInset
        width: Math.min(implicitWidth, parent.width - Theme.spacingL * 2)
        visible: root.selecting
        tracks: root.selectedTracks
        total: root.tracks.length
        playlistId: root.playlistId
        context: root.context
        onRemoveRequested: {
            Router.removeFromPlaylist(root.playlistId, root.selectedPositions);
            root.clearSelection();
        }
        onDone: root.clearSelection()
        onSelectAllRequested: root.selectAll()
    }
}
