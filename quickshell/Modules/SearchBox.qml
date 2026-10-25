import QtQuick
import qs.Common
import qs.Services

// Home's search over everything, large, in the launcher's look: typing opens the search page with the query.
ScopedSearch {
    width: parent?.width ?? 0
    height: Theme.listItemHeight
    font.pixelSize: Theme.fontSizeLarge
    placeholderText: I18n.tr("Search albums, artists and tracks", "search field placeholder")
    onTextEdited: {
        if (text.trim() === "")
            return;
        PlayerService.search(text);
        Router.go("search");
    }
}
