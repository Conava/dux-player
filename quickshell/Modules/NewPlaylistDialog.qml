pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

WindowDialog {
    id: root

    readonly property var servers: PlayerService.libraries.filter(l => l.enabled && PlayerService.hasCapability(l.id, "playlistEdit"))
    readonly property var places: [
        {
            "text": I18n.tr("This device", "new playlist location option"),
            "icon": "computer"
        }
    ].concat(servers.map(l => ({
                "text": l.name,
                "icon": "cloud"
            })))
    readonly property int serverIndex: placeGroup.currentIndex - 1
    readonly property bool valid: nameField.text.trim() !== ""
    // tracks the new playlist starts with (from Add to playlist)
    property var tracks: []

    function show(initialTracks) {
        tracks = initialTracks ?? [];
        nameField.text = "";
        placeGroup.currentIndex = 0;
        opened = true;
        nameField.forceActiveFocus();
    }

    function create() {
        if (!valid)
            return;
        const libraryId = serverIndex >= 0 ? servers[serverIndex].id : "";
        const picked = tracks;
        PlaylistService.createPlaylist(nameField.text.trim(), libraryId, id => {
            if (picked.length === 0)
                Router.go("playlist", id);
        }, picked);
        if (libraryId !== "")
            CatalogService.reportSkipped(picked.filter(t => t.library !== libraryId).length);
        opened = false;
    }

    title: I18n.tr("New playlist", "action and dialog that create a playlist")
    acceptEnabled: valid
    onAccepted: create()
    onRejected: opened = false

    DTextField {
        id: nameField
        width: parent.width
        outlined: true
        labelText: I18n.tr("Name", "a name: field label, sort key")
        onAccepted: root.create()
    }

    StyledText {
        width: parent.width
        text: I18n.tr("Save to", "new playlist dialog location label")
        font.pixelSize: Theme.fontSizeMedium
        font.weight: Theme.fontWeightMedium
        color: Theme.surfaceText
    }

    DButtonGroup {
        id: placeGroup
        model: root.places
        currentIndex: 0
        onSelectionChanged: (index, selected) => {
            if (selected)
                currentIndex = index;
        }
    }

    StyledText {
        width: parent.width
        text: root.serverIndex >= 0 ? I18n.tr("Can hold tracks from this server only. Available on all your devices.", "new playlist dialog hint for a server playlist") : I18n.tr("Can hold tracks from any library.", "new playlist dialog hint for a local playlist")
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.surfaceVariantText
        wrapMode: Text.WordWrap
    }

    actions: [
        DButton {
            text: I18n.tr("Cancel", "dialog dismiss button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: root.opened = false
        },
        DButton {
            text: I18n.tr("Create", "new playlist dialog confirm button")
            enabled: root.valid
            onClicked: root.create()
        }
    ]
}
