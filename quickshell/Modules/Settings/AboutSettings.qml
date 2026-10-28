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

    function componentName(id) {
        switch (id) {
        case "ffmpeg":
            return "FFmpeg";
        case "pipewire":
            return "PipeWire";
        case "wpctl":
            return "WirePlumber (wpctl)";
        case "keyring":
            return I18n.tr("System keyring", "about page component: the Secret Service that stores passwords");
        }
        return id;
    }

    // what a component is for, or what is unavailable without it
    function componentText(c) {
        const ok = c.status === "ok";
        if (c.status === "restart")
            return I18n.tr("Installed. Restart Dux Player to use it.", "about page: a component was installed after the player started");
        switch (c.id) {
        case "mpv":
            if (c.status === "tooOld")
                return I18n.tr("Version %1 is too old. Requires %2 or newer.", "about page: the installed mpv can't be used").arg(c.version).arg(c.minVersion);
            if (c.status === "failed")
                return I18n.tr("Installed, but doesn't start. Playback is unavailable.", "about page: mpv is installed but broken");
            return ok ? I18n.tr("Audio playback", "about page credit") : I18n.tr("Not installed. Playback is unavailable.", "about page: mpv is missing");
        case "ffmpeg":
            return ok ? I18n.tr("Loudness and silence measurement", "about page: what ffmpeg is used for") : I18n.tr("Not installed. Loudness and silence aren't measured.", "about page: ffmpeg is missing");
        case "pipewire":
            return ok ? I18n.tr("Multiple outputs and the equalizer", "about page: what the PipeWire tools are used for") : I18n.tr("Not installed. Multiple outputs and the equalizer are unavailable.", "about page: the PipeWire tools are missing");
        case "wpctl":
            return ok ? I18n.tr("System volume", "volume controls option") : I18n.tr("Not installed. System volume is unavailable.", "about page: wpctl is missing");
        case "keyring":
            return ok ? I18n.tr("Server passwords and service keys", "about page: what the keyring is used for") : I18n.tr("Not running. Passwords and keys can't be saved.", "about page: no Secret Service is running");
        case "cast-helper":
            return ok ? I18n.tr("Casting", "about page: what cast-helper is used for") : I18n.tr("Not installed. Casting is unavailable.", "about page: cast-helper is missing");
        }
        return ok ? "" : I18n.tr("Not installed", "about page: a component is missing");
    }

    Component.onCompleted: SettingsService.checkComponents()

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
            keywords: "github repository"
            subtitle: root.host(root.repoUrl)
            url: root.repoUrl
        }

        SettingsLinkRow {
            iconName: "bug_report"
            title: I18n.tr("Report an issue", "about page link to the issue tracker")
            keywords: "bug github"
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
        title: I18n.tr("Installed components", "about page section: the external programs and services the player uses")
        keywords: "mpv ffmpeg pipewire wpctl wireplumber keyring secret service cast helper dependencies requirements missing programs"
        headerActions: DActionButton {
            iconName: "refresh"
            enabled: !SettingsService.componentsChecking
            tooltipText: I18n.tr("Check again", "about page action that checks the installed components again")
            Accessible.name: tooltipText
            onClicked: SettingsService.checkComponents()
        }

        Repeater {
            model: SettingsService.components

            SettingsRow {
                required property var modelData
                readonly property bool ok: modelData.status === "ok"
                readonly property color stateColor: ok ? Theme.primary : modelData.required ? Theme.error : Theme.warning

                title: root.componentName(modelData.id)
                subtitle: root.componentText(modelData)
                subtitleColor: ok ? Theme.surfaceVariantText : stateColor
                iconName: ok ? "check_circle" : modelData.required ? "error" : "warning"
                iconColor: stateColor
                trailingBadge: ok ? modelData.version || I18n.tr("Installed", "about page: a component is installed") : ""
                searchable: false
            }
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
        keywords: "privacy network"

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
