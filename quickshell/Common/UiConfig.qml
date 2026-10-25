pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import qs.Services
import "Format.js" as Format
import "Layouts.js" as Layouts
import "../Services/Catalog.js" as Catalog

// Effective customization and the labels and choices the editors offer. Other shapes (plain JSON in
// ui-settings.json):
//   undo toast    {mode: "undo"|"confirm"|"none" (removals), seconds: int}
//   lyrics        {textSize: "s"|"m"|"l", alignment: "center"|"start", wordSync: bool, backgroundVocals: bool}
Singleton {
    id: root

    // a page section: the playlists' locations, or a media type
    function sectionLabel(type) {
        switch (type) {
        case "local":
            return I18n.tr("On this device", "playlists section for local playlists");
        case "server":
            return I18n.tr("On servers", "playlists section for the playlists of every server");
        }
        return typeLabel(type);
    }

    function sectionIcon(type) {
        switch (type) {
        case "local":
            return "folder";
        case "server":
            return "cloud";
        }
        return pageIcon(type);
    }

    function typeLabel(type) {
        switch (type) {
        case "albums":
            return I18n.tr("Albums", "navigation destination");
        case "artists":
            return I18n.tr("Artists", "navigation destination");
        case "tracks":
            return I18n.tr("Tracks", "navigation destination");
        case "playlists":
            return I18n.tr("Playlists", "playlists: page, section, library feature");
        case "genres":
            return I18n.tr("Genres", "navigation destination");
        }
        return type;
    }

    function layoutChoices(type) {
        return [
            {
                "value": "list",
                "text": I18n.tr("List", "view layout option"),
                "icon": "view_list"
            },
            {
                "value": "grid",
                "text": I18n.tr("Grid", "view layout option"),
                "icon": "grid_view"
            },
            {
                "value": "compact",
                "text": I18n.tr("Compact", "view layout option"),
                "icon": "view_headline"
            }
        ];
    }

    function lineLabel(line) {
        switch (line) {
        case "artist":
            return I18n.tr("Artist", "track list column");
        case "album":
            return I18n.tr("Album", "an album: page label, list column, ReplayGain mode");
        case "year":
            return I18n.tr("Year", "view info line");
        case "genre":
            return I18n.tr("Genre", "view info line");
        case "trackCount":
            return I18n.tr("Track count", "view info line");
        case "albumCount":
            return I18n.tr("Album count", "view info line");
        case "duration":
            return I18n.tr("Duration", "view info line");
        case "library":
            return I18n.tr("Library", "track list column");
        case "libraries":
            return I18n.tr("Libraries", "the music libraries: page, settings tab, section");
        case "rating":
            return I18n.tr("Rating", "view info line");
        case "playCount":
            return I18n.tr("Plays", "view info line and sort key");
        case "favorite":
            return I18n.tr("Favorite", "view info line");
        }
        return line;
    }

    function lineChoices(type) {
        switch (type) {
        case "albums":
            return ["artist", "year", "genre", "trackCount", "duration", "library", "rating", "playCount"];
        case "artists":
            return ["albumCount", "trackCount", "library"];
        case "tracks":
            return ["artist", "album", "year", "genre", "duration", "library", "rating", "playCount"];
        case "playlists":
            return ["trackCount", "duration", "libraries"];
        case "genres":
            return ["albumCount", "trackCount"];
        }
        return [];
    }

    function kindOf(type) {
        return {
            "albums": "album",
            "artists": "artist",
            "tracks": "track",
            "playlists": "playlist",
            "genres": "genre"
        }[type] ?? type;
    }

    function stars(rating) {
        return "\u2605".repeat(rating) + "\u2606".repeat(5 - rating);
    }

    // The secondary text of a browse item, from the info lines a view shows.
    function infoText(item, lines) {
        const parts = [];
        for (const line of lines) {
            switch (line) {
            case "artist":
            case "album":
            case "genre":
                if (item[line])
                    parts.push(item[line]);
                break;
            case "year":
                if (item.year)
                    parts.push(String(item.year));
                break;
            case "trackCount":
                parts.push(Phrases.tracks(item.trackCount ?? 0));
                break;
            case "albumCount":
                parts.push(Phrases.albums(item.albumCount ?? 0));
                break;
            case "duration":
                parts.push(Format.duration(item.duration ?? 0));
                break;
            case "library":
                parts.push(PlayerService.libraryName(item.library));
                break;
            case "libraries":
                if ((item.libraries ?? []).length > 1)
                    parts.push(I18n.tr("from %1 libraries", "playlist info line").arg(item.libraries.length));
                break;
            case "rating":
                if (item.rating > 0)
                    parts.push(stars(item.rating));
                break;
            case "playCount":
                parts.push(Phrases.plays(item.playCount ?? 0));
                break;
            }
        }
        return parts.join(" \u00b7 ");
    }

    // Track list columns for the wide layout, shared by the rows and the sortable header.
    // fill: share of the leftover width; width: fixed width instead.
    function trackColumns(lines) {
        const specs = {
            "artist": {
                "fill": 2
            },
            "album": {
                "fill": 2
            },
            "genre": {
                "width": PlayerMetrics.columnGenreWidth
            },
            "year": {
                "width": PlayerMetrics.columnYearWidth
            },
            "library": {
                "width": PlayerMetrics.columnLibraryWidth
            },
            "rating": {
                "width": PlayerMetrics.columnRatingWidth
            },
            "playCount": {
                "width": PlayerMetrics.columnYearWidth
            }
        };
        return lines.filter(line => specs[line] !== undefined).map(line => Object.assign({
                "line": line
            }, specs[line]));
    }

    function sortLabel(key) {
        switch (key) {
        case "title":
            return I18n.tr("Title", "track list column");
        case "name":
            return I18n.tr("Name", "a name: field label, sort key");
        case "artist":
            return I18n.tr("Artist", "track list column");
        case "album":
            return I18n.tr("Album", "an album: page label, list column, ReplayGain mode");
        case "year":
            return I18n.tr("Year", "view info line");
        case "added":
            return I18n.tr("Date added", "sort key");
        case "duration":
            return I18n.tr("Time", "track list column");
        case "library":
            return I18n.tr("Library", "track list column");
        case "albumCount":
            return I18n.tr("Album count", "view info line");
        case "trackCount":
            return I18n.tr("Track count", "view info line");
        case "rating":
            return I18n.tr("Rating", "view info line");
        case "playCount":
            return I18n.tr("Plays", "view info line and sort key");
        case "lastPlayed":
            return I18n.tr("Last played", "sort key");
        case "position":
            return I18n.tr("Playlist order", "sort key: the order a playlist keeps");
        }
        return key;
    }

    function sortChoices(type) {
        switch (type) {
        case "albums":
            return ["title", "artist", "year", "added", "playCount", "rating", "lastPlayed"];
        case "artists":
            return ["name", "albumCount", "trackCount"];
        case "tracks":
            return ["title", "artist", "album", "year", "duration", "library", "playCount", "rating", "lastPlayed"];
        case "playlists":
            return ["name", "trackCount", "duration"];
        case "genres":
            return ["name", "albumCount", "trackCount"];
        }
        return [];
    }

    function filterKinds(type) {
        switch (type) {
        case "albums":
        case "tracks":
            return ["library", "genre", "favorites"];
        case "artists":
            return ["library", "favorites"];
        }
        return ["library"];
    }

    // Customization: the stored `layout` (SettingsData.layout) with the defaults applied. Shapes and rules are
    // in Layouts.js; this adds the labels and icons.
    readonly property var effective: _stable(Layouts.effective(SettingsData.layout))
    // the last effective layout, held outside the property system so the binding above doesn't depend on it
    readonly property var _last: ({})

    // a write rebuilds only the views of what it changed
    function _stable(next) {
        _last.value = Layouts.keepUnchanged(_last.value, next);
        return _last.value;
    }

    // List options: a default per page kind, and a change per page (per item for kinds with many pages)

    readonly property var pageKinds: ["tracks", "albums", "artists", "playlists", "genres", "favorites", "mostPlayed", "history", "playlist", "album", "artist", "genre", "library", "search", "queue"].map(kind => ({
                "kind": kind,
                "label": pageKindLabel(kind),
                "icon": pageKindIcon(kind),
                "multi": ["playlist", "album", "artist", "genre", "library"].indexOf(kind) >= 0,
                "route": kind === "queue" || ["playlist", "album", "artist", "genre", "library"].indexOf(kind) >= 0 ? "" : kind
            }))

    function pageKindLabel(kind) {
        switch (kind) {
        case "playlist":
            return I18n.tr("Each playlist", "settings: pages of a kind with many instances");
        case "album":
            return I18n.tr("Each album page", "settings: pages of a kind with many instances");
        case "artist":
            return I18n.tr("Each artist page", "settings: pages of a kind with many instances");
        case "genre":
            return I18n.tr("Each genre page", "settings: pages of a kind with many instances");
        case "library":
            return I18n.tr("Each library page", "settings: pages of a kind with many instances");
        case "search":
            return I18n.tr("Search results", "playback context label");
        case "queue":
            return I18n.tr("Queue", "the play queue: panel, toggle");
        }
        return pageLabel(kind);
    }

    function pageKindIcon(kind) {
        switch (kind) {
        case "playlist":
            return "queue_music";
        case "album":
            return "album";
        case "artist":
            return "artist";
        case "genre":
            return "category";
        case "library":
            return "library_music";
        case "search":
            return "search";
        case "queue":
            return "queue_music";
        }
        return pageIcon(kind);
    }

    // itemId: the playlist, album, artist, genre or library for kinds with many pages; "" for the kind default
    function pageOptions(kind, itemId) {
        return Layouts.pageOptions(effective, kind, itemId ?? "");
    }

    function setPageOptions(kind, itemId, options) {
        SettingsData.layout = Layouts.setPageOptions(SettingsData.layout, kind, itemId ?? "", options);
    }

    function resetPageOptions(kind, itemId) {
        SettingsData.layout = Layouts.resetPageOptions(SettingsData.layout, kind, itemId ?? "");
    }

    function kindDefault(kind) {
        return Layouts.kindDefault(effective, kind);
    }

    function setKindDefault(kind, options) {
        SettingsData.layout = Layouts.setKindDefault(SettingsData.layout, kind, options);
    }

    // the kind's default only; each page keeps its own change
    function resetKindDefault(kind) {
        SettingsData.layout = Layouts.resetKindDefault(SettingsData.layout, kind);
    }

    // the kind's default and every page's change
    function resetKind(kind) {
        SettingsData.layout = Layouts.resetKind(SettingsData.layout, kind);
    }

    function changedPages(kind) {
        return Layouts.changedPages(effective, kind);
    }

    // Saved media views

    function savedView(id) {
        return effective.savedViews.find(v => v.id === id) ?? null;
    }

    function setSavedViews(list) {
        SettingsData.layout = Layouts.setList(SettingsData.layout, "savedViews", list);
    }

    function saveView(name, type, options) {
        const id = "view-" + Date.now();
        setSavedViews(effective.savedViews.concat([
            {
                "id": id,
                "name": name,
                "type": type,
                "options": options
            }
        ]));
        return id;
    }

    function updateSavedView(id, patch) {
        setSavedViews(effective.savedViews.map(v => v.id === id ? Object.assign({}, v, patch) : v));
    }

    function deleteSavedView(id) {
        setSavedViews(effective.savedViews.filter(v => v.id !== id));
        setNavigation(navigation.filter(e => !(e.kind === "view" && e.target === id)));
        const shelves = effective.surfaces.home.zones.shelves;
        const kept = shelves.filter(s => !(s.type === "view" && s.options.source === id));
        if (kept.length !== shelves.length)
            setSurface("home", {
                "zones": {
                    "shelves": kept
                }
            });
    }

    // Home

    readonly property var shelfSourceTypes: ["search", "continue", "favoriteTracks", "playlists", "recentTracks", "mostPlayed", "recentlyPlayed", "recentlyAdded", "randomAlbums", "favoriteAlbums"]

    function shelfSourceLabel(type) {
        switch (type) {
        case "search":
            return I18n.tr("Search field", "home shelf: the large search field");
        case "continue":
            return I18n.tr("Continue listening", "home shelf with the last queue");
        case "favoriteTracks":
            return I18n.tr("Favorites", "favorite tracks: page, cache mode, library feature");
        case "recentTracks":
            return I18n.tr("Recently played", "home shelf of recently played tracks");
        case "recentlyPlayed":
            return I18n.tr("Recently played albums", "home shelf of recently played albums");
        case "recentlyAdded":
            return I18n.tr("Recently added", "home shelf of newly indexed albums");
        case "mostPlayed":
            return I18n.tr("Most played", "home shelf and page of the most played tracks");
        case "randomAlbums":
            return I18n.tr("Random albums", "home shelf");
        case "favoriteAlbums":
            return I18n.tr("Favorite albums", "home shelf");
        case "playlists":
            return I18n.tr("Playlists", "playlists: page, section, library feature");
        case "playlist":
            return I18n.tr("Playlist", "home shelf source kind");
        case "genre":
            return I18n.tr("Genre", "view info line");
        case "library":
            return I18n.tr("Library", "track list column");
        case "view":
            return I18n.tr("Saved view", "a saved view: page label, playback context");
        case "artist":
            return I18n.tr("Artist", "track list column");
        }
        return type;
    }

    // a home shelf instance's title: the item's name for shelves of one playlist, genre, library or view
    function shelfTitle(shelf) {
        const id = shelf.options?.source ?? "";
        switch (shelf.type) {
        case "playlist":
            return PlayerService.playlist(id)?.name ?? shelfSourceLabel("playlist");
        case "genre":
            return PlayerService.genre(id)?.name ?? shelfSourceLabel("genre");
        case "library":
            return PlayerService.libraryName(id) || shelfSourceLabel("library");
        case "view":
            return savedView(id)?.name ?? shelfSourceLabel("view");
        }
        return shelfSourceLabel(shelf.type);
    }

    function shelfIcon(shelf) {
        switch (shelf.type) {
        case "search":
            return "search";
        case "continue":
            return "resume";
        case "favoriteTracks":
        case "favoriteAlbums":
            return "favorite";
        case "recentTracks":
        case "recentlyPlayed":
            return "history";
        case "recentlyAdded":
            return "new_releases";
        case "mostPlayed":
            return "trending_up";
        case "randomAlbums":
            return "shuffle";
        case "playlists":
            return "queue_music";
        }
        return entryIcon({
            "kind": shelf.type,
            "target": shelf.options?.source ?? ""
        });
    }

    // "Playlist · Road trip": where the current queue came from

    function contextLabel(context) {
        const id = context?.id ?? "";
        switch (context?.type) {
        case "album":
            return Phrases.join(I18n.tr("Album", "an album: page label, list column, ReplayGain mode"), PlayerService.album(id)?.title ?? context.name ?? "");
        case "playlist":
            return Phrases.join(I18n.tr("Playlist", "home shelf source kind"), PlayerService.playlist(id)?.name ?? context.name ?? "");
        case "artist":
            return Phrases.join(I18n.tr("Artist", "track list column"), PlayerService.artist(id)?.name ?? context.name ?? "");
        case "genre":
            return Phrases.join(I18n.tr("Genre", "view info line"), PlayerService.genre(id)?.name ?? context.name ?? "");
        case "library":
            return Phrases.join(I18n.tr("Library", "track list column"), PlayerService.libraryName(id));
        case "view":
            return Phrases.join(I18n.tr("Saved view", "a saved view: page label, playback context"), savedView(id)?.name ?? "");
        case "favorites":
            return pageLabel("favorites");
        case "history":
            return pageLabel("history");
        case "search":
            return I18n.tr("Search results", "playback context label");
        }
        return typeLabel("tracks");
    }

    // Navigation

    readonly property var pageTargets: ["home", "playlists", "favorites", "tracks", "albums", "artists", "genres", "libraries", "history", "mostPlayed"]
    // every entry, turned off ones included, in the order set in Settings
    readonly property var navigation: Catalog.navEntries(effective.navigation, pageTargets, ["home", "playlists", "favorites", "tracks", "albums", "artists", "libraries"])
    readonly property var shownNavigation: navigation.filter(e => !e.hidden && entryValid(e))

    function pageLabel(page) {
        switch (page) {
        case "home":
            return I18n.tr("Home", "navigation destination");
        case "libraries":
            return I18n.tr("Libraries", "the music libraries: page, settings tab, section");
        case "history":
            return I18n.tr("History", "navigation destination");
        case "favorites":
            return I18n.tr("Favorites", "favorite tracks: page, cache mode, library feature");
        case "mostPlayed":
            return I18n.tr("Most played", "home shelf and page of the most played tracks");
        }
        return typeLabel(page);
    }

    function pageIcon(page) {
        switch (page) {
        case "home":
            return "home";
        case "albums":
            return "album";
        case "artists":
            return "artist";
        case "tracks":
            return "music_note";
        case "playlists":
            return "queue_music";
        case "genres":
            return "category";
        case "libraries":
            return "library_music";
        case "history":
            return "history";
        case "favorites":
            return "favorite";
        case "mostPlayed":
            return "trending_up";
        }
        return "circle";
    }

    function defaultEntryLabel(entry) {
        switch (entry.kind) {
        case "page":
            return pageLabel(entry.target);
        case "playlist":
            return PlayerService.playlist(entry.target)?.name ?? "";
        case "genre":
            return PlayerService.genre(entry.target)?.name ?? "";
        case "library":
            return PlayerService.libraryName(entry.target);
        case "view":
            return savedView(entry.target)?.name ?? "";
        case "artist":
            return PlayerService.artist(entry.target)?.name ?? entry.name ?? "";
        }
        return "";
    }

    function entryLabel(entry) {
        return entry.label || defaultEntryLabel(entry);
    }

    // a server playlist shows a cloud, a local one the playlist icon
    function playlistIcon(playlist) {
        return playlist?.kind === "server" ? "cloud" : "queue_music";
    }

    function entryIcon(entry) {
        switch (entry.kind) {
        case "page":
            return pageIcon(entry.target);
        case "playlist":
            return playlistIcon(PlayerService.playlist(entry.target));
        case "genre":
            return "category";
        case "library":
            return PlayerService.isRemote(entry.target) ? "cloud" : "folder";
        case "view":
            return "bookmark";
        case "artist":
            return "artist";
        }
        return "circle";
    }

    // the item an entry points at may have been deleted
    function entryValid(entry) {
        return entry.kind === "page" || defaultEntryLabel(entry) !== "";
    }

    function entryRoute(entry) {
        if (entry.kind === "page")
            return {
                "page": entry.target,
                "itemId": ""
            };
        return {
            "page": entry.kind,
            "itemId": entry.target
        };
    }

    function setNavigation(list) {
        SettingsData.layout = Layouts.setList(SettingsData.layout, "navigation", list);
    }

    function isPinned(kind, target) {
        return navigation.some(e => e.kind === kind && e.target === target && !e.hidden);
    }

    function pin(kind, target, name) {
        setNavigation(Catalog.setPinned(navigation, kind, target, true, name));
    }

    function unpin(kind, target) {
        setNavigation(Catalog.setPinned(navigation, kind, target, false));
    }

    // drops a pinned item's entry for good (a deleted playlist, a removed entry)
    function removeEntry(kind, target) {
        setNavigation(navigation.filter(e => !(e.kind === kind && e.target === target)));
    }

    function renameEntry(key, label) {
        setNavigation(navigation.map(e => e.key === key ? Object.assign({}, e, {
                "label": label
            }) : e));
    }

    // Now playing: surfaces (bar, side, fullBento, fullM3e, home) of zones of element instances, and the media
    // types' row and menu actions (track, album, artist, playlist, genre, queue)

    // {start: "bar"|"side"|"full", style: "bento"|"m3e", openPane: "last"|"lyrics"|"queue"|"cover"}
    readonly property var nowPlaying: effective.nowPlaying
    readonly property var playerStyles: ["bento", "m3e"]
    // the pane the full view opens with: "lyrics" | "queue" | "" (the cover alone)
    readonly property string startPane: {
        const pane = nowPlaying.openPane === "last" ? SettingsData.lastPane : nowPlaying.openPane;
        return pane === "lyrics" || pane === "queue" ? pane : "";
    }
    // the side panel's tabs: "cover" | "lyrics" | "queue", in order
    readonly property var sidePanes: effective.sidePanes
    // the side panel's zones taken from the bar while it follows the bar
    readonly property var sideFollowedZones: Layouts.sideFollowedZones()

    function setNowPlaying(patch) {
        SettingsData.layout = Layouts.setNowPlaying(SettingsData.layout, patch);
    }

    function surface(id) {
        return effective.surfaces[id] ?? null;
    }

    function setSurface(id, value) {
        SettingsData.layout = Layouts.setSurface(SettingsData.layout, id, value);
    }

    function resetSurface(id) {
        SettingsData.layout = Layouts.resetSurface(SettingsData.layout, id);
    }

    function item(type) {
        return effective.items[type] ?? null;
    }

    // patch: {zones?: {zoneId: [instance]}, contextSameAsMenu?}
    function setItem(type, patch) {
        SettingsData.layout = Layouts.setItem(SettingsData.layout, type, patch);
    }

    function resetItem(type) {
        SettingsData.layout = Layouts.resetItem(SettingsData.layout, type);
    }

    // id: a surface or a media type
    function zonesOf(id) {
        return Layouts.zonesOf(id).map(z => ({
                    "id": z.id,
                    "label": zoneText(id, z.id),
                    "row": z.kind !== "column",
                    "overflows": z.overflows,
                    "kind": z.kind
                }));
    }

    // what the picker offers for a zone; options: {key: {values: [..] | null (short text), default?}}
    function typesFor(id, zone) {
        const forItem = Layouts.isItemType(id);
        return Layouts.typesFor(id, zone).map(type => ({
                    "type": type,
                    "label": forItem ? actionLabel(id, type) : id === "home" ? shelfSourceLabel(type) : elementLabel(type),
                    "icon": forItem ? actionIcon(id, type) : elementIcon(type),
                    "group": forItem ? "" : Layouts.groupOf(type),
                    "options": Layouts.optionSchema(id, type)
                }));
    }

    // whether an element of type can move into zone, which holds list
    function accepts(id, zone, list, type) {
        return Layouts.accepts(id, zone, list, type);
    }

    // false for a type a newer version stored: kept, listed greyed, never rendered
    function isKnown(type) {
        return Layouts.isKnownType(type);
    }

    function problems(id) {
        return Layouts.problems(effective, id).map(code => {
            switch (code) {
            case "noPlayPause":
                return I18n.tr("No play/pause button. Media keys and the CLI still work.", "customization warning");
            case "noFullView":
                return I18n.tr("No cover, title or open button. Clicking the bar still opens the full view.", "customization warning");
            case "noMenuButton":
                return I18n.tr("No ⋮ button. Right-click and the Menu key still open the menu.", "customization warning");
            }
            return code;
        });
    }

    // all customization back to the defaults; saved views are kept
    function resetAll() {
        SettingsData.layout = Layouts.resetAll(SettingsData.layout);
    }

    function exportLayout() {
        return JSON.stringify(Layouts.sanitize(SettingsData.layout), null, 2);
    }

    // "" when imported, else why not
    function importLayout(text) {
        const result = Layouts.parseImport(text);
        switch (result.problem) {
        case "":
            // a file without navigation or saved views keeps the local ones
            for (const key of ["navigation", "savedViews"]) {
                if (result.layout[key] === undefined && SettingsData.layout?.[key] !== undefined)
                    result.layout[key] = SettingsData.layout[key];
            }
            SettingsData.layout = result.layout;
            return "";
        case "tooLarge":
            return I18n.tr("The file is too large", "why a layout file can't be imported");
        }
        return I18n.tr("Not a layout file", "why a layout file can't be imported");
    }

    function zoneText(id, zone) {
        switch (zone) {
        case "left":
            return I18n.tr("Left", "now playing zone");
        case "right":
            return I18n.tr("Right", "now playing zone");
        case "controls":
            return id === "bar" ? I18n.tr("Above the seekbar", "now playing zone") : I18n.tr("Transport row", "now playing zone");
        case "seek":
            return I18n.tr("Seekbar row", "now playing zone");
        case "top":
            return I18n.tr("Top row", "now playing zone");
        case "bottom":
            return I18n.tr("Bottom row", "now playing zone");
        case "panes":
            return I18n.tr("Panes", "now playing zone: the side panel's tabs");
        case "title":
            return I18n.tr("Title card", "now playing zone");
        case "cover":
            return I18n.tr("Cover", "now playing element");
        case "header":
            return id === "fullM3e" ? I18n.tr("Header start", "now playing zone") : I18n.tr("Header", "now playing zone");
        case "buttons":
            return I18n.tr("Button group", "now playing zone");
        case "end":
            return I18n.tr("Far end", "now playing zone");
        case "shelves":
            return I18n.tr("Shelves", "home zone");
        case "trailing":
            return I18n.tr("End of row", "row actions zone");
        case "hover":
            return I18n.tr("On hover", "row actions zone");
        case "menu":
            return I18n.tr("⋮ menu", "row actions zone");
        case "context":
            return I18n.tr("Right-click menu", "row actions zone");
        }
        return zone;
    }

    // an element option's value: sizes and shelf layouts
    function optionLabel(value) {
        switch (value) {
        case "s":
            return I18n.tr("Small", "size option");
        case "m":
            return I18n.tr("Medium", "size option");
        case "l":
            return I18n.tr("Large", "size option");
        case "fill":
            return I18n.tr("Fill", "spacer size option");
        case "carousel":
            return I18n.tr("Carousel", "home shelf layout option");
        case "grid":
            return I18n.tr("Grid", "view layout option");
        case "list":
            return I18n.tr("List", "view layout option");
        }
        return value;
    }

    function elementLabel(type) {
        switch (type) {
        case "cover":
            return I18n.tr("Cover", "now playing element");
        case "title":
            return I18n.tr("Title", "track list column");
        case "artists":
            return I18n.tr("Artists", "navigation destination");
        case "album":
            return I18n.tr("Album", "an album: page label, list column, ReplayGain mode");
        case "libraryBadge":
            return I18n.tr("Library badge", "now playing element");
        case "playingFrom":
            return I18n.tr("Playing from", "now playing: label before where the queue came from");
        case "castTarget":
            return I18n.tr("Playing on", "now playing element: the cast device while casting");
        case "expand":
            return I18n.tr("Open now playing", "now playing bar button");
        case "dock":
            return I18n.tr("Move to sidebar", "button moving now playing from the bottom bar into the sidebar");
        case "favorite":
            return I18n.tr("Favorite", "view info line");
        case "rating":
            return I18n.tr("Rating", "view info line");
        case "shuffle":
            return I18n.tr("Shuffle", "button or mode that plays in random order");
        case "previous":
            return I18n.tr("Previous", "transport button");
        case "playPause":
            return I18n.tr("Play/pause", "now playing element");
        case "next":
            return I18n.tr("Next", "transport button");
        case "repeat":
            return I18n.tr("Repeat", "now playing element");
        case "elapsed":
            return I18n.tr("Elapsed time", "now playing element");
        case "seekbar":
            return I18n.tr("Seekbar", "bar setting");
        case "duration":
            return I18n.tr("Duration", "now playing element");
        case "volume":
            return I18n.tr("Volume", "audio volume: settings section, slider");
        case "queue":
            return I18n.tr("Queue", "the play queue: panel, toggle");
        case "lyrics":
            return I18n.tr("Lyrics", "song lyrics: page, toggle, settings tab");
        case "device":
            return I18n.tr("Playback device", "playback device: settings tab, button, dialog");
        case "sleep":
            return I18n.tr("Sleep timer", "now playing button");
        case "speed":
            return I18n.tr("Playback speed", "now playing button");
        case "addToPlaylist":
            return I18n.tr("Add to playlist…", "track menu action");
        case "goToAlbum":
            return I18n.tr("Go to album", "track menu action");
        case "goToArtist":
            return I18n.tr("Go to artist", "track menu action");
        case "details":
            return I18n.tr("Details", "menu action that shows everything known about a track or album");
        case "spacer":
            return I18n.tr("Spacer", "layout element: empty space");
        case "divider":
            return I18n.tr("Divider", "layout element: a separating line");
        }
        return type;
    }

    function elementIcon(type) {
        switch (type) {
        case "cover":
            return "image";
        case "title":
            return "title";
        case "artists":
            return "person";
        case "album":
            return "album";
        case "libraryBadge":
            return "library_music";
        case "playingFrom":
            return "queue_music";
        case "castTarget":
            return "cast";
        case "expand":
            return "open_in_full";
        case "dock":
            return "dock_to_left";
        case "favorite":
            return "favorite";
        case "rating":
            return "star";
        case "shuffle":
            return "shuffle";
        case "previous":
            return "skip_previous";
        case "playPause":
            return "play_pause";
        case "next":
            return "skip_next";
        case "repeat":
            return "repeat";
        case "elapsed":
            return "timer";
        case "seekbar":
            return "linear_scale";
        case "duration":
            return "schedule";
        case "volume":
            return "volume_up";
        case "queue":
            return "queue_music";
        case "lyrics":
            return "lyrics";
        case "device":
            return "speaker";
        case "sleep":
            return "bedtime";
        case "speed":
            return "speed";
        case "addToPlaylist":
            return "playlist_add";
        case "goToAlbum":
            return "album";
        case "goToArtist":
            return "artist";
        case "details":
            return "info";
        case "spacer":
            return "space_bar";
        case "divider":
            return "horizontal_rule";
        }
        return "circle";
    }

    // type: the media type, for the actions whose words differ by type
    function actionLabel(type, id) {
        switch (id) {
        case "play":
            return I18n.tr("Play", "button that starts playback");
        case "shuffle":
            return I18n.tr("Shuffle", "button or mode that plays in random order");
        case "playNext":
            return I18n.tr("Play next", "track row action");
        case "playAfter":
            return I18n.tr("Play after this one", "queue row action");
        case "addToQueue":
            return I18n.tr("Add to queue", "track row action");
        case "addToPlaylist":
            return I18n.tr("Add to playlist…", "track menu action");
        case "favorite":
            return I18n.tr("Favorite", "view info line");
        case "rating":
            return I18n.tr("Rating", "view info line");
        case "goToAlbum":
            return I18n.tr("Go to album", "track menu action");
        case "goToArtist":
            return I18n.tr("Go to artist", "track menu action");
        case "details":
            return I18n.tr("Details", "menu action that shows everything known about a track or album");
        case "removeFromPlaylist":
            return I18n.tr("Remove from playlist", "playlist track action");
        case "remove":
        case "removeFromQueue":
            return I18n.tr("Remove from queue", "queue row action");
        case "keepOffline":
            return I18n.tr("Keep on this device", "toggle that caches an item's tracks for offline play");
        case "rename":
            return I18n.tr("Rename", "menu action that renames an item");
        case "duplicate":
            return I18n.tr("Duplicate", "playlist menu action");
        case "export":
            return I18n.tr("Export…", "playlist menu action");
        case "addToHome":
            return I18n.tr("Add to home", "page header action adding a home shelf");
        case "pin":
            return I18n.tr("Pin to navigation", "pin button");
        case "delete":
            return I18n.tr("Delete", "confirm button that deletes an item");
        case "library":
            return I18n.tr("Library", "track list column");
        case "cloud":
            return I18n.tr("Server status", "row element: the cloud icon of a track from a server");
        case "duration":
            return I18n.tr("Duration", "view info line");
        case "addedByYou":
            return I18n.tr("Added by you", "queue row: a track put in with play next or add to queue");
        case "more":
            return I18n.tr("More", "track row menu button");
        case "spacer":
        case "divider":
            return elementLabel(id);
        }
        return id;
    }

    function actionIcon(type, id) {
        switch (id) {
        case "play":
            return "play_arrow";
        case "shuffle":
            return "shuffle";
        case "playNext":
            return "queue_play_next";
        case "playAfter":
            return "low_priority";
        case "addToQueue":
            return "add_to_queue";
        case "favorite":
            return "favorite";
        case "rating":
            return "star";
        case "removeFromPlaylist":
        case "removeFromQueue":
        case "remove":
            return "remove_circle_outline";
        case "keepOffline":
            return "download_for_offline";
        case "rename":
            return "edit";
        case "duplicate":
            return "content_copy";
        case "export":
            return "file_export";
        case "addToHome":
            return "add_home";
        case "pin":
            return "push_pin";
        case "delete":
            return "delete";
        case "library":
            return "library_music";
        case "cloud":
            return "cloud";
        case "duration":
            return "schedule";
        case "addedByYou":
            return "person_add";
        case "more":
            return "more_vert";
        }
        return elementIcon(id);
    }

    // Keyboard shortcuts the user can change: searchPage focuses the page's own search, searchAll opens the
    // search over everything
    readonly property var shortcutDefaults: ({
            "searchPage": "Ctrl+F",
            "searchAll": "Ctrl+Shift+F"
        })

    function shortcut(id) {
        const stored = SettingsData.shortcuts?.[id];
        return typeof stored === "string" && shortcutProblem(stored) === "" ? stored : shortcutDefaults[id];
    }

    // why a sequence can't be used, or ""; other: the shortcut it must not collide with
    function shortcutProblem(sequence, other) {
        if (typeof sequence !== "string" || sequence.length === 0 || sequence.length > 64 || /[\u0000-\u001f\u007f]/.test(sequence))
            return I18n.tr("Not a key combination", "why a keyboard shortcut can't be used");
        const parts = sequence.split("+");
        const key = parts[parts.length - 1];
        const mods = parts.slice(0, -1);
        if (key === "" || mods.some(m => ["Ctrl", "Alt", "Shift", "Meta"].indexOf(m) < 0))
            return I18n.tr("Not a key combination", "why a keyboard shortcut can't be used");
        // a plain or shifted key would be typed into fields instead
        if (!mods.some(m => m !== "Shift") && !/^F\d+$/.test(key))
            return I18n.tr("Include Ctrl, Alt or Super", "why a keyboard shortcut can't be used");
        if (other !== undefined && sequence.toLowerCase() === shortcut(other).toLowerCase())
            return I18n.tr("Already used by %1", "why a keyboard shortcut can't be used, %1 is the other shortcut").arg(shortcutLabel(other));
        return "";
    }

    function shortcutLabel(id) {
        return id === "searchAll" ? I18n.tr("Search everything", "keyboard shortcut name") : I18n.tr("Search this page", "keyboard shortcut name");
    }

    // "" when saved, else the reason it wasn't
    function setShortcut(id, sequence) {
        const problem = shortcutProblem(sequence, id === "searchAll" ? "searchPage" : "searchAll");
        if (problem !== "")
            return problem;
        const all = Object.assign({}, SettingsData.shortcuts);
        if (sequence === shortcutDefaults[id])
            delete all[id];
        else
            all[id] = sequence;
        SettingsData.shortcuts = all;
        return "";
    }
    readonly property var navStyles: ["auto", "rail", "railLabels", "bar"]

    function navStyleLabel(style) {
        switch (style) {
        case "auto":
            return I18n.tr("Automatic", "option picked automatically");
        case "rail":
            return I18n.tr("Rail", "navigation style option");
        case "railLabels":
            return I18n.tr("Rail with labels", "navigation style option");
        case "bar":
            return I18n.tr("Bar", "navigation style option");
        }
        return style;
    }

    function deviceIcon(kind) {
        switch (kind) {
        case "usb":
            return "usb";
        case "hdmi":
            return "tv";
        case "bluetooth":
            return "bluetooth";
        case "headset":
            return "headphones";
        case "cast":
            return "cast";
        }
        return "speaker";
    }

    // How removals go: "undo" (a toast with Undo) | "confirm" (a dialog first) | "none"; the old
    // {enabled: false} meant confirm
    readonly property string removeMode: SettingsData.undoToast.mode ?? (SettingsData.undoToast.enabled === false ? "confirm" : "undo")
    readonly property int undoSeconds: SettingsData.undoToast.seconds ?? 5

    function setUndoToast(patch) {
        SettingsData.undoToast = Object.assign({}, SettingsData.undoToast, patch);
    }

    // Lyrics display

    readonly property string lyricsTextSize: SettingsData.lyricsDisplay.textSize ?? "m"
    readonly property string lyricsAlignment: SettingsData.lyricsDisplay.alignment ?? "center"
    readonly property bool lyricsWordSync: SettingsData.lyricsDisplay.wordSync ?? true
    readonly property bool lyricsBackgroundVocals: SettingsData.lyricsDisplay.backgroundVocals ?? true

    function setLyricsDisplay(patch) {
        SettingsData.lyricsDisplay = Object.assign({}, SettingsData.lyricsDisplay, patch);
    }
}
