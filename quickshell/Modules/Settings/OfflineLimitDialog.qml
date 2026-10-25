import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets

// The limit of a Played or Most played offline rule: a track count or a size. Adds the rule or changes it.
WindowDialog {
    id: root

    property string libraryId: ""
    // "played" | "mostPlayed"
    property string kind: "played"
    property bool adding: false
    property bool bySize: false

    readonly property real gigabyte: 1e9
    readonly property real number: Number(field.text.trim().replace(",", "."))
    readonly property string blocker: {
        if (bySize)
            return number >= PlayerMetrics.offlineRuleMinGB ? "" : I18n.tr("Enter at least %1 GB.", "offline rule limit dialog, why save is disabled").arg(PlayerMetrics.offlineRuleMinGB);
        return Number.isInteger(number) && number >= 1 && number <= PlayerMetrics.offlineRuleMaxTracks ? "" : I18n.tr("Enter 1 to %1 tracks.", "offline rule limit dialog, why save is disabled").arg(PlayerMetrics.offlineRuleMaxTracks);
    }

    // limit: {songs} or {bytes}
    function open(libraryId, kind, limit, adding) {
        root.libraryId = libraryId;
        root.kind = kind;
        root.adding = adding;
        bySize = (limit?.bytes ?? 0) > 0;
        field.text = bySize ? String(Math.round(limit.bytes / gigabyte * 100) / 100) : String(limit?.songs ?? PlayerMetrics.offlineRuleDefaultTracks);
        opened = true;
        Qt.callLater(field.forceActiveFocus);
    }

    function save() {
        if (blocker !== "")
            return;
        const limit = bySize ? {
            "bytes": Math.round(number * gigabyte)
        } : {
            "songs": number
        };
        if (adding)
            SettingsService.addCacheRule(libraryId, kind, "", limit);
        else
            SettingsService.updateCacheRule(libraryId, kind, limit);
        opened = false;
    }

    title: kind === "played" ? I18n.tr("Recently played limit", "offline rule limit dialog title") : I18n.tr("Most played limit", "offline rule limit dialog title")
    supportingText: kind === "played" ? I18n.tr("Keeps the most recently played tracks up to this limit.", "offline rule limit dialog text") : I18n.tr("Keeps the most played tracks up to this limit.", "offline rule limit dialog text")
    acceptEnabled: blocker === ""
    onAccepted: save()
    onRejected: opened = false

    DButtonGroup {
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
        width: parent.width
        outlined: true
        labelText: root.bySize ? I18n.tr("Size in GB", "offline rule limit field") : I18n.tr("Number of tracks", "offline rule limit field")
        isError: root.blocker !== ""
        supportingText: root.blocker
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
