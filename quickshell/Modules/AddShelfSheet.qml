import QtQuick
import qs.Common

// Emits a new home shelf instance; the id is left to the layout model.
PinTargetSheet {
    id: root

    required property var shelves

    signal add(var shelf)

    function has(type, id) {
        return shelves.some(s => s.type === type && (s.options.source ?? "") === (id ?? ""));
    }

    function newOptions(type) {
        switch (type) {
        case "search":
        case "genreChips":
            return {};
        case "speedDial":
            return {
                "size": "m"
            };
        }
        return {
            "layout": ["mostPlayed", "playlist", "favoriteTracks", "recentTracks"].indexOf(type) >= 0 ? "list" : "carousel",
            "size": "m"
        };
    }

    title: I18n.tr("Add shelf", "home edit action")
    doneFormat: I18n.tr("%1 (already on home)", "picker item that is already a home shelf")
    enabledLibrariesOnly: true
    isDone: (kind, id) => root.has(kind, id)
    leading: [
        {
            "title": I18n.tr("Suggested", "add shelf section"),
            "items": UiConfig.shelfSourceTypes.map(type => ({
                        "label": UiConfig.shelfSourceLabel(type),
                        "icon": UiConfig.shelfIcon({
                            "type": type
                        }),
                        "value": [type, ""],
                        "done": root.has(type, "")
                    }))
        }
    ]
    onPicked: value => {
        const options = root.newOptions(value[0]);
        if (value[1])
            options.source = value[1];
        root.add({
            "type": value[0],
            "hidden": false,
            "options": options
        });
    }
}
