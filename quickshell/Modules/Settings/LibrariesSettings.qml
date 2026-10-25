pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets
import qs.Widgets

Item {
    id: root

    property var pendingRemove: null
    // the library's offline overview sub-page: a server library id, or ""
    property string cachePage: ""
    // null while the list loads
    property var cachedSongs: null
    readonly property var servers: PlayerService.libraries.filter(l => PlayerService.isRemote(l.id))
    readonly property var ruleKinds: ["everything", "favorites", "played", "mostPlayed", "playlist", "album", "artist"]
    readonly property real ruleIndent: SettingsMetrics.rowPaddingH + Theme.spacingXL
    readonly property real gigabyte: 1e9
    // Home's empty state asks for the add dialog
    readonly property bool addWanted: Router.wantAddLibrary

    onAddWantedChanged: {
        if (!addWanted)
            return;
        // later: clearing the flag from its own change handler is a binding loop
        Qt.callLater(() => {
            Router.wantAddLibrary = false;
            root.cachePage = "";
            dialog.add();
        });
    }

    function size(bytes) {
        return bytes >= gigabyte ? I18n.tr("%1 GB", "file size").arg((bytes / gigabyte).toFixed(1)) : I18n.tr("%1 MB", "file size").arg(Math.round(bytes / 1e6));
    }

    function kindLabel(kind) {
        switch (kind) {
        case "everything":
            return I18n.tr("Everything", "offline rule: the whole library");
        case "favorites":
            return I18n.tr("Favorites", "favorite tracks: page, cache mode, library feature");
        case "played":
            return I18n.tr("Recently played", "offline rule: the tracks you played last");
        case "mostPlayed":
            return I18n.tr("Most played", "home shelf and page of the most played tracks");
        case "playlist":
            return I18n.tr("Playlist…", "offline rule kind that asks which playlist");
        case "album":
            return I18n.tr("Album…", "offline rule kind that asks which album");
        case "artist":
            return I18n.tr("Artist…", "offline rule kind that asks which artist");
        }
        return kind;
    }

    // a rule or a rule reference ({kind, id}); libraryId "" for local playlists
    function ruleLabel(libraryId, rule) {
        const ref = libraryId + ":" + rule.id;
        switch (rule.kind) {
        case "playlist":
            return Phrases.join(I18n.tr("Playlist", "home shelf source kind"), PlaylistService.playlist(ref)?.name ?? rule.id);
        case "album":
            return Phrases.join(I18n.tr("Album", "an album: page label, list column, ReplayGain mode"), CatalogService.album(ref)?.title ?? rule.id);
        case "artist":
            return Phrases.join(I18n.tr("Artist", "track list column"), CatalogService.artist(ref)?.name ?? rule.id);
        }
        return kindLabel(rule.kind);
    }

    function ruleIcon(kind) {
        switch (kind) {
        case "everything":
            return "select_all";
        case "favorites":
            return "favorite";
        case "played":
            return "history";
        case "mostPlayed":
            return "trending_up";
        case "playlist":
            return "queue_music";
        case "album":
            return "album";
        }
        return "artist";
    }

    function limitText(limit) {
        return (limit?.bytes ?? 0) > 0 ? size(limit.bytes) : Phrases.tracks(limit?.songs ?? 0);
    }

    // what a rule holds: "412 tracks · 1.6 GB", "400 of 412 tracks · 1.6 GB"; dynamic rules lead with their limit
    function ruleFacts(rule) {
        const held = rule.cached < rule.tracks ? I18n.tr("%1 of %2", "offline rule progress: cached of all, %2 is a track count").arg(rule.cached).arg(Phrases.tracks(rule.tracks)) : Phrases.tracks(rule.cached);
        const facts = Phrases.join(held, size(rule.size ?? 0));
        if (rule.kind === "played" || rule.kind === "mostPlayed")
            return Phrases.join(I18n.tr("Limit %1", "dynamic offline rule, %1 is its limit").arg(limitText(rule.limit)), facts);
        return facts;
    }

    // kinds this server can list and doesn't have yet (playlists, albums and artists can always take one more)
    function addableKinds(libraryId) {
        const rules = SettingsService.cacheRules(libraryId);
        return ruleKinds.filter(k => {
            const cap = SettingsService.ruleCapability(k);
            if (cap !== "" && !CatalogService.hasCapability(libraryId, cap))
                return false;
            return k === "playlist" || k === "album" || k === "artist" || !rules.some(r => r.kind === k);
        });
    }

    function openAddSheet(libraryId) {
        addSheet.libraryId = libraryId;
        addSheet.opened = true;
    }

    function addRule(libraryId, kind) {
        switch (kind) {
        case "played":
            limitDialog.open(libraryId, kind, {
                "bytes": PlayerMetrics.offlinePlayedDefaultBytes
            }, true);
            return;
        case "mostPlayed":
            limitDialog.open(libraryId, kind, {
                "songs": PlayerMetrics.offlineRuleDefaultTracks
            }, true);
            return;
        case "playlist":
        case "album":
        case "artist":
            itemPicker.open(libraryId, kind);
            return;
        }
        SettingsService.addCacheRule(libraryId, kind);
    }

    // Follows the removing setting. The removal waits for the Undo toast; meanwhile the rule is hidden.
    function removeRule(libraryId, rule) {
        const label = ruleLabel(libraryId, rule);
        const run = () => {
            SettingsService.hideRule(libraryId, rule.key, true);
            Router.offerUndo(I18n.tr("Removed %1. Tracks kept by other rules stay downloaded.", "toast after removing an offline rule, %1 is the rule").arg(label), () => SettingsService.hideRule(libraryId, rule.key, false), done => SettingsService.removeCacheRule(libraryId, rule.kind, rule.id, () => {
                    SettingsService.hideRule(libraryId, rule.key, false);
                    done?.();
                }));
        };
        // Router.askRemove would ask in the main window
        if (UiConfig.removeMode !== "confirm") {
            run();
            return;
        }
        removeRuleDialog.run = run;
        removeRuleDialog.title = I18n.tr("Remove %1?", "remove offline rule confirmation title, %1 is the rule").arg(label);
        removeRuleDialog.ask();
    }

    function openCachePage(libraryId) {
        cachedSongs = null;
        cachePage = libraryId;
        SettingsService.cacheList(libraryId, list => {
            if (root.cachePage === libraryId)
                root.cachedSongs = list;
        });
    }

    function describe(library) {
        if (PlayerService.isRemote(library.id))
            return Phrases.join(PlayerService.kindLabel(library.kind), I18n.tr("%1 at %2", "server account: user at address").arg(library.user).arg(library.url.replace(/^https?:\/\//, "")));
        return Phrases.join(library.folders.join(", "), Phrases.tracks(Phrases.libraryTrackCount(library.id)));
    }

    function status(library) {
        if (!library.enabled)
            return I18n.tr("Off. Its tracks are hidden and skipped in playlists.", "disabled library status");
        if (library.error)
            return Phrases.libraryError(library);
        if (library.scanning)
            return Phrases.libraryBusy(library);
        if (!library.lastScan)
            return "";
        return I18n.tr("Last scanned %1", "library row status, %1 is a date and time").arg(new Date(library.lastScan * 1000).toLocaleString(Qt.locale(), Locale.ShortFormat));
    }

    SettingsPage {
        id: page
        anchors.fill: parent

        bottomPadding: SettingsMetrics.pagePaddingV + fab.height + Theme.spacingL

        Column {
            id: groupColumn
            visible: root.cachePage === ""
            width: parent.width
            spacing: Theme.spacingL

            SettingsPageIntro {
                text: I18n.tr("Turning a library off hides its music without removing it.", "libraries settings page intro")
            }

            SettingsGroup {
                Repeater {
                    model: PlayerService.libraries

                    SettingsRow {
                        id: row
                        required property var modelData
                        readonly property bool remote: PlayerService.isRemote(modelData.id)

                        title: modelData.name
                        subtitle: root.describe(modelData)
                        clickable: true
                        onClicked: dialog.edit(modelData)

                        leading: Rectangle {
                            width: Theme.iconButtonSize
                            height: Theme.iconButtonSize
                            radius: Theme.fullRadius(width, height)
                            anchors.verticalCenter: parent.verticalCenter
                            color: row.remote ? Theme.tertiaryContainer : Theme.secondaryContainer

                            DIcon {
                                anchors.centerIn: parent
                                name: UiConfig.entryIcon({
                                    "kind": "library",
                                    "target": row.modelData.id
                                })
                                size: Theme.iconSizeMedium
                                color: row.remote ? Theme.onTertiaryContainer : Theme.onSecondaryContainer
                            }
                        }

                        body: Column {
                            width: parent.width

                            StyledText {
                                width: parent.width
                                visible: text !== ""
                                text: PlayerService.featureSummary(row.modelData.id)
                                font.pixelSize: Theme.fontSizeSmall
                                color: Theme.surfaceVariantText
                                wrapMode: Text.WordWrap
                                horizontalAlignment: Text.AlignLeft
                            }

                            StyledText {
                                width: parent.width
                                visible: text !== ""
                                text: root.status(row.modelData)
                                font.pixelSize: Theme.fontSizeSmall
                                color: row.modelData.scanning ? Theme.primary : row.modelData.error && row.modelData.enabled ? Theme.error : Theme.surfaceVariantText
                                wrapMode: Text.WordWrap
                                horizontalAlignment: Text.AlignLeft
                            }

                            Repeater {
                                model: row.modelData.noticeCodes

                                StyledText {
                                    // {code, message, detail}
                                    required property var modelData
                                    width: parent.width
                                    text: Phrases.reason(modelData.code, modelData.message, modelData.detail)
                                    font.pixelSize: Theme.fontSizeSmall
                                    color: Theme.surfaceVariantText
                                    wrapMode: Text.WordWrap
                                    horizontalAlignment: Text.AlignLeft
                                }
                            }
                        }

                        DActionButton {
                            anchors.verticalCenter: parent.verticalCenter
                            visible: row.modelData.enabled && !row.modelData.available
                            iconName: "sync_problem"
                            tooltipText: I18n.tr("Try again", "library action that checks an unavailable library again")
                            Accessible.name: tooltipText
                            onClicked: PlayerService.retryLibrary(row.modelData.id)
                        }

                        DActionButton {
                            anchors.verticalCenter: parent.verticalCenter
                            iconName: "refresh"
                            enabled: row.modelData.enabled && !row.modelData.scanning
                            tooltipText: row.remote ? I18n.tr("Sync now", "library row action") : I18n.tr("Rescan", "library row action")
                            Accessible.name: tooltipText
                            onClicked: PlayerService.rescanLibrary(row.modelData.id)
                        }

                        DActionButton {
                            anchors.verticalCenter: parent.verticalCenter
                            iconName: "delete"
                            iconColor: Theme.error
                            tooltipText: I18n.tr("Remove", "button that removes an item")
                            Accessible.name: tooltipText
                            onClicked: {
                                root.pendingRemove = row.modelData;
                                removeDialog.ask();
                            }
                        }

                        DToggle {
                            anchors.verticalCenter: parent.verticalCenter
                            hideText: true
                            text: I18n.tr("Enabled", "library row switch")
                            checked: row.modelData.enabled
                            onToggled: value => PlayerService.setLibraryEnabled(row.modelData.id, value)
                        }
                    }
                }
            }

            StyledText {
                width: parent.width
                visible: PlayerService.libraries.length === 0
                text: I18n.tr("No libraries", "libraries settings empty state")
                font.pixelSize: Theme.fontSizeMedium
                color: Theme.surfaceVariantText
                horizontalAlignment: Text.AlignLeft
            }
        }

        SettingsCard {
            width: parent.width
            visible: root.cachePage === "" && root.servers.length > 0
            title: I18n.tr("Offline", "libraries settings section about cached server tracks")

            SettingsRow {
                title: I18n.tr("Space used", "offline storage usage row")
                subtitle: I18n.tr("%1 of %2", "storage usage: used of limit").arg(root.size(SettingsService.cache.used ?? 0)).arg(root.size(SettingsService.cache.limit ?? 0))
                iconName: "hard_drive"

                body: Rectangle {
                    width: parent.width
                    height: SettingsMetrics.usageBarHeight
                    radius: Theme.fullRadius(width, height)
                    color: Theme.secondaryContainer
                    Accessible.role: Accessible.ProgressBar
                    Accessible.name: I18n.tr("Space used", "offline storage usage row")

                    Rectangle {
                        width: parent.width * Math.min(1, (SettingsService.cache.used ?? 0) / Math.max(1, SettingsService.cache.limit ?? 0))
                        height: parent.height
                        radius: parent.radius
                        color: SettingsService.cache.full || SettingsService.cache.pinnedOverLimit ? Theme.error : Theme.primary
                    }
                }
            }

            SettingsRow {
                visible: SettingsService.cache.pinnedOverLimit === true || SettingsService.cache.full === true
                title: SettingsService.cache.pinnedOverLimit ? I18n.tr("Kept tracks exceed the size limit", "offline warning title") : I18n.tr("Offline storage is full", "offline warning title")
                subtitle: SettingsService.cache.pinnedOverLimit ? I18n.tr("Rules other than Recently played and Most played keep %1, over the %2 size limit. Downloads are paused.", "offline warning: pinned songs exceed the size limit").arg(root.size(SettingsService.cache.pinned ?? 0)).arg(root.size(SettingsService.cache.limit ?? 0)) : I18n.tr("Downloads are paused. Raise the size limit or remove a rule.", "offline warning: the cache is full")
                subtitleColor: Theme.error
                iconName: "warning"
                iconColor: Theme.error
            }

            SettingsRow {
                id: limitRow
                readonly property int saved: Math.round((SettingsService.cache.limit ?? 0) / root.gigabyte)
                property int draft: saved
                title: I18n.tr("Size limit", "offline storage limit slider")
                subtitle: I18n.tr("Recently played and Most played tracks are removed first.", "offline storage limit description")

                function commit() {
                    if (draft !== saved)
                        SettingsService.setCacheLimit(draft * root.gigabyte / 1e6);
                }

                onSavedChanged: draft = saved

                body: DPillSlider {
                    width: parent.width
                    minimum: 1
                    maximum: 200
                    step: 1
                    value: limitRow.draft
                    showValue: true
                    unit: I18n.tr(" GB", "unit suffix for gigabytes with a space")
                    Accessible.name: limitRow.title
                    // a drag saves once, on release
                    onMoved: value => {
                        limitRow.draft = value;
                        if (!dragging)
                            limitRow.commit();
                    }
                    onDraggingChanged: {
                        if (!dragging)
                            limitRow.commit();
                    }
                }
            }

            SettingsButtonGroupRow {
                text: I18n.tr("Download upcoming tracks", "offline setting: how many of the next server tracks download ahead")
                description: I18n.tr("Downloads the next tracks of the queue while one plays.", "offline setting description")
                model: [I18n.tr("Off", "option that turns something off"), "1", "2", "3"]
                currentIndex: SettingsService.settings.prefetchTracks
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        SettingsService.setSetting("prefetchTracks", index);
                }
            }

            SettingsToggleRow {
                text: I18n.tr("Download on metered connections", "offline setting: keep downloading upcoming tracks on a metered connection")
                description: enabled ? "" : I18n.tr("Available when Download upcoming tracks is on.", "why the metered connections setting is unavailable")
                enabled: SettingsService.settings.prefetchTracks > 0
                checked: SettingsService.settings.prefetchOnMetered
                onToggled: value => SettingsService.setSetting("prefetchOnMetered", value)
            }

            SettingsRow {
                title: I18n.tr("Clear all downloads", "offline action")
                subtitle: (SettingsService.cache.count ?? 0) > 0 ? I18n.tr("Deletes downloaded tracks. Rules are kept.", "offline clear action description") : I18n.tr("No downloaded tracks", "why clearing the cache is disabled")
                iconName: "delete_sweep"

                DButton {
                    anchors.verticalCenter: parent.verticalCenter
                    text: I18n.tr("Clear", "button that empties a list")
                    backgroundColor: Theme.secondaryContainer
                    textColor: Theme.onSecondaryContainer
                    enabled: (SettingsService.cache.count ?? 0) > 0
                    onClicked: SettingsService.clearCache("")
                }
            }
        }

        // per server library: its row (to the overview), its rules, and Add rule
        Repeater {
            model: root.cachePage === "" ? root.servers : []

            SettingsGroup {
                id: libraryGroup
                required property var modelData
                readonly property var usage: (SettingsService.cache.libraries ?? []).find(c => c.library === modelData.id) ?? {}
                readonly property var rules: SettingsService.cacheRules(modelData.id)
                readonly property var addKinds: root.addableKinds(modelData.id)
                width: parent.width

                SettingsRow {
                    title: libraryGroup.modelData.name
                    iconName: "cloud"
                    trailingBadge: (libraryGroup.usage.count ?? 0) > 0 ? Phrases.join(Phrases.tracks(libraryGroup.usage.count), root.size(libraryGroup.usage.used ?? 0)) : libraryGroup.rules.length === 0 ? I18n.tr("No rules", "a server library without offline rules") : ""
                    showChevron: true
                    clickable: true
                    onClicked: root.openCachePage(libraryGroup.modelData.id)
                }

                Repeater {
                    model: libraryGroup.rules

                    RuleRow {
                        libraryId: libraryGroup.modelData.id
                    }
                }

                SettingsRow {
                    title: I18n.tr("Add rule", "offline action that keeps more of a library on this device")
                    subtitle: libraryGroup.addKinds.map(k => root.kindLabel(k)).join(" · ")
                    iconName: "add"
                    paddingH: root.ruleIndent
                    clickable: libraryGroup.addKinds.length > 0
                    enabled: clickable
                    onClicked: root.openAddSheet(libraryGroup.modelData.id)
                }
            }
        }

        // local playlists can hold tracks of every server library
        SettingsCard {
            width: parent.width
            visible: root.cachePage === "" && SettingsService.cacheRules("").length > 0
            title: I18n.tr("Local playlists", "offline section for local playlists kept on this device")

            Repeater {
                model: root.cachePage === "" ? SettingsService.cacheRules("") : []

                RuleRow {
                    libraryId: ""
                }
            }
        }

        // the library overview sub-page
        SettingsBackRow {
            visible: root.cachePage !== ""
            onClicked: root.cachePage = ""
        }

        SettingsCard {
            width: parent.width
            visible: root.cachePage !== ""
            title: I18n.tr("Offline rules for %1", "library offline overview section, %1 is the library").arg(PlayerService.libraryName(root.cachePage))

            Repeater {
                model: root.cachePage !== "" ? SettingsService.cacheRules(root.cachePage) : []

                RuleRow {
                    libraryId: root.cachePage
                    paddingH: SettingsMetrics.rowPaddingH
                }
            }

            SettingsRow {
                visible: SettingsService.cacheRules(root.cachePage).length === 0
                title: I18n.tr("No rules", "a server library without offline rules")
                subtitle: I18n.tr("Tracks without a rule are removed first when the size limit is reached.", "library offline overview without rules")
                iconName: "rule"
            }

            SettingsRow {
                title: I18n.tr("Clear downloads", "offline action for one library")
                subtitle: I18n.tr("Deletes downloaded tracks. Rules are kept.", "offline clear action description")
                iconName: "delete_sweep"

                DButton {
                    anchors.verticalCenter: parent.verticalCenter
                    text: I18n.tr("Clear", "button that empties a list")
                    backgroundColor: Theme.secondaryContainer
                    textColor: Theme.onSecondaryContainer
                    enabled: (root.cachedSongs ?? []).length > 0
                    onClicked: {
                        SettingsService.clearCache(root.cachePage);
                        root.cachePage = "";
                    }
                }
            }
        }

        SettingsCard {
            width: parent.width
            visible: root.cachePage !== ""
            title: root.cachedSongs ? Phrases.join(I18n.tr("Downloaded tracks", "library offline overview section"), Phrases.tracks(root.cachedSongs.length)) : I18n.tr("Downloaded tracks", "library offline overview section")

            SettingsRow {
                visible: root.cachedSongs === null
                title: I18n.tr("Loading", "something still waiting for data: the play button, a list")

                DLoadingIndicator {
                    anchors.verticalCenter: parent.verticalCenter
                }
            }

            SettingsRow {
                visible: (root.cachedSongs ?? [null]).length === 0
                title: I18n.tr("No downloaded tracks", "why clearing the cache is disabled")
            }

            // One row per cached track, capped; a paged list if caches of thousands become common
            Repeater {
                model: root.cachePage !== "" ? (root.cachedSongs ?? []).slice(0, PlayerMetrics.cacheListLimit) : []

                SettingsRow {
                    required property var modelData
                    title: modelData.track.title
                    subtitle: Phrases.join(Phrases.join(modelData.track.artist, root.size(modelData.size ?? 0)), (modelData.rules ?? []).length > 0 ? I18n.tr("Kept by %1", "offline song row, %1 lists the rules holding it").arg(modelData.rules.map(r => root.ruleLabel(r.local ? "" : root.cachePage, r)).join(", ")) : I18n.tr("No rule", "offline song row for a song no rule holds"))
                }
            }

            SettingsRow {
                visible: (root.cachedSongs ?? []).length > PlayerMetrics.cacheListLimit
                title: Phrases.moreTracks((root.cachedSongs ?? []).length - PlayerMetrics.cacheListLimit)
            }
        }
    }

    DFab {
        id: fab
        visible: root.cachePage === ""
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.bottom
        anchors.bottomMargin: Theme.spacingL
        text: I18n.tr("Add library", "libraries settings primary action")
        iconName: "add"
        onClicked: dialog.add()
    }

    LibraryDialog {
        id: dialog
        host: root
    }

    ConfirmDialog {
        id: removeDialog
        host: root
        title: I18n.tr("Remove \"%1\"?", "remove library confirmation title").arg(root.pendingRemove?.name ?? "")
        supportingText: PlayerService.isRemote(root.pendingRemove?.id ?? "") ? I18n.tr("The server and its saved password are removed from Dux Player. Nothing is deleted on the server.", "remove server library confirmation") : I18n.tr("The folders are removed from Dux Player. No files are deleted.", "remove local library confirmation")
        confirmText: I18n.tr("Remove", "button that removes an item")
        onConfirmed: PlayerService.removeLibrary(root.pendingRemove.id)
    }

    ConfirmDialog {
        id: removeRuleDialog
        property var run: null
        host: root
        supportingText: I18n.tr("Its downloaded tracks are deleted unless another rule keeps them.", "remove offline rule confirmation")
        confirmText: I18n.tr("Remove", "button that removes an item")
        onConfirmed: run?.()
    }

    PickerSheet {
        id: addSheet
        property string libraryId: ""
        title: I18n.tr("Add rule", "offline action that keeps more of a library on this device")
        sections: [
            {
                "title": I18n.tr("Keep on this device", "toggle that caches an item's tracks for offline play"),
                "items": root.addableKinds(addSheet.libraryId).map(k => ({
                            "label": root.kindLabel(k),
                            "icon": root.ruleIcon(k),
                            "value": k
                        }))
            }
        ]
        onPicked: value => root.addRule(addSheet.libraryId, value)
    }

    OfflineLimitDialog {
        id: limitDialog
        host: root
    }

    OfflineItemPicker {
        id: itemPicker
        host: root
    }

    // a rule of a library ("" for local playlists): its facts, remove, and the limit for dynamic ones
    component RuleRow: SettingsRow {
        id: ruleRow
        required property var modelData
        property string libraryId: ""
        readonly property bool dynamic: modelData.kind === "played" || modelData.kind === "mostPlayed"

        title: root.ruleLabel(libraryId, modelData)
        subtitle: root.ruleFacts(modelData)
        iconName: root.ruleIcon(modelData.kind)
        paddingH: root.ruleIndent
        clickable: dynamic
        onClicked: limitDialog.open(libraryId, modelData.kind, modelData.limit, false)

        DActionButton {
            anchors.verticalCenter: parent.verticalCenter
            visible: ruleRow.dynamic
            iconName: "edit"
            tooltipText: I18n.tr("Change limit", "offline rule action")
            Accessible.name: Phrases.named(tooltipText, ruleRow.title)
            onClicked: ruleRow.clicked()
        }

        DActionButton {
            anchors.verticalCenter: parent.verticalCenter
            iconName: "close"
            tooltipText: I18n.tr("Remove", "button that removes an item")
            Accessible.name: Phrases.named(tooltipText, ruleRow.title)
            onClicked: root.removeRule(ruleRow.libraryId, ruleRow.modelData)
        }
    }
}
