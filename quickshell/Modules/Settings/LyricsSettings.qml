pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets

SettingsPage {
    id: root

    readonly property var providers: PlayerService.lyricsProviders

    SettingsPageIntro {
        text: I18n.tr("Lyrics files next to your music are used first, then the providers below in order. Drag to reorder.", "lyrics settings intro")
    }

    SettingsCard {
        title: I18n.tr("Display", "lyrics settings section label")

        SettingsButtonGroupRow {
            text: I18n.tr("Text size", "appearance setting")
            model: [I18n.tr("Small", "size option"), I18n.tr("Medium", "size option"), I18n.tr("Large", "size option")]
            currentIndex: ["s", "m", "l"].indexOf(UiConfig.lyricsTextSize)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    UiConfig.setLyricsDisplay({
                        "textSize": ["s", "m", "l"][index]
                    });
            }
        }

        SettingsButtonGroupRow {
            text: I18n.tr("Alignment", "lyrics setting")
            description: I18n.tr("In duets, each singer keeps their own side.", "lyrics setting description")
            model: [I18n.tr("Center", "lyrics alignment"), I18n.tr("Start", "lyrics alignment")]
            currentIndex: UiConfig.lyricsAlignment === "start" ? 1 : 0
            onSelectionChanged: (index, selected) => {
                if (selected)
                    UiConfig.setLyricsDisplay({
                        "alignment": index === 1 ? "start" : "center"
                    });
            }
        }

        SettingsToggleRow {
            text: I18n.tr("Word by word", "lyrics setting")
            description: I18n.tr("Highlights each word as it's sung, when the lyrics include word timing.", "lyrics setting description")
            checked: UiConfig.lyricsWordSync
            onToggled: value => UiConfig.setLyricsDisplay({
                    "wordSync": value
                })
        }

        SettingsToggleRow {
            text: I18n.tr("Background vocals", "lyrics setting")
            checked: UiConfig.lyricsBackgroundVocals
            onToggled: value => UiConfig.setLyricsDisplay({
                    "backgroundVocals": value
                })
        }
    }

    SectionLabel {
        text: I18n.tr("Providers", "lyrics settings section label")
    }

    DReorderList {
        id: list
        width: parent.width
        model: root.providers
        flickable: root
        onReordered: indices => PlayerService.moveLyricsProviders(indices)

        delegate: ReorderRow {
            id: entry
            required property var modelData
            readonly property string name: modelData.id === "server" ? I18n.tr("Your server", "lyrics provider row for a server library's own lyrics") : modelData.name
            view: list
            label: name
            onClicked: PlayerService.setLyricsProviderEnabled(modelData.id, !modelData.enabled)

            StyledText {
                anchors.left: entry.handle.right
                anchors.leftMargin: Theme.spacingS
                anchors.right: toggle.left
                anchors.verticalCenter: parent.verticalCenter
                text: entry.name
                font.pixelSize: Theme.fontSizeMedium
                color: entry.contentColor
                opacity: entry.modelData.enabled ? 1 : Theme.disabledOpacity
                horizontalAlignment: Text.AlignLeft
            }

            DToggle {
                id: toggle
                anchors.right: parent.right
                anchors.rightMargin: Theme.spacingL
                anchors.verticalCenter: parent.verticalCenter
                hideText: true
                text: entry.name
                checked: entry.modelData.enabled
                onToggled: value => PlayerService.setLyricsProviderEnabled(entry.modelData.id, value)
            }
        }
    }
}
