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

    readonly property var placements: ["auto", "keep", "overflow"]
    readonly property var optionKeys: Object.keys(schema).filter(k => Array.isArray(schema[k].values))

    signal edited(var instance)

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
        case "shape":
            return I18n.tr("Shape", "element option: the artwork's outline");
        case "rotate":
            return I18n.tr("Rotate artwork", "element option: the cookie-shaped artwork turns while playing");
        case "favorite":
            return UiConfig.elementLabel("favorite");
        case "face":
            return I18n.tr("Title style", "element option: the title's typeface");
        case "width":
            return I18n.tr("Width", "element option: how wide the track info is");
        case "smooth":
            return I18n.tr("Smooth highlight", "element option: the sung lyric fills gradually instead of word by word");
        case "hideInLyrics":
            return I18n.tr("Hide while lyrics are open", "element option: the lyric line goes while the lyrics pane shows");
        case "adaptive":
            return I18n.tr("Adaptive width", "element option: the track info fits the title instead of keeping a fixed width");
        }
        return key;
    }

    function isToggle(key) {
        const values = schema[key].values;
        return values.length === 2 && values.indexOf(true) >= 0 && values.indexOf(false) >= 0;
    }

    // why an option does nothing with the element's other options, or ""
    function blocker(key, options) {
        if (key === "rotate" && options?.shape !== "cookie")
            return I18n.tr("Only for the cookie shape", "why the rotate artwork option is unavailable");
        if (key === "smooth" && !UiConfig.lyricsWordSync)
            return I18n.tr("Turn on word by word in the lyrics settings", "why the smooth highlight option is unavailable");
        return "";
    }

    // what an option does, when its name doesn't say
    function hint(key) {
        if (key === "smooth")
            return I18n.tr("The sung line fills gradually instead of word by word.", "lyric line option description");
        return "";
    }

    function patched(patch) {
        const next = JSON.parse(JSON.stringify(instance));
        Object.assign(next, patch);
        return next;
    }

    SettingsCard {
        title: root.instance ? UiConfig.elementLabel(root.instance.type) : ""

        SettingsRow {
            visible: root.placeable
            title: I18n.tr("Placement", "element option: where an element goes when its row runs short")
            subtitle: I18n.tr("Auto moves it to ⋮ when space runs out.", "element placement description")

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

        TemplateRow {
            visible: root.schema.template !== undefined
            nowPlaying: true
            template: root.instance?.options?.template ?? ""
            onEdited: value => root.edited(root.patched({
                    "options": Object.assign({}, root.instance.options, {
                        "template": value
                    })
                }))
        }

        Repeater {
            model: root.optionKeys.filter(k => !root.isToggle(k))

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

        // on/off options; one that needs another option's value is greyed out with the reason
        Repeater {
            model: root.optionKeys.filter(k => root.isToggle(k))

            SettingsToggleRow {
                id: toggleRow
                required property string modelData
                readonly property string blocker: root.blocker(modelData, root.instance?.options)
                text: root.optionName(modelData)
                description: blocker !== "" ? blocker : root.hint(modelData)
                enabled: blocker === ""
                checked: root.instance?.options?.[modelData] ?? root.schema[modelData]?.default ?? false
                onToggled: value => {
                    const options = Object.assign({}, root.instance.options);
                    options[toggleRow.modelData] = value;
                    root.edited(root.patched({
                        "options": options
                    }));
                }
            }
        }
    }
}
