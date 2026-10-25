import QtQuick
import qs.Common
import qs.Services

// Picks something to pin: the leading sections, then playlists, saved views, genres and libraries.
// Every item's value is [kind, id].
PickerSheet {
    id: root

    property var leading: []
    property bool enabledLibrariesOnly: false
    // (kind, id) => true when that target is already in place
    property var isDone: (kind, id) => false

    function item(kind, id, label) {
        return {
            "label": label,
            "icon": UiConfig.entryIcon({
                "kind": kind,
                "target": id
            }),
            "value": [kind, id],
            "done": root.isDone(kind, id)
        };
    }

    sections: leading.concat([
        {
            "title": I18n.tr("Playlists", "playlists: page, section, library feature"),
            "items": PlayerService.playlists.map(p => root.item("playlist", p.id, p.name))
        },
        {
            "title": I18n.tr("Saved views", "add shelf section"),
            "items": UiConfig.effective.savedViews.map(v => root.item("view", v.id, v.name))
        },
        {
            "title": I18n.tr("Genres", "navigation destination"),
            "items": PlayerService.genres.map(g => root.item("genre", g.id, g.name))
        },
        {
            "title": I18n.tr("Libraries", "the music libraries: page, settings tab, section"),
            "items": PlayerService.libraries.filter(l => l.enabled || !root.enabledLibrariesOnly).map(l => root.item("library", l.id, l.name))
        }
    ])
}
