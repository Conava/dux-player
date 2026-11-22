pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

SettingsPage {
    id: root

    // runtime only: each visit starts collapsed
    property bool discordExpanded: false
    readonly property bool discordOn: SettingsService.settings.discordPresence
    readonly property bool discordOpen: discordOn && discordExpanded
    readonly property var discordSample: PlayerService.current ?? CatalogService.visibleTracks()[0] ?? null
    // the album the preview shows a cover for, "" when it shows the logo
    readonly property string discordCoverAlbum: discordOpen && SettingsService.settings.discordCover && (discordSample?.album ?? "") !== "" ? [discordSample.albumArtist, discordSample.artist, discordSample.album].join("\n") : ""
    property string discordImage: ""

    onDiscordCoverAlbumChanged: {
        discordImage = "";
        const album = discordCoverAlbum;
        if (album === "")
            return;
        SettingsService.discordCover(discordSample, url => {
            if (root.discordCoverAlbum === album)
                root.discordImage = url;
        });
    }

    SettingsCard {
        SettingsRow {
            title: I18n.tr("Discord Rich Presence", "integrations setting")
            subtitle: I18n.tr("Shows the playing track on the Discord profile.", "discord presence description")
            keywords: "discord status activity"
            clickable: root.discordOn
            Accessible.role: Accessible.Button
            Accessible.checkable: root.discordOn
            Accessible.checked: root.discordOpen
            onClicked: root.discordExpanded = !root.discordExpanded

            DToggle {
                anchors.verticalCenter: parent.verticalCenter
                hideText: true
                text: I18n.tr("Discord Rich Presence", "integrations setting")
                checked: root.discordOn
                onToggled: value => SettingsService.setSetting("discordPresence", value)
            }

            // keeps its room while off so the switch doesn't move
            DIcon {
                anchors.verticalCenter: parent.verticalCenter
                opacity: root.discordOn ? 1 : 0
                name: root.discordOpen ? "expand_less" : "expand_more"
                size: Theme.iconSize
                color: Theme.surfaceVariantText
            }
        }

        TemplateRow {
            visible: root.discordOpen
            searchable: false
            showPreview: false
            title: I18n.tr("First line", "discord presence template")
            template: SettingsService.settings.discordDetails
            onEdited: value => SettingsService.setSetting("discordDetails", value)
        }

        TemplateRow {
            visible: root.discordOpen
            searchable: false
            showPreview: false
            title: I18n.tr("Second line", "discord presence template")
            template: SettingsService.settings.discordState
            onEdited: value => SettingsService.setSetting("discordState", value)
        }

        TemplateRow {
            visible: root.discordOpen
            searchable: false
            showPreview: false
            title: I18n.tr("Image tooltip", "discord presence template: text shown when hovering the image on the card")
            template: SettingsService.settings.discordLargeText
            onEdited: value => SettingsService.setSetting("discordLargeText", value)
        }

        SettingsToggleRow {
            visible: root.discordOpen
            searchable: false
            text: I18n.tr("Show album cover", "discord presence setting")
            description: I18n.tr("Sends artist and album to Last.fm and MusicBrainz.", "discord album cover description")
            checked: SettingsService.settings.discordCover
            onToggled: value => SettingsService.setSetting("discordCover", value)
        }

        SettingsRow {
            visible: root.discordOpen
            searchable: false
            title: I18n.tr("Preview", "discord card preview")

            body: DiscordCard {
                playing: PlayerService.playing && root.discordSample === PlayerService.current
                position: PlayerService.position
                detailsText: UiConfig.templateText(SettingsService.settings.discordDetails, root.discordSample, false)
                stateText: UiConfig.templateText(SettingsService.settings.discordState, root.discordSample, false)
                largeText: UiConfig.templateText(SettingsService.settings.discordLargeText, root.discordSample, false)
                image: root.discordImage
            }
        }
    }
}
