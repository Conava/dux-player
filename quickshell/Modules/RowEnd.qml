pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import "../Common/Format.js" as Format

// The hover buttons and the end of a row, as set up for its type (track, album, …, queue rows, and TrackList's
// header). Each slot's width depends only on its type and item, so a list's columns line up.
Row {
    id: end

    // a TrackMenu or MediaMenu: its type, hoverEntries, open() and run()
    required property var actions
    property var item: null
    property bool hovering: false
    // a queue entry put in by hand
    property bool added: false
    // in columns, the library badge takes a column's width
    property bool fixedLibrary: false
    // TrackList's header: every slot keeps its width, only this shows, in the duration's place
    property Component timeHeader: null
    // the row's supporting color (it changes with the selection)
    property color textColor: Theme.surfaceVariantText

    readonly property bool header: timeHeader !== null
    readonly property bool pointerInside: endHover.hovered
    readonly property var slots: (UiConfig.item(actions.type)?.zones.trailing ?? []).filter(i => !i.hidden && UiConfig.isKnown(i.type))
    readonly property string favoriteKind: actions.type === "queue" ? "track" : actions.type
    readonly property bool remote: PlayerService.isRemote(item?.library ?? "")
    readonly property bool serverDown: remote && PlayerService.library(item?.library ?? "")?.available === false
    // with one library the badge has nothing to tell
    readonly property bool manyLibraries: CatalogService.libraries.filter(l => l.enabled).length > 1
    property Item moreButton: null

    spacing: 0

    HoverHandler {
        id: endHover
    }

    Repeater {
        model: end.actions.hoverEntries

        DActionButton {
            required property var modelData
            anchors.verticalCenter: parent?.verticalCenter
            iconName: modelData.icon
            iconFilled: modelData.iconFilled ?? false
            tooltipText: modelData.description || modelData.text
            Accessible.name: modelData.text
            Accessible.description: modelData.description ?? ""
            enabled: !end.header && !modelData.inert
            opacity: !enabled || !end.hovering && !activeFocus ? 0 : modelData.enabled === false ? Theme.disabledOpacity : 1
            // blocked: still clickable, so the reason shows
            onClicked: modelData.enabled === false ? PlayerService.notice(modelData.description) : end.actions.run(modelData.id, this)
        }
    }

    Item {
        width: end.actions.hoverEntries.length > 0 ? Theme.spacingM : 0
        height: 1
    }

    Repeater {
        model: end.slots

        Loader {
            id: slot
            required property var modelData
            anchors.verticalCenter: parent?.verticalCenter
            enabled: !end.header || modelData.type === "duration"
            opacity: end.header && modelData.type !== "duration" ? 0 : 1
            sourceComponent: {
                switch (modelData.type) {
                case "favorite":
                    return favoriteSlot;
                case "rating":
                    return ratingSlot;
                case "library":
                    return librarySlot;
                case "cloud":
                    return cloudSlot;
                case "duration":
                    return end.header ? end.timeHeader : durationSlot;
                case "play":
                    return playSlot;
                case "addedByYou":
                    return addedSlot;
                case "remove":
                    return removeSlot;
                case "more":
                    return moreSlot;
                case "spacer":
                    return spacerSlot;
                }
                return null;
            }

            Component {
                id: favoriteSlot

                // shown when set, otherwise on hover; where the library has no favorites it keeps its place only
                FavoriteButton {
                    kind: end.favoriteKind
                    item: end.item
                    enabled: blocker === ""
                    opacity: enabled && (favorite || end.hovering || activeFocus) ? 1 : 0
                }
            }

            Component {
                id: ratingSlot

                RatingStars {
                    kind: end.favoriteKind
                    item: end.item
                }
            }

            Component {
                id: librarySlot

                Item {
                    width: !end.manyLibraries ? 0 : (end.fixedLibrary ? PlayerMetrics.columnLibraryWidth : badge.implicitWidth) + Theme.spacingS
                    height: badge.height

                    LibraryBadge {
                        id: badge
                        x: Theme.spacingXS
                        anchors.verticalCenter: parent.verticalCenter
                        visible: end.manyLibraries && libraryId !== ""
                        libraryId: end.item?.library ?? ""
                        maximumWidth: PlayerMetrics.columnLibraryWidth
                    }
                }
            }

            // streams from a server: struck through while the server is down, unless a copy is cached
            Component {
                id: cloudSlot

                Item {
                    width: end.remote ? Theme.iconSizeSmall + Theme.spacingS : 0
                    height: Theme.iconSizeSmall

                    DIcon {
                        anchors.right: parent.right
                        anchors.verticalCenter: parent.verticalCenter
                        visible: end.remote
                        name: !end.serverDown ? "cloud" : PlayerService.isCached(end.item) ? "cloud_done" : "cloud_off"
                        size: Theme.iconSizeSmall
                        color: end.textColor
                        Accessible.name: !end.serverDown ? I18n.tr("Streams from a server", "track row cloud icon") : PlayerService.isCached(end.item) ? I18n.tr("Available offline", "track row cloud icon while the server is down") : I18n.tr("Server unreachable", "track row cloud icon while the server is down")
                    }
                }
            }

            Component {
                id: durationSlot

                StyledText {
                    width: PlayerMetrics.columnTimeWidth
                    text: Format.duration(end.item?.duration ?? 0)
                    font.pixelSize: Theme.fontSizeSmall
                    color: end.textColor
                    horizontalAlignment: Text.AlignRight
                }
            }

            Component {
                id: playSlot

                DIconButton {
                    iconName: "play_arrow"
                    iconFilled: true
                    Accessible.name: I18n.tr("Play %1", "play button, %1 is the album or playlist").arg(end.item?.title ?? end.item?.name ?? "")
                    onClicked: end.actions.run("play", this)
                }
            }

            Component {
                id: addedSlot

                Item {
                    width: end.added ? Theme.iconSizeSmall + Theme.spacingS : 0
                    height: Theme.iconSizeSmall

                    DIcon {
                        anchors.right: parent.right
                        anchors.verticalCenter: parent.verticalCenter
                        visible: end.added
                        name: "add_to_queue"
                        size: Theme.iconSizeSmall
                        color: end.textColor
                        Accessible.name: I18n.tr("Added by you", "queue row: a track put in with play next or add to queue")
                    }
                }
            }

            Component {
                id: removeSlot

                DActionButton {
                    iconName: "close"
                    tooltipText: UiConfig.actionLabel(end.actions.type, "removeFromQueue")
                    Accessible.name: tooltipText
                    onClicked: end.actions.run("removeFromQueue", this)
                }
            }

            Component {
                id: moreSlot

                DActionButton {
                    iconName: "more_vert"
                    tooltipText: I18n.tr("More", "track row menu button")
                    Accessible.name: tooltipText
                    onClicked: end.actions.open(this)
                    Component.onCompleted: end.moreButton = this
                }
            }

            // "Fill" is the large size here; a row's end has no free space to fill
            Component {
                id: spacerSlot

                Item {
                    width: slot.modelData.options.size === "s" ? Theme.spacingS : slot.modelData.options.size === "m" ? Theme.spacingL : Theme.spacingXL
                    height: 1
                }
            }
        }
    }
}
