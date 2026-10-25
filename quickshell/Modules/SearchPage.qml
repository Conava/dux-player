pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import "../Services/Catalog.js" as Catalog

// Search over every enabled library: the result groups in the order the page's sections set.
Item {
    id: root

    readonly property var options: UiConfig.pageOptions("search", "")
    readonly property bool editing: Router.editing
    readonly property var shownSections: (options.sections ?? []).filter(s => !s.hidden).map(s => s.type)
    readonly property string query: PlayerService.searchQuery.trim()
    readonly property var results: Object.assign({}, PlayerService.searchResults, {
        "playlists": query.length >= PlayerMetrics.searchMinLength ? Catalog.filterText(PlayerService.playlists, query) : []
    })
    readonly property var tracks: results.tracks
    readonly property bool empty: shownSections.every(s => (results[s] ?? []).length === 0)

    property var selectedIndexes: []
    property int anchorIndex: -1
    readonly property bool selecting: selectedIndexes.length > 0
    readonly property var selectedSet: {
        const set = {};
        for (const i of selectedIndexes)
            set[i] = true;
        return set;
    }
    readonly property var selectedTracks: selectedIndexes.slice().sort((a, b) => a - b).map(i => tracks[i]).filter(t => t !== undefined)

    // new results are a new list
    onTracksChanged: clearSelection()

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

    component SectionTitle: StyledText {
        width: parent?.width ?? 0
        font.pixelSize: Theme.fontSizeLarge
        font.weight: Theme.fontWeightMedium
        color: Theme.surfaceText
        horizontalAlignment: Text.AlignLeft
    }

    PageHeader {
        id: header
        anchors.top: parent.top
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.leftMargin: PlayerMetrics.pagePadding
        anchors.rightMargin: PlayerMetrics.pagePadding
        title: root.editing ? UiConfig.pageKindLabel("search") : ""
        editable: true
        editing: root.editing
        onEditRequested: Router.editing = true
        onDone: Router.editing = false
        onResetRequested: editor.reset()
    }

    // the shell's launcher search: a pill on the chip surface, large text, the search icon and a clear button
    ScopedSearch {
        id: field
        anchors.top: header.bottom
        anchors.horizontalCenter: parent.horizontalCenter
        width: Math.min(parent.width - PlayerMetrics.pagePadding * 2, SettingsMetrics.contentMaxWidth)
        height: Theme.listItemHeight
        font.pixelSize: Theme.fontSizeLarge
        placeholderText: I18n.tr("Search albums, artists and tracks", "search field placeholder")
        text: PlayerService.searchQuery
        onTextChanged: PlayerService.search(text)
        Component.onCompleted: forceActiveFocus()
    }

    PageEditor {
        id: editor
        anchors.top: field.bottom
        anchors.topMargin: visible ? Theme.spacingL : 0
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.leftMargin: PlayerMetrics.pagePadding
        anchors.rightMargin: PlayerMetrics.pagePadding
        visible: root.editing
        height: visible ? implicitHeight : 0
        sheetHost: root
        kind: "search"
        type: "tracks"
        itemType: "track"
    }

    DFlickable {
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: editor.bottom
        anchors.bottom: parent.bottom
        anchors.topMargin: Theme.spacingL
        contentHeight: column.height + PlayerMetrics.pagePadding + (root.selecting ? selectionBar.height + Theme.spacingL : 0)
        contentWidth: width
        clip: true
        // a square edge fade would paint over the pane's rounded corners
        fadeSideInset: PlayerMetrics.paneRadius

        Column {
            id: column
            x: PlayerMetrics.pagePadding
            width: parent.width - PlayerMetrics.pagePadding * 2
            spacing: Theme.spacingL

            StyledText {
                width: parent.width
                visible: root.empty
                text: root.query === "" ? I18n.tr("Search all enabled libraries.", "search page hint") : I18n.tr("Nothing matches \"%1\".", "search page empty result").arg(PlayerService.searchQuery)
                font.pixelSize: Theme.fontSizeMedium
                color: Theme.surfaceVariantText
                horizontalAlignment: Text.AlignHCenter
            }

            Repeater {
                model: root.shownSections

                Loader {
                    required property string modelData
                    width: column.width
                    visible: (root.results[modelData] ?? []).length > 0
                    sourceComponent: {
                        switch (modelData) {
                        case "artists":
                            return artistsSection;
                        case "albums":
                            return albumsSection;
                        case "playlists":
                            return playlistsSection;
                        }
                        return tracksSection;
                    }
                }
            }
        }
    }

    Component {
        id: artistsSection

        Column {
            spacing: Theme.spacingL

            SectionTitle {
                text: UiConfig.typeLabel("artists")
            }

            Flow {
                width: parent.width
                spacing: Theme.spacingS

                Repeater {
                    model: root.results.artists

                    // a chip with the artist's round picture in place of the icon
                    StyledButton {
                        id: artistChip
                        required property var modelData
                        width: chipRow.width + Theme.spacingL + Theme.spacingXS
                        height: Theme.buttonHeightM
                        radius: Theme.fullRadius(width, height)
                        color: Theme.secondaryContainer
                        Accessible.role: Accessible.Button
                        Accessible.name: modelData.name
                        onClicked: Router.go("artist", modelData.id)

                        StateLayer {
                            control: artistChip
                            stateColor: Theme.onSecondaryContainer
                        }

                        Row {
                            id: chipRow
                            x: Theme.spacingXS
                            anchors.verticalCenter: parent.verticalCenter
                            spacing: Theme.spacingS

                            MediaArt {
                                width: artistChip.height - Theme.spacingXS * 2
                                height: width
                                kind: "artist"
                                item: artistChip.modelData
                            }

                            StyledText {
                                anchors.verticalCenter: parent.verticalCenter
                                text: artistChip.modelData.name
                                font.pixelSize: Theme.fontSizeMedium
                                font.weight: Theme.fontWeightMedium
                                color: Theme.onSecondaryContainer
                            }
                        }
                    }
                }
            }
        }
    }

    Component {
        id: albumsSection

        Column {
            spacing: Theme.spacingL

            SectionTitle {
                text: UiConfig.typeLabel("albums")
            }

            Flow {
                width: parent.width

                Repeater {
                    model: root.results.albums

                    MediaTile {
                        required property var modelData
                        kind: "album"
                        item: modelData
                        lines: UiConfig.pageOptions("albums", "").lines
                    }
                }
            }
        }
    }

    Component {
        id: playlistsSection

        Column {
            spacing: Theme.spacingL

            SectionTitle {
                text: UiConfig.typeLabel("playlists")
            }

            Flow {
                width: parent.width

                Repeater {
                    model: root.results.playlists

                    MediaTile {
                        required property var modelData
                        kind: "playlist"
                        item: modelData
                        lines: UiConfig.pageOptions("playlists", "").lines
                    }
                }
            }
        }
    }

    Component {
        id: tracksSection

        Column {
            spacing: Theme.spacingL

            SectionTitle {
                text: UiConfig.typeLabel("tracks")
            }

            Column {
                width: parent.width
                spacing: Theme.groupedListGap

                Repeater {
                    model: root.tracks

                    TrackRow {
                        required property var modelData
                        required property int index
                        readonly property bool inSelection: root.selectedSet[index] === true
                        width: parent.width
                        track: modelData
                        showArt: true
                        lines: root.options.lines
                        selecting: root.selecting
                        selected: inSelection
                        actionTracks: inSelection && root.selectedIndexes.length > 1 ? root.selectedTracks : [modelData]
                        firstInGroup: index === 0
                        lastInGroup: index === root.tracks.length - 1
                        onActivated: PlayerService.playTracks(PlayerService.refsOf(root.tracks), index, {
                            "type": "search"
                        })
                        onSelectRequested: modifiers => root.select(index, modifiers)
                    }
                }
            }
        }
    }

    SelectionBar {
        id: selectionBar
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.bottom
        anchors.bottomMargin: Theme.spacingL
        width: Math.min(implicitWidth, parent.width - Theme.spacingL * 2)
        visible: root.selecting
        tracks: root.selectedTracks
        total: root.tracks.length
        context: ({
                "type": "search"
            })
        onDone: root.clearSelection()
        onSelectAllRequested: root.selectAll()
    }
}
