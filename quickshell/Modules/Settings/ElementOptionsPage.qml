pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.DCommon.Widgets

// One element's options in a now-playing surface: its placement in a row that overflows, then its own options.
SettingsPage {
    id: root

    property var instance: null
    property bool placeable: false
    // the element type's options: {key: {values: [..] | null, default?}}
    property var schema: ({})
    property string backText: ""

    readonly property var placements: ["auto", "keep", "overflow"]
    readonly property var optionKeys: Object.keys(schema).filter(k => Array.isArray(schema[k].values))

    signal edited(var instance)
    signal back

    function placementLabel(placement) {
        switch (placement) {
        case "keep":
            return I18n.tr("Always shown", "element placement: never moved into the ⋮ menu");
        case "overflow":
            return I18n.tr("Always in ⋮", "element placement: always in the ⋮ menu");
        }
        return I18n.tr("Auto", "element placement: moved into ⋮ when the row runs short");
    }

    function optionName(key) {
        switch (key) {
        case "size":
            return I18n.tr("Size", "element option: how big the element is");
        }
        return key;
    }

    function patched(patch) {
        const next = JSON.parse(JSON.stringify(instance));
        Object.assign(next, patch);
        return next;
    }

    SettingsBackRow {
        text: root.backText
        onClicked: root.back()
    }

    SettingsCard {
        title: root.instance ? UiConfig.elementLabel(root.instance.type) : ""

        SettingsRow {
            visible: root.placeable
            title: I18n.tr("Placement", "element option: where an element goes when its row runs short")
            subtitle: I18n.tr("Auto moves it into ⋮ when the row runs out of space.", "element placement description")

            DDropdown {
                id: placementDropdown
                dropdownWidth: SettingsMetrics.dropdownWidth
                options: root.placements.map(p => root.placementLabel(p))
                optionValues: root.placements
                onValueChanged: value => root.edited(root.patched({
                        "placement": value
                    }))
            }

            Binding {
                target: placementDropdown
                property: "currentValue"
                value: root.instance?.placement ?? "auto"
            }
        }

        Repeater {
            model: root.optionKeys

            SettingsButtonGroupRow {
                id: optionRow
                required property string modelData
                readonly property var values: root.schema[modelData]?.values ?? []
                text: root.optionName(modelData)
                model: values.map(v => UiConfig.optionLabel(v))
                currentIndex: values.indexOf(root.instance?.options?.[modelData] ?? root.schema[modelData]?.default)
                onSelectionChanged: (index, selected) => {
                    if (!selected)
                        return;
                    const options = Object.assign({}, root.instance.options);
                    options[optionRow.modelData] = optionRow.values[index];
                    root.edited(root.patched({
                        "options": options
                    }));
                }
            }
        }
    }
}
