import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

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

    iconName: "favorite"
    iconFilled: favorite
    iconColor: favorite ? activeColor : inactiveColor
    opacity: blocker === "" ? 1 : Theme.disabledOpacity
    tooltipText: {
        if (blocker !== "")
            return blocker;
        if (synced)
            return favorite ? I18n.tr("Remove from favorites on %1", "favorite button tooltip, %1 is the server library").arg(PlayerService.libraryName(item.library)) : I18n.tr("Add to favorites on %1", "favorite button tooltip, %1 is the server library").arg(PlayerService.libraryName(item.library));
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
}
