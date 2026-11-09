#!/usr/bin/env bash
# Runs the real daemon, window and mpv in a nested Hyprland, invisible on the desktop and with no access to the
# developer's session, config or audio. usage: run.sh [scenario.sh]  (default: smoke.sh; see lib.sh for its helpers)
set -u
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
scenario=$(realpath "${1:-$here/smoke.sh}")
# Short on purpose: Hyprland's socket path below it has to fit in 108 bytes.
W=$(mktemp -d /tmp/dph.XXXXXX)
mkdir -p "$W"/{bin,home,out,rt,music,tree,config/qtengine,state,cache,data}
chmod 700 "$W/rt"

# Null audio on top of the private runtime dir, which already keeps mpv away from the real PipeWire.
real_mpv=$(command -v mpv)
printf '#!/bin/sh\nexec %s --ao=null "$@"\n' "$real_mpv" >"$W/bin/mpv"
chmod +x "$W/bin/mpv"

cd "$W/bin" || exit 1
wayland-scanner client-header "$here/vp.xml" vp.h
wayland-scanner private-code "$here/vp.xml" vp.c
gcc -O1 -o ptr "$here/ptr.c" vp.c -I. -lwayland-client || exit 1

# A copy of the UI tree whose shell.qml adds a "headless" IPC target, so scenarios can navigate without clicking.
for f in "$repo"/quickshell/* "$repo"/quickshell/.[a-z]*; do
    [ "$(basename "$f")" = shell.qml ] || ln -s "$f" "$W/tree/"
done
sed -e '0,/^import Quickshell$/s//import Quickshell\nimport Quickshell.Io/' -e '$d' "$repo/quickshell/shell.qml" >"$W/tree/shell.qml"
cat >>"$W/tree/shell.qml" <<'Q'

    // ms between frames the window really presented: a stall shows as one long gap
    QtObject {
        id: frameProbe
        property var times: []
        property var target: null
        function window() {
            return windowLoader.item.contentItem.Window.window;
        }
        function swapped() {
            times.push(Date.now());
            if (tracked)
                samples.push(tracked());
        }
        property var tracked: null
        property var samples: []
        function showing(item) {
            return item.visible ? [item].concat(...Array.from(item.children).map(showing)) : [];
        }
        // the moving parts of the main window, found by a property only they have
        function parts() {
            const all = [];
            const walk = item => {
                all.push(item);
                for (const child of item.children)
                    walk(child);
            };
            walk(window().contentItem);
            return {
                bar: all.find(i => i.barReveal !== undefined),
                host: all.find(i => i.fullWidth !== undefined),
                page: all.find(i => i.route !== undefined),
                card: all.find(i => i.sourcePos !== undefined),
                view: all.find(i => i.entry !== undefined)
            };
        }
    }

    IpcHandler {
        target: "headless"
        function framesStart(): void {
            frameProbe.times = [];
            frameProbe.target = frameProbe.window();
            frameProbe.target.frameSwapped.connect(frameProbe, frameProbe.swapped);
        }
        function framesStop(): string {
            frameProbe.target.frameSwapped.disconnect(frameProbe, frameProbe.swapped);
            return frameProbe.times.slice(1).map((t, i) => t - frameProbe.times[i]).join(" ");
        }
        // one JSON line per presented frame with where the moving parts are
        function trackStart(): void {
            const p = frameProbe.parts();
            const panel = Array.from(p.host.children).find(c => c.sourceComponent !== undefined);
            frameProbe.samples = [];
            frameProbe.tracked = () => {
                const host = p.host.mapToItem(p.card.parent, 0, 0);
                const bar = p.bar.mapToItem(p.card.parent, 0, 0);
                return JSON.stringify({
                    t: Date.now(),
                    bar: p.bar.y,
                    barShows: p.bar.visible,
                    barX: bar.x,
                    barY: bar.y,
                    barW: p.bar.width,
                    barH: p.bar.height,
                    host: p.host.width,
                    hostX: host.x,
                    hostY: host.y,
                    hostH: p.host.height,
                    kind: String(panel.item).split("_")[0],
                    panelFade: panel.opacity,
                    page: p.page.height,
                    pages: p.bar.parent.opacity,
                    card: p.card.visible,
                    cardX: p.card.x,
                    cardY: p.card.y,
                    cardW: p.card.width,
                    cardH: p.card.height,
                    view: p.view.opacity
                });
            };
            frameProbe.target = frameProbe.window();
            frameProbe.target.frameSwapped.connect(frameProbe, frameProbe.swapped);
        }
        function trackStop(): string {
            frameProbe.target.frameSwapped.disconnect(frameProbe, frameProbe.swapped);
            frameProbe.tracked = null;
            return frameProbe.samples.join("\n");
        }
        // the bar in window coordinates: x y width height
        function barRect(): string {
            const bar = frameProbe.parts().bar;
            const at = bar.mapToItem(null, 0, 0);
            return [at.x, at.y, bar.width, bar.height].map(Math.round).join(" ");
        }
        // the now-playing track info in window coordinates: x y width height, then the drawn text's width
        function stackRect(): string {
            const stack = frameProbe.showing(frameProbe.window().contentItem).find(i => i.textExtent !== undefined && i.items.some(t => t.type === "title"));
            const at = stack.mapToItem(null, 0, 0);
            return [at.x, at.y, stack.width, stack.height, stack.textExtent].map(Math.round).join(" ");
        }
        // the side panel's cover in window coordinates, then the panel's left edge: x y width height left
        function coverRect(): string {
            const host = frameProbe.parts().host;
            const cover = frameProbe.showing(host).find(i => i.morphSpring !== undefined);
            const at = cover.mapToItem(null, 0, 0);
            return [at.x, at.y, cover.width, cover.height, host.mapToItem(null, 0, 0).x].map(Math.round).join(" ");
        }
        function viewOpen(): bool {
            return Router.nowPlayingOpen;
        }
        function go(page: string): void {
            Router.go(page);
        }
        function open(page: string, id: string): void {
            Router.go(page, id);
        }
        function route(): string {
            return Router.route;
        }
        function offset(): real {
            const view = Router.pageScroll?.view;
            return view ? Math.max(0, view.contentY - view.originY) : 0;
        }
        function nowPlaying(): bool {
            return Router.nowPlayingOpen;
        }
        function entry(index: int): void {
            Router.openEntry(Router.navEntries[index]);
        }
        function expand(on: bool): void {
            if (on)
                Router.openNowPlaying();
            else
                Router.nowPlayingOpen = false;
        }
        function searchEverything(): void {
            Router.searchEverything();
        }
        // the full view's device button, pressed as a click does; true when the sheet opened knowing its button
        function pressDevice(): bool {
            const find = item => {
                if (!item || !item.visible)
                    return null;
                if (String(item).startsWith("DeviceButton") && item.context === "view")
                    return item;
                for (const child of item.children) {
                    const found = find(child);
                    if (found)
                        return found;
                }
                return null;
            };
            find(frameProbe.window().contentItem)?.clicked();
            return Router.playOnOpen && Router.playOnFrom !== null;
        }
        function focused(): string {
            return String(frameProbe.window()?.activeFocusItem ?? "");
        }
        function focusSearch(): void {
            Router.pageSearch.forceActiveFocus();
        }
        function focusRole(): int {
            return Router.pageScroll?.view.Window.activeFocusItem?.Accessible.role ?? 0;
        }
        function deletePlaylist(id: string): void {
            Router.deletePlaylist(id);
        }
        function playOnOpen(): bool {
            return Router.playOnOpen;
        }
        // a new custom page as the first navigation entry, open in edit mode with no history
        function editFirstView(): void {
            pinFirst("view", "");
            Router.openEntry(Router.navEntries[0]);
            Router.editing = true;
        }
        // pins an item as the first navigation entry without navigating; the view target "" makes a new custom page.
        // Returns its id.
        function pinFirst(kind: string, target: string): string {
            const id = kind === "view" && target === "" ? UiConfig.saveView("First", "albums", {}) : target;
            UiConfig.pin(kind, id, "First");
            const first = e => e.kind === kind && e.target === id;
            UiConfig.setNavigation(UiConfig.navigation.filter(first).concat(UiConfig.navigation.filter(e => !first(e))));
            return id;
        }
        // removes the first navigation entry without navigating
        function unpinFirst(): void {
            const e = Router.navEntries[0];
            UiConfig.removeEntry(e.kind, e.target);
        }
        function albumId(title: string): string {
            return PlayerService.query("albums", {}).find(a => a.title === title)?.id ?? "";
        }
        function shortcutFree(sequence: string): bool {
            return UiConfig.shortcutProblem(sequence) === "";
        }
        function searchText(): string {
            return Router.pageSearch?.text ?? "";
        }
        // the dialog holding the keyboard focus while closed, "" for none
        function closedDialogFocus(): string {
            for (let item = Router.pageScroll?.view.Window.activeFocusItem ?? null; item; item = item.parent) {
                if (item.embedded === false && item.opened === false)
                    return String(item);
            }
            return "";
        }
        // the item with the keyboard focus: its accessible name, else its type
        function focusName(): string {
            const item = Router.pageScroll?.view.Window.activeFocusItem ?? null;
            return item ? item.Accessible.name || String(item).replace(/[_(].*/, "") : "";
        }
        function elementOptions(id: string): void {
            const find = item => {
                if (!item)
                    return null;
                if (item.optionsId !== undefined)
                    return item;
                for (const child of item.children) {
                    const found = find(child);
                    if (found)
                        return found;
                }
                return null;
            };
            find(settingsLoader.item?.contentItem).optionsId = id;
        }
        // every row that shows: in a group, and rounded by where it sits on screen. One line per fault.
        function cornerFaults(): string {
            const faults = [];
            const walk = item => {
                if (!item || !item.visible)
                    return;
                if (item.isSettingsRow === true && item.parent?.isSettingsGroup !== true)
                    faults.push("outside a group: " + item.title);
                if (item.isSettingsGroup === true) {
                    const rows = Array.from(item.children).filter(c => c.visible && c.isSettingsRow === true).sort((a, b) => a.y - b.y);
                    rows.forEach((row, i) => {
                        const top = i === 0 ? Theme.groupedListOuterRadius : Theme.groupedListInnerRadius;
                        const bottom = i === rows.length - 1 ? Theme.groupedListOuterRadius : Theme.groupedListInnerRadius;
                        if (row.topRadius !== top || row.bottomRadius !== bottom)
                            faults.push("corners of " + row.title + ": " + row.topRadius + "/" + row.bottomRadius + ", wanted " + top + "/" + bottom);
                    });
                }
                for (const child of item.children)
                    walk(child);
            };
            walk(settingsLoader.item?.contentItem);
            return faults.join("\n");
        }
        // one line per home shelf: type, shown or hidden, item count and the item ids
        function shelves(): string {
            const out = [];
            const walk = item => {
                if (!item)
                    return;
                if (item.modelData?.type !== undefined && item.sourceComponent !== undefined) {
                    const items = item.item?.items ?? [];
                    out.push([item.modelData.type, item.visible ? "shown" : "hidden", items.length, items.map(i => i.id).join(",")].join(" "));
                    return;
                }
                for (const child of item.children)
                    walk(child);
            };
            walk(Router.pageScroll?.view.contentItem);
            return out.join("\n");
        }
        function headerSearch(mode: string): void {
            SettingsData.headerSearch = mode;
        }
        // the time, then the middle of every lyrics line as drawn on screen, top to bottom
        function lyricsRows(): string {
            const rows = [];
            const walk = item => {
                if (!item || !item.visible)
                    return;
                if (item.layout?.part)
                    rows.push(item.mapToItem(null, 0, item.height / 2).y);
                for (const child of item.children)
                    walk(child);
            };
            walk(windowLoader.item?.contentItem);
            return Date.now() + " " + rows.sort((a, b) => a - b).join(" ");
        }
        // the lyrics view's centred line and its source menu, as JSON; points are in window coordinates
        function lyricsMenu(): string {
            let view = null;
            const walk = item => {
                if (!item || view)
                    return;
                if (item.trackedCentre !== undefined && item.visible)
                    view = item;
                for (const child of item.children)
                    walk(child);
            };
            walk(windowLoader.item?.contentItem);
            if (!view)
                return "{}";
            const centre = (item => ({
                        "t": item.t,
                        "text": item.text
                    }))(view.items[view.centreItem] ?? {});
            const kids = item => [item].concat(...Array.from(item.children).map(kids));
            const point = item => {
                const p = item.mapToItem(null, item.width / 2, item.height / 2);
                return [Math.round(p.x), Math.round(p.y)];
            };
            const button = kids(view).find(i => i.iconName === "more_vert");
            const menu = kids(view).find(i => i.sources !== undefined && i.opened !== undefined);
            const popup = menu?.opened ? menu.children.find(c => c.item)?.item : null;
            const rows = {};
            const inside = popup?.contentItem ? kids(popup.contentItem) : [];
            for (const row of inside)
                if (row.entry?.id !== undefined && row.entry?.divider !== true && row.height > 0)
                    rows[row.entry.id] = point(row);
            const slider = inside.find(i => i.isDragging !== undefined && i.stepBy !== undefined);
            const end = ratio => {
                const p = slider.mapToItem(null, slider.handleWidth / 2 + (slider.width - slider.handleWidth) * ratio, slider.height / 2);
                return [Math.round(p.x), Math.round(p.y)];
            };
            const reset = inside.find(i => i.iconName === "restart_alt");
            const infos = {};
            for (const row of inside) {
                const info = row.entry?.info ? kids(row).find(i => i.iconName === "info" && i.visible) : null;
                if (info)
                    infos[row.entry.id] = {
                        "at": point(info),
                        "text": info.tooltipText,
                        "description": kids(row).find(i => i.Accessible.role === Accessible.MenuItem)?.Accessible.description ?? ""
                    };
            }
            return JSON.stringify({
                "infos": infos,
                "centre": centre,
                "button": button ? point(button) : null,
                "open": menu?.opened === true,
                "offset": PlayerService.lyricsOffset,
                "width": popup ? popup.menuWidth : 0,
                "highlight": menu?.model[menu.highlightIndex]?.id ?? null,
                "rows": rows,
                "slider": slider ? {
                    "value": slider.value,
                    "start": end(0),
                    "end": end(1)
                } : null,
                "reset": reset?.visible ? point(reset) : null
            });
        }
        function customize(surface: string): void {
            Router.customize(surface);
        }
        function settings(page: string): void {
            Router.openSettings(page);
        }
        // clicks the nth visible control of the settings window or its dialogs labelled `label` (text, title or tooltip); a negative n counts
        // from the last. Returns the number of matches, 0 when nothing was pressed.
        function press(label: string, n: int): int {
            const hits = [];
            const walk = item => {
                if (!item || !item.visible)
                    return;
                const match = (item.text === label || item.title === label || item.tooltipText === label) && typeof item.clicked === "function";
                if (match)
                    hits.push(item);
                // a button wrapping another with the same label counts once
                for (const child of item.children) {
                    if (match && child.tooltipText === label)
                        continue;
                    walk(child);
                }
            };
            // from the window's root item: dialogs move there, next to the content item
            walk(settingsLoader.item?.contentItem?.Window.window?.contentItem);
            const hit = hits[n < 0 ? hits.length + n : n];
            hit?.clicked();
            return hit ? hits.length : 0;
        }
        // the number of visible items of the settings window or its dialogs labelled `label` (text, title or tooltip),
        // not counting those inside one
        function shown(label: string): int {
            let n = 0;
            const walk = item => {
                if (!item || !item.visible)
                    return;
                if (item.text === label || item.title === label || item.tooltipText === label) {
                    n++;
                    return;
                }
                for (const child of item.children)
                    walk(child);
            };
            walk(settingsLoader.item?.contentItem?.Window.window?.contentItem);
            return n;
        }
        // clicks the row labelled `label` of an open picker sheet in the settings window; true when one was pressed
        function pick(label: string): bool {
            let hit = null;
            const walk = item => {
                if (!item || !item.visible || hit)
                    return;
                if (item.modelData?.label === label && typeof item.clicked === "function") {
                    hit = item;
                    return;
                }
                for (const child of item.children)
                    walk(child);
            };
            walk(settingsLoader.item?.contentItem?.Window.window?.contentItem);
            hit?.clicked();
            return hit !== null;
        }
        function look(mode: string, style: string): void {
            SettingsData.themeMode = mode;
            UiConfig.setNowPlaying({
                style: style
            });
        }
        function playerLook(look: string): void {
            UiConfig.setNowPlaying({
                playerLook: look
            });
        }
        function setStyle(key: string, style: string): void {
            const patch = {};
            patch[key] = style;
            UiConfig.setNowPlaying(patch);
        }
        function barTitle(width: string, adaptive: bool): void {
            const surface = JSON.parse(JSON.stringify(UiConfig.surface("bar")));
            const title = surface.zones.left.find(i => i.type === "title");
            title.options.width = width;
            title.options.adaptive = adaptive;
            UiConfig.setSurface("bar", surface);
        }
        function paneLook(look: string): void {
            UiConfig.setNowPlaying({
                paneLook: look
            });
        }
        function coverShape(shape: string): void {
            const surface = JSON.parse(JSON.stringify(UiConfig.surface("fullBento")));
            surface.zones.cover[0].options.shape = shape;
            UiConfig.setSurface("fullBento", surface);
        }
        function artColors(on: bool): void {
            SettingsData.artColors = on;
        }
        function element(type: string, context: string): void {
            Router.runElement(type, context);
        }
        function reduceMotion(on: bool): void {
            SettingsData.reduceMotion = on;
        }
        function animationDuration(ms: int): void {
            SettingsData.animationDuration = ms;
        }
        function lyricsDisplay(key: string, value: string): void {
            UiConfig.setLyricsDisplay({
                [key]: value === "true" || (value !== "false" && value)
            });
        }
        function edit(page: string): void {
            Router.editPage(page, "");
        }
        function pageOptions(kind: string, json: string): void {
            UiConfig.setPageOptions(kind, "", Object.assign({}, UiConfig.pageOptions(kind, ""), JSON.parse(json)));
        }
        function navStyle(style: string): void {
            SettingsData.navStyle = style;
        }
        function resetSurface(id: string): void {
            UiConfig.resetSurface(id);
        }
        function surface(id: string, json: string): void {
            UiConfig.setSurface(id, JSON.parse(json));
        }
        function addElement(surface: string, zone: string, json: string): void {
            const zones = JSON.parse(JSON.stringify(UiConfig.surface(surface).zones));
            zones[zone].push(Object.assign({
                hidden: false,
                placement: "auto",
                options: {}
            }, JSON.parse(json)));
            UiConfig.setSurface(surface, {
                zones: zones
            });
        }
    }
}
Q

# Generated music, so no scenario depends on anyone's library.
album="$W/music/Test Artist/Test Album"
mkdir -p "$album"
for i in 1 2 3 4 5 6 7 8; do
    ffmpeg -loglevel error -f lavfi -i "sine=frequency=$((220 * i)):duration=20" \
        -metadata title="Track $i" -metadata artist="Test Artist" -metadata album="Test Album" -metadata track="$i" \
        "$album/$i.flac" || exit 1
done

# SERVER=1: files and a manifest for fakesubsonic.py, which session.sh adds as a server library.
if [ -n "${SERVER:-}" ]; then
    mkdir -p "$W/server"
    n=0
    {
        printf '{"artists": ['
        sep_artist=
        for spec in "Server Artist One|First Light:7,Second Wind:6" "Server Artist Two|Night Drive:8"; do
            artist=${spec%%|*}
            printf '%s{"name": "%s", "albums": [' "$sep_artist" "$artist"
            sep_artist=,
            sep_album=
            IFS=, read -ra list <<<"${spec#*|}"
            for entry in "${list[@]}"; do
                name=${entry%%:*}
                dir="$W/server/$artist/$name"
                mkdir -p "$dir"
                printf '%s{"name": "%s", "year": %d, "genre": "Electronic", "tracks": [' "$sep_album" "$name" "$((2018 + n % 5))"
                sep_album=,
                for i in $(seq 1 "${entry#*:}"); do
                    n=$((n + 1))
                    duration=$((40 + n * 3))
                    ffmpeg -loglevel error -f lavfi -i "sine=frequency=$((180 + n * 37)):duration=$duration" \
                        -metadata title="$name $i" -metadata artist="$artist" -metadata album="$name" -metadata track="$i" \
                        "$dir/$i.flac" </dev/null || exit 1
                    [ "$i" -gt 1 ] && printf ','
                    printf '{"title": "%s %d", "track": %d, "duration": %d, "path": "%s"}' "$name" "$i" "$i" "$duration" "$dir/$i.flac"
                done
                printf ']}'
            done
            printf ']}'
        done
        printf ']}\n'
    } >"$W/server/manifest.json" || exit 1
fi

# Icon lookups only cost what they cost on a real desktop with a real theme.
[ -f "$HOME/.config/qtengine/config.json" ] && cp "$HOME/.config/qtengine/config.json" "$W/config/qtengine/"

env -i LANG=C.UTF-8 USER="$USER" HOME="$W/home" PATH="$W/bin:$repo/core/bin:/usr/bin:/bin" XDG_RUNTIME_DIR="$W/rt" \
    XDG_CONFIG_HOME="$W/config" XDG_STATE_HOME="$W/state" XDG_CACHE_HOME="$W/cache" XDG_DATA_HOME="$W/data" \
    XDG_DATA_DIRS="${XDG_DATA_DIRS:-/usr/share}" QT_QPA_PLATFORMTHEME=qtengine DPLAYER_WINRULES=off DPLAYER_CAST=off DPLAYER_BACKEND="${DPLAYER_BACKEND:-}" \
    DPLAYER_DISABLE_HOT_RELOAD=1 HYPRLAND_NO_SD_NOTIFY=1 HYPRLAND_NO_SD_VARS=1 HYPRLAND_NO_CRASHREPORTER=1 \
    W="$W" LIB="$here/lib.sh" SCENARIO="$scenario" SERVER="${SERVER:-}" \
    timeout -k 30 "${HEADLESS_TIMEOUT:-420}" dbus-run-session -- bash "$here/session.sh"
status=$?

log="$W/out/ui.log"
errors=$(grep -hE 'TypeError|ReferenceError|Cannot assign|is not a function|Cannot specify anchors for items inside|\.qml[:[][0-9]+.*(rror|nable)' "$log" 2>/dev/null | sort -u)
[ -n "$errors" ] && { echo "QML errors:"; echo "$errors" | cut -c1-200 | head -20; status=1; }
echo "output: $W/out"
ls "$W/out"
exit "$status"
