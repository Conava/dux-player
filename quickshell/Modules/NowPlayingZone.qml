pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets
import "../Common/Overflow.js" as Overflow
import "../DCommon/Common/FocusNavigation.js" as FocusNavigation

// One zone of a now-playing surface: its shown element instances in a row, runs of track info stacked into one
// block (a column zone is a single block). With `room` set, what doesn't fit moves into a ⋮ menu at the end. With
// `fill` the transport shares the width (play wider). The group look joins the buttons (and ⋮) into connected runs,
// broken by spacers and dividers.
RowLayout {
    id: root

    // [{id, type, hidden, placement, options}]
    property var instances: []
    property string context: "bar"
    property string size: "s"
    property string look: "plain"
    // the transport's button height when it isn't the size's (Bento's deck)
    property real buttonHeight: 0
    // per element: the plain look for everything but the transport
    property bool transportOnly: false
    property bool fill: false
    property string textLook: "bar"
    property bool centered: false
    property bool column: false
    // the track info block opens the full view (bar, side panel)
    property bool opensView: false
    // the width this zone may take before elements move into its ⋮ menu; below 0 it never overflows
    property real room: -1

    readonly property var textTypes: ["title", "artists", "album", "libraryBadge", "playingFrom", "castTarget"]
    readonly property var track: PlayerService.current
    readonly property var shown: instances.filter(i => !i.hidden && UiConfig.isKnown(i.type))
    // a string, so a resize that moves nothing doesn't rebuild the row
    readonly property string overflowKeys: room < 0 ? "" : Overflow.overflow(shown.map(i => ({
                "key": i.id,
                "type": i.type,
                "placement": i.placement,
                "width": elementWidth(i)
            })), room, moreButton.implicitWidth, spacing).join(" ")
    readonly property var inMenu: overflowKeys === "" ? [] : shown.filter(i => overflowKeys.split(" ").indexOf(i.id) >= 0)
    // one entry per type; spacers and dividers just go
    readonly property var menuTypes: inMenu.map(i => i.type).filter((t, n, all) => all.indexOf(t) === n && t !== "spacer" && t !== "divider")
    readonly property var inRow: inMenu.length === 0 ? shown : shown.filter(i => inMenu.indexOf(i) < 0)
    // [{key, stack, items}]
    readonly property var segments: {
        const out = [];
        for (const inst of inRow) {
            const text = column || textTypes.indexOf(inst.type) >= 0;
            const last = out[out.length - 1];
            if (text && last?.stack)
                last.items.push(inst);
            else
                out.push({
                    "key": inst.id,
                    "stack": text,
                    "items": [inst]
                });
        }
        return out;
    }
    // the group look: each shown segment's place in its run of buttons, by key; "more" is the ⋮ button
    readonly property var groupPositions: {
        if (look !== "group")
            return ({});
        const out = {};
        const runs = [[]];
        const keys = segments.filter(s => s.items.some(i => shows(i.type))).map(s => s.items[0].type === "spacer" || s.items[0].type === "divider" ? "" : s.key);
        for (const key of keys.concat(menuTypes.length > 0 ? ["more"] : [])) {
            if (key === "")
                runs.push([]);
            else
                runs[runs.length - 1].push(key);
        }
        for (const run of runs)
            run.forEach((key, i) => out[key] = run.length === 1 ? "only" : i === 0 ? "start" : i === run.length - 1 ? "end" : "middle");
        return out;
    }
    // the bar's track info block: the title's size option, or with adaptive width as much of it as the text needs
    readonly property var titleOptions: instances.find(i => i.type === "title")?.options ?? {}
    readonly property real infoMaxWidth: PlayerMetrics.barInfoWidths[titleOptions.width ?? "m"]
    readonly property real infoTextWidth: Math.max(titleSize.implicitWidth + (titleOptions.favorite ? Theme.buttonHeightXS + Theme.spacingXS : 0), artistSize.implicitWidth)
    readonly property real infoWidth: titleOptions.adaptive === true ? Math.max(PlayerMetrics.barInfoMinWidth, Math.min(infoMaxWidth, infoTextWidth)) : infoMaxWidth
    // how much narrower the bar's track info blocks get before anything moves into the menu
    readonly property real shrinkable: textLook === "bar" && !column ? segments.filter(s => s.stack).length * (infoWidth - PlayerMetrics.barInfoMinWidth) : 0

    // a cast target only while casting, an album only when the track has one
    function shows(type) {
        switch (type) {
        case "castTarget":
            return PlayerService.casting;
        case "lyrics":
            return UiConfig.lyricsOn;
        case "playingFrom":
        case "title":
            return track !== null;
        case "artists":
            return (track?.artists ?? []).length > 0;
        case "album":
            return (track?.album ?? "") !== "";
        case "libraryBadge":
            return (track?.library ?? "") !== "";
        }
        return true;
    }

    // what an element needs in a row, from the same tokens the elements size by
    function elementWidth(inst) {
        const button = size === "m" ? Theme.buttonHeightM : Theme.buttonHeightS;
        switch (inst.type) {
        case "volume":
            return UiConfig.styleOf(context) === "m3e" ? button : Theme.buttonHeightXS + (Theme.outlineWidthFocused + Theme.dividerWidth + Theme.spacingXXS) * 2;
        case "rating":
            return Theme.buttonHeightXXS * 5;
        case "cover":
            return PlayerMetrics.barArtSize;
        case "device":
            return look === "row" ? Math.round(room * PlayerMetrics.mediaDeviceNameWidthRatio) : button;
        case "seekbar":
            return PlayerMetrics.barSeekMinWidth;
        case "title":
            return PlayerMetrics.barInfoMinWidth;
        case "spacer":
            return ({
                    "s": Theme.spacingS,
                    "m": Theme.spacingL,
                    "l": Theme.spacingXL
                })[inst.options?.size] ?? 0;
        case "divider":
            return Theme.dividerWidth + Theme.spacingS * 2;
        }
        if (textTypes.indexOf(inst.type) >= 0)
            return 0;
        if (fill)
            return Theme.iconSizeMedium + Theme.spacingXS * 2;
        if (look === "group")
            return groupProbe.width;
        if (look === "transport" && inst.type === "playPause")
            return Math.round(button * PlayerMetrics.mediaPlayWidthRatio);
        return button;
    }

    function menuEntry(type) {
        const entry = {
            "id": type,
            "text": UiConfig.elementLabel(type),
            "icon": UiConfig.elementIcon(type)
        };
        switch (type) {
        case "dock":
            entry.text = Router.dockMode === "side" ? I18n.tr("Move to bottom bar", "button moving now playing out of the sidebar") : entry.text;
            entry.icon = Router.dockMode === "side" ? "bottom_panel_open" : "right_panel_open";
            break;
        case "shuffle":
            entry.active = PlayerService.shuffle;
            break;
        case "repeat":
            entry.active = PlayerService.repeat !== "off";
            entry.text = Phrases.repeatState(PlayerService.repeat);
            entry.icon = PlayerService.repeat === "one" ? "repeat_one" : "repeat";
            break;
        case "lyrics":
        case "queue":
            entry.active = context === "bar" ? Router.panel === type : context === "side" ? Router.sideTab === type : Router.viewPane === type;
            break;
        case "sleep":
            entry.active = PlayerService.sleepMode !== "off";
            break;
        case "speed":
            entry.active = PlayerService.effectiveSpeed !== 1;
            break;
        case "favorite":
            entry.active = CatalogService.withMarks(track)?.favorite === true;
            entry.enabled = track !== null && PlayerService.whyNot("favorite", track) === "";
            break;
        case "rating":
            entry.description = PlayerService.whyNot("rate", track);
            entry.enabled = track !== null && entry.description === "";
            break;
        case "goToAlbum":
            entry.enabled = (track?.albumId ?? "") !== "";
            break;
        case "previous":
        case "next":
        case "addToPlaylist":
        case "goToArtist":
        case "details":
            entry.enabled = track !== null;
            break;
        }
        return entry;
    }

    // has something to show; read this rather than visible, which also follows the parents
    readonly property bool populated: segments.length > 0 || menuTypes.length > 0

    spacing: look === "group" ? Theme.groupedListGap : Theme.spacingS
    visible: populated
    LayoutMirroring.enabled: false
    LayoutMirroring.childrenInherit: true

    // Left and Right move along a button group; Tab leaves it
    Keys.enabled: look === "group"
    Keys.onPressed: event => {
        const buttons = Array.from(children).filter(c => c.visible && (c === moreButton || c.element)).sort((a, b) => a.x - b.x);
        event.accepted = FocusNavigation.handleHorizontalKey(event, buttons.map(c => c === moreButton ? c : c.element.item), false);
    }

    // a wide icon button, the width every group member takes
    DIconButton {
        id: groupProbe
        visible: false
        size: root.size
        buttonSize: root.buttonHeight > 0 ? root.buttonHeight : (root.size === "m" ? Theme.buttonHeightM : Theme.buttonHeightS)
        widthMode: "wide"
    }

    component Element: NowPlayingElement {
        context: root.context
        size: root.size
        buttonHeight: root.buttonHeight
        groupWidth: groupProbe.width
        look: root.transportOnly && !transportId ? "plain" : root.look
        textLook: root.textLook
        centered: root.centered
        fill: root.fill
        weight: elementId === "playPause" ? PlayerMetrics.mediaPlayWidthRatio : 1
        shown: root.shows(elementId)
        rowWidth: root.width
    }

    // track info, one line each; the bar's block shrinks before buttons move into the menu
    component TextStack: Item {
        id: stack

        property var items: []
        readonly property bool bar: root.textLook === "bar" && !root.column
        readonly property bool shown: items.some(i => root.shows(i.type))
        readonly property real preferredWidth: bar ? root.infoWidth : -1
        readonly property real minimumWidth: bar ? PlayerMetrics.barInfoMinWidth : -1
        readonly property real padding: root.opensView ? Theme.spacingS : 0

        implicitWidth: bar ? root.infoWidth : lines.implicitWidth
        implicitHeight: lines.implicitHeight + padding * 2

        StyledButton {
            id: opener
            anchors.fill: parent
            visible: root.opensView
            radius: Theme.cornerRadiusM
            color: "transparent"
            Accessible.name: I18n.tr("Open now playing", "now playing bar button")
            onClicked: Router.openNowPlaying("")

            StateLayer {
                control: opener
                stateColor: ArtColors.text
            }
        }

        Column {
            id: lines
            x: stack.padding
            width: parent.width - stack.padding * 2
            anchors.verticalCenter: parent.verticalCenter
            spacing: root.textLook === "bar" ? Theme.spacingXXS : Theme.spacingXS

            Repeater {
                model: stack.items

                Element {
                    required property var modelData
                    width: lines.width
                    elementId: modelData.type
                    options: modelData.options ?? {}
                    inColumn: true
                }
            }
        }
    }

    StyledText {
        id: titleSize
        visible: false
        text: root.titleOptions.adaptive === true ? PlayerService.current?.title ?? "" : ""
        font.pixelSize: Theme.fontSizeMedium
        font.weight: Theme.fontWeightMedium
        wrapMode: Text.NoWrap
        elide: Text.ElideNone
    }

    StyledText {
        id: artistSize
        visible: false
        text: root.titleOptions.adaptive === true ? PlayerService.current?.artist ?? "" : ""
        font.pixelSize: Theme.fontSizeSmall
        wrapMode: Text.NoWrap
        elide: Text.ElideNone
    }

    Repeater {
        model: root.segments

        // a row lays out only its own children, so the wrapper carries the element's layout wishes
        Loader {
            id: segment

            required property var modelData

            readonly property NowPlayingElement element: item as NowPlayingElement
            readonly property TextStack stack: item as TextStack

            visible: element ? element.shown : stack ? stack.shown : item !== null
            sourceComponent: modelData.stack ? stackComponent : elementComponent
            Layout.fillWidth: element ? element.fillsWidth : item !== null
            Layout.preferredWidth: element ? element.preferredWidth : stack ? stack.preferredWidth : -1
            Layout.minimumWidth: element ? element.minimumWidth : stack ? stack.minimumWidth : -1
            Layout.maximumWidth: stack && stack.preferredWidth > 0 ? stack.preferredWidth : Number.POSITIVE_INFINITY
            Layout.alignment: Qt.AlignVCenter

            Component {
                id: stackComponent
                TextStack {
                    items: segment.modelData.items
                }
            }

            Component {
                id: elementComponent
                Element {
                    groupPosition: root.groupPositions[segment.modelData.key] ?? "only"
                    elementId: segment.modelData.items[0].type
                    options: segment.modelData.items[0].options ?? {}
                }
            }
        }
    }

    DIconButton {
        id: moreButton
        readonly property bool grouped: root.look === "group"
        readonly property string groupPosition: root.groupPositions.more ?? "only"
        visible: root.menuTypes.length > 0
        size: root.size
        buttonSize: groupProbe.buttonSize
        widthMode: grouped ? "wide" : "default"
        variant: grouped ? "tonal" : "standard"
        containerColor: ArtColors.accentContainer
        contentColor: ArtColors.onAccentContainer
        radius: grouped ? (pressed ? Theme.cornerRadiusXS : Theme.cornerRadiusS) : pressed ? Theme.buttonRadius(width, height, buttonSize, true, false) : roundRadius
        topLeftRadius: grouped && (groupPosition === "start" || groupPosition === "only") ? roundRadius : radius
        bottomLeftRadius: topLeftRadius
        topRightRadius: grouped && (groupPosition === "end" || groupPosition === "only") ? roundRadius : radius
        bottomRightRadius: topRightRadius
        iconName: "more_vert"
        iconColor: grouped ? contentColor : ArtColors.textVariant
        tooltipText: I18n.tr("More", "track row menu button")
        Accessible.name: tooltipText
        onClicked: moreMenu.open(moreButton)

        DMenu {
            id: moreMenu
            minimumWidth: 0
            model: root.menuTypes.map(type => root.menuEntry(type))
            onTriggered: id => {
                switch (id) {
                case "volume":
                    volumePanel.show();
                    return;
                case "rating":
                    // after this menu's popup is gone, so the next one can take the grab
                    Qt.callLater(() => ratingMenu.openLike(moreMenu));
                    return;
                }
                Router.runElement(id, root.context);
            }
        }

        DMenu {
            id: ratingMenu
            readonly property int rating: root.track?.rating ?? 0
            model: [1, 2, 3, 4, 5].map(n => ({
                        "id": String(n),
                        "text": I18n.tr("%1 of %2 stars", "Value of a star rating control").arg(n).arg(5),
                        "icon": "star",
                        "iconFilled": n <= rating,
                        "checked": n === rating
                    }))
            onTriggered: id => PlayerService.setRating("track", root.track, Number(id) === rating ? 0 : Number(id))
        }

        VolumePanel {
            id: volumePanel
            anchorItem: moreButton
        }
    }
}
