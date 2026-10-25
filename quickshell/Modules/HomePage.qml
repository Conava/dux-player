pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// Home: the user's shelves, the large search field among them. Edit page turns them into a list to reorder,
// hide, remove and set each shelf's layout and size.
Item {
    id: root

    readonly property bool editing: Router.editing
    readonly property var shelves: UiConfig.surface("home").zones.shelves
    readonly property bool hasLibraries: CatalogService.libraries.length > 0
    readonly property var shown: shelves.filter(s => !s.hidden && UiConfig.isKnown(s.type) && (s.type === "search" || hasLibraries))
    readonly property var layouts: ["carousel", "grid", "list"]
    readonly property var sizes: ["s", "m", "l"]

    function hasOptions(shelf) {
        return UiConfig.isKnown(shelf.type) && shelf.type !== "search" && shelf.type !== "continue";
    }

    function setShelves(list) {
        UiConfig.setSurface("home", {
            "zones": {
                "shelves": list
            }
        });
    }

    function patchShelf(index, patch) {
        setShelves(shelves.map((s, i) => i === index ? Object.assign({}, s, patch) : s));
    }

    function setOption(index, key, value) {
        const options = Object.assign({}, shelves[index].options);
        options[key] = value;
        patchShelf(index, {
            "options": options
        });
    }

    function removeShelf(index) {
        const before = UiConfig.surface("home");
        const name = UiConfig.shelfTitle(shelves[index]);
        const run = () => {
            setShelves(shelves.filter((s, i) => i !== index));
            Router.offerUndo(I18n.tr("Removed \"%1\" from home", "toast after removing a home shelf").arg(name), () => UiConfig.setSurface("home", before));
        };
        Router.askRemove(I18n.tr("Remove \"%1\" from home?", "remove home shelf confirmation title").arg(name), I18n.tr("You can add it again with Add shelf.", "remove home shelf confirmation"), run);
    }

    DFlickable {
        id: flick
        anchors.fill: parent
        contentHeight: column.height + PlayerMetrics.pagePadding
        contentWidth: width
        clip: true
        // a square edge fade would paint over the pane's rounded corners
        fadeSideInset: PlayerMetrics.paneRadius

        Column {
            id: column
            x: PlayerMetrics.pagePadding
            width: flick.width - PlayerMetrics.pagePadding * 2
            spacing: Theme.spacingXL

            PageHeader {
                title: I18n.tr("Home", "navigation destination")
                subtitle: root.editing ? I18n.tr("Drag to reorder. Each shelf has its own layout and size.", "home edit mode hint") : ""
                editable: true
                editing: root.editing
                onEditRequested: Router.editing = true
                onResetRequested: resetDialog.ask()
                onDone: Router.editing = false

                DIconButton {
                    anchors.verticalCenter: parent.verticalCenter
                    iconName: "history"
                    tooltipText: I18n.tr("History", "navigation destination")
                    Accessible.name: tooltipText
                    onClicked: Router.go("history")
                }
            }

            Repeater {
                model: root.editing ? [] : root.shown

                Loader {
                    id: shelfLoader
                    required property var modelData
                    width: parent?.width ?? 0
                    // an empty shelf takes no spacing
                    visible: modelData.type === "search" || ((item as Shelf)?.hasContent ?? false)
                    sourceComponent: modelData.type === "search" ? searchBox : shelf

                    Component {
                        id: searchBox
                        SearchBox {}
                    }

                    Component {
                        id: shelf
                        Shelf {
                            shelf: shelfLoader.modelData
                            title: UiConfig.shelfTitle(shelfLoader.modelData)
                        }
                    }
                }
            }

            // nothing to show until a library exists
            Column {
                width: parent.width
                visible: !root.editing && CatalogService.librariesLoaded && !root.hasLibraries
                spacing: Theme.spacingM
                topPadding: Theme.spacingXL

                DIcon {
                    name: "library_music"
                    size: Theme.iconSizeLarge
                    color: Theme.surfaceVariantText
                }

                StyledText {
                    width: parent.width
                    text: I18n.tr("No music", "home empty state title when no library is set up")
                    font.pixelSize: Theme.fontSizeLarge
                    color: Theme.surfaceText
                }

                StyledText {
                    width: parent.width
                    text: I18n.tr("Add a music folder or server.", "home empty state when no library is set up")
                    font.pixelSize: Theme.fontSizeMedium
                    color: Theme.surfaceVariantText
                    wrapMode: Text.WordWrap
                }

                DButton {
                    text: I18n.tr("Add library", "libraries settings primary action")
                    iconName: "add"
                    onClicked: Router.addLibrary()
                }
            }

            StyledText {
                width: parent.width
                visible: !root.editing && root.hasLibraries && !root.shown.some(s => s.type !== "search")
                text: I18n.tr("Home is empty. Use Edit page to add shelves.", "home empty state")
                font.pixelSize: Theme.fontSizeMedium
                color: Theme.surfaceVariantText
            }

            DReorderList {
                id: reorder
                width: parent.width
                visible: root.editing
                flickable: flick
                model: root.editing ? root.shelves : []
                onReordered: indices => root.setShelves(indices.map(i => root.shelves[i]))

                delegate: DInstanceRow {
                    required property var modelData
                    readonly property bool known: UiConfig.isKnown(modelData.type)
                    readonly property string name: UiConfig.shelfTitle(modelData)
                    readonly property string sourceLabel: UiConfig.shelfSourceLabel(modelData.type)

                    view: reorder
                    iconName: UiConfig.shelfIcon(modelData)
                    surfaceColor: Theme.chipSurface
                    title: known ? name : modelData.type
                    subtitle: {
                        if (!known)
                            return I18n.tr("Not available in this version", "editor row of an element saved by a newer version");
                        const source = sourceLabel !== name ? sourceLabel : "";
                        if (!root.hasOptions(modelData))
                            return source;
                        return Phrases.joinAll([source, UiConfig.optionLabel(modelData.options.layout), UiConfig.optionLabel(modelData.options.size)]);
                    }
                    hidden: modelData.hidden
                    dimmed: hidden || !known
                    hideable: known
                    hasOptions: root.hasOptions(modelData)
                    onOptionsRequested: optionsDialog.show(index)
                    onHiddenToggled: hidden => root.patchShelf(index, {
                            "hidden": hidden
                        })
                    onRemoveRequested: root.removeShelf(index)
                }
            }

            DButton {
                visible: root.editing
                text: I18n.tr("Add shelf", "home edit action")
                iconName: "add"
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                onClicked: addSheet.opened = true
            }
        }
    }

    AddShelfSheet {
        id: addSheet
        shelves: root.shelves
        onAdd: shelf => root.setShelves(root.shelves.concat([shelf]))
    }

    WindowDialog {
        id: optionsDialog

        property int index: -1
        readonly property var shelf: root.shelves[index] ?? null

        function show(i) {
            index = i;
            opened = true;
            forceActiveFocus();
        }

        host: root
        title: shelf ? UiConfig.shelfTitle(shelf) : ""
        acceptEnabled: false
        onRejected: opened = false

        Column {
            width: parent?.width ?? 0
            spacing: Theme.spacingM

            StyledText {
                text: I18n.tr("Layout", "view options section")
                font.pixelSize: Theme.fontSizeMedium
                color: Theme.surfaceVariantText
            }

            DButtonGroup {
                model: root.layouts.map(l => UiConfig.optionLabel(l))
                currentIndex: root.layouts.indexOf(optionsDialog.shelf?.options.layout ?? "")
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        root.setOption(optionsDialog.index, "layout", root.layouts[index]);
                }
            }

            StyledText {
                text: I18n.tr("Tile size", "view options section")
                font.pixelSize: Theme.fontSizeMedium
                color: Theme.surfaceVariantText
            }

            DButtonGroup {
                model: root.sizes.map(s => UiConfig.optionLabel(s))
                currentIndex: root.sizes.indexOf(optionsDialog.shelf?.options.size ?? "")
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        root.setOption(optionsDialog.index, "size", root.sizes[index]);
                }
            }
        }

        actions: [
            DButton {
                text: I18n.tr("Close", "dialog dismiss button")
                backgroundColor: "transparent"
                textColor: Theme.primary
                onClicked: optionsDialog.opened = false
            }
        ]
    }

    ConfirmDialog {
        id: resetDialog
        host: root
        title: I18n.tr("Reset home?", "reset home confirmation title")
        supportingText: I18n.tr("Home shelves return to the defaults.", "reset home confirmation")
        confirmText: I18n.tr("Reset", "button that restores the defaults")
        onConfirmed: UiConfig.resetSurface("home")
    }
}
