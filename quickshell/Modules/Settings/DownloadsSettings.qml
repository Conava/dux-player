pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets

Item {
    id: root

    // "" for the overview, a server library id for its downloaded tracks, "excluded:<id>" for its excluded ones
    property string subPage: ""
    // the settings search reading a sub-page: it fetches nothing
    property bool quiet: false
    // a turned-off server lists no capabilities; its rules keep its card
    readonly property var servers: CatalogService.libraries.filter(l => CatalogService.isRemote(l.id) || SettingsService.cacheRules().some(r => r.library === l.id))
    readonly property var subViews: servers.map(l => l.id)
    readonly property string noServerReason: I18n.tr("Add a server library first.", "why the offline settings are hidden")
    readonly property var ruleKinds: ["everything", "favorites", "played", "mostPlayed", "playlist", "album", "artist"]
    // General adds rules for all servers, and local playlists
    readonly property var generalKinds: ["everything", "favorites", "played", "mostPlayed"].filter(k => servers.some(l => l.enabled && supports(l.id, k))).concat(["playlist"])
    // the scope Add rule was opened for: a server library, "" from General
    property string addScope: ""
    readonly property var addKinds: addScope === "" ? generalKinds : libraryKinds(addScope)
    // runtime only, by library id; unset follows expandedByDefault
    property var expandedLibraries: ({})
    readonly property bool expandedByDefault: servers.length === 1
    readonly property string excludedPrefix: "excluded:"
    readonly property real gigabyte: 1e9

    function supports(libraryId, kind) {
        const cap = SettingsService.ruleCapability(kind);
        return cap === "" || CatalogService.hasCapability(libraryId, cap);
    }

    function libraryKinds(libraryId) {
        return ruleKinds.filter(k => supports(libraryId, k));
    }

    function isExpanded(libraryId) {
        return expandedLibraries[libraryId] ?? expandedByDefault;
    }

    function toggleExpanded(libraryId) {
        const next = Object.assign({}, expandedLibraries);
        next[libraryId] = !isExpanded(libraryId);
        expandedLibraries = next;
    }

    // the rule kinds offered by Add rule; a playlist, album or artist is picked next
    function kindLabel(kind) {
        switch (kind) {
        case "playlist":
            return I18n.tr("Playlist…", "offline rule kind that asks which playlist");
        case "album":
            return I18n.tr("Album…", "offline rule kind that asks which album");
        case "artist":
            return I18n.tr("Artist…", "offline rule kind that asks which artist");
        }
        return Phrases.offlineRule(kind);
    }

    // local playlists have library ""
    function ruleLabel(rule) {
        return Phrases.offlineRule(rule.kind, (rule.library ?? "") + ":" + rule.id, rule.id);
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

    // the General card's subtitle lead; a library card's rules need none
    function ruleScope(rule) {
        return rule.local ? I18n.tr("Local playlist", "playlist kind label") : I18n.tr("All servers", "offline rule scope: every server library");
    }

    function limitText(limit) {
        return (limit?.bytes ?? 0) > 0 ? Phrases.size(limit.bytes) : Phrases.tracks(limit?.songs ?? 0);
    }

    // what a rule holds: "412 tracks · 1.6 GB", "400 of 412 tracks · 1.6 GB"; dynamic rules lead with their limit
    function ruleFacts(rule) {
        const held = rule.cached < rule.tracks ? I18n.tr("%1 of %2", "offline rule progress: cached of all, %2 is a track count").arg(rule.cached).arg(Phrases.tracks(rule.tracks)) : Phrases.tracks(rule.cached);
        const facts = Phrases.join(held, Phrases.size(rule.size ?? 0));
        if (rule.kind === "played" || rule.kind === "mostPlayed")
            return Phrases.join(I18n.tr("Limit %1", "dynamic offline rule, %1 is its limit").arg(limitText(rule.limit)), facts);
        return facts;
    }

    function addRule(kind) {
        switch (kind) {
        case "playlist":
        case "album":
        case "artist":
            itemPicker.open(kind, addScope);
            return;
        }
        ruleDialog.open(null, kind, addScope);
    }

    function openAddSheet(scope) {
        addScope = scope;
        addSheet.opened = true;
    }

    // Follows the removing setting. The removal waits for the Undo toast; meanwhile the rule is hidden.
    function removeRule(rule) {
        const label = ruleLabel(rule);
        const library = rule.library ?? "";
        const run = () => {
            SettingsService.hideRule(library, rule.key, true);
            Router.offerUndo(I18n.tr("Removed %1", "toast after removing an offline rule, %1 is the rule").arg(label), () => SettingsService.hideRule(library, rule.key, false), done => SettingsService.removeCacheRule(library, rule.kind, rule.id, () => {
                    SettingsService.hideRule(library, rule.key, false);
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

    function openSubView(id, quiet) {
        root.quiet = quiet === true;
        subPage = id;
    }

    SettingsPage {
        id: page
        anchors.fill: parent

        SettingsPageIntro {
            visible: root.servers.length === 0
            text: I18n.tr("Downloads keep tracks from server libraries on this device.", "downloads settings page without a server library")
        }

        SettingsCard {
            visible: root.subPage === "" && root.servers.length > 0
            unavailableReason: root.noServerReason
            title: I18n.tr("Storage", "downloads settings section: space used and its limit")

            SettingsRow {
                title: I18n.tr("Space used", "offline storage usage row")
                keywords: "cache download storage disk"
                subtitle: I18n.tr("%1 of %2", "storage usage: used of limit").arg(Phrases.size(SettingsService.cache.used ?? 0)).arg(Phrases.size(SettingsService.cache.limit ?? 0))
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
                searchable: false
                title: SettingsService.cache.pinnedOverLimit ? I18n.tr("Kept tracks exceed the size limit", "offline warning title") : I18n.tr("Offline storage is full", "offline warning title")
                subtitle: SettingsService.cache.pinnedOverLimit ? I18n.tr("Kept tracks use %1, over the %2 limit. Downloads are paused.", "offline warning: pinned songs exceed the size limit").arg(Phrases.size(SettingsService.cache.pinned ?? 0)).arg(Phrases.size(SettingsService.cache.limit ?? 0)) : I18n.tr("Downloads are paused. Raise the size limit or remove a rule.", "offline warning: the cache is full")
                subtitleColor: Theme.error
                iconName: "warning"
                iconColor: Theme.error
            }

            SettingsSliderRow {
                text: I18n.tr("Size limit", "offline storage limit slider")
                keywords: "cache download storage disk"
                description: I18n.tr("Recently played and Most played tracks are removed first.", "offline storage limit description")
                minimum: 1
                maximum: 200
                unit: I18n.tr(" GB", "unit suffix for gigabytes with a space")
                value: Math.round((SettingsService.cache.limit ?? 0) / root.gigabyte)
                onSliderDragFinished: finalValue => SettingsService.setCacheLimit(finalValue * root.gigabyte / 1e6)
            }

            SettingsButtonGroupRow {
                text: I18n.tr("Download upcoming tracks", "offline setting: how many of the next server tracks download ahead")
                keywords: "cache prefetch"
                model: [I18n.tr("Off", "option that turns something off"), "1", "2", "3"]
                currentIndex: SettingsService.settings.prefetchTracks
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        SettingsService.setSetting("prefetchTracks", index);
                }
            }

            SettingsToggleRow {
                text: I18n.tr("Download on metered connections", "offline setting: keep downloading upcoming tracks on a metered connection")
                keywords: "cache mobile data"
                description: enabled ? "" : I18n.tr("Available when Download upcoming tracks is on.", "why the metered connections setting is unavailable")
                enabled: SettingsService.settings.prefetchTracks > 0
                checked: SettingsService.settings.prefetchOnMetered
                onToggled: value => SettingsService.setSetting("prefetchOnMetered", value)
            }

            SettingsRow {
                title: I18n.tr("Clear all downloads", "offline action")
                keywords: "cache delete"
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

        SettingsCard {
            visible: root.subPage === "" && root.servers.length > 0
            unavailableReason: root.noServerReason
            title: I18n.tr("General", "settings sidebar tab label")

            Repeater {
                model: root.subPage === "" ? SettingsService.cacheRules().filter(r => r.local || (r.library ?? "") === "") : []

                RuleRow {
                    subtitle: Phrases.join(root.ruleScope(modelData), root.ruleFacts(modelData))
                }
            }

            SettingsRow {
                title: I18n.tr("Add rule", "offline action that keeps more of a library on this device")
                subtitle: root.generalKinds.map(k => root.kindLabel(k)).join(" · ")
                iconName: "add"
                clickable: true
                onClicked: root.openAddSheet("")
            }
        }

        Column {
            // read by the settings search
            property string unavailableReason: root.noServerReason
            width: parent.width
            visible: root.subPage === "" && root.servers.length > 0

            SectionLabel {
                text: I18n.tr("Libraries", "the music libraries: page, settings tab, section")
            }

            Column {
                width: parent.width
                spacing: Theme.spacingM

                Repeater {
                    model: root.subPage === "" ? root.servers : []

                    SettingsGroup {
                        id: libraryCard
                        required property var modelData
                        readonly property bool expanded: root.isExpanded(modelData.id)
                        readonly property var usage: SettingsService.cacheLibrary(modelData.id)

                        SettingsRow {
                            title: libraryCard.modelData.name
                            searchable: false
                            subtitle: libraryCard.modelData.enabled ? "" : I18n.tr("Turned off in settings", "library card status")
                            iconName: "cloud"
                            clickable: true
                            Accessible.checkable: true
                            Accessible.checked: libraryCard.expanded
                            onClicked: root.toggleExpanded(libraryCard.modelData.id)

                            // the badge would sit after the expand arrow
                            StyledText {
                                anchors.verticalCenter: parent.verticalCenter
                                // a turned-off library has no usage entry
                                visible: libraryCard.usage.library !== undefined
                                text: Phrases.join(Phrases.tracks(libraryCard.usage.count ?? 0), Phrases.size(libraryCard.usage.used ?? 0))
                                font.pixelSize: Theme.fontSizeSmall
                                color: Theme.surfaceVariantText
                            }

                            DIcon {
                                anchors.verticalCenter: parent.verticalCenter
                                name: libraryCard.expanded ? "expand_less" : "expand_more"
                                size: Theme.iconSize
                                color: Theme.surfaceVariantText
                            }
                        }

                        Repeater {
                            model: libraryCard.expanded ? SettingsService.cacheRules().filter(r => !r.local && r.library === libraryCard.modelData.id) : []

                            RuleRow {
                                subtitle: root.ruleFacts(modelData)
                            }
                        }

                        SettingsRow {
                            visible: libraryCard.expanded && (libraryCard.usage.excluded ?? 0) > 0
                            title: I18n.tr("Excluded tracks", "offline tracks no rule may download")
                            searchable: false
                            subtitle: Phrases.tracks(libraryCard.usage.excluded ?? 0)
                            iconName: "block"
                            showChevron: true
                            clickable: true
                            onClicked: root.openSubView(root.excludedPrefix + libraryCard.modelData.id, false)
                        }

                        SettingsRow {
                            visible: libraryCard.expanded
                            title: I18n.tr("Downloaded tracks", "library offline overview section")
                            searchable: false
                            iconName: "download_done"
                            showChevron: true
                            clickable: true
                            onClicked: root.openSubView(libraryCard.modelData.id, false)
                        }

                        // a turned-off library takes no new rules
                        SettingsRow {
                            visible: libraryCard.expanded && libraryCard.modelData.enabled
                            title: I18n.tr("Add rule", "offline action that keeps more of a library on this device")
                            searchable: false
                            subtitle: root.libraryKinds(libraryCard.modelData.id).map(k => root.kindLabel(k)).join(" · ")
                            iconName: "add"
                            clickable: true
                            onClicked: root.openAddSheet(libraryCard.modelData.id)
                        }
                    }
                }
            }
        }

        SettingsBackRow {
            visible: root.subPage !== ""
            onClicked: root.openSubView("", false)
        }

        DownloadedTracks {
            width: parent.width
            visible: root.subPage !== ""
            excludedOnly: root.subPage.startsWith(root.excludedPrefix)
            libraryId: excludedOnly ? root.subPage.substring(root.excludedPrefix.length) : root.subPage
            flickable: page
            quiet: root.quiet
        }
    }

    component RuleRow: SettingsRow {
        id: ruleRow
        required property var modelData
        // the kinds whose scope or limit can change
        readonly property bool editable: !modelData.local && ["everything", "favorites", "played", "mostPlayed"].indexOf(modelData.kind) >= 0

        title: root.ruleLabel(modelData)
        searchable: false
        iconName: root.ruleIcon(modelData.kind)
        clickable: editable
        onClicked: ruleDialog.open(modelData, modelData.kind, "")

        DActionButton {
            anchors.verticalCenter: parent.verticalCenter
            visible: ruleRow.editable
            iconName: "edit"
            tooltipText: I18n.tr("Edit", "offline rule action: change its scope or limit")
            Accessible.name: Phrases.named(tooltipText, ruleRow.title)
            onClicked: ruleRow.clicked()
        }

        DActionButton {
            anchors.verticalCenter: parent.verticalCenter
            iconName: "close"
            tooltipText: I18n.tr("Remove", "button that removes an item")
            Accessible.name: Phrases.named(tooltipText, ruleRow.title)
            onClicked: root.removeRule(ruleRow.modelData)
        }
    }

    ConfirmDialog {
        id: removeRuleDialog
        property var run: null
        host: root
        supportingText: I18n.tr("Tracks kept by other rules stay downloaded.", "remove offline rule confirmation")
        confirmText: I18n.tr("Remove", "button that removes an item")
        onConfirmed: run?.()
    }

    PickerSheet {
        id: addSheet
        title: I18n.tr("Add rule", "offline action that keeps more of a library on this device")
        sections: [
            {
                "title": I18n.tr("Keep on this device", "toggle that caches an item's tracks for offline play"),
                "items": root.addKinds.map(k => ({
                            "label": root.kindLabel(k),
                            "icon": root.ruleIcon(k),
                            "value": k
                        }))
            }
        ]
        onPicked: value => root.addRule(value)
    }

    OfflineRuleDialog {
        id: ruleDialog
        host: root
    }

    OfflineItemPicker {
        id: itemPicker
        host: root
    }
}
