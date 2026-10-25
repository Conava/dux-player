# Translations

Same pipeline as Dux Shell and Dux Files: strings are wrapped in `I18n.tr(term, context)` in QML, extracted to a POEditor-compatible `en.json`, and translated exports go into `poexports/` as `<locale>.json` files.

## Adding strings

```qml
text: I18n.tr("New playlist", "action and dialog that create a playlist")
```

- The second argument tells translators where and how the term is used. A term is translated once, whatever its contexts, so reuse an existing term with the same context where it reads right (search `en.json`).
- Placeholders use `%1`/`%2` with `.arg()`; never build a sentence from pieces. Counts get a "1 …" term and a "%1 …" term (`Common/Phrases.qml` has the common ones).
- Strings under `DCommon/` belong to dux-qml-common.

## Extracting

```sh
make i18n-extract    # or: python3 quickshell/translations/extract_translations.py
```

Regenerates `en.json` (the POEditor source) and `template.json` (an empty template). Run it after adding or changing strings; never edit them by hand.
