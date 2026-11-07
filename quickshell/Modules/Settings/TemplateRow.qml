import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// A text template: the field, a preview on the current track and a menu of fields to insert at the cursor.
SettingsRow {
    id: root

    property string template: ""
    // offers the fields only the playing track has
    property bool nowPlaying: false

    readonly property var sample: PlayerService.current ?? CatalogService.visibleTracks()[0] ?? null
    readonly property var fieldNames: UiConfig.templateFieldNames.filter(f => nowPlaying || f !== "playingfrom")
    readonly property string preview: UiConfig.templateText(template, sample, nowPlaying)

    signal edited(string value)

    title: I18n.tr("Template", "template field: the text with its %field% placeholders")

    function insert(field) {
        const at = input.cursorPosition;
        const token = "%" + field + "%";
        input.text = input.text.slice(0, at) + token + input.text.slice(at);
        input.cursorPosition = at + token.length;
    }

    body: Column {
        width: parent.width
        spacing: Theme.spacingS

        DTextField {
            id: input
            width: parent.width
            outlined: true
            maximumLength: 256
            onTextEdited: {
                if (text !== root.template)
                    root.edited(text);
            }
        }

        StyledText {
            width: parent.width
            visible: root.preview !== ""
            text: I18n.tr("Preview: %1", "template field: the text on the current track").arg(root.preview)
            font.pixelSize: Theme.fontSizeSmall
            color: Theme.surfaceVariantText
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
        }

        DDropdown {
            id: insertField
            dropdownWidth: SettingsMetrics.dropdownWidth
            emptyText: I18n.tr("Insert field", "template field: menu of track fields")
            options: root.fieldNames.map(f => UiConfig.templateFieldLabel(f))
            optionValues: root.fieldNames
            onValueChanged: value => {
                root.insert(value);
                currentValue = "";
            }
        }
    }

    // a typing user keeps the field; the stored value comes back when it loses focus
    Binding {
        target: input
        property: "text"
        value: root.template
        when: !input.inputFocused
        restoreMode: Binding.RestoreNone
    }
}
