pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Modules.Settings
import "../Services/Catalog.js" as Catalog

// Every browse view: albums, artists, tracks, playlists, genres, favorites, most played, a genre, a library,
// recently added albums and saved views. Display options come from UiConfig (or the saved view) and change in
// the page's edit mode.
Item {
    id: root

    readonly property var savedView: Router.page === "view" ? UiConfig.effective.savedViews.find(v => v.id === Router.itemId) ?? null : null
    readonly property string libraryShown: Router.page === "library" ? Router.itemId : ""
    // the Favorites tabs in their set order, hidden ones left out
    readonly property var favoriteTabs: (UiConfig.pageOptions("favorites", "")?.sections ?? []).filter(s => !s.hidden).map(s => s.type)
    property string pickedFavorites: ""
    readonly property string favoritesType: favoriteTabs.indexOf(pickedFavorites) >= 0 ? pickedFavorites : favoriteTabs[0] ?? "tracks"
    readonly property var context: {
        const page = Router.page;
        const id = Router.itemId;
        switch (page) {
        case "mostPlayed":
            return {
                "type": "tracks",
                "kind": "mostPlayed",
                "title": UiConfig.pageLabel(page),
                "filters": ["library"],
                "pin": {
                    "kind": "page",
                    "target": page
                }
            };
        case "favorites":
            return {
                "type": favoritesType,
                "kind": "favorites-" + favoritesType,
                "title": UiConfig.pageLabel(page),
                "locked": {
                    "favorites": true
                },
                "filters": ["library"],
                "pin": {
                    "kind": "page",
                    "target": page
                }
            };
        case "genre":
            return {
                "type": "albums",
                "kind": "genre",
                "itemId": id,
                "scope": [I18n.tr("This genre", "page edit scope"), I18n.tr("All genres", "page edit scope")],
                "title": PlayerService.genre(id)?.name ?? "",
                "locked": {
                    "genre": id
                },
                "filters": ["library"],
                "pin": {
                    "kind": "genre",
                    "target": id
                },
                "shelf": {
                    "type": "genre",
                    "id": id
                }
            };
        case "library":
            return {
                "type": "albums",
                "kind": "library",
                "itemId": id,
                "scope": [I18n.tr("This library", "page edit scope"), I18n.tr("All libraries", "page edit scope")],
                "title": PlayerService.libraryName(id),
                "locked": {
                    "library": id
                },
                "filters": ["genre"],
                "pin": {
                    "kind": "library",
                    "target": id
                },
                "shelf": {
                    "type": "library",
                    "id": id
                }
            };
        case "view":
            return {
                "type": savedView?.type ?? "albums",
                "kind": savedView?.type ?? "albums",
                "title": savedView?.name ?? "",
                "filters": UiConfig.filterKinds(savedView?.type ?? "albums"),
                "pin": {
                    "kind": "view",
                    "target": id
                },
                "shelf": {
                    "type": "view",
                    "id": id
                }
            };
        }
        // Home's Recently added opens the albums newest first
        if (page === "albums" && id === "recent")
            return {
                "type": "albums",
                "kind": "albums",
                "title": UiConfig.shelfSourceLabel("recentlyAdded"),
                "sort": {
                    "sort": "added",
                    "descending": true
                },
                "filters": UiConfig.filterKinds("albums"),
                "pin": {
                    "kind": "page",
                    "target": page
                }
            };
        return {
            "type": page,
            "kind": page,
            "title": UiConfig.typeLabel(page),
            "filters": page === "genres" ? [] : page === "playlists" ? ["library"] : UiConfig.filterKinds(page),
            "pin": {
                "kind": "page",
                "target": page
            }
        };
    }
    readonly property string type: context.type
    readonly property string pageItem: context.itemId ?? ""
    readonly property var playContext: ({
            "type": Router.page === "mostPlayed" || Router.page === "tracks" ? "tracks" : Router.page,
            "id": Router.itemId
        })
    readonly property string kind: UiConfig.kindOf(type)
    readonly property var options: {
        if (savedView) {
            const base = UiConfig.kindDefault(savedView.type);
            return Object.assign({}, base, savedView.options, {
                "filters": Object.assign({}, base.filters, savedView.options.filters ?? {})
            });
        }
        return Object.assign({}, UiConfig.pageOptions(context.kind, pageItem), context.sort ?? {});
    }
    readonly property var queryOptions: Object.assign({}, options, {
        "filters": Object.assign({}, options.filters, context.locked ?? {}),
        "base": context.kind === "mostPlayed" ? "mostPlayed" : ""
    })
    readonly property bool editing: Router.editing
    // a page that lists things searches those things; a page of one item searches in it
    readonly property string searchPlaceholder: {
        if (Router.page === "favorites") {
            switch (favoritesType) {
            case "albums":
                return I18n.tr("Search favorite albums", "Favorites page search field, Albums tab");
            case "artists":
                return I18n.tr("Search favorite artists", "Favorites page search field, Artists tab");
            }
            return I18n.tr("Search favorite tracks", "Favorites page search field, Tracks tab");
        }
        if (Router.page === type || context.sort)
            switch (type) {
            case "albums":
                return I18n.tr("Search albums", "Albums page search field");
            case "artists":
                return I18n.tr("Search artists", "Artists page search field");
            case "tracks":
                return I18n.tr("Search tracks", "Tracks page search field");
            case "playlists":
                return I18n.tr("Search playlists", "Playlists page search field");
            case "genres":
                return I18n.tr("Search genres", "Genres page search field");
            }
        return I18n.tr("Search in %1", "a page's own search field, %1 is the page, playlist or artist").arg(context.title);
    }
    // what the header's search field holds; a new page starts unfiltered
    property string filterText: ""
    readonly property string pageKey: Router.page + ":" + Router.itemId
    onPageKeyChanged: filterText = ""
    readonly property var items: Catalog.filterText(allItems, filterText)
    // playlists: grouped by where they live, in the order of the page's sections; a hidden section's are left out
    readonly property bool grouped: type === "playlists" && options.group === "location" && options.layout !== "grid"
    readonly property var locationOrder: (options.sections ?? []).filter(s => !s.hidden).map(s => s.type)
    readonly property var allItems: {
        const list = PlayerService.query(type, queryOptions);
        if (type !== "playlists" || !grouped)
            return list;
        const where = p => p.kind === "server" ? "server" : "local";
        return list.filter(p => locationOrder.indexOf(where(p)) >= 0).map(p => Object.assign({}, p, {
                "location": p.kind === "server" ? I18n.tr("On %1", "playlists section for one server, %1 is the library name").arg(PlayerService.libraryName(p.library)) : I18n.tr("On this device", "playlists section for local playlists")
            })).sort((a, b) => locationOrder.indexOf(where(a)) - locationOrder.indexOf(where(b)));
    }
    // the user narrowed the list (or it is a ranking): shuffling it can't hand the whole thing to the daemon
    readonly property bool filtered: context.kind === "mostPlayed" || filterText.trim() !== "" || Object.keys(options.filters ?? {}).some(k => (options.filters[k] ?? "") !== "" && options.filters[k] !== false)
    readonly property real tileWidth: options.tileSize === "s" ? PlayerMetrics.tileSizeS : options.tileSize === "l" ? PlayerMetrics.tileSizeL : PlayerMetrics.gridCellWidth
    readonly property bool isPlaylists: type === "playlists" && Router.page === "playlists"
    readonly property var homeShelves: UiConfig.surface("home")?.zones.shelves ?? []
    readonly property bool onHome: context.shelf !== undefined && homeShelves.some(s => s.type === context.shelf.type && (s.options.source ?? "") === context.shelf.id)

    onLibraryShownChanged: CatalogService.openedLibrary(libraryShown)

    function setOptions(next) {
        if (savedView) {
            UiConfig.updateSavedView(savedView.id, {
                "options": next
            });
            return;
        }
        UiConfig.setPageOptions(context.kind, pageItem, next);
    }

    function sortBy(key) {
        setOptions(Object.assign({}, options, key === options.sort ? {
            "descending": !options.descending
        } : {
            "sort": key,
            "descending": false
        }));
    }

    function addToHome() {
        UiConfig.setSurface("home", {
            "zones": {
                "shelves": homeShelves.concat([
                    {
                        "type": context.shelf.type,
                        "options": {
                            "source": context.shelf.id
                        }
                    }
                ])
            }
        });
    }

    component Header: Column {
        width: parent?.width ?? 0
        spacing: Theme.spacingS
        bottomPadding: Theme.spacingM

        PageHeader {
            title: root.context.title
            searchPlaceholder: root.searchPlaceholder
            searchText: root.filterText
            onSearchEdited: text => root.filterText = text
            editable: true
            editing: root.editing
            onEditRequested: Router.editing = true
            onDone: Router.editing = false
            onResetRequested: (editorLoader.item as PageEditor)?.reset()
            subtitle: {
                if (root.savedView)
                    return I18n.tr("Saved view · %1", "saved view page subtitle, %1 is the view type").arg(UiConfig.typeLabel(root.type));
                const count = root.items.length === 1 ? I18n.tr("1 item", "browse page item count, singular") : I18n.tr("%1 items", "browse page item count, plural").arg(root.items.length);
                return root.type === "tracks" && CatalogService.tracksLoading ? I18n.tr("%1, loading more…", "browse page item count while tracks still load").arg(count) : count;
            }

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                visible: root.isPlaylists
                text: I18n.tr("Import", "button that imports a file")
                iconName: "file_open"
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                onClicked: Router.prompt(I18n.tr("Import playlist", "playlists page action"), "~/", I18n.tr("Import", "button that imports a file"), path => PlayerService.importPlaylist(Paths.expandTilde(path), id => Router.go("playlist", id)), I18n.tr("File (.m3u, .m3u8 or .pls)", "playlist import path field"))
            }

            DButtonGroup {
                anchors.verticalCenter: parent.verticalCenter
                visible: Router.page === "favorites" && root.favoriteTabs.length > 1
                size: "small"
                model: root.favoriteTabs.map(t => UiConfig.typeLabel(t))
                currentIndex: root.favoriteTabs.indexOf(root.favoritesType)
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        root.pickedFavorites = root.favoriteTabs[index];
                }
            }

            DIconButton {
                anchors.verticalCenter: parent.verticalCenter
                visible: root.context.shelf !== undefined
                iconName: "dashboard_customize"
                enabled: root.context.shelf !== undefined && !root.onHome
                tooltipText: enabled ? I18n.tr("Add to home", "page header action adding a home shelf") : I18n.tr("Already on home", "an item that is already a home shelf")
                Accessible.name: tooltipText
                onClicked: root.addToHome()
            }
        }

        // built only while editing: a hidden editor still re-evaluated on every tab switch
        Loader {
            id: editorLoader
            width: parent.width
            active: root.editing
            visible: active

            sourceComponent: PageEditor {
                id: editor
                sheetHost: root
                kind: root.context.kind
                itemId: root.pageItem
                type: root.type
                scopeThis: root.context.scope?.[0] ?? ""
                scopeAll: root.context.scope?.[1] ?? ""
                layouts: root.type === "tracks" ? ["list", "compact"] : ["list", "grid", "compact"]
                sorts: root.context.sort ? [] : UiConfig.sortChoices(root.type)
                filterKinds: root.context.filters ?? []
                itemType: root.kind
                savedView: root.savedView
                savable: root.savedView === null
                sectionsKind: Router.page === "favorites" ? "favorites" : root.context.kind
                tabs: Router.page === "favorites" ? UiConfig.pageOptions("favorites", "").sections.map(s => s.type) : []
                currentTab: root.favoritesType
                onTabPicked: tab => root.pickedFavorites = tab
                onSaveAsRequested: Router.prompt(I18n.tr("Save view", "save view dialog title"), root.context.title, I18n.tr("Save", "dialog confirm button"), name => Router.go("view", UiConfig.saveView(name, root.type, root.queryOptions)))
                onRenameRequested: Router.prompt(I18n.tr("Rename view", "saved view action"), root.savedView.name, I18n.tr("Save", "dialog confirm button"), name => UiConfig.updateSavedView(root.savedView.id, {
                        "name": name
                    }))
                onDeleteRequested: {
                    const id = root.savedView.id;
                    Router.leave();
                    UiConfig.deleteSavedView(id);
                }

                SettingsToggleRow {
                    visible: root.type === "playlists"
                    text: I18n.tr("Group by location", "page edit mode: playlists under On this device and On <server>")
                    checked: editor.options.group === "location"
                    onToggled: value => editor.apply({
                            "group": value ? "location" : ""
                        })
                }
            }
        }

        DButton {
            visible: root.type === "tracks" && root.items.length > 0 && !root.editing
            text: I18n.tr("Shuffle all", "tracks page action")
            iconName: "shuffle"
            onClicked: PlayerService.shuffleTracks(root.items, root.playContext, root.filtered)
        }

        StyledText {
            width: parent.width
            visible: root.items.length === 0
            topPadding: Theme.spacingL
            text: Router.page === "favorites" && root.filterText === "" ? I18n.tr("No favorites. Mark tracks, albums and artists with the heart.", "favorites empty state") : I18n.tr("Nothing matches these filters.", "browse empty state")
            font.pixelSize: Theme.fontSizeMedium
            color: Theme.surfaceVariantText
            wrapMode: Text.WordWrap
        }
    }

    Loader {
        anchors.fill: parent
        anchors.leftMargin: PlayerMetrics.pagePadding
        anchors.rightMargin: PlayerMetrics.pagePadding
        sourceComponent: root.options.layout === "grid" ? gridComponent : root.type === "tracks" ? tracksComponent : listComponent
    }

    Component {
        id: tracksComponent

        TrackList {
            tracks: root.items
            context: root.playContext
            lines: root.options.lines
            layout: root.options.layout
            sortable: true
            sortKey: root.options.sort
            descending: root.options.descending
            header: Component {
                Header {}
            }
            onSortRequested: key => root.sortBy(key)
        }
    }

    Component {
        id: listComponent

        DListView {
            id: list
            clip: true
            spacing: Theme.groupedListGap
            bottomMargin: PlayerMetrics.pagePadding + (root.isPlaylists ? fab.height + Theme.spacingL : 0)
            model: root.items
            header: Header {
                width: list.width
            }
            section.property: root.grouped ? "location" : ""
            section.delegate: StyledText {
                required property string section
                width: list.width
                topPadding: Theme.spacingL
                bottomPadding: Theme.spacingS
                text: section
                font.pixelSize: Theme.fontSizeMedium
                font.weight: Theme.fontWeightMedium
                color: Theme.primary
            }

            delegate: MediaRow {
                required property var modelData
                required property int index
                width: list.width
                kind: root.kind
                item: modelData
                lines: root.options.lines
                compact: root.options.layout === "compact"
                firstInGroup: index === 0 || (root.grouped && root.items[index - 1]?.location !== modelData.location)
                lastInGroup: index === root.items.length - 1 || (root.grouped && root.items[index + 1]?.location !== modelData.location)
            }
        }
    }

    Component {
        id: gridComponent

        DGridView {
            id: grid
            readonly property int columnCount: Math.max(1, Math.floor(width / root.tileWidth))
            clip: true
            cellWidth: Math.floor(width / columnCount)
            cellHeight: cellWidth + PlayerMetrics.gridCaptionHeight
            bottomMargin: root.isPlaylists ? fab.height + Theme.spacingL : 0
            model: root.items
            header: Header {
                width: grid.width
            }

            delegate: MediaTile {
                required property var modelData
                required property int index
                width: grid.cellWidth
                height: grid.cellHeight
                kind: root.kind
                item: modelData
                lines: root.options.lines
                onActivated: PlayerService.playTracks(root.items, index, root.playContext)
            }
        }
    }

    DFab {
        id: fab
        visible: root.isPlaylists && !root.editing
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.bottom
        anchors.bottomMargin: Theme.spacingL
        text: I18n.tr("New playlist", "action and dialog that create a playlist")
        iconName: "add"
        onClicked: newPlaylist.show()
    }

    NewPlaylistDialog {
        id: newPlaylist
        host: root
    }
}
