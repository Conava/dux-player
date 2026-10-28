#!/usr/bin/env python3
"""Builds Modules/Settings/SettingsSearchIndex.js, the settings search index, from the settings pages.

Every row, card and section label whose title is a literal I18n.tr call (or a plain string, for brand names)
becomes an entry; its description, option and button labels, card title and `keywords` are searchable with it.
A row with `searchable: false` is left out. `--check` exits 1 when the committed index is stale.
"""
import json
import re
import sys
from pathlib import Path

SETTINGS = Path(__file__).resolve().parent.parent / "Modules" / "Settings"
OUTPUT = SETTINGS / "SettingsSearchIndex.js"

# page id → file, in sidebar order
PAGES = {
    "libraries": "LibrariesSettings.qml",
    "general": "GeneralSettings.qml",
    "appearance": "AppearanceSettings.qml",
    "pages": "PagesSettings.qml",
    "nowplaying": "NowPlayingSettings.qml",
    "playback": "PlaybackSettings.qml",
    "lyrics": "LyricsSettings.qml",
    "scrobble": "ScrobbleSettings.qml",
    "devices": "DevicesSettings.qml",
    "about": "AboutSettings.qml",
}

CARD = "SettingsCard"
# element type → (title property, description property)
ENTRIES = {
    "SettingsRow": ("title", "subtitle"),
    "SettingsFieldRow": ("title", "subtitle"),
    "SettingsLinkRow": ("title", "subtitle"),
    "SettingsToggleRow": ("text", "description"),
    "SettingsSliderRow": ("text", "description"),
    "SettingsButtonGroupRow": ("text", "description"),
    "SurfaceCard": ("title", "subtitle"),
    "SectionLabel": ("text", None),
    "DFab": ("text", None),
    CARD: ("title", None),
}
# properties of a row or anything inside it whose labels are searchable: option lists and button texts
OPTION_PROPS = ("model", "options", "text")

STRING = r'"((?:\\.|[^"\\])*)"'
TR = re.compile(r'I18n\.tr\(\s*' + STRING + r'\s*,\s*' + STRING + r'\s*\)')
ONLY_TR = re.compile(r'^\s*' + TR.pattern + r'\s*$')
LITERAL = re.compile(r'^\s*' + STRING + r'\s*$')
COMPONENT = re.compile(r'^\s*component\s+(\w+)\s*:\s*([A-Z][\w.]*)\s*\{')
ELEMENT = re.compile(r'^\s*(?:[a-z]\w*\s*:\s*)?([A-Z][\w.]*)\s*\{')
PROPERTY = re.compile(r'^\s*([a-z]\w*)\s*:\s*(.*)$')


def strip_line(line):
    """The line without its // comment, and the net change in {} and ([]) depth outside strings."""
    out, braces, parens, quote, i = [], 0, 0, None, 0
    while i < len(line):
        c = line[i]
        if quote:
            if c == "\\":
                out.append(line[i:i + 2])
                i += 2
                continue
            if c == quote:
                quote = None
        elif c in "\"'`":
            quote = c
        elif line.startswith("//", i):
            break
        elif c == "{":
            braces += 1
        elif c == "}":
            braces -= 1
        elif c in "([":
            parens += 1
        elif c in ")]":
            parens -= 1
        out.append(c)
        i += 1
    return "".join(out), braces, parens


def unquote(s):
    return json.loads('"' + s + '"')


def tr_pairs(value):
    return [[unquote(t), unquote(c)] for t, c in TR.findall(value)]


def label(value):
    """A [source, context] pair for a title that is one I18n.tr call, [text, ""] for a plain string, else None."""
    m = ONLY_TR.match(value)
    if m:
        return [unquote(m.group(1)), unquote(m.group(2))]
    m = LITERAL.match(value)
    return [unquote(m.group(1)), ""] if m and m.group(1) else None


def scan(page, source):
    """Entries of one page file in page order."""
    # inline components count as their base type (AppearanceSettings' Group is a card)
    aliases = {m.group(1): m.group(2) for m in map(COMPONENT.match, source.splitlines()) if m}
    entries, stack, depth, pending, opened = [], [], 0, None, 0
    for raw in source.splitlines():
        line, braces, parens = strip_line(raw)
        if pending:
            pending["value"] += " " + line
            pending["parens"] += parens
            if pending["parens"] <= 0:
                pending["frame"]["props"][pending["name"]] = pending["value"]
                pending = None
            depth += braces
            continue
        inner = stack[-1] if stack else None
        component = COMPONENT.match(line)
        element = ELEMENT.match(line) if not component else None
        prop = PROPERTY.match(line) if not (element or component) else None
        if prop and inner and depth == inner["depth"] + 1:
            name, value = prop.group(1), prop.group(2)
            if parens > 0:
                pending = {"frame": inner, "name": name, "value": value, "parens": parens}
            else:
                inner["props"][name] = value
        depth += braces
        if (element or component) and braces > 0:
            # a component definition is a template, not an entry of the page
            kind = "component" if component else aliases.get(element.group(1), element.group(1))
            stack.append({"type": kind, "depth": depth - braces, "props": {}, "options": [], "seq": opened})
            opened += 1
        while stack and depth <= stack[-1]["depth"]:
            close(page, stack.pop(), stack, entries)
    entries.sort(key=lambda e: e.pop("seq"))
    return entries


def close(page, frame, stack, entries):
    props = frame["props"]
    options = frame["options"] + [p for name in OPTION_PROPS if name in props for p in tr_pairs(props[name])]
    # a button outside any row ("Add entry" under the navigation list) is found by its label
    if frame["type"] == "DButton" and not any(f["type"] in ENTRIES for f in stack):
        frame["type"] = "DFab"
    if frame["type"] not in ENTRIES:
        # an element inside a row (a dropdown, a button) adds its labels to the row
        if stack:
            stack[-1]["options"].extend(options)
        return
    title_prop, description_prop = ENTRIES[frame["type"]]
    title = label(props.get(title_prop, ""))
    if not title or props.get("searchable", "").strip() == "false":
        return
    entry = {"seq": frame["seq"], "page": page, "title": title}
    card = next((f for f in reversed(stack) if f["type"] == CARD), None)
    section = label(card["props"].get("title", "")) if card else None
    if section:
        entry["section"] = section
    text = tr_pairs(props.get(description_prop, "")) if description_prop else []
    if frame["type"] != CARD:
        text += options
    text = [t for i, t in enumerate(text) if t != title and t not in text[:i]]
    if text:
        entry["text"] = text
    keywords = LITERAL.match(props.get("keywords", ""))
    if keywords:
        entry["keywords"] = unquote(keywords.group(1))
    entries.append(entry)


def merge(entries):
    """One entry per page, section and title: variants of a row (one per compositor, say) share their text."""
    out = {}
    for e in entries:
        key = json.dumps([e["page"], e.get("section"), e["title"]])
        if key not in out:
            out[key] = e
            continue
        kept = out[key]
        kept["text"] = kept.get("text", []) + [t for t in e.get("text", []) if t not in kept.get("text", [])]
        if not kept["text"]:
            del kept["text"]
        words = " ".join(filter(None, [kept.get("keywords"), e.get("keywords")]))
        if words:
            kept["keywords"] = " ".join(dict.fromkeys(words.split()))
    return list(out.values())


def build():
    entries = []
    for page, name in PAGES.items():
        entries += scan(page, (SETTINGS / name).read_text(encoding="utf-8"))
    body = ",\n".join("    " + json.dumps(e, ensure_ascii=False) for e in merge(entries))
    return (".pragma library\n\n"
            "// Generated by quickshell/scripts/settings_index.py (make settings-index) from the settings pages; don't edit.\n"
            "// title, section and text are [source, context] pairs for I18n.tr; an empty context is a plain string.\n\n"
            "var entries = [\n" + body + "\n];\n")


def main():
    text = build()
    if "--check" in sys.argv:
        if not OUTPUT.exists() or OUTPUT.read_text(encoding="utf-8") != text:
            print("Modules/Settings/SettingsSearchIndex.js is stale; run make settings-index")
            return 1
        return 0
    OUTPUT.write_text(text, encoding="utf-8")
    print(f"settings index: {text.count(chr(10) + '    {')} entries")
    return 0


if __name__ == "__main__":
    sys.exit(main())
