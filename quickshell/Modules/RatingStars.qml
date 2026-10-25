import QtQuick
import qs.Common
import qs.Services
import qs.Widgets

// Unsupported: dimmed but clickable, so the reason shows as tooltip and snackbar.
DRatingBar {
    id: root

    property string kind: "track"
    property var item: null
    readonly property string blocker: PlayerService.whyNot("rate", item)

    rating: item?.rating ?? 0
    note: blocker
    opacity: blocker === "" ? 1 : Theme.disabledOpacity
    onRated: stars => {
        if (blocker !== "") {
            PlayerService.notice(blocker);
            return;
        }
        PlayerService.setRating(kind, item, stars);
    }
}
