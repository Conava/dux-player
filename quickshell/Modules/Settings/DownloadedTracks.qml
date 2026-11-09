pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets

// One server library's downloaded tracks, or only its excluded ones, with why each is kept. Shown inside the
// Downloads page, which scrolls it.
Column {
    id: root

    property string libraryId: ""
    property bool excludedOnly: false
    // the page that scrolls this list; the next page of tracks loads near its end
    property Flickable flickable: null
    // the settings search reading the page: fetch nothing
    property bool quiet: false

    readonly property int pageSize: 100
    // the daemon's cap on one page
    readonly property int fetchMax: 500
    // downloads change the status often; the list reloads at most this often
    readonly property int reloadDelay: 1000
    readonly property var library: SettingsService.cacheLibrary(libraryId)
    readonly property bool active: visible && !quiet && libraryId !== ""
    // Changes when a copy arrives, goes, is kept or excluded, or a rule comes, goes or covers other tracks. The
    // excluded list only follows exclusions.
    readonly property string cacheStamp: excludedOnly ? String(library.excluded ?? 0) : JSON.stringify([library, SettingsService.cache.pinned ?? 0, SettingsService.cacheRules().map(r => [r.library ?? "", r.key, r.cached, r.tracks])])
    readonly property bool nearEnd: flickable !== null && flickable.contentY + 2 * flickable.height >= flickable.contentHeight
    // the rows on screen: without those an Undo toast holds back
    readonly property var shown: items.filter(i => !hidden[i.track.id])
    readonly property var blank: ({
            "track": {
                "id": "",
                "title": "",
                "artist": ""
            },
            "rules": [],
            "excluded": false,
            "size": 0
        })

    property var items: []
    property int total: 0
    property bool loaded: false
    property bool loading: false
    // replies to an older request are dropped
    property int _request: 0
    property var pendingDelete: null
    // track id: true while its deletion waits for the Undo toast
    property var hidden: ({})

    // a rule reference {kind, id, local, global} that holds a track of this library
    function ruleLabel(ref) {
        const label = Phrases.offlineRule(ref.kind, (ref.local ? "" : libraryId) + ":" + ref.id, ref.id);
        return ref.global ? I18n.tr("%1 (all servers)", "an offline rule that covers every server library, %1 is the rule").arg(label) : label;
    }

    // "track" is a track kept from its menu, not a rule
    function reason(item) {
        if (item.excluded)
            return I18n.tr("Excluded", "a downloaded track no offline rule may keep");
        const named = item.rules.filter(r => r.kind !== "track");
        if (named.length > 0)
            return I18n.tr("Kept by %1", "offline song row, %1 lists the rules holding it").arg(named.map(r => ruleLabel(r)).join(", "));
        return item.rules.length > 0 ? I18n.tr("Kept on this device", "offline song row for a song kept on its own") : I18n.tr("Not kept by a rule", "offline song row for a song no rule holds");
    }

    // the loaded rows again, read in pages the daemon accepts
    function reload() {
        _fetch(false);
    }

    function _more() {
        if (active && loaded && !loading && nearEnd && items.length < total)
            _fetch(true);
    }

    // Appending reads the next page; reloading reads as many rows as are loaded. Rows move while the list is read
    // (it is ordered by last use), so a track already read is skipped.
    function _fetch(append) {
        if (!active)
            return;
        const request = ++_request;
        const got = append ? items.slice() : [];
        const seen = new Set(got.map(i => i.track.id));
        const want = append ? items.length + pageSize : Math.max(pageSize, items.length);
        let offset = got.length;
        loading = true;
        const step = () => SettingsService.cacheTracks(libraryId, offset, Math.min(fetchMax, want - offset), excludedOnly, page => {
                if (request !== root._request)
                    return;
                offset += page.items.length;
                for (const item of page.items) {
                    if (!seen.has(item.track.id)) {
                        seen.add(item.track.id);
                        got.push(item);
                    }
                }
                if (offset < want && offset < page.total && page.items.length > 0) {
                    step();
                    return;
                }
                const grew = !append || got.length > root.items.length;
                root.items = got;
                root.total = page.total;
                root.loaded = true;
                root.loading = false;
                if (grew)
                    Qt.callLater(root._more);
            }, () => {
                if (request !== root._request)
                    return;
                root.loading = false;
                root.loaded = true;
            });
        step();
    }

    function _reset() {
        _request++;
        items = [];
        total = 0;
        loaded = false;
        loading = false;
        reload();
    }

    function _hide(id, on) {
        const next = Object.assign({}, hidden);
        if (on)
            next[id] = true;
        else
            delete next[id];
        hidden = next;
    }

    // A copy no rule or keep holds goes at once. A hand-kept track isn't fetched again, so it follows the removing
    // setting; only a rule makes deleting a question of excluding.
    function deleteTrack(item) {
        if (item.rules.some(r => r.kind !== "track")) {
            pendingDelete = item;
            root.deleteDialog.opened = true;
            return;
        }
        if (item.rules.length === 0) {
            SettingsService.deleteDownload(item.track, false, () => root.reload());
            return;
        }
        const id = item.track.id;
        const run = () => {
            root._hide(id, true);
            Router.offerUndo(I18n.tr("Deleted \"%1\"", "toast after deleting a playlist").arg(item.track.title), () => root._hide(id, false), done => SettingsService.deleteDownload(item.track, false, () => {
                    root.items = root.items.filter(i => i.track.id !== id);
                    root.total = Math.max(0, root.total - 1);
                    root._hide(id, false);
                    done?.();
                }, () => {
                    root._hide(id, false);
                    done?.();
                }));
        };
        // Router.askRemove would ask in the main window
        if (UiConfig.removeMode !== "confirm") {
            run();
            return;
        }
        root.confirmDialog.run = run;
        root.confirmDialog.title = I18n.tr("Delete \"%1\"?", "delete playlist confirmation title").arg(item.track.title);
        root.confirmDialog.ask();
    }

    onLibraryIdChanged: _reset()
    onExcludedOnlyChanged: _reset()
    onActiveChanged: _reset()
    onCacheStampChanged: {
        if (!reloadTimer.running)
            reloadTimer.start();
    }
    onNearEndChanged: _more()
    Component.onCompleted: reload()

    Timer {
        id: reloadTimer
        interval: root.reloadDelay
        onTriggered: root.reload()
    }

    spacing: Theme.spacingM

    SettingsCard {
        width: parent.width
        title: root.excludedOnly ? Phrases.join(I18n.tr("Excluded tracks", "offline tracks no rule may download"), CatalogService.libraryName(root.libraryId)) : Phrases.join(CatalogService.libraryName(root.libraryId), Phrases.join(Phrases.tracks(root.library.count ?? 0), Phrases.size(root.library.used ?? 0)))

        SettingsRow {
            visible: !root.excludedOnly
            title: I18n.tr("Clear downloads", "offline action for one library")
            subtitle: (root.library.count ?? 0) > 0 ? I18n.tr("Deletes downloaded tracks. Rules are kept.", "offline clear action description") : I18n.tr("No downloaded tracks", "why clearing the cache is disabled")
            iconName: "delete_sweep"

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                text: I18n.tr("Clear", "button that empties a list")
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                enabled: (root.library.count ?? 0) > 0
                onClicked: SettingsService.clearCache(root.libraryId)
            }
        }

        SettingsRow {
            visible: !root.loaded
            title: I18n.tr("Loading", "something still waiting for data: the play button, a list")
            searchable: false

            DLoadingIndicator {
                anchors.verticalCenter: parent.verticalCenter
            }
        }

        SettingsRow {
            visible: root.loaded && root.total === 0
            title: root.excludedOnly ? I18n.tr("No excluded tracks", "the offline tracks list filtered to exclusions, empty") : I18n.tr("No downloaded tracks", "why clearing the cache is disabled")
            searchable: false
        }
    }

    Column {
        width: parent.width
        spacing: Theme.groupedListGap

        Repeater {
            // rows stay when the list reloads, so focus and hover do too
            model: RowModel {
                rows: root.shown.length
            }

            DListItem {
                id: row
                required property int index
                readonly property var modelData: root.shown[index] ?? root.blank
                readonly property string detail: Phrases.joinAll([modelData.track.artist, modelData.excluded ? "" : Phrases.size(modelData.size), root.reason(modelData)])
                width: parent.width
                implicitHeight: PlayerMetrics.listRowTwoLineHeight
                surfaceColor: SettingsMetrics.rowColor
                firstInGroup: index === 0
                lastInGroup: index === root.shown.length - 1
                Accessible.name: Phrases.named(modelData.track.title, detail)

                Column {
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.spacingL
                    anchors.right: action.left
                    anchors.rightMargin: Theme.spacingM
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: Theme.spacingXXS

                    StyledText {
                        width: parent.width
                        text: row.modelData.track.title
                        font.pixelSize: Theme.fontSizeMedium
                        color: row.contentColor
                        elide: Text.ElideRight
                        wrapMode: Text.NoWrap
                        horizontalAlignment: Text.AlignLeft
                    }

                    StyledText {
                        width: parent.width
                        text: row.detail
                        font.pixelSize: Theme.fontSizeSmall
                        color: row.supportingContentColor
                        elide: Text.ElideRight
                        wrapMode: Text.NoWrap
                        horizontalAlignment: Text.AlignLeft
                    }
                }

                Item {
                    id: action
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.spacingM
                    anchors.verticalCenter: parent.verticalCenter
                    width: row.modelData.excluded ? again.width : remove.width
                    height: Math.max(again.height, remove.height)

                    DActionButton {
                        id: remove
                        anchors.centerIn: parent
                        visible: !row.modelData.excluded
                        iconName: "delete"
                        tooltipText: I18n.tr("Delete", "confirm button that deletes an item")
                        Accessible.name: Phrases.named(tooltipText, row.modelData.track.title)
                        onClicked: root.deleteTrack(row.modelData)
                    }

                    DButton {
                        id: again
                        anchors.centerIn: parent
                        visible: row.modelData.excluded
                        text: I18n.tr("Download again", "button that lets offline rules download an excluded track again")
                        backgroundColor: Theme.secondaryContainer
                        textColor: Theme.onSecondaryContainer
                        Accessible.name: Phrases.named(text, row.modelData.track.title)
                        onClicked: SettingsService.includeDownload(row.modelData.track, () => root.reload())
                    }
                }
            }
        }
    }

    readonly property ConfirmDialog confirmDialog: ConfirmDialog {
        property var run: null
        host: root
        confirmText: I18n.tr("Delete", "confirm button that deletes an item")
        onConfirmed: run?.()
    }

    // held in a property: as a Column child the Column would position it and drop its anchors
    readonly property WindowDialog deleteDialog: WindowDialog {
        host: root
        title: root.pendingDelete?.track.title ?? ""
        supportingText: I18n.tr("A rule keeps this track and may download it again. Exclude creates an exception for this track from all rules.", "asks how to delete a downloaded track an offline rule keeps")
        onRejected: opened = false

        actions: [
            DButton {
                text: I18n.tr("Keep", "dialog button that keeps a downloaded track")
                backgroundColor: "transparent"
                textColor: Theme.primary
                onClicked: root.deleteDialog.rejected()
            },
            DButton {
                text: I18n.tr("Delete now", "dialog button that deletes a downloaded track a rule may download again")
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                onClicked: {
                    root.deleteDialog.opened = false;
                    SettingsService.deleteDownload(root.pendingDelete.track, false, () => root.reload());
                }
            },
            DButton {
                text: I18n.tr("Delete and exclude", "dialog button that deletes a downloaded track and keeps rules from downloading it")
                onClicked: {
                    root.deleteDialog.opened = false;
                    SettingsService.deleteDownload(root.pendingDelete.track, true, () => root.reload());
                }
            }
        ]
    }
}
