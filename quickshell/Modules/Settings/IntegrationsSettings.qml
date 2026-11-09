pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

SettingsPage {
    id: root

    SettingsCard {
        SettingsToggleRow {
            text: I18n.tr("Discord Rich Presence", "integrations setting")
            description: I18n.tr("Shows the playing track on your Discord profile.", "discord presence description")
            keywords: "discord status activity"
            checked: SettingsService.settings.discordPresence
            onToggled: value => SettingsService.setSetting("discordPresence", value)
        }
    }
}
