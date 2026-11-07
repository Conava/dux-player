pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets
import qs.Modules.Settings
import "../Common/Layouts.js" as Layouts
import "../Services/Rules.js" as Rules

// Edit page: the block at the top of a page in edit mode. It edits this page's change or, for kinds with many
// pages, the kind's default; the list below reads the same options, so it follows every edit live.
Item {
    id: root

    property string kind: ""
    // the playlist, album, artist, genre or library for kinds with many pages
    property string itemId: ""
    // the browse type whose info lines and sorts apply
    property string type: "tracks"
    // "This playlist" and "All playlists"; empty for kinds with one page
    property string scopeThis: ""
    property string scopeAll: ""
    // layout values offered; fewer than two means the layout is fixed
    property var layouts: []
    // sort keys offered; none means the page keeps its own order
    property var sorts: []
    property bool linesEditable: true
    // the media type whose rows and menus every page of it shares; "" for none
    property string itemType: ""
    // the kind whose sections show here: the Favorites tabs live on "favorites", each tab's options on its own
    property string sectionsKind: kind
    // the sections that pick which list is edited below them (the Favorites tabs), and the one picked
    property var tabs: []
    property string currentTab: ""
    // a saved view's record: edits go into it, and it can be renamed and deleted here
    property var savedView: null
    property bool savable: false
    // where the add pickers open: the page
    property Item sheetHost: null
    // the block takes at most this share of the page and scrolls on its own, so the list stays in view
    readonly property real maxShare: 0.6
    property real pageHeight: sheetHost?.height ?? Infinity
    default property alias extra: listGroup.data

    readonly property int scope: Router.editScope
    readonly property bool allScope: scopeThis !== "" && scope === 1
    readonly property var options: {
        if (savedView)
            return UiConfig.savedViewOptions(savedView);
        return (allScope ? UiConfig.kindDefault(kind) : UiConfig.pageOptions(kind, itemId)) ?? {};
    }
    readonly property var sectionsOptions: sectionsKind === kind ? options : UiConfig.pageOptions(sectionsKind, "") ?? {}
    readonly property var sections: sectionsOptions.sections ?? []
    readonly property var layoutChoices: UiConfig.layoutChoices(type).filter(l => layouts.indexOf(l.value) >= 0)
    readonly property var libraries: PlayerService.libraries.filter(l => l.enabled)
    readonly property var lineChoices: UiConfig.lineChoices(type).filter(l => type !== "tracks" || ["duration", "library"].indexOf(l) < 0)
    readonly property var item: itemType !== "" ? UiConfig.item(itemType) : null
    readonly property var itemZoneMeta: itemType !== "" ? UiConfig.zonesOf(itemType) : []

    // whether the page applies filter rules; only the browse pages do
    property bool filterable: false

    signal tabPicked(string tab)
    signal saveAsRequested
    signal renameRequested
    signal deleteRequested

    function apply(patch) {
        const next = Object.assign({}, options, patch);
        if (savedView)
            UiConfig.updateSavedView(savedView.id, {
                "options": next
            });
        else if (allScope)
            UiConfig.setKindDefault(kind, next);
        else
            UiConfig.setPageOptions(kind, itemId, next);
    }

    function applyFilters(patch) {
        apply({
            "filters": Object.assign({}, options.filters, patch)
        });
    }

    function applySections(list) {
        if (sectionsKind === kind)
            apply({
                "sections": list
            });
        else
            UiConfig.setPageOptions(sectionsKind, "", Object.assign({}, sectionsOptions, {
                "sections": list
            }));
    }

    // clears what the scope shows: this page's change, or the kind's default
    function reset() {
        ruleEditor.discard();
        if (savedView) {
            const id = savedView.id;
            // the options are the view's whole definition
            Router.confirm(I18n.tr("Reset %1?", "settings pages: reset confirmation title, %1 is the page").arg(savedView.name), I18n.tr("Filters, sort and display options are reset.", "reset saved view confirmation"), I18n.tr("Reset", "button that restores the defaults"), () => UiConfig.updateSavedView(id, {
                    "options": {}
                }));
            return;
        }
        if (allScope)
            UiConfig.resetKindDefault(kind);
        else
            UiConfig.resetPageOptions(kind, itemId);
        if (sectionsKind !== kind)
            UiConfig.resetPageOptions(sectionsKind, "");
    }

    // a custom column is a sort only while it is shown
    readonly property var sortList: type === "tracks" && sorts.length > 0 ? sorts.concat((options.lines ?? []).filter(Layouts.isCustomLine)) : sorts
    // track pages edit their info lines as the list's columns, between the fixed Title and Length
    readonly property bool columnsEditor: type === "tracks"
    readonly property var lineOptions: options.lineOptions ?? ({})
    readonly property var columnKeys: columnsEditor ? ["title"].concat(options.lines ?? [], ["duration"]) : options.lines ?? []
    // the column whose card is open
    property string openColumn: ""
    readonly property bool columnOpen: columnsEditor && openColumn !== "" && columnKeys.indexOf(openColumn) >= 0
    readonly property bool openIsCustom: Layouts.isCustomLine(openColumn)
    readonly property string linesTitle: columnsEditor ? I18n.tr("Columns", "page edit mode section: the columns of a track list") : I18n.tr("Info lines", "page edit mode section: the facts shown under an item")
    readonly property string addColumnText: I18n.tr("Add column", "button under the columns in a page's edit mode")
    readonly property var columnSortChoices: (openIsCustom ? [openColumn] : []).concat(UiConfig.sortChoices("tracks"))

    function sortText(key) {
        return UiConfig.sortLabel(key, lineOptions);
    }

    // one column's options: undefined removes a key, an empty record removes the column's entry
    function applyColumn(key, patch) {
        const next = Object.assign({}, lineOptions);
        const entry = Object.assign({}, next[key], patch);
        for (const k of Object.keys(entry)) {
            if (entry[k] === undefined)
                delete entry[k];
        }
        if (Object.keys(entry).length > 0)
            next[key] = entry;
        else
            delete next[key];
        apply({
            "lineOptions": next
        });
    }

    function sizeText(name) {
        return {
            "s": I18n.tr("Small", "size option"),
            "m": I18n.tr("Medium", "size option"),
            "l": I18n.tr("Large", "size option")
        }[name] ?? "";
    }

    function columnSummary(key) {
        const parts = [];
        if (Layouts.isCustomLine(key))
            parts.push(lineOptions[key]?.template ?? "");
        const sort = Layouts.columnSort(options, key);
        if (key !== "duration" && sort !== key)
            parts.push(I18n.tr("Sorts by %1", "page edit mode: what a click on the column header sorts by").arg(sortText(sort)));
        parts.push(sizeText(Layouts.sizeName(Layouts.columnSize(options, key))));
        return parts.filter(p => p !== "").join(" · ");
    }

    function columnRow(key) {
        const fixed = key === "title" || key === "duration";
        return {
            "title": UiConfig.sortLabel(key, lineOptions),
            "subtitle": columnSummary(key),
            "hideable": false,
            "hasOptions": true,
            "movable": !fixed,
            "removable": !fixed
        };
    }

    // the zone editor counts the fixed Title row; lines don't
    function lineIndex(index) {
        return index - (columnsEditor ? 1 : 0);
    }

    function removeLine(index) {
        const key = options.lines[index];
        const next = Object.assign({}, lineOptions);
        delete next[key];
        apply({
            "lines": options.lines.filter((_, i) => i !== index),
            "lineOptions": next
        });
    }

    function moved(list, fromIndex, toIndex) {
        const next = list.slice();
        const moving = next.splice(fromIndex, 1);
        next.splice(toIndex, 0, moving[0]);
        return next;
    }

    // Rows and menus: the item type's zones, shared by every page of that type

    function itemZonesCopy() {
        return JSON.parse(JSON.stringify(item.zones));
    }

    function writeItem(zones) {
        const patch = {
            "zones": zones
        };
        if (item.contextSameAsMenu)
            delete zones.context;
        UiConfig.setItem(itemType, patch);
    }

    function actionRow(inst) {
        if (!UiConfig.isKnown(inst.type))
            return {
                "title": inst.type,
                "icon": "help",
                "subtitle": I18n.tr("Not available in this version", "an element saved by a newer version of the app"),
                "dimmed": true,
                "hideable": false
            };
        return {
            "title": UiConfig.actionLabel(itemType, inst.type),
            "icon": UiConfig.actionIcon(itemType, inst.type),
            "hidden": inst.hidden,
            "hideable": inst.type !== "spacer" && inst.type !== "divider"
        };
    }

    function moveAction(fromZone, fromIndex, toZone, toIndex) {
        const zones = itemZonesCopy();
        const inst = zones[fromZone]?.[fromIndex];
        if (!inst)
            return;
        if (fromZone !== toZone && !UiConfig.accepts(itemType, toZone, zones[toZone] ?? [], inst.type)) {
            itemEditor.revision++;
            Router.notify(I18n.tr("%1 can't go in %2", "toast when an element is dropped on a zone that doesn't take it; %1 is the element, %2 the zone").arg(UiConfig.actionLabel(itemType, inst.type)).arg(itemZoneMeta.find(z => z.id === toZone)?.label ?? toZone));
            return;
        }
        zones[fromZone].splice(fromIndex, 1);
        zones[toZone].splice(toIndex, 0, inst);
        writeItem(zones);
    }

    function removeAction(zone, index) {
        const before = itemZonesCopy();
        const inst = before[zone]?.[index];
        if (!inst)
            return;
        const label = UiConfig.isKnown(inst.type) ? UiConfig.actionLabel(itemType, inst.type) : inst.type;
        const type = itemType;
        Router.askRemove(I18n.tr("Remove %1?", "remove an element from a now-playing surface confirmation title, %1 is the element").arg(label), "", () => {
            const zones = root.itemZonesCopy();
            zones[zone].splice(index, 1);
            root.writeItem(zones);
            Router.offerUndo(I18n.tr("Removed %1", "toast after removing an element from a now-playing surface, %1 is the element").arg(label), () => UiConfig.setItem(type, {
                    "zones": before
                }));
        });
    }

    function hideAction(zone, index, hidden) {
        const zones = itemZonesCopy();
        if (!zones[zone]?.[index])
            return;
        zones[zone][index].hidden = hidden;
        writeItem(zones);
    }

    function pickAction(zone) {
        const present = (item.zones[zone] ?? []).map(i => i.type);
        addSheet.target = "action:" + zone;
        addSheet.title = I18n.tr("Add element", "button under a zone in a layout editor");
        addSheet.sections = [
            {
                "title": itemZoneMeta.find(z => z.id === zone)?.label ?? "",
                "items": UiConfig.typesFor(itemType, zone).map(t => ({
                            "label": t.label,
                            "icon": t.icon,
                            "value": t.type,
                            "done": t.type !== "spacer" && t.type !== "divider" && present.indexOf(t.type) >= 0
                        }))
            }
        ];
        addSheet.opened = true;
    }

    function pickLine() {
        addSheet.target = "line";
        addSheet.title = columnsEditor ? addColumnText : I18n.tr("Add line", "button under the info lines in a page's edit mode");
        addSheet.sections = [
            {
                "title": linesTitle,
                "items": lineChoices.map(l => ({
                            "label": UiConfig.lineLabel(l),
                            "value": l,
                            "icon": "add",
                            "done": l === "custom" ? !Layouts.canAddCustom(root.options.lines) : root.options.lines.indexOf(l) >= 0
                        }))
            }
        ];
        addSheet.opened = true;
    }

    function picked(value) {
        if (addSheet.target === "line") {
            if (value !== "custom") {
                apply({
                    "lines": options.lines.concat([value])
                });
                return;
            }
            const line = Layouts.newCustomLine(options.lines, options);
            apply({
                "lines": options.lines.concat([line]),
                "lineOptions": Object.assign({}, lineOptions, {
                    [line]: {
                        "template": UiConfig.defaultTemplate
                    }
                })
            });
            openColumn = line;
            return;
        }
        const zone = addSheet.target.slice(7);
        const zones = itemZonesCopy();
        zones[zone] = (zones[zone] ?? []).concat([
            {
                "type": value,
                "hidden": false,
                "placement": "auto",
                "options": {}
            }
        ]);
        writeItem(zones);
    }

    function itemNoun(type) {
        switch (type) {
        case "track":
            return I18n.tr("Track rows and menus", "page edit mode section shared by every page of the type");
        case "album":
            return I18n.tr("Album rows and menus", "page edit mode section shared by every page of the type");
        case "artist":
            return I18n.tr("Artist rows and menus", "page edit mode section shared by every page of the type");
        case "playlist":
            return I18n.tr("Playlist rows and menus", "page edit mode section shared by every page of the type");
        case "genre":
            return I18n.tr("Genre rows and menus", "page edit mode section shared by every page of the type");
        case "queue":
            return I18n.tr("Queue rows and menus", "page edit mode section shared by every page of the type");
        }
        return "";
    }

    implicitWidth: parent?.width ?? 0
    width: implicitWidth
    // the card plus the gap before the page's list
    implicitHeight: card.height + Theme.spacingL
    height: implicitHeight

    Rectangle {
        id: card
        width: parent.width
        height: Math.min(column.height + Theme.spacingL * 2, root.pageHeight * root.maxShare)
        radius: Theme.groupedListOuterRadius
        color: Theme.surfaceContainerLow
        border.width: Theme.layerOutlineWidth
        border.color: Theme.outlineMedium

        DFlickable {
            id: flick
            anchors.fill: parent
            anchors.margins: Theme.spacingL
            contentWidth: width
            contentHeight: column.height
            interactive: contentHeight > height
            clip: true

            Column {
                id: column
                width: flick.width
                spacing: Theme.spacingM

                SettingsGroup {
                    visible: root.scopeThis !== ""

                    SettingsButtonGroupRow {
                        text: I18n.tr("Editing", "page edit mode: whether this page or every page of its kind is edited")
                        model: [root.scopeThis, root.scopeAll]
                        currentIndex: root.scope
                        onSelectionChanged: (index, selected) => {
                            if (selected)
                                Router.editScope = index;
                        }
                    }
                }

                Loader {
                    width: parent.width
                    active: root.tabs.length > 0
                    visible: active
                    sourceComponent: sectionsBlock
                }

                DButtonGroup {
                    visible: root.tabs.length > 1
                    size: "small"
                    model: root.tabs.map(t => UiConfig.sectionLabel(t))
                    currentIndex: root.tabs.indexOf(root.currentTab)
                    onSelectionChanged: (index, selected) => {
                        if (selected)
                            root.tabPicked(root.tabs[index]);
                    }
                }

                SectionLabel {
                    visible: listGroup.firstRow !== null
                    text: I18n.tr("Layout and sort", "page edit mode section")
                }

                SettingsGroup {
                    id: listGroup

                    SettingsButtonGroupRow {
                        visible: root.layoutChoices.length > 1
                        text: I18n.tr("Layout", "view options section")
                        model: root.layoutChoices
                        currentIndex: root.layoutChoices.findIndex(l => l.value === root.options.layout)
                        onSelectionChanged: (index, selected) => {
                            if (selected)
                                root.apply({
                                    "layout": root.layoutChoices[index].value
                                });
                        }
                    }

                    SettingsButtonGroupRow {
                        visible: root.options.layout === "grid" && root.layouts.indexOf("grid") >= 0
                        text: I18n.tr("Tile size", "view options section")
                        model: [I18n.tr("Small", "size option"), I18n.tr("Medium", "size option"), I18n.tr("Large", "size option")]
                        currentIndex: ["s", "m", "l"].indexOf(root.options.tileSize)
                        onSelectionChanged: (index, selected) => {
                            if (selected)
                                root.apply({
                                    "tileSize": ["s", "m", "l"][index]
                                });
                        }
                    }

                    SettingsRow {
                        visible: root.sorts.length > 0
                        title: I18n.tr("Sort by", "view options section")

                        DDropdown {
                            id: sortDropdown
                            anchors.verticalCenter: parent?.verticalCenter
                            dropdownWidth: SettingsMetrics.dropdownWidth
                            options: root.sortList.map(k => root.sortText(k))
                            optionValues: root.sortList
                            onValueChanged: value => root.apply({
                                    "sort": value
                                })
                        }

                        Binding {
                            target: sortDropdown
                            property: "currentValue"
                            value: root.options.sort ?? ""
                        }

                        DButtonGroup {
                            anchors.verticalCenter: parent?.verticalCenter
                            size: "small"
                            iconOnly: true
                            model: [
                                {
                                    "text": I18n.tr("Ascending", "sort direction"),
                                    "icon": "arrow_upward"
                                },
                                {
                                    "text": I18n.tr("Descending", "sort direction"),
                                    "icon": "arrow_downward"
                                }
                            ]
                            currentIndex: root.options.descending ? 1 : 0
                            onSelectionChanged: (index, selected) => {
                                if (selected)
                                    root.apply({
                                        "descending": index === 1
                                    });
                            }
                        }
                    }
                }

                SectionLabel {
                    visible: filterGroup.firstRow !== null || ruleEditor.visible
                    text: I18n.tr("Filter", "view options section")
                }

                RuleEditor {
                    id: ruleEditor
                    visible: root.filterable && Rules.fields(root.type).length > 0
                    target: JSON.stringify({
                        "kind": root.kind,
                        "itemId": root.itemId,
                        "allScope": root.allScope,
                        "view": root.savedView?.id ?? ""
                    })
                    width: parent.width
                    type: root.type
                    rules: root.options.rules ?? ({})
                    sheetHost: root.sheetHost ?? root
                    apply: rules => root.apply({
                            "rules": rules
                        })
                }

                SettingsGroup {
                    id: filterGroup

                    SettingsRow {
                        visible: root.type === "playlists" && root.libraries.length > 1
                        title: I18n.tr("Library", "track list column")

                        DDropdown {
                            id: libraryDropdown
                            anchors.verticalCenter: parent?.verticalCenter
                            dropdownWidth: SettingsMetrics.dropdownWidth
                            // the "" value shows emptyText, not its option label
                            emptyText: I18n.tr("All libraries", "library filter chip")
                            options: [I18n.tr("All libraries", "library filter chip")].concat(root.libraries.map(l => l.name))
                            optionValues: [""].concat(root.libraries.map(l => l.id))
                            onValueChanged: value => root.applyFilters({
                                    "library": value
                                })
                        }

                        Binding {
                            target: libraryDropdown
                            property: "currentValue"
                            value: root.options.filters?.library ?? ""
                        }
                    }
                }

                Loader {
                    width: parent.width
                    active: root.tabs.length === 0 && root.sections.length > 0
                    visible: active
                    sourceComponent: sectionsBlock
                }

                DZoneEditor {
                    visible: root.linesEditable
                    flickable: flick
                    rowColor: SettingsMetrics.rowColor
                    addText: root.columnsEditor ? root.addColumnText : I18n.tr("Add line", "button under the info lines in a page's edit mode")
                    emptyText: I18n.tr("No info lines", "page edit mode: the info lines zone is empty")
                    zones: [
                        {
                            "id": "lines",
                            "label": root.linesTitle,
                            "addable": root.lineChoices.some(l => l === "custom" ? Layouts.canAddCustom(root.options.lines ?? []) : (root.options.lines ?? []).indexOf(l) < 0),
                            "rows": root.columnKeys.map(k => root.columnsEditor ? root.columnRow(k) : ({
                                        "title": UiConfig.lineLabel(k),
                                        "hideable": false
                                    }))
                        }
                    ]
                    onMoved: (fromZone, fromIndex, toZone, toIndex) => {
                        const from = root.lineIndex(fromIndex);
                        const to = Math.max(0, Math.min(root.options.lines.length - 1, root.lineIndex(toIndex)));
                        if (from >= 0 && from < root.options.lines.length)
                            root.apply({
                                "lines": root.moved(root.options.lines, from, to)
                            });
                    }
                    onRemoved: (zone, index) => {
                        if (root.options.lines[root.lineIndex(index)] === root.openColumn)
                            root.openColumn = "";
                        root.removeLine(root.lineIndex(index));
                    }
                    onOptionsRequested: (zone, index) => {
                        const key = root.columnKeys[index];
                        root.openColumn = root.openColumn === key ? "" : key;
                    }
                    onAddRequested: root.pickLine()
                }

                SectionLabel {
                    visible: root.linesEditable && root.columnOpen
                    text: UiConfig.sortLabel(root.openColumn, root.lineOptions)
                }

                SettingsGroup {
                    visible: root.linesEditable && root.columnOpen

                    SettingsFieldRow {
                        id: lineTitleRow
                        visible: root.openIsCustom
                        title: I18n.tr("Column title", "page edit mode: the header of the custom text column")
                        field.placeholderText: UiConfig.lineLabel("custom")
                        field.maximumLength: 64
                        field.onTextEdited: {
                            if (field.text !== (root.lineOptions[root.openColumn]?.title ?? ""))
                                root.applyColumn(root.openColumn, {
                                    "title": field.text === "" ? undefined : field.text
                                });
                        }

                        Binding {
                            target: lineTitleRow.field
                            property: "text"
                            value: root.lineOptions[root.openColumn]?.title ?? ""
                            when: !lineTitleRow.field.inputFocused
                            restoreMode: Binding.RestoreNone
                        }
                    }

                    TemplateRow {
                        visible: root.openIsCustom
                        template: root.lineOptions[root.openColumn]?.template ?? ""
                        onEdited: value => root.applyColumn(root.openColumn, {
                                "template": value
                            })
                    }

                    SettingsRow {
                        visible: root.openColumn !== "duration"
                        title: I18n.tr("Sort by", "view options section")

                        DDropdown {
                            id: columnSortDropdown
                            anchors.verticalCenter: parent?.verticalCenter
                            dropdownWidth: SettingsMetrics.dropdownWidth
                            options: root.columnSortChoices.map(k => root.openIsCustom && k === root.openColumn ? I18n.tr("Its text", "page edit mode: a custom column sorts by what it shows") : root.sortText(k))
                            optionValues: root.columnSortChoices
                            onValueChanged: value => root.applyColumn(root.openColumn, {
                                    "sort": value === root.openColumn ? undefined : value
                                })
                        }

                        Binding {
                            target: columnSortDropdown
                            property: "currentValue"
                            value: Layouts.columnSort(root.options, root.openColumn)
                        }
                    }

                    SettingsButtonGroupRow {
                        text: I18n.tr("Width", "element option: how wide the track info is")
                        model: [root.sizeText("s"), root.sizeText("m"), root.sizeText("l")]
                        currentIndex: ["s", "m", "l"].indexOf(Layouts.sizeName(Layouts.columnSize(root.options, root.openColumn)))
                        onSelectionChanged: (index, selected) => {
                            if (!selected)
                                return;
                            const size = Layouts.sizeValue(["s", "m", "l"][index]);
                            root.applyColumn(root.openColumn, {
                                "size": size === 1 ? undefined : size
                            });
                        }
                    }
                }

                Row {
                    visible: root.savable || root.savedView !== null
                    spacing: Theme.spacingS

                    DButton {
                        visible: root.savedView === null
                        text: I18n.tr("Save as view…", "view options action")
                        iconName: "bookmark_add"
                        backgroundColor: Theme.secondaryContainer
                        textColor: Theme.onSecondaryContainer
                        onClicked: root.saveAsRequested()
                    }

                    DButton {
                        visible: root.savedView !== null
                        text: I18n.tr("Rename view", "saved view action")
                        iconName: "edit"
                        backgroundColor: Theme.secondaryContainer
                        textColor: Theme.onSecondaryContainer
                        onClicked: root.renameRequested()
                    }

                    DButton {
                        visible: root.savedView !== null
                        text: I18n.tr("Delete view", "saved view action")
                        iconName: "delete"
                        backgroundColor: "transparent"
                        textColor: Theme.error
                        onClicked: deleteDialog.ask()
                    }
                }

                DCollapsibleSection {
                    id: itemSection
                    width: parent.width
                    visible: root.item !== null
                    title: root.itemNoun(root.itemType)
                    description: I18n.tr("Shared by all pages.", "page edit mode: rows and menus are the same on every page of the type")

                    Column {
                        Layout.fillWidth: true
                        spacing: Theme.spacingM

                        Repeater {
                            model: root.itemType !== "" ? UiConfig.problems(root.itemType) : []

                            StyledText {
                                required property string modelData
                                width: parent.width
                                text: "⚠ " + modelData
                                font.pixelSize: Theme.fontSizeSmall
                                color: Theme.error
                                wrapMode: Text.WordWrap
                                horizontalAlignment: Text.AlignLeft
                            }
                        }

                        SettingsGroup {
                            SettingsToggleRow {
                                text: I18n.tr("Right-click shows the ⋮ menu", "page edit mode: the right-click menu follows the ⋮ menu")
                                checked: root.item?.contextSameAsMenu ?? true
                                onToggled: value => UiConfig.setItem(root.itemType, {
                                        "contextSameAsMenu": value
                                    })
                            }
                        }

                        DZoneEditor {
                            id: itemEditor
                            property int revision: 0
                            flickable: flick
                            rowColor: SettingsMetrics.rowColor
                            zones: {
                                // a refused move rebuilds the rows it left
                                const it = revision >= 0 ? root.item : null;
                                if (!it)
                                    return [];
                                return root.itemZoneMeta.map(z => {
                                    const following = z.id === "context" && it.contextSameAsMenu;
                                    return {
                                        "id": z.id,
                                        "label": z.label,
                                        "note": following ? I18n.tr("Same as the ⋮ menu", "page edit mode: the right-click menu zone while it follows") : "",
                                        "disabled": following,
                                        "addable": !following,
                                        "rows": (it.zones[z.id] ?? []).map(inst => root.actionRow(inst))
                                    };
                                });
                            }
                            onMoved: (fromZone, fromIndex, toZone, toIndex) => root.moveAction(fromZone, fromIndex, toZone, toIndex)
                            onRemoved: (zone, index) => root.removeAction(zone, index)
                            onHiddenToggled: (zone, index, hidden) => root.hideAction(zone, index, hidden)
                            onAddRequested: zone => root.pickAction(zone)
                        }

                        DButton {
                            text: I18n.tr("Reset rows and menus", "page edit mode: restores the shared rows and menus of a type")
                            backgroundColor: "transparent"
                            textColor: Theme.primary
                            onClicked: UiConfig.resetItem(root.itemType)
                        }
                    }
                }
            }
        }
    }

    Component {
        id: sectionsBlock

        DZoneEditor {
            readonly property int visibleCount: root.sections.filter(s => !s.hidden).length
            flickable: flick
            rowColor: SettingsMetrics.rowColor
            zones: [
                {
                    "id": "sections",
                    "label": I18n.tr("Sections", "page edit mode section: the page's own parts, ordered and hidden"),
                    "note": root.sectionsKind === "artist" ? I18n.tr("The page opens on the first one shown.", "page edit mode: the artist page's sections") : "",
                    "addable": false,
                    "rows": root.sections.map(s => {
                        const last = !s.hidden && visibleCount === 1;
                        return {
                            "title": UiConfig.sectionLabel(s.type),
                            "icon": UiConfig.sectionIcon(s.type),
                            "hidden": s.hidden,
                            "removable": false,
                            "hideEnabled": !last,
                            "subtitle": last ? I18n.tr("One section stays shown", "page edit mode: why the last shown section can't be hidden") : ""
                        };
                    })
                }
            ]
            onMoved: (fromZone, fromIndex, toZone, toIndex) => root.applySections(root.moved(root.sections, fromIndex, toIndex))
            onHiddenToggled: (zone, index, hidden) => root.applySections(root.sections.map((s, i) => i === index ? Object.assign({}, s, {
                        "hidden": hidden
                    }) : s))
        }
    }

    PickerSheet {
        id: addSheet
        property string target: ""
        parent: root.sheetHost ?? root
        doneFormat: I18n.tr("%1 (added)", "picker entry already in place, %1 is its name")
        onPicked: value => root.picked(value)
    }

    ConfirmDialog {
        id: deleteDialog
        host: root.sheetHost ?? root
        title: I18n.tr("Delete this view?", "delete saved view confirmation title")
        supportingText: I18n.tr("Also removes its navigation entry and home shelf. No music is deleted.", "delete saved view confirmation")
        confirmText: I18n.tr("Delete", "confirm button that deletes an item")
        onConfirmed: root.deleteRequested()
    }
}
