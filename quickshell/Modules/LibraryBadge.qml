import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

DBadge {
    id: root

    property string libraryId: ""

    readonly property bool remote: PlayerService.isRemote(libraryId)

    text: PlayerService.libraryName(libraryId)
    color: remote ? Theme.tertiaryContainer : Theme.secondaryContainer
    textColor: remote ? Theme.onTertiaryContainer : Theme.onSecondaryContainer
    tooltipText: remote ? I18n.tr("Streams from a %1 server", "library badge tooltip, %1 is the server kind").arg(PlayerService.kindLabel(PlayerService.library(libraryId)?.kind ?? "")) : I18n.tr("Local library", "library badge tooltip")
}
