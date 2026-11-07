pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets
import qs.Modules.Settings
import "../Services/Rules.js" as Rules
import "../Common/Layouts.js" as Layouts

// A list's filter rules, one per row. Dropdowns apply at once, typed values after a pause or on Enter.
Column {
    id: root

    // the browse type whose fields apply
    property string type: "tracks"
    // {match, list}, as stored
    property var rules: ({})
    // JSON of {kind, itemId, allScope, view}: the page these rules belong to. A change drops a typed value
    // still pending, so it can't be saved to the page navigated to
    property string target: "{}"
    onTargetChanged: discard()
    // where the value picker opens
    property Item sheetHost: null
    // called with the new rules; a var signal argument would turn the lists into something Array.isArray refuses
    property var apply: null

    // rules with a typed value not applied yet
    property var draft: null
    readonly property var shown: draft ?? rules
    readonly property var list: shown.list ?? []
    readonly property var fieldKeys: Rules.fields(type)
    readonly property var fieldNames: fieldKeys.map(f => fieldName(f))
    // the row's two dropdowns, a value of the same width and its remove button side by side
    readonly property bool narrow: width < SettingsMetrics.dropdownWidth * 3 + Theme.iconButtonSize + Theme.spacingS * 3 + SettingsMetrics.rowPaddingH * 2
    // the row whose value picker is open
    property int pickIndex: -1
    readonly property var pickRule: list[pickIndex] ?? null

    function fieldName(field) {
        return field === "favorite" ? UiConfig.lineLabel(field) : UiConfig.sortLabel(field);
    }

    function opName(op) {
        switch (op) {
        case "contains":
            return I18n.tr("Contains", "filter rule condition");
        case "notContains":
            return I18n.tr("Doesn't contain", "filter rule condition");
        case "is":
            return I18n.tr("Is", "filter rule condition");
        case "isNot":
            return I18n.tr("Is not", "filter rule condition");
        case "startsWith":
            return I18n.tr("Starts with", "filter rule condition");
        case "isEmpty":
            return I18n.tr("Is empty", "filter rule condition");
        case "isNotEmpty":
            return I18n.tr("Is not empty", "filter rule condition");
        case "isAny":
            return I18n.tr("Is one of", "filter rule condition");
        case "isNone":
            return I18n.tr("Is none of", "filter rule condition");
        case "atLeast":
            return I18n.tr("At least", "filter rule condition");
        case "atMost":
            return I18n.tr("At most", "filter rule condition");
        case "between":
            return I18n.tr("Between", "filter rule condition");
        case "inLast":
            return I18n.tr("In the last", "filter rule condition, followed by a number of days");
        case "notInLast":
            return I18n.tr("Not in the last", "filter rule condition, followed by a number of days");
        case "never":
            return I18n.tr("Never", "filter rule condition: a track that was never played");
        }
        return op;
    }

    function pickName(field, value) {
        switch (field) {
        case "genre":
            return PlayerService.genre(value)?.name ?? value;
        case "library":
            return PlayerService.libraryName(value) || value;
        }
        return value;
    }

    // what a picker offers; a pick that left the catalog stays, so it can be unticked
    function pickChoices(rule) {
        if (!rule)
            return [];
        const known = rule.field === "genre" ? PlayerService.genres.map(g => g.id) : rule.field === "library" ? PlayerService.libraries.map(l => l.id) : CatalogService.trackFormats();
        const gone = Rules.picked(rule.value).filter(v => known.indexOf(v) < 0);
        return known.concat(gone).map(v => ({
                    "label": pickName(rule.field, v),
                    "value": v
                }));
    }

    function withList(next) {
        return Object.assign({}, shown, {
            "list": next
        });
    }

    function withRule(index, rule) {
        return withList(list.map((r, i) => i === index ? rule : r));
    }

    function withValue(rule, value) {
        const next = Object.assign({}, rule);
        if (value === undefined)
            delete next.value;
        else
            next.value = value;
        return next;
    }

    function commit(next) {
        applyDelay.stop();
        apply(next);
        draft = null;
    }

    function discard() {
        applyDelay.stop();
        draft = null;
    }

    function flush() {
        if (draft !== null)
            commit(draft);
    }

    function stage(next) {
        draft = next;
        applyDelay.restart();
    }

    function setMatch(match) {
        commit(Object.assign({}, shown, {
            "match": match
        }));
    }

    function addRule() {
        commit(withList(list.concat([Rules.newRule(type)])));
    }

    function removeRule(index) {
        const before = shown;
        const at = JSON.parse(target);
        const name = fieldKeys.indexOf(list[index].field) >= 0 ? fieldName(list[index].field) : list[index].field;
        Router.askRemove(I18n.tr("Remove %1?", "remove an element from a now-playing surface confirmation title, %1 is the element").arg(name), "", () => {
            root.commit(root.withList(root.list.filter((_, i) => i !== index)));
            Router.offerUndo(I18n.tr("Removed %1", "toast after removing an element from a now-playing surface, %1 is the element").arg(name), UiConfig.optionsUndo(at, {
                "rules": before
            }));
        });
    }

    function togglePick(value) {
        const rule = list[pickIndex];
        const picks = Rules.picked(rule.value);
        const next = picks.indexOf(value) >= 0 ? picks.filter(v => v !== value) : picks.concat([value]);
        commit(withRule(pickIndex, withValue(rule, next.length > 0 ? next : undefined)));
    }

    spacing: Theme.spacingS
    Component.onDestruction: flush()

    Timer {
        id: applyDelay
        interval: PlayerMetrics.ruleEditDelay
        onTriggered: root.flush()
    }

    SettingsGroup {
        width: parent.width
        visible: root.list.length > 0

        SettingsButtonGroupRow {
            visible: root.list.length > 1
            text: I18n.tr("Match", "filter rules: whether all rules or any rule must hold")
            model: [I18n.tr("All rules", "filter rules: every rule must hold"), I18n.tr("Any rule", "filter rules: one rule is enough")]
            currentIndex: root.shown.match === "any" ? 1 : 0
            onSelectionChanged: (index, selected) => {
                if (selected)
                    root.setMatch(index === 1 ? "any" : "all");
            }
        }

        // counted, not the array: a new array would rebuild the rows and drop the focus of a field being typed in
        Repeater {
            model: root.list.length

            SettingsRow {
                id: row
                required property int index
                readonly property var rule: root.list[index] ?? ({})
                readonly property string shape: Rules.shape(root.type, rule)
                readonly property bool known: shape !== ""
                readonly property bool stars: shape === "number" && rule.field === "rating"
                readonly property bool typed: shape === "text" || shape === "days" || shape === "range" || (shape === "number" && !stars)
                readonly property var ops: Rules.ops(root.type, rule.field ?? "")
                readonly property bool numeric: typed && shape !== "text"
                readonly property string unit: {
                    if (shape === "days")
                        return I18n.tr("days", "filter rule value unit");
                    return numeric && rule.field === "duration" ? I18n.tr("minutes", "filter rule value unit") : "";
                }
                readonly property real dropdownWidth: root.narrow ? (grid.width - removeButton.width - grid.columnSpacing * 2) / 2 : SettingsMetrics.dropdownWidth

                function numberOf(text) {
                    return text === "" || isNaN(Number(text)) ? null : Number(text);
                }

                function typedValue() {
                    if (shape === "text")
                        return valueField.text === "" ? undefined : valueField.text;
                    if (shape === "range")
                        return valueField.text === "" && upperField.text === "" ? undefined : [numberOf(valueField.text), numberOf(upperField.text)];
                    return numberOf(valueField.text) ?? undefined;
                }

                // DTextField also reports text set from the stored value; only a focused field is being typed in
                function stageTyped() {
                    if (valueField.inputFocused || upperField.inputFocused)
                        root.stage(root.withRule(index, root.withValue(rule, typedValue())));
                }

                function shownText(end) {
                    const v = shape === "range" ? Rules.picked(rule.value)[end] : end === 0 ? rule.value : undefined;
                    return v === undefined || v === null || Array.isArray(v) ? "" : String(v);
                }

                focusPolicy: Qt.StrongFocus
                Accessible.name: known ? root.fieldName(rule.field) : (rule.field ?? "")
                Keys.onPressed: event => {
                    if (event.key !== Qt.Key_Delete && event.key !== Qt.Key_Backspace)
                        return;
                    if (valueField.inputFocused || upperField.inputFocused)
                        return;
                    root.removeRule(index);
                    event.accepted = true;
                }

                body: Item {
                    width: parent.width
                    height: grid.implicitHeight

                    FocusRing {
                        radius: Theme.groupedListInnerRadius
                        visible: row.visualFocus
                    }

                    GridLayout {
                        id: grid
                        anchors.fill: parent
                        columnSpacing: Theme.spacingS
                        rowSpacing: Theme.spacingS

                        Column {
                            visible: !row.known
                            Layout.row: 0
                            Layout.column: 0
                            Layout.columnSpan: root.narrow ? 2 : 3
                            Layout.fillWidth: true
                            opacity: Theme.disabledOpacity

                            StyledText {
                                width: parent.width
                                text: row.rule.field ?? ""
                                font.pixelSize: Theme.fontSizeMedium
                                elide: Text.ElideRight
                                wrapMode: Text.NoWrap
                                horizontalAlignment: Text.AlignLeft
                            }

                            StyledText {
                                width: parent.width
                                text: I18n.tr("Not available in this version", "an element saved by a newer version of the app")
                                font.pixelSize: Theme.fontSizeSmall
                                color: Theme.surfaceVariantText
                                horizontalAlignment: Text.AlignLeft
                            }
                        }

                        DDropdown {
                            id: fieldDropdown
                            visible: row.known
                            Layout.row: 0
                            Layout.column: 0
                            Layout.preferredWidth: row.dropdownWidth
                            dropdownWidth: row.dropdownWidth
                            triggerHeight: Theme.fieldHeight
                            options: root.fieldNames
                            optionValues: root.fieldKeys
                            onValueChanged: value => root.commit(root.withRule(row.index, Rules.withField(root.type, row.rule, value)))
                        }

                        Binding {
                            target: fieldDropdown
                            property: "currentValue"
                            value: row.rule.field ?? ""
                        }

                        DDropdown {
                            id: opDropdown
                            visible: row.known
                            Layout.row: 0
                            Layout.column: 1
                            Layout.preferredWidth: row.dropdownWidth
                            dropdownWidth: row.dropdownWidth
                            triggerHeight: Theme.fieldHeight
                            options: row.ops.map(o => root.opName(o))
                            optionValues: row.ops
                            onValueChanged: value => root.commit(root.withRule(row.index, Rules.withOp(root.type, row.rule, value)))
                        }

                        Binding {
                            target: opDropdown
                            property: "currentValue"
                            value: row.rule.op ?? ""
                        }

                        RowLayout {
                            visible: row.known && (!root.narrow || row.shape !== "none")
                            Layout.row: root.narrow ? 1 : 0
                            Layout.column: root.narrow ? 0 : 2
                            Layout.columnSpan: root.narrow ? 3 : 1
                            Layout.fillWidth: true
                            Layout.minimumHeight: Theme.fieldHeight
                            spacing: Theme.spacingS

                            DTextField {
                                id: valueField
                                visible: row.typed
                                Layout.fillWidth: true
                                outlined: true
                                controlHeight: Theme.fieldHeight
                                maximumLength: 256
                                validator: row.numeric ? numberValidator : null
                                onTextEdited: row.stageTyped()
                                onAccepted: root.flush()
                            }

                            StyledText {
                                visible: row.shape === "range"
                                text: "–"
                                color: Theme.surfaceVariantText
                            }

                            DTextField {
                                id: upperField
                                visible: row.shape === "range"
                                Layout.fillWidth: true
                                outlined: true
                                controlHeight: Theme.fieldHeight
                                validator: numberValidator
                                onTextEdited: row.stageTyped()
                                onAccepted: root.flush()
                            }

                            StyledText {
                                visible: row.unit !== ""
                                text: row.unit
                                font.pixelSize: Theme.fontSizeSmall
                                color: Theme.surfaceVariantText
                            }

                            // looks like the dropdowns beside it, but opens the multi-select sheet
                            StyledButton {
                                id: pickButton
                                readonly property string picks: (row.shape === "list" ? Rules.picked(row.rule.value) : []).map(v => root.pickName(row.rule.field, v)).join(", ")
                                visible: row.shape === "list"
                                Layout.fillWidth: true
                                Layout.preferredHeight: Theme.fieldHeight
                                radius: fieldDropdown.triggerRadius
                                color: hovered ? fieldDropdown.hoverBackgroundColor : fieldDropdown.backgroundColor
                                border.width: Theme.outlineWidth
                                border.color: visualFocus ? fieldDropdown.focusedBorderColor : fieldDropdown.normalBorderColor
                                Accessible.name: picks !== "" ? picks : pickLabel.text
                                onClicked: {
                                    root.pickIndex = row.index;
                                    picker.opened = true;
                                }

                                StyledText {
                                    id: pickLabel
                                    anchors.left: parent.left
                                    anchors.right: pickArrow.left
                                    anchors.leftMargin: Theme.spacingM
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: pickButton.picks !== "" ? pickButton.picks : I18n.tr("Choose…", "filter rule: opens the list of values")
                                    font.pixelSize: Theme.fontSizeMedium
                                    color: pickButton.picks !== "" ? Theme.surfaceText : Theme.onSurfaceVariant
                                    elide: Text.ElideRight
                                    wrapMode: Text.NoWrap
                                    horizontalAlignment: Text.AlignLeft
                                }

                                DIcon {
                                    id: pickArrow
                                    anchors.right: parent.right
                                    anchors.rightMargin: Theme.spacingS
                                    anchors.verticalCenter: parent.verticalCenter
                                    name: "arrow_drop_down"
                                    size: Theme.iconSize
                                    color: Theme.onSurfaceVariant
                                }
                            }

                            DRatingBar {
                                visible: row.stars
                                rating: typeof row.rule.value === "number" ? row.rule.value : 0
                                onRated: stars => root.commit(root.withRule(row.index, root.withValue(row.rule, stars)))
                            }

                            DButtonGroup {
                                visible: row.shape === "flag"
                                size: "small"
                                model: [I18n.tr("Yes", "details value"), I18n.tr("No", "details value")]
                                currentIndex: row.rule.value === true ? 0 : row.rule.value === false ? 1 : -1
                                onSelectionChanged: (index, selected) => {
                                    if (selected)
                                        root.commit(root.withRule(row.index, root.withValue(row.rule, index === 0)));
                                }
                            }

                            Item {
                                visible: row.stars || row.shape === "flag" || row.shape === "none"
                                Layout.fillWidth: true
                            }
                        }

                        DIconButton {
                            id: removeButton
                            Layout.row: 0
                            Layout.column: root.narrow ? 2 : 3
                            Layout.alignment: Qt.AlignTop
                            iconName: "close"
                            tooltipText: I18n.tr("Remove", "button that removes an item")
                            Accessible.name: tooltipText
                            onClicked: root.removeRule(row.index)
                        }
                    }
                }

                Binding {
                    target: valueField
                    property: "text"
                    value: row.shownText(0)
                    when: !valueField.inputFocused
                    restoreMode: Binding.RestoreNone
                }

                Binding {
                    target: upperField
                    property: "text"
                    value: row.shownText(1)
                    when: !upperField.inputFocused
                    restoreMode: Binding.RestoreNone
                }
            }
        }
    }

    RegularExpressionValidator {
        id: numberValidator
        regularExpression: /[0-9]{0,9}([.][0-9]{0,2})?/
    }

    DButton {
        anchors.horizontalCenter: parent.horizontalCenter
        visible: Layouts.canAddRule(root.list)
        text: I18n.tr("Add rule", "button under the filter rules in a page's edit mode")
        iconName: "add"
        backgroundColor: Theme.secondaryContainer
        textColor: Theme.onSecondaryContainer
        onClicked: root.addRule()
    }

    PickerSheet {
        id: picker
        parent: root.sheetHost
        multiple: true
        title: root.pickRule ? root.fieldName(root.pickRule.field) : ""
        chosen: Rules.picked(root.pickRule?.value)
        sections: [
            {
                "title": root.pickRule ? root.opName(root.pickRule.op) : "",
                "items": opened ? root.pickChoices(root.pickRule) : []
            }
        ]
        onPicked: value => root.togglePick(value)
    }
}
