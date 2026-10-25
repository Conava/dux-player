pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

DFlickable {
    id: root

    contentHeight: column.height + PlayerMetrics.pagePadding
    contentWidth: width
    clip: true
    // a square edge fade would paint over the pane's rounded corners
    fadeSideInset: PlayerMetrics.paneRadius

    Column {
        id: column
        x: PlayerMetrics.pagePadding
        width: root.width - PlayerMetrics.pagePadding * 2
        spacing: Theme.spacingL

        PageHeader {
            title: I18n.tr("Libraries", "the music libraries: page, settings tab, section")
            subtitle: I18n.tr("Folders and servers", "libraries page subtitle")

            DButton {
                text: I18n.tr("Manage", "libraries page button opening library settings")
                iconName: "settings"
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                onClicked: Router.openSettings("libraries")
            }
        }

        Flow {
            width: parent.width
            spacing: Theme.spacingM

            Repeater {
                model: PlayerService.libraries

                DCard {
                    id: card
                    required property var modelData
                    readonly property bool remote: PlayerService.isRemote(modelData.id)

                    width: Math.min(column.width, PlayerMetrics.gridCellWidth * 2)
                    height: cardColumn.implicitHeight + pad * 2
                    pad: Theme.spacingL
                    clickable: modelData.enabled
                    tone: remote ? "tertiary" : "secondary"
                    opacity: modelData.enabled ? 1 : Theme.disabledOpacity
                    Accessible.name: modelData.name
                    onClicked: Router.go("library", modelData.id)

                    Column {
                        id: cardColumn
                        width: parent.width
                        spacing: Theme.spacingS

                        Row {
                            width: parent.width
                            spacing: Theme.spacingM

                            DIcon {
                                anchors.verticalCenter: parent.verticalCenter
                                name: UiConfig.entryIcon({
                                    "kind": "library",
                                    "target": card.modelData.id
                                })
                                size: Theme.iconSizeLarge
                                color: card.accentColor
                            }

                            Column {
                                anchors.verticalCenter: parent.verticalCenter
                                width: parent.width - Theme.iconSizeLarge - parent.spacing
                                spacing: Theme.spacingXXS

                                StyledText {
                                    width: parent.width
                                    text: card.modelData.name
                                    font.pixelSize: Theme.fontSizeLarge
                                    font.weight: Theme.fontWeightMedium
                                    color: card.contentColor
                                    elide: Text.ElideRight
                                    wrapMode: Text.NoWrap
                                }

                                StyledText {
                                    width: parent.width
                                    text: card.remote ? Phrases.join(PlayerService.kindLabel(card.modelData.kind), card.modelData.url) : (card.modelData.folders ?? []).join(", ")
                                    font.pixelSize: Theme.fontSizeSmall
                                    color: card.mutedColor
                                    elide: Text.ElideMiddle
                                    wrapMode: Text.NoWrap
                                }
                            }
                        }

                        StyledText {
                            width: parent.width
                            text: {
                                if (!card.modelData.enabled)
                                    return I18n.tr("Turned off in settings", "library card status");
                                if (!card.modelData.available)
                                    return Phrases.libraryError(card.modelData) || I18n.tr("Unavailable", "library card status");
                                if (card.modelData.scanning)
                                    return Phrases.libraryBusy(card.modelData);
                                return Phrases.libraryCounts(card.modelData.id);
                            }
                            font.pixelSize: Theme.fontSizeMedium
                            color: card.modelData.enabled && !card.modelData.available ? Theme.error : card.contentColor
                            wrapMode: Text.WordWrap
                        }

                        DButton {
                            visible: card.modelData.enabled && !card.modelData.available
                            text: I18n.tr("Try again", "library action that checks an unavailable library again")
                            iconName: "refresh"
                            buttonHeight: Theme.buttonHeightXS
                            backgroundColor: Theme.secondaryContainer
                            textColor: Theme.onSecondaryContainer
                            onClicked: PlayerService.retryLibrary(card.modelData.id)
                        }

                        StyledText {
                            width: parent.width
                            visible: text !== ""
                            text: PlayerService.featureSummary(card.modelData.id)
                            font.pixelSize: Theme.fontSizeSmall
                            color: card.mutedColor
                            wrapMode: Text.WordWrap
                        }
                    }
                }
            }
        }
    }
}
