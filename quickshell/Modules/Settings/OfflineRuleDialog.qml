import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets

// Adds or edits an offline rule: which servers it covers and, for Recently played and Most played, a track count or size limit.
WindowDialog {
    id: root

    // the rule being edited, or null when adding
    property var rule: null
    // "everything" | "favorites" | "played" | "mostPlayed"
    property string kind: "played"
    // "" = all servers
    property string scope: ""
    property bool bySize: false

    readonly property bool adding: rule === null
    readonly property bool limited: kind === "played" || kind === "mostPlayed"
    // the edited rule's own library stays a choice while it is turned off
    readonly property var scopeLibraries: CatalogService.libraries.filter(l => l.id === (rule?.library ?? "") || (l.enabled && CatalogService.isRemote(l.id) && (SettingsService.ruleCapability(kind) === "" || CatalogService.hasCapability(l.id, SettingsService.ruleCapability(kind)))))
    readonly property real gigabyte: 1e9
    readonly property real number: Number(field.text.trim().replace(",", "."))
    // scopes that already hold another rule of this kind; the daemon keeps one per scope
    readonly property var takenScopes: SettingsService.cacheRules().filter(r => !r.local && !r.id && r.kind === kind && (adding || (r.library ?? "") !== (rule.library ?? ""))).map(r => r.library ?? "")
    readonly property string scopeBlocker: {
        if (takenScopes.indexOf(scope) < 0)
            return "";
        return scope === "" ? I18n.tr("This rule already exists.", "offline rule dialog, why save is disabled") : I18n.tr("%1 already has this rule.", "offline rule dialog, why save is disabled, %1 is a server library").arg(CatalogService.libraryName(scope));
    }
    readonly property string limitBlocker: {
        if (!limited)
            return "";
        if (bySize)
            return number >= PlayerMetrics.offlineRuleMinGB ? "" : I18n.tr("Enter at least %1 GB.", "offline rule limit dialog, why save is disabled").arg(PlayerMetrics.offlineRuleMinGB);
        return Number.isInteger(number) && number >= 1 && number <= PlayerMetrics.offlineRuleMaxTracks ? "" : I18n.tr("Enter 1 to %1 tracks.", "offline rule limit dialog, why save is disabled").arg(PlayerMetrics.offlineRuleMaxTracks);
    }
    readonly property string blocker: scopeBlocker || limitBlocker

    function kindTitle() {
        switch (kind) {
        case "played":
            return I18n.tr("Recently played limit", "offline rule limit dialog title");
        case "mostPlayed":
            return I18n.tr("Most played limit", "offline rule limit dialog title");
        case "favorites":
            return I18n.tr("Favorites", "favorite tracks: page, cache mode, library feature");
        }
        return I18n.tr("Everything", "offline rule: the whole library");
    }

    // rule: an existing rule to edit, or null to add one of `kind` to `scope`
    function open(rule, kind, scope) {
        root.rule = rule;
        root.kind = rule ? rule.kind : kind;
        root.scope = rule ? (rule.library ?? "") : scope;
        const limit = rule ? rule.limit : (root.kind === "played" ? {
                "bytes": PlayerMetrics.offlinePlayedDefaultBytes
            } : null);
        bySize = (limit?.bytes ?? 0) > 0;
        field.text = bySize ? String(Math.round(limit.bytes / gigabyte * 100) / 100) : String(limit?.songs ?? PlayerMetrics.offlineRuleDefaultTracks);
        opened = true;
        if (limited)
            Qt.callLater(field.forceActiveFocus);
        else if (!adding)
            Qt.callLater(scopeDropdown.forceActiveFocus);
    }

    function save() {
        if (blocker !== "")
            return;
        const limit = bySize ? {
            "bytes": Math.round(number * gigabyte)
        } : {
            "songs": number
        };
        if (adding) {
            SettingsService.addCacheRule(scope, kind, "", limited ? limit : undefined);
        } else {
            const changes = {};
            if (limited && ((rule.limit?.bytes ?? 0) !== (limit.bytes ?? 0) || (rule.limit?.songs ?? 0) !== (limit.songs ?? 0)))
                changes.limit = limit;
            if (scope !== (rule.library ?? ""))
                changes.scope = scope;
            if (Object.keys(changes).length > 0)
                SettingsService.updateCacheRule(rule, changes);
        }
        opened = false;
    }

    title: kindTitle()
    supportingText: kind === "played" ? I18n.tr("Keeps the most recently played tracks up to this limit.", "offline rule limit dialog text") : kind === "mostPlayed" ? I18n.tr("Keeps the most played tracks up to this limit.", "offline rule limit dialog text") : ""
    acceptEnabled: blocker === ""
    onAccepted: save()
    onRejected: opened = false

    // adding from a card, the card is the scope
    DDropdown {
        id: scopeDropdown
        visible: !root.adding
        width: parent.width
        dropdownWidth: SettingsMetrics.dropdownWidth
        text: I18n.tr("Scope", "offline rule dialog: which servers a rule covers")
        // the "" value shows emptyText, not its option label
        emptyText: I18n.tr("All servers", "offline rule scope: every server library")
        description: root.scopeBlocker
        options: [I18n.tr("All servers", "offline rule scope: every server library")].concat(root.scopeLibraries.map(l => l.name))
        optionValues: [""].concat(root.scopeLibraries.map(l => l.id))
        onValueChanged: value => root.scope = value
    }

    Binding {
        target: scopeDropdown
        property: "currentValue"
        value: root.scope
    }

    StyledText {
        width: parent.width
        visible: root.adding && root.scopeBlocker !== ""
        text: root.scopeBlocker
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.error
        wrapMode: Text.WordWrap
    }

    DButtonGroup {
        visible: root.limited
        model: [
            {
                "text": I18n.tr("Tracks", "navigation destination"),
                "icon": "music_note"
            },
            {
                "text": I18n.tr("Size", "details field: file size"),
                "icon": "hard_drive"
            }
        ]
        currentIndex: root.bySize ? 1 : 0
        onSelectionChanged: (index, selected) => {
            if (selected && root.bySize !== (index === 1)) {
                root.bySize = index === 1;
                field.text = root.bySize ? "1" : String(PlayerMetrics.offlineRuleDefaultTracks);
            }
        }
    }

    DTextField {
        id: field
        visible: root.limited
        width: parent.width
        outlined: true
        labelText: root.bySize ? I18n.tr("Size in GB", "offline rule limit field") : I18n.tr("Number of tracks", "offline rule limit field")
        isError: root.limitBlocker !== ""
        supportingText: root.limitBlocker
        onAccepted: root.save()
    }

    actions: [
        DButton {
            text: I18n.tr("Cancel", "dialog dismiss button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: root.rejected()
        },
        DButton {
            text: root.adding ? I18n.tr("Add", "dialog confirm button") : I18n.tr("Save", "dialog confirm button")
            enabled: root.blocker === ""
            onClicked: root.save()
        }
    ]
}
