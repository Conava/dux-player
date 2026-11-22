pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

DActionButton {
    id: root

    // "track" | "album" | "artist"
    property string kind: "track"
    property var item: null
    // a track's heart follows marks events even when the row holds an older copy
    readonly property bool favorite: (kind === "track" ? CatalogService.withMarks(item) : kind === "artist" ? CatalogService.withArtistMarks(item) : item)?.favorite === true
    readonly property bool synced: item?.library !== undefined && PlayerService.isRemote(item.library)
    // unsupported: dimmed but clickable, so the reason shows as tooltip and snackbar
    readonly property string blocker: PlayerService.whyNot("favorite", item)

    property color activeColor: Theme.primary
    property color inactiveColor: Theme.onSurfaceVariant

    readonly property string itemKey: (item?.library ?? "") + "/" + (item?.id ?? "")
    property bool ready: false
    property bool itemChanging: false

    // while it pops, the heart is drawn by its own icon, so it can scale without scaling the button
    iconName: pop.active ? "" : "favorite"
    iconFilled: favorite
    iconColor: favorite ? activeColor : inactiveColor
    opacity: blocker === "" ? 1 : Theme.disabledOpacity
    tooltipText: {
        if (blocker !== "")
            return blocker;
        if (synced)
            return favorite ? I18n.tr("Remove from favorites on %1", "favorite button tooltip, %1 is the server library").arg(PlayerService.libraryName(item?.library)) : I18n.tr("Add to favorites on %1", "favorite button tooltip, %1 is the server library").arg(PlayerService.libraryName(item?.library));
        return favorite ? I18n.tr("Remove from favorites", "track menu action") : I18n.tr("Add to favorites", "track menu action");
    }
    Accessible.role: Accessible.CheckBox
    Accessible.name: I18n.tr("Favorite", "view info line")
    Accessible.checked: favorite
    Accessible.description: blocker
    onClicked: {
        if (blocker !== "") {
            PlayerService.notice(blocker);
            return;
        }
        PlayerService.setFavorite(kind, item, !favorite);
    }

    Component.onCompleted: ready = true

    // a favorite that arrives with another track doesn't pop
    onItemKeyChanged: {
        itemChanging = true;
        Qt.callLater(() => itemChanging = false);
    }

    onFavoriteChanged: {
        if (favorite && ready && !itemChanging && visible && Theme.animationsEnabled) {
            pop.active = false;
            pop.active = true;
        }
    }

    // built only for a pop: every track row has a heart
    Loader {
        id: pop
        anchors.centerIn: parent
        active: false

        sourceComponent: DIcon {
            id: heart
            name: "favorite"
            size: root.iconSize
            filled: true
            color: root.enabled ? root.iconColor : Theme.onSurface_38

            SequentialAnimation on scale {
                onFinished: pop.active = false
                NumberAnimation {
                    to: PlayerMetrics.popScale
                    duration: Theme.expressiveDurations.expressiveFastSpatial / 2
                    easing.type: Easing.BezierSpline
                    easing.bezierCurve: Theme.expressiveCurves.expressiveFastSpatial
                }
                NumberAnimation {
                    to: 1
                    duration: Theme.expressiveDurations.expressiveFastSpatial / 2
                    easing.type: Easing.BezierSpline
                    easing.bezierCurve: Theme.expressiveCurves.expressiveFastSpatial
                }
            }
        }
    }
}
