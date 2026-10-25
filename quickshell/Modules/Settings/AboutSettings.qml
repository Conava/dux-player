pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// After the sibling apps' about page: the hero, the project's links, the build, then what the player is built
// on and the online services it talks to.
SettingsPage {
    id: root

    readonly property string repoUrl: "https://github.com/Conava/dux-player"
    readonly property string version: SettingsService.daemonBuild.version ?? ""
    readonly property string commit: SettingsService.daemonBuild.commit ?? ""
    // a release has a plain version; anything else is a development build
    readonly property bool devBuild: version === "" || version === "dev" || version.indexOf("+") >= 0

    function host(url) {
        return url.replace(/^https?:\/\//, "").replace(/\/$/, "");
    }

    DHeroCard {
        brand: I18n.tr("Player", "about page hero: the app's short name")
        caption: root.version !== "" ? "dplayer " + root.version : "dplayer"
    }

    SettingsGroup {
        SettingsLinkRow {
            iconName: "person"
            title: I18n.tr("By Marlon Kranz", "about page author row")
            subtitle: "github.com/Conava"
            url: "https://github.com/Conava"
        }

        SettingsLinkRow {
            iconName: "code"
            title: I18n.tr("Source code", "about page link to the repository")
            subtitle: root.host(root.repoUrl)
            url: root.repoUrl
        }

        SettingsLinkRow {
            iconName: "bug_report"
            title: I18n.tr("Report an issue", "about page link to the issue tracker")
            subtitle: root.host(root.repoUrl + "/issues")
            url: root.repoUrl + "/issues"
        }
    }

    SettingsCard {
        title: I18n.tr("This build", "about page section")

        SettingsRow {
            title: I18n.tr("Version", "about page daemon version column label")
            trailingBadge: root.version !== "" ? root.version : "—"
        }

        SettingsRow {
            visible: root.devBuild && root.commit !== "" && root.commit !== "unknown"
            title: I18n.tr("Commit", "about page row: the git commit of a development build")
            trailingBadge: root.commit
        }

        SettingsLinkRow {
            title: I18n.tr("MIT License", "about page license footer link")
            subtitle: I18n.tr("Copyright © 2026 Marlon Kranz", "about page license row")
            url: root.repoUrl + "/blob/master/LICENSE"
        }
    }

    SettingsCard {
        title: I18n.tr("Credits", "about page section: the projects the player is made from")

        SettingsLinkRow {
            title: "Dux Shell"
            subtitle: I18n.tr("Design system", "about page credit")
            url: "https://github.com/duxlabs-dev/dux-shell"
        }

        SettingsLinkRow {
            title: "dux-qml-common"
            subtitle: I18n.tr("UI components and theming", "about page credit")
            url: "https://github.com/duxlabs-dev/dux-qml-common"
        }

        SettingsLinkRow {
            title: "dux-go"
            subtitle: I18n.tr("Go libraries and lyrics engine", "about page credit")
            url: "https://github.com/duxlabs-dev/dux-go"
        }

        SettingsLinkRow {
            title: "mpv"
            subtitle: I18n.tr("Audio playback", "about page credit")
            url: "https://mpv.io"
        }

        SettingsLinkRow {
            title: "TagLib"
            subtitle: I18n.tr("Tag reading and writing (LGPL-2.1)", "about page credit")
            url: "https://taglib.org"
        }
    }

    SettingsCard {
        title: I18n.tr("Online services", "about page section")

        SettingsRow {
            title: I18n.tr("Lyrics", "song lyrics: page, toggle, settings tab")
            subtitle: (SettingsService.lyricsProviders ?? []).filter(p => p.enabled && p.id !== "server").map(p => p.name).join(", ") || I18n.tr("Off", "option that turns something off")
        }

        SettingsLinkRow {
            title: "MusicBrainz"
            subtitle: I18n.tr("Artist identification", "about page credit")
            url: "https://musicbrainz.org"
        }

        SettingsLinkRow {
            title: "Wikidata / Wikipedia"
            subtitle: I18n.tr("Artist biographies (CC BY-SA)", "about page credit")
            url: "https://creativecommons.org/licenses/by-sa/4.0/"
        }

        SettingsRow {
            title: "fanart.tv / Last.fm"
            subtitle: I18n.tr("Artist images (requires an API key)", "about page credit")
        }

        SettingsRow {
            title: "ListenBrainz / Last.fm"
            subtitle: I18n.tr("Scrobbling (requires sign-in)", "about page credit")
        }
    }
}
