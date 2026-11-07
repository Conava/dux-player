pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets
import qs.Widgets

// Every page in one list: the navigation entries in their order (shown or hidden, renamed, pinned items), then
// the pages that have no entry. Edit opens a page in its edit mode in the main window; reset clears its changes.
Item {
    id: root

    readonly property var entries: UiConfig.navigation
    readonly property int shownCount: UiConfig.shownNavigation.length
    readonly property var lists: UiConfig.effective.lists
    readonly property var itemTypes: ["track", "album", "artist", "playlist", "genre", "queue"]
    // the page kinds a navigation entry can't reach: one page per item, search results, the queue
    readonly property var otherKinds: UiConfig.pageKinds.filter(p => p.multi || p.route === "search" || p.kind === "queue")
    // the reset the confirm dialog runs
    property var pendingReset: null

    signal closeRequested

    function hasKeys(o) {
        return Object.keys(o ?? {}).length > 0;
    }

    // the Favorites page keeps its tabs' options under kinds of their own
    function kindsOf(kind) {
        return kind === "favorites" ? ["favorites", "favorites-tracks", "favorites-albums", "favorites-artists"] : [kind];
    }

    function kindChanged(kind) {
        return kindsOf(kind).some(k => hasKeys(lists.defaults[k]) || (UiConfig.pageKinds.find(p => p.kind === k)?.multi ? UiConfig.changedPages(k) > 0 : hasKeys(lists.overrides[k])));
    }

    function summary(entry) {
        const o = entry.multi ? UiConfig.kindDefault(entry.kind) : UiConfig.pageOptions(entry.kind, "");
        const parts = [];
        if (o.sections)
            parts.push(o.sections.filter(s => !s.hidden).map(s => UiConfig.sectionLabel(s.type)).join(", "));
        else
            parts.push(UiConfig.layoutChoices().find(l => l.value === o.layout)?.text ?? "");
        if (["history", "album", "search", "queue", "favorites"].indexOf(entry.kind) < 0)
            parts.push(UiConfig.sortLabel(o.sort, o.lineOptions) + (o.descending ? " ↓" : " ↑"));
        const pages = entry.multi ? UiConfig.changedPages(entry.kind) : 0;
        if (pages > 0)
            parts.push(I18n.tr("%1 changed", "settings pages: how many pages of a kind have their own options, %1 is the count").arg(pages));
        return parts.filter(p => p !== "").join(" · ");
    }

    // what Edit and reset act on for a navigation entry: {route, itemId, changed, reset}; null without a page editor
    function pageOf(entry) {
        if (entry.kind === "page") {
            if (entry.target === "home")
                return {
                    "route": "home",
                    "itemId": "",
                    "changed": SettingsData.layout?.surfaces?.home !== undefined,
                    "summary": I18n.tr("%1 shelves", "settings pages: the home page's shelf count").arg((UiConfig.surface("home")?.zones.shelves ?? []).length),
                    "reset": () => UiConfig.resetSurface("home")
                };
            const kind = UiConfig.pageKinds.find(p => p.kind === entry.target && !p.multi);
            if (!kind)
                return null;
            return {
                "route": kind.route,
                "itemId": "",
                "changed": kindChanged(kind.kind),
                "summary": summary(kind),
                "reset": () => kindsOf(kind.kind).forEach(k => UiConfig.resetKind(k))
            };
        }
        if (entry.kind === "view")
            return {
                "route": "view",
                "itemId": entry.target,
                "changed": false,
                "summary": "",
                "reset": null
            };
        return {
            "route": entry.kind,
            "itemId": entry.target,
            "changed": hasKeys(lists.overrides[entry.kind + ":" + entry.target]),
            "summary": I18n.tr("Changed", "settings pages: a page or type with changes"),
            "reset": () => UiConfig.resetPageOptions(entry.kind, entry.target)
        };
    }

    function edit(route, itemId) {
        Router.editPage(route, itemId);
        root.closeRequested();
    }

    function ask(title, run) {
        resetDialog.title = title;
        pendingReset = run;
        resetDialog.ask();
    }

    function setShown(entry, on) {
        UiConfig.setNavigation(entries.map(e => e.key === entry.key ? Object.assign({}, e, {
                "hidden": !on
            }) : e));
    }

    function remove(entry) {
        const before = root.entries;
        const run = () => {
            UiConfig.removeEntry(entry.kind, entry.target);
            Router.offerUndo(I18n.tr("Removed \"%1\" from the navigation", "toast after removing a navigation entry").arg(UiConfig.entryLabel(entry)), () => UiConfig.setNavigation(before));
        };
        // Router.askRemove would ask in the main window
        if (UiConfig.removeMode !== "confirm") {
            run();
            return;
        }
        removeDialog.run = run;
        removeDialog.title = I18n.tr("Remove \"%1\" from the navigation?", "remove navigation entry confirmation title").arg(UiConfig.entryLabel(entry));
        removeDialog.ask();
    }

    component EditButton: DButton {
        anchors.verticalCenter: parent?.verticalCenter
        text: I18n.tr("Edit", "settings pages: opens the page in edit mode in the main window")
        backgroundColor: Theme.secondaryContainer
        textColor: Theme.onSecondaryContainer
    }

    // keeps its width while hidden, so the Edit buttons of all rows line up
    component EditSlot: Item {
        id: editSlot
        property alias shown: editButton.visible
        signal clicked
        anchors.verticalCenter: parent?.verticalCenter
        width: editButton.width
        height: editButton.height

        EditButton {
            id: editButton
            onClicked: editSlot.clicked()
        }
    }

    // Edit and reset of a plain settings row, in the columns of the navigation rows above (DInstanceRow: actions,
    // eye and remove, spacingXS apart, spacingS from the edge; the rename pencil is the last action)
    component TrailingColumns: Row {
        readonly property real endGap: Theme.buttonHeightXS * 3 + Theme.spacingXS * 3 + Theme.spacingS - SettingsMetrics.rowPaddingH

        anchors.verticalCenter: parent?.verticalCenter
        spacing: Theme.spacingXS
        rightPadding: I18n.isRtl ? 0 : endGap
        leftPadding: I18n.isRtl ? endGap : 0
    }

    component ResetButton: DActionButton {
        property string label: ""
        signal reset
        anchors.verticalCenter: parent?.verticalCenter
        iconName: "restart_alt"
        tooltipText: I18n.tr("Reset %1", "settings pages: resets one page or type, %1 is its name").arg(label)
        Accessible.name: tooltipText
        onClicked: reset()
    }

    SettingsPage {
        id: page

        SettingsCard {
            width: parent.width
            title: I18n.tr("Style", "navigation settings section")

            SettingsButtonGroupRow {
                text: I18n.tr("Navigation", "settings sidebar tab label")
                keywords: "rail bar sidebar"
                model: UiConfig.navStyles.map(style => UiConfig.navStyleLabel(style))
                currentIndex: UiConfig.navStyles.indexOf(SettingsData.navStyle)
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        SettingsData.navStyle = UiConfig.navStyles[index];
                }
            }

            SettingsToggleRow {
                text: I18n.tr("Open album on track tile click", "pages setting")
                keywords: "click grid home shelf tiles"
                checked: SettingsData.trackTileOpensAlbum
                onToggled: value => SettingsData.trackTileOpensAlbum = value
            }
        }

        SettingsPageIntro {
            text: I18n.tr("Drag to reorder. Use the pencil to rename an entry.", "settings pages intro")
        }

        DReorderList {
            id: list
            width: parent.width
            flickable: page
            model: root.entries
            onReordered: indices => UiConfig.setNavigation(indices.map(i => root.entries[i]))

            delegate: DInstanceRow {
                id: entryRow
                required property var modelData
                readonly property bool valid: UiConfig.entryValid(modelData)
                readonly property bool lastShown: !modelData.hidden && root.shownCount <= 1
                readonly property var target: valid ? root.pageOf(modelData) : null
                view: list
                surfaceColor: SettingsMetrics.rowColor
                iconName: UiConfig.entryIcon(modelData)
                title: valid ? UiConfig.entryLabel(modelData) : I18n.tr("Removed item", "navigation entry whose target was deleted")
                subtitle: {
                    if (lastShown)
                        return I18n.tr("At least one entry must stay visible", "why the last shown navigation entry can't be turned off");
                    const kind = modelData.kind === "page" ? I18n.tr("Page", "navigation entry kind") : UiConfig.shelfSourceLabel(modelData.kind);
                    const name = modelData.label ? I18n.tr("%1, renamed from %2", "navigation entry subtitle: kind, original name").arg(kind).arg(UiConfig.defaultEntryLabel(modelData)) : kind;
                    return target?.changed ? Phrases.join(name, target.summary) : name;
                }
                hidden: modelData.hidden === true
                dimmed: hidden || !valid
                hideEnabled: !lastShown && valid
                // pages always have a place; pinned items can go
                removable: modelData.kind !== "page"
                onHiddenToggled: hidden => root.setShown(modelData, !hidden)
                onRemoveRequested: root.remove(modelData)
                actions: [
                    EditSlot {
                        shown: entryRow.target !== null
                        onClicked: root.edit(entryRow.target.route, entryRow.target.itemId)
                    },
                    // kept when there is nothing to reset, so the buttons line up
                    Item {
                        anchors.verticalCenter: parent?.verticalCenter
                        width: entryReset.width
                        height: entryReset.height

                        ResetButton {
                            id: entryReset
                            visible: entryRow.target?.reset ? true : false
                            enabled: entryRow.target?.changed ?? false
                            label: entryRow.title
                            onReset: root.ask(I18n.tr("Reset %1?", "settings pages: reset confirmation title, %1 is the page").arg(label), entryRow.target.reset)
                        }
                    },
                    Item {
                        anchors.verticalCenter: parent?.verticalCenter
                        width: rename.width
                        height: rename.height

                        DActionButton {
                            id: rename
                            visible: entryRow.valid
                            iconName: "edit"
                            tooltipText: I18n.tr("Rename", "settings pages: renames a navigation entry")
                            Accessible.name: I18n.tr("Rename %1", "settings pages: renames a navigation entry, %1 is its name").arg(entryRow.title)
                            onClicked: prompt.ask(I18n.tr("Rename entry", "navigation rename dialog title"), UiConfig.entryLabel(entryRow.modelData), I18n.tr("Save", "dialog confirm button"), name => UiConfig.renameEntry(entryRow.modelData.key, name === UiConfig.defaultEntryLabel(entryRow.modelData) ? "" : name))
                        }
                    }
                ]
            }
        }

        DButton {
            text: I18n.tr("Add entry", "navigation settings action")
            iconName: "add"
            backgroundColor: Theme.secondaryContainer
            textColor: Theme.onSecondaryContainer
            onClicked: addSheet.opened = true
        }

        SettingsCard {
            width: parent.width
            title: I18n.tr("Other pages", "settings pages section: pages without a navigation entry")

            Repeater {
                model: root.otherKinds

                SettingsRow {
                    id: pageRow
                    required property var modelData
                    readonly property bool edited: root.kindChanged(modelData.kind)
                    width: parent?.width ?? 0
                    iconName: modelData.icon
                    title: modelData.label
                    subtitle: edited ? root.summary(modelData) : I18n.tr("Default", "settings pages: a page nobody has changed")

                    TrailingColumns {
                        EditSlot {
                            shown: pageRow.modelData.route !== ""
                            onClicked: root.edit(pageRow.modelData.route, "")
                        }

                        ResetButton {
                            enabled: pageRow.edited
                            label: pageRow.modelData.label
                            onReset: root.ask(I18n.tr("Reset %1?", "settings pages: reset confirmation title, %1 is the page").arg(label), () => UiConfig.resetKind(pageRow.modelData.kind))
                        }
                    }
                }
            }
        }

        SettingsCard {
            width: parent.width
            title: I18n.tr("Rows and menus", "settings pages section: the shared row and menu actions per type")
            keywords: "quick actions context menu"

            Repeater {
                model: root.itemTypes

                SettingsRow {
                    id: itemRow
                    required property string modelData
                    readonly property bool edited: SettingsData.layout?.items?.[modelData] !== undefined
                    width: parent?.width ?? 0
                    iconName: UiConfig.pageKindIcon(modelData === "queue" ? "queue" : modelData + "s")
                    title: modelData === "queue" ? UiConfig.pageKindLabel("queue") : UiConfig.typeLabel(modelData + "s")
                    subtitle: UiConfig.problems(modelData)[0] ?? (edited ? I18n.tr("Changed", "settings pages: a page or type with changes") : I18n.tr("Default", "settings pages: a page nobody has changed"))
                    subtitleColor: UiConfig.problems(modelData).length > 0 ? Theme.error : Theme.surfaceVariantText

                    TrailingColumns {
                        EditSlot {
                            shown: false
                        }

                        ResetButton {
                            enabled: itemRow.edited
                            label: itemRow.title
                            onReset: root.ask(I18n.tr("Reset %1?", "settings pages: reset confirmation title, %1 is the page").arg(label), () => UiConfig.resetItem(itemRow.modelData))
                        }
                    }
                }
            }
        }

        SettingsCard {
            width: parent.width

            SettingsRow {
                title: I18n.tr("Reset navigation", "navigation settings action")
                keywords: "defaults"
                subtitle: I18n.tr("Restores the default entries, names and order.", "navigation reset description")
                iconName: "restart_alt"
                clickable: true
                onClicked: navResetDialog.ask()
            }
        }
    }

    TextPromptDialog {
        id: prompt
        host: root
    }

    ConfirmDialog {
        id: removeDialog
        property var run: null
        host: root
        supportingText: I18n.tr("It can be added again with Add entry.", "remove navigation entry confirmation")
        confirmText: I18n.tr("Remove", "button that removes an item")
        onConfirmed: run?.()
    }

    ConfirmDialog {
        id: resetDialog
        host: root
        supportingText: I18n.tr("Its options and page changes are reset.", "settings pages: reset confirmation")
        confirmText: I18n.tr("Reset", "button that restores the defaults")
        onConfirmed: {
            root.pendingReset?.();
            root.pendingReset = null;
        }
        onRejected: root.pendingReset = null
    }

    ConfirmDialog {
        id: navResetDialog
        host: root
        title: I18n.tr("Reset navigation?", "reset navigation confirmation title")
        supportingText: I18n.tr("Pinned entries and custom names are removed.", "reset navigation confirmation")
        confirmText: I18n.tr("Reset", "button that restores the defaults")
        onConfirmed: UiConfig.setNavigation([])
    }

    PinTargetSheet {
        id: addSheet
        title: I18n.tr("Add entry", "navigation settings action")
        // closing, the sheet hands focus back; with the dialog open that would take it from the name field
        returnFocusItem: newPageDialog.opened ? newPageDialog.field : null
        doneFormat: I18n.tr("%1 (already pinned)", "picker item that is already in the navigation")
        isDone: (kind, id) => UiConfig.isPinned(kind, id)
        leading: [
            {
                "title": I18n.tr("Pages", "add entry section"),
                "items": [
                    {
                        "label": I18n.tr("New custom page", "action and dialog that create a custom page"),
                        "icon": "add",
                        "value": ["new", ""],
                        "done": false
                    }
                ].concat(UiConfig.pageTargets.map(p => addSheet.item("page", p, UiConfig.pageLabel(p))))
            }
        ]
        onPicked: value => {
            if (value[0] === "new")
                newPageDialog.show();
            else
                UiConfig.pin(value[0], value[1]);
        }
    }

    NewPageDialog {
        id: newPageDialog
        host: root
        onCreated: (name, type) => {
            const id = UiConfig.saveView(name, type, {});
            UiConfig.pin("view", id);
            root.edit("view", id);
        }
    }
}
