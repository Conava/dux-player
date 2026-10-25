pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// The track list every view shares: rows, multi-select with a selection bar, drag to queue or playlist,
// per-track menu, and on wide windows sortable columns.
Item {
    id: root

    property var tracks: []
    property var lines: ["artist", "album"]
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
    property real bottomInset: 0

    property var selectedIndexes: []
    property int anchorIndex: -1

    readonly property bool columns: sortable && layout === "list" && width >= PlayerMetrics.compactBreakpoint
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
    // scrolled to the last row, for lists that load more on demand
    signal endReached

    function positionsOf(indexes) {
        return indexes.map(i => root.tracks[i]?.position ?? i);
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

    DListView {
        id: list
        anchors.fill: parent
        clip: true
        spacing: Theme.groupedListGap
        bottomMargin: PlayerMetrics.pagePadding + root.bottomInset + (root.selecting ? selectionBar.height + Theme.spacingL : 0)
        // a count, not the array: Qt copies an array model item by item (a quarter second at 100k tracks), and
        // a re-sorted list of the same length keeps its rows. Sections need roles, so they take the array.
        model: root.sectionProperty === "" ? root.tracks.length : root.tracks
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
                width: parent?.width ?? 0
                visible: root.columns
                spacing: Theme.spacingM

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
                    Layout.fillWidth: true
                    Layout.preferredWidth: 3
                    label: UiConfig.sortLabel("title")
                    key: "title"
                }

                Repeater {
                    model: root.columns ? UiConfig.trackColumns(root.lines.filter(l => l !== "duration" && l !== "library")) : []

                    SortCell {
                        required property var modelData
                        Layout.fillWidth: modelData.fill !== undefined
                        Layout.preferredWidth: modelData.fill ?? modelData.width
                        label: UiConfig.lineLabel(modelData.line)
                        key: modelData.line
                    }
                }

                RowEnd {
                    Layout.rightMargin: Theme.spacingS
                    actions: headerActions
                    item: root.tracks[0] ?? null
                    fixedLibrary: true
                    timeHeader: Component {
                        SortCell {
                            width: PlayerMetrics.columnTimeWidth
                            label: UiConfig.sortLabel("duration")
                            key: "duration"
                            align: Text.AlignRight
                        }
                    }
                }
            }
        }

        section.property: root.sectionProperty
        // without a section property a count model would still get a header per row
        section.delegate: root.sectionProperty !== "" ? sectionHeader : null

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
            number: root.numbered ? (modelData.position !== undefined ? modelData.position + 1 : modelData.number ?? index + 1) : 0
            showArt: root.showArt
            columns: root.columns
            compact: root.layout === "compact"
            lines: root.lines
            selecting: root.selecting
            selected: inSelection
            actionTracks: inSelection && root.selectedIndexes.length > 1 ? root.selectedTracks : [modelData]
            playlistId: root.playlistId
            playlistIndexes: inSelection && root.selectedIndexes.length > 1 ? root.selectedPositions : root.positionsOf([index])
            firstInGroup: index === 0
            lastInGroup: index === root.tracks.length - 1
            onActivated: PlayerService.playTracks(root.tracks, index, root.context)
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
