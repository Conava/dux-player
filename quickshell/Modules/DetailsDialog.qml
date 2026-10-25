pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import qs.Services
import qs.Modules.Settings
import qs.DCommon.Widgets
import "../Common/Format.js" as Format

// Everything known about a track or album (Router.showDetails): what the list already has, then what
// browse.details adds (tags, file, audio format, ReplayGain, MusicBrainz ids…). One contained row per fact.
WindowDialog {
    id: root

    readonly property var request: Router.details
    readonly property string kind: request?.kind ?? "track"
    readonly property var item: request ? CatalogService.withMarks(request.item) : null
    property var fetched: ({})
    readonly property var facts: item ? rows(Object.assign({}, item, fetched)) : []
    // a local file's own tags, by TagLib name
    readonly property var tags: Object.keys(fetched.tags ?? {}).sort().map(k => ({
                "label": k,
                "value": fetched.tags[k].join("; ")
            }))

    onRequestChanged: {
        fetched = {};
        if (!request)
            return;
        const asked = request;
        const take = d => {
            if (root.request === asked)
                root.fetched = d ?? {};
        };
        // request.kind: the kind binding may not have caught up with the new request yet
        if (request.kind === "artist")
            CatalogService.artistInfo(request.item.id, take);
        else
            CatalogService.details(request.kind, request.item, take);
    }

    function date(seconds) {
        return seconds > 0 ? new Date(seconds * 1000).toLocaleString(Qt.locale(), Locale.ShortFormat) : "";
    }

    function bytes(n) {
        return n >= 1e9 ? I18n.tr("%1 GB", "file size").arg((n / 1e9).toFixed(1)) : I18n.tr("%1 MB", "file size").arg((n / 1e6).toFixed(1));
    }

    function gain(db, peak) {
        if (db === undefined || db === null)
            return "";
        const text = I18n.tr("%1 dB", "ReplayGain value").arg(Number(db).toFixed(2));
        return peak ? Phrases.join(text, I18n.tr("peak %1", "ReplayGain peak value").arg(Number(peak).toFixed(3))) : text;
    }

    // [{label, value, copy}] for every fact that has a value
    function rows(d) {
        const lib = CatalogService.library(d.library);
        const rg = d.replayGain ?? {};
        const mb = d.musicBrainz ?? {};
        const list = [[I18n.tr("Title", "track list column"), d.title ?? d.name], [I18n.tr("Artist", "track list column"), d.artist, root.item?.artists], [I18n.tr("Album artist", "details field"), d.albumArtist !== d.artist ? d.albumArtist : ""]];
        if (kind === "artist")
            list.push([I18n.tr("Albums", "navigation destination"), d.albumCount > 0 ? String(d.albumCount) : ""], [I18n.tr("Tracks", "navigation destination"), d.trackCount > 0 ? String(d.trackCount) : ""], [I18n.tr("MusicBrainz artist", "details field"), d.musicBrainzId]);
        else if (kind === "track")
            list.push([I18n.tr("Album", "an album: page label, list column, ReplayGain mode"), d.album], [I18n.tr("Track", "details field: track and disc number"), d.number > 0 ? (d.discCount > 1 || d.disc > 1 ? I18n.tr("%1, disc %2", "track number and disc number").arg(d.number).arg(d.disc) : String(d.number)) : ""]);
        else
            list.push([I18n.tr("Tracks", "navigation destination"), d.trackCount > 0 ? String(d.trackCount) : ""]);
        list.push([I18n.tr("Year", "view info line"), d.year > 0 ? String(d.year) : ""], [I18n.tr("Genre", "view info line"), (d.genres ?? []).length > 0 ? d.genres.join(", ") : d.genre], [I18n.tr("Duration", "view info line"), d.duration > 0 ? Format.duration(d.duration) : ""], [I18n.tr("Format", "details field: codec or file type"), Phrases.joinAll([d.format ?? "", d.codec && d.codec.toUpperCase() !== (d.format ?? "").toUpperCase() ? d.codec : "", d.bitrate > 0 ? I18n.tr("%1 kbps", "bitrate").arg(d.bitrate) : "", d.sampleRate > 0 ? I18n.tr("%1 kHz", "sample rate").arg(d.sampleRate / 1000) : "", d.bitDepth > 0 ? I18n.tr("%1 bit", "bit depth").arg(d.bitDepth) : "", d.channels > 0 ? I18n.tr("%1 ch", "audio channel count").arg(d.channels) : ""])], [I18n.tr("Size", "details field: file size"), d.size > 0 ? bytes(d.size) : ""], [I18n.tr("ReplayGain (track)", "details field"), gain(rg.trackGain, rg.trackPeak)], [I18n.tr("ReplayGain (album)", "details field"), gain(rg.albumGain, rg.albumPeak)], [I18n.tr("Measured gain", "details field: loudness gain measured on this device"), gain(rg.measured)], [I18n.tr("Plays", "view info line and sort key"), d.playCount > 0 ? String(d.playCount) : ""], [I18n.tr("Last played", "sort key"), date(d.lastPlayed)], [I18n.tr("Date added", "sort key"), date(d.added)], [I18n.tr("Rating", "view info line"), d.rating > 0 ? UiConfig.stars(d.rating) : ""], [I18n.tr("Favorite", "view info line"), d.favorite ? I18n.tr("Yes", "details value") : ""], [I18n.tr("Library", "track list column"), Phrases.joinAll([lib?.name ?? d.libraryName ?? "", lib ? CatalogService.kindLabel(lib.kind) : ""])], [I18n.tr("Server", "details field: the server address"), d.server], [I18n.tr("Downloaded", "details field: whether a server track is cached"), d.cached === true ? I18n.tr("Yes", "details value") : d.cached === false ? I18n.tr("No", "details value") : ""], [I18n.tr("MusicBrainz track", "details field"), mb.trackId], [I18n.tr("MusicBrainz recording", "details field"), mb.recordingId], [I18n.tr("MusicBrainz album", "details field"), mb.albumId], [I18n.tr("MusicBrainz artist", "details field"), mb.artistId], [I18n.tr("MusicBrainz album artist", "details field"), mb.albumArtistId], [I18n.tr("MusicBrainz release group", "details field"), mb.releaseGroupId], [I18n.tr("Comment", "details field: the comment tag"), d.comment]);
        const out = list.filter(r => r[1] !== undefined && r[1] !== null && r[1] !== "").map(r => ({
                    "label": r[0],
                    "value": String(r[1]),
                    "artists": (r[2] ?? []).length > 0 ? r[2] : null,
                    "copy": false
                }));
        if (d.path)
            out.push({
                "label": I18n.tr("File", "details field: the path on disk"),
                "value": d.path,
                "copy": true
            });
        return out;
    }

    title: kind === "album" ? I18n.tr("Album details", "details dialog title") : kind === "artist" ? I18n.tr("Artist details", "details dialog title") : I18n.tr("Track details", "details dialog title")
    opened: request !== null
    acceptEnabled: false
    onRejected: Router.details = null
    onOpenedChanged: {
        if (opened)
            Qt.callLater(forceActiveFocus);
    }

    Column {
        width: parent.width
        spacing: Theme.spacingM

        SettingsGroup {
            width: parent.width

            Repeater {
                model: root.facts

                SettingsRow {
                    id: fact
                    required property var modelData
                    width: parent?.width ?? 0
                    title: modelData.label
                    subtitle: modelData.artists ? "" : modelData.value
                    subtitleContent: ArtistLinks {
                        visible: !!fact.modelData.artists
                        width: Math.min(implicitWidth, parent?.width ?? 0)
                        artists: fact.modelData.artists ?? []
                        onActivated: Router.details = null
                    }

                    DActionButton {
                        visible: fact.modelData.copy
                        anchors.verticalCenter: parent?.verticalCenter
                        iconName: "content_copy"
                        tooltipText: I18n.tr("Copy", "copy a config snippet")
                        Accessible.name: tooltipText
                        onClicked: Quickshell.clipboardText = fact.modelData.value
                    }
                }
            }
        }

        SectionLabel {
            visible: root.tags.length > 0
            text: I18n.tr("File tags", "details dialog section")
        }

        SettingsGroup {
            width: parent.width
            visible: root.tags.length > 0

            Repeater {
                model: root.tags

                SettingsRow {
                    required property var modelData
                    width: parent?.width ?? 0
                    title: modelData.label
                    subtitle: modelData.value
                }
            }
        }
    }
}
