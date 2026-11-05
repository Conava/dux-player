import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets
import "../DCommon/Common/FocusNavigation.js" as FocusNavigation

// A connected group of icon buttons. Children are DActionGroup.Button items, or any item with a
// `groupPosition` property; the group sets it from the visual order ("start" is leftmost) and moves focus with Left/Right.
Row {
    id: root

    property var members: []

    // The connected look: outer ends fully round, inner corners small, a checked button fully round.
    component Button: DIconButton {
        property string groupPosition: "only"
        readonly property bool leftEnd: groupPosition === "start" || groupPosition === "only"
        readonly property bool rightEnd: groupPosition === "end" || groupPosition === "only"
        readonly property real outerRadius: Style.fullRadius(width, height)

        widthMode: "wide"
        variant: "tonal"
        // inner corners follow radius, so its Behavior animates press and toggle
        radius: pressed ? Style.cornerRadiusXS : (checked ? outerRadius : Style.cornerRadiusS)
        topLeftRadius: leftEnd ? outerRadius : radius
        bottomLeftRadius: leftEnd ? outerRadius : radius
        topRightRadius: rightEnd ? outerRadius : radius
        bottomRightRadius: rightEnd ? outerRadius : radius
        containerColor: checked ? Style.buttonBg : Style.secondaryContainer
        contentColor: checked ? Style.buttonText : Style.onSecondaryContainer
        Accessible.role: checkable ? Accessible.CheckBox : Accessible.Button
    }

    function updateMembers() {
        const shown = Array.from(children).filter(child => child.groupPosition !== undefined && child.visible).sort((a, b) => a.x - b.x);
        shown.forEach((item, i) => item.groupPosition = shown.length === 1 ? "only" : i === 0 ? "start" : i === shown.length - 1 ? "end" : "middle");
        members = shown;
    }

    spacing: Style.groupedListGap
    LayoutMirroring.enabled: I18n.isRtl
    Accessible.role: Accessible.Grouping
    onPositioningComplete: updateMembers()
    Keys.onPressed: event => event.accepted = FocusNavigation.handleHorizontalKey(event, root.members, false)
}
