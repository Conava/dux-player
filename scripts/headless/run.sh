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
            if (layout)
                layouts.push(layout());
        }
        // timedFrom and timed's newer actions count every frame of the window and of the menus it opens
        property bool every: false
        property var menus: []
        property var menuWindows: []
        property Timer menuTimer: Timer {
            interval: 20
            repeat: true
            onTriggered: {
                for (const menu of frameProbe.menus) {
                    // a destroyed menu has no children left
                    const w = Array.from(menu?.children ?? []).find(c => c.item)?.item?.contentItem?.Window.window;
                    if (w && frameProbe.menuWindows.indexOf(w) < 0) {
                        frameProbe.menuWindows.push(w);
                        w.frameSwapped.connect(frameProbe, frameProbe.swapped);
                    }
                }
            }
        }
        // the page's and its scrolling view's widths per frame: a change means a layout
        property var layout: null
        property var layouts: []
        property real started: 0
        property int idleCount: 0
        property var idleTarget: null
        function idleFrame() {
            idleCount++;
        }
        function windowOf(name) {
            return name === "settings" ? settingsLoader.item?.contentItem?.Window.window ?? null : window();
        }
        // the main window, the settings window and the open menus' popups
        function roots() {
            const out = [window().contentItem];
            const settings = windowOf("settings");
            if (settings?.visible)
                out.push(settings.contentItem);
            for (const menu of allMenus()) {
                const popup = menu.opened ? menu.children.find(c => c.item)?.item : null;
                if (popup?.contentItem)
                    out.push(popup.contentItem);
            }
            return out;
        }
        function allMenus() {
            const out = [];
            const walk = item => {
                if (!item)
                    return;
                if (item.highlightIndex !== undefined && item.minimumWidth !== undefined && item.opened !== undefined)
                    out.push(item);
                for (const child of item.children)
                    walk(child);
            };
            walk(window().contentItem);
            walk(windowOf("settings")?.contentItem);
            return out;
        }
        // a menu built on its first open takes the focus as it opens
        function focusMoved() {
            const item = window().activeFocusItem;
            if (item?.highlightIndex !== undefined && item.minimumWidth !== undefined && menus.indexOf(item) < 0)
                menus.push(item);
        }
        function startTiming(target) {
            const route = parts().page;
            layout = () => [route.width, Router.pageScroll?.view?.width ?? 0];
            layouts = [layout()];
            times = [];
            started = Date.now();
            target.frameSwapped.connect(frameProbe, frameProbe.swapped);
            frameProbe.target = target;
            menus = allMenus();
            menuWindows = [];
            menuTimer.start();
            window().activeFocusItemChanged.connect(frameProbe, frameProbe.focusMoved);
        }
        function stopTiming() {
            menuTimer.stop();
            window().activeFocusItemChanged.disconnect(frameProbe, frameProbe.focusMoved);
            target.frameSwapped.disconnect(frameProbe, frameProbe.swapped);
            for (const w of menuWindows) {
                try {
                    w.frameSwapped.disconnect(frameProbe, frameProbe.swapped);
                } catch (e) {
                    // the popup went with its menu
                }
            }
            layout = null;
        }
        // the actions timed and act share; true when ACTION is one of them
        function act(action, arg) {
            const tracks = () => PlayerService.query("tracks", {});
            switch (action) {
            case "forward":
                Router.forward();
                return true;
            case "next":
                PlayerService.next();
                return true;
            case "prev":
                PlayerService.previous();
                return true;
            case "queueAdd":
                PlayerService.enqueue(tracks().slice(0, Number(arg)), true);
                return true;
            case "queueRemove":
                PlayerService.queueRemove(Number(arg));
                return true;
            case "queueMove":
                PlayerService.queueMove(Number(arg.split(" ")[0]), Number(arg.split(" ")[1]));
                return true;
            case "pane":
                if (Router.nowPlayingOpen)
                    Router.viewPane = arg;
                else if (Router.dockMode === "side")
                    Router.setSideTab(arg);
                else
                    Router.panel = arg;
                return true;
            case "libraryTab":
                SettingsData.libraryTab = arg;
                return true;
            case "railLabels":
                windowLoader.item.toggleRailLabels();
                return true;
            case "dock":
                Router.runElement("dock", "bar");
                return true;
            case "prompt":
                Router.prompt(arg, "", arg, () => {});
                return true;
            }
            return false;
        }
        property real t0: 0
        // the open menu and dialog sizes, one sample every few ms while one shows
        property var popupSamples: []
        property var shownMenu: null
        property var shownDialog: null
        function firstIn(item, wanted) {
            if (wanted(item))
                return item;
            for (const child of item.children) {
                const found = firstIn(child, wanted);
                if (found)
                    return found;
            }
            return null;
        }
        property var shownPopup: null
        function sizeOf(kind) {
            if (kind === "menu") {
                const popup = shownMenu?.children.find(c => c.item)?.item;
                return popup ? {
                    "kind": kind,
                    "t": Date.now(),
                    "w": popup.menuWidth,
                    "h": popup.menuHeight
                } : null;
            }
            const surface = shownDialog?.children.find(c => c.entryScale !== undefined);
            return surface ? {
                "kind": kind,
                "t": Date.now(),
                "w": surface.width,
                "h": surface.height,
                "scale": surface.scale
            } : null;
        }
        // a sample per frame the popup window or the main window presented, so a size that only existed between frames doesn't count
        function menuFrame() {
            const sample = sizeOf("menu");
            if (sample)
                popupSamples.push(sample);
        }
        function dialogFrame() {
            const sample = sizeOf("dialog");
            if (sample)
                popupSamples.push(sample);
        }
        // finds the open menu and dialog and listens to their windows' frames
        function samplePopups() {
            const root = window().contentItem;
            if (!shownMenu?.opened)
                shownMenu = firstIn(root, i => i.opened === true && i.highlightIndex !== undefined && i.minimumWidth !== undefined);
            if (!shownDialog?.overlayOpen)
                shownDialog = firstIn(root, i => i.overlayOpen === true && i.embedded === false);
            const popup = shownMenu?.children.find(c => c.item)?.item ?? null;
            if (popup?.contentItem?.Window.window && popup.contentItem.Window.window !== shownPopup) {
                shownPopup = popup.contentItem.Window.window;
                shownPopup?.frameSwapped.connect(frameProbe, frameProbe.menuFrame);
            }
        }
        property Timer popupTimer: Timer {
            interval: 2
            repeat: true
            onTriggered: frameProbe.samplePopups()
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
                view: all.find(i => typeof i.entry === "number")
            };
        }
        // where the moving parts are now
        function sampler() {
            const p = parts();
            const panel = Array.from(p.host.children).find(c => c.sourceComponent !== undefined);
            return () => {
                const host = p.host.mapToItem(p.card.parent, 0, 0);
                const bar = p.bar.mapToItem(p.card.parent, 0, 0);
                return {
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
                    remaining: p.card.remaining,
                    fullW: p.card.parent.width,
                    fullH: p.card.parent.height,
                    view: p.view.opacity
                };
            };
        }
    }

    IpcHandler {
        target: "headless"
        // only the frames from the first to the last one where a moving part moved: the wait for the move to start is
        // input latency, and a stray repaint before or after it would otherwise count that idle time as a gap
        function framesStart(): void {
            const sample = frameProbe.sampler();
            frameProbe.times = [];
            frameProbe.samples = [JSON.stringify(sample())];
            frameProbe.tracked = () => JSON.stringify(sample());
            frameProbe.target = frameProbe.window();
            frameProbe.target.frameSwapped.connect(frameProbe, frameProbe.swapped);
        }
        function framesStop(): string {
            frameProbe.target.frameSwapped.disconnect(frameProbe, frameProbe.swapped);
            frameProbe.tracked = null;
            const s = frameProbe.samples;
            const moved = frameProbe.times.map((_, i) => s[i + 1] !== s[i]);
            const t = frameProbe.times.slice(moved.indexOf(true), moved.lastIndexOf(true) + 1);
            return t.slice(1).map((x, i) => x - t[i]).join(" ");
        }
        // one JSON line per presented frame with where the moving parts are
        function trackStart(): void {
            const sample = frameProbe.sampler();
            frameProbe.samples = [];
            frameProbe.tracked = () => JSON.stringify(Object.assign({
                    t: Date.now()
                }, sample()));
            frameProbe.target = frameProbe.window();
            frameProbe.target.frameSwapped.connect(frameProbe, frameProbe.swapped);
        }
        // the number of samples; trackLines reads them in pages, since the runtime's IPC server drops a reply of more than about 200 kB
        function trackStop(): int {
            frameProbe.target.frameSwapped.disconnect(frameProbe, frameProbe.swapped);
            frameProbe.tracked = null;
            return frameProbe.samples.length;
        }
        function trackLines(from: int, count: int): string {
            return frameProbe.samples.slice(from, from + count).join("\n");
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
        function back(): void {
            Router.back();
        }
        function route(): string {
            return Router.route;
        }
        function panel(): string {
            return Router.panel;
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
            // from the window's root item: dialogs move there, next to the content item
            walk(settingsLoader.item?.contentItem?.Window.window?.contentItem);
            return faults.join("\n");
        }
        // one line per home shelf: type, shown or hidden, item count and the item ids
        function shelves(): string {
            const out = [];
            const walk = item => {
                if (!item)
                    return;
                if (item.modelData?.type !== undefined && item.sourceComponent !== undefined) {
                    // shelves name their content differently: items, tiles (speed dial) or genres (chips)
                    const items = item.item?.items ?? item.item?.tiles ?? item.item?.genres ?? [];
                    out.push([item.modelData.type, item.visible ? "shown" : "hidden", items.length, items.map(i => i.id ?? i.name ?? i).join(",")].join(" "));
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
        function navEntries(): string {
            return Router.navEntries.map(e => e.kind + ":" + e.target).join(" ");
        }
        function titleBars(on: bool): void {
            SettingsData.windowTitleBars = on;
        }
        function fullViewTopBar(on: bool): void {
            SettingsData.fullViewTopBar = on;
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
            const follow = kids(view).find(i => i.iconName === "my_location" && i.visible);
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
                "follow": follow ? point(follow) : null,
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
        // timed(ACTION, ARG): starts recording every presented frame, then does the navigation in the same call, so a
        // build that blocks the window shows as a long wait before the first frame. ACTION: go PAGE | open "PAGE ID" |
        // back | expand true|false count up to the last frame in which a moving part moved; act's actions (forward,
        // next, prev, queueAdd N, queueRemove I, queueMove "FROM TO", pane lyrics|queue, libraryTab NAME, railLabels,
        // dock, prompt TITLE) count every frame of the window and its menus. queueAdd puts the first N tracks after
        // the current one; pane shows that pane in the full view when it is open, else in the docked side panel or
        // the panel beside the page; prompt opens a text prompt dialog
        function timed(action: string, arg: string): void {
            if (["go", "open", "back", "expand"].indexOf(action) < 0) {
                frameProbe.every = true;
                frameProbe.tracked = null;
                frameProbe.startTiming(frameProbe.window());
                frameProbe.t0 = Date.now();
                frameProbe.act(action, arg);
                return;
            }
            frameProbe.every = false;
            const sample = frameProbe.sampler();
            const all = [];
            const walk = item => {
                all.push(item);
                for (const child of item.children)
                    walk(child);
            };
            walk(frameProbe.window().contentItem);
            const loaders = all.find(i => i.objectName === "keptHome").parent.children.filter(c => c.sourceComponent !== undefined);
            frameProbe.tracked = () => JSON.stringify([sample(), loaders.map(l => [l.opacity, l.scale, l.visible, String(l.item)])]);
            frameProbe.samples = [frameProbe.tracked()];
            frameProbe.startTiming(frameProbe.window());
            frameProbe.t0 = Date.now();
            switch (action) {
            case "go":
                Router.go(arg);
                break;
            case "open":
                Router.go(arg.split(" ")[0], arg.split(" ")[1]);
                break;
            case "back":
                Router.back();
                break;
            case "expand":
                if (arg === "true")
                    Router.openNowPlaying();
                else
                    Router.nowPlayingOpen = false;
                break;
            }
        }
        // the ms from timed() to the first frame, then between the frames after it; the pause after the last one is not a gap
        function timedStop(): string {
            frameProbe.stopTiming();
            frameProbe.tracked = null;
            if (frameProbe.every) {
                // timedFrom has no t0: its action comes later, from the scenario
                const t = (frameProbe.t0 ? [frameProbe.t0] : []).concat(frameProbe.times.slice().sort((a, b) => a - b));
                return t.slice(1).map((x, i) => x - t[i]).join(" ");
            }
            // up to the last frame in which something moved: a timer firing later repaints the idle window unchanged
            const s = frameProbe.samples;
            const moved = frameProbe.times.map((_, i) => s[i + 1] !== s[i]);
            const t = [frameProbe.t0].concat(frameProbe.times.slice(0, moved.lastIndexOf(true) + 1));
            return t.slice(1).map((x, i) => x - t[i]).join(" ");
        }
        // timedFrom(WINDOW): records like timed, without acting: main, or settings for the settings window. The scenario
        // acts (click, pointer, wtype) and calls timedStop, whose gaps start at the first frame.
        function timedFrom(window: string): void {
            frameProbe.every = true;
            frameProbe.tracked = null;
            frameProbe.t0 = 0;
            frameProbe.startTiming(frameProbe.windowOf(window));
        }
        // the ms since the last frame of the recording, or since it started
        function sinceFrame(): int {
            const last = frameProbe.times.length ? Math.max(...frameProbe.times) : frameProbe.t0 || frameProbe.started;
            return Date.now() - last;
        }
        // how often the page's width and its scrolling view's width changed during the recording: "page view"
        function timedLayouts(): string {
            const l = frameProbe.layouts;
            const changes = k => l.filter((x, i) => i > 0 && x[k] !== l[i - 1][k] && x[k] > 0 && l[i - 1][k] > 0).length;
            return changes(0) + " " + changes(1);
        }
        // idleStart(WINDOW) and idleStop(): the frames the main (or settings) window presented in between
        function idleStart(window: string): void {
            frameProbe.idleCount = 0;
            frameProbe.idleTarget = frameProbe.windowOf(window);
            frameProbe.idleTarget.frameSwapped.connect(frameProbe, frameProbe.idleFrame);
        }
        function idleStop(): int {
            frameProbe.idleTarget.frameSwapped.disconnect(frameProbe, frameProbe.idleFrame);
            return frameProbe.idleCount;
        }
        // act(ACTION, ARG): one of timed's newer actions, untimed
        function act(action: string, arg: string): bool {
            return frameProbe.act(action, arg);
        }
        // props(TYPE, PROP): JSON array of PROP over the visible items of the main and settings windows and the open
        // menus whose type name starts with TYPE, or with ".NAME" that have a property NAME. A PROP ending in "*" is
        // multiplied along the item's parents (opacity* is the opacity it is drawn with).
        function props(type: string, prop: string): string {
            const name = prop.replace(/\*$/, "");
            const along = prop.endsWith("*");
            const wanted = type.startsWith(".") ? i => i[type.slice(1)] !== undefined : i => String(i).startsWith(type);
            const out = [];
            const value = item => {
                if (!along)
                    return item[name];
                let v = 1;
                for (let i = item; i; i = i.parent)
                    v *= i[name] ?? 1;
                return v;
            };
            const walk = item => {
                if (!item || !item.visible)
                    return;
                if (wanted(item))
                    out.push(value(item));
                for (const child of item.children)
                    walk(child);
            };
            for (const root of frameProbe.roots())
                walk(root);
            return JSON.stringify(out);
        }
        // JSON array of PROP over the rows of the open menus; PROP* as in props
        function menuRows(prop: string): string {
            const name = prop.replace(/\*$/, "");
            const out = [];
            for (const menu of frameProbe.allMenus()) {
                const popup = menu.opened ? menu.children.find(c => c.item)?.item : null;
                const walk = item => {
                    if (!item || !item.visible)
                        return;
                    if (item.entry !== undefined) {
                        let v = item[name];
                        if (prop.endsWith("*"))
                            for (let i = item.parent; i; i = i.parent)
                                v *= i[name] ?? 1;
                        out.push(v);
                        return;
                    }
                    for (const child of item.children)
                        walk(child);
                };
                walk(popup?.contentItem);
            }
            return JSON.stringify(out);
        }
        // the smallest scale an item at window point X Y of the main (or settings) window is drawn with, its parents' included
        function scaleAt(x: real, y: real, window: string): real {
            let least = 1;
            const walk = (item, scale) => {
                if (!item || !item.visible)
                    return;
                const s = scale * item.scale;
                const p = item.mapFromItem(null, x, y);
                if (p.x >= 0 && p.y >= 0 && p.x <= item.width && p.y <= item.height)
                    least = Math.min(least, s);
                for (const child of item.children)
                    walk(child, s);
            };
            walk(frameProbe.windowOf(window).contentItem, 1);
            return Math.round(least * 1000) / 1000;
        }
        function popupsStart(): void {
            frameProbe.popupSamples = [];
            frameProbe.shownMenu = null;
            frameProbe.shownDialog = null;
            frameProbe.shownPopup = null;
            frameProbe.target = frameProbe.window();
            frameProbe.target.frameSwapped.connect(frameProbe, frameProbe.dialogFrame);
            frameProbe.popupTimer.start();
        }
        function popupsStop(): string {
            frameProbe.popupTimer.stop();
            frameProbe.target.frameSwapped.disconnect(frameProbe, frameProbe.dialogFrame);
            try {
                frameProbe.shownPopup?.frameSwapped.disconnect(frameProbe, frameProbe.menuFrame);
            } catch (e) {
                // the popup went with its menu
            }
            return frameProbe.popupSamples.map(s => JSON.stringify(s)).join("\n");
        }
        // the centre of the first showing track row in window coordinates: "X Y", "" when none
        function trackPoint(): string {
            const row = frameProbe.firstIn(frameProbe.window().contentItem, i => i.visible && i.width > 0 && String(i).startsWith("TrackRow"));
            const p = row ? row.mapToItem(null, row.width / 2, row.height / 2) : null;
            return p ? Math.round(p.x) + " " + Math.round(p.y) : "";
        }
        // one line "X Y W H" per visible item of the main window whose accessible name or text is `label`, in tree order,
        // relative to the window: the rail entry before the tile of the same name
        function namedAll(label: string): string {
            const out = [];
            const walk = item => {
                if (!item || !item.visible)
                    return;
                // a button's own text label doesn't count again
                if (item.width > 0 && (item.Accessible.name === label || (item.Accessible.name === "" && item.text === label && !String(item).startsWith("StyledText")))) {
                    const p = item.mapToItem(null, 0, 0);
                    out.push([p.x, p.y, item.width, item.height].map(Math.round).join(" "));
                }
                for (const child of item.children)
                    walk(child);
            };
            walk(frameProbe.window().contentItem);
            return out.join("\n");
        }
        // "X Y": the centre of the first visible item named `label` inside the first visible item with that objectName, "" when none
        function namedUnder(objectName: string, label: string): string {
            const root = frameProbe.firstIn(frameProbe.window().contentItem, i => i.visible && i.objectName === objectName);
            const item = root ? frameProbe.firstIn(root, i => i.visible && i.width > 0 && (i.Accessible.name === label || i.text === label)) : null;
            const p = item ? item.mapToItem(null, item.width / 2, item.height / 2) : null;
            return p ? Math.round(p.x) + " " + Math.round(p.y) : "";
        }
        // one line "caption X Y W H showsArt" per visible Speed dial tile, relative to the window
        function dialTiles(): string {
            const out = [];
            const walk = item => {
                if (!item || !item.visible)
                    return;
                if (item.caption !== undefined && item.showsArt !== undefined) {
                    const p = item.mapToItem(null, 0, 0);
                    out.push([item.caption, Math.round(p.x), Math.round(p.y), Math.round(item.width), Math.round(item.height), item.showsArt].join(" "));
                }
                for (const child of item.children)
                    walk(child);
            };
            walk(frameProbe.window().contentItem);
            return out.join("\n");
        }
        // the navigation entry that shows as selected: "kind:target", "" when none
        function navLit(): string {
            const e = Router.navEntries[Router.navIndex];
            return e ? e.kind + ":" + e.target : "";
        }
        // the first visible page tint: "shown HSV-saturation", "" when none
        function tint(): string {
            const item = frameProbe.firstIn(frameProbe.window().contentItem, i => i.visible && String(i).startsWith("PageTint"));
            return item ? "shown " + item.tint.hsvSaturation.toFixed(2) : "";
        }
        // the titles of the visible tiles that carry the playing plate, separated by ";"
        function playingTiles(): string {
            const out = [];
            const walk = item => {
                if (!item || !item.visible)
                    return;
                if (String(item).startsWith("MediaTile") && item.playing && item.color.a > 0)
                    out.push(item.item?.title ?? item.item?.name);
                for (const child of item.children)
                    walk(child);
            };
            walk(frameProbe.window().contentItem);
            return out.join(";");
        }
        // the largest horizontal offset of the page's visible carousels
        function carouselX(): real {
            const lists = [];
            const walk = item => {
                if (!item || !item.visible)
                    return;
                if (String(item).startsWith("DHorizontalList"))
                    lists.push(item);
                for (const child of item.children)
                    walk(child);
            };
            walk(Router.pageScroll?.view.contentItem);
            return Math.max(-1, ...lists.map(l => Math.round(l.contentX - l.originX)));
        }
        // the title of the tile or row at the top of the page's list or grid once the page's header is scrolled away, "" when none
        function topItem(): string {
            const view = Router.pageScroll?.view;
            const row = view ? view.itemAt(view.contentX + view.width / 4, Math.max(view.contentY, view.originY + (view.headerItem?.height ?? 0)) + 4) : null;
            const it = row?.track ?? row?.item ?? row?.modelData ?? null;
            return it ? it.title ?? it.name ?? "" : "";
        }
        // "X Y W H" of the first visible detail hero (an album's cover and facts) relative to the window, "" when none
        function heroRect(): string {
            const hero = frameProbe.firstIn(frameProbe.window().contentItem, i => i.visible && String(i).startsWith("DetailHero"));
            const p = hero ? hero.mapToItem(null, 0, 0) : null;
            return p ? [p.x, p.y, hero.width, hero.height].map(Math.round).join(" ") : "";
        }
        // the app setting `name` as JSON
        function prefValue(name: string): string {
            return JSON.stringify(SettingsData[name]);
        }
        // sets SettingsData[name] to the JSON value; names SettingsData doesn't have are ignored
        function pref(name: string, json: string): void {
            if (name in SettingsData)
                SettingsData[name] = JSON.parse(json);
        }
        // "X Y W H enabled" of the first visible item with that objectName, in the main window then the settings
        // window, relative to that window's top-left corner; "" when none shows
        function rect(objectName: string): string {
            const wanted = i => i.visible && i.objectName === objectName;
            const main = frameProbe.window().contentItem;
            let root = main;
            let hit = frameProbe.firstIn(main, wanted);
            if (!hit) {
                root = settingsLoader.item?.contentItem?.Window.window?.contentItem;
                hit = root ? frameProbe.firstIn(root, wanted) : null;
            }
            if (!hit)
                return "";
            const p = hit.mapToItem(root, 0, 0);
            return [p.x, p.y, hit.width, hit.height].map(Math.round).join(" ") + " " + hit.enabled;
        }
        // "X Y": the centre of the first visible item of the main window whose tooltip is `label`, "" when none
        function pagePoint(label: string): string {
            const item = frameProbe.firstIn(frameProbe.window().contentItem, i => i.visible && i.width > 0 && i.tooltipText === label);
            const p = item ? item.mapToItem(null, item.width / 2, item.height / 2) : null;
            return p ? Math.round(p.x) + " " + Math.round(p.y) : "";
        }
        function showDetails(title: string): void {
            Router.showDetails("track", PlayerService.query("tracks", {}).find(t => t.title === title));
        }
        // the visible rating bar of the open page: its stars, or -1 when none shows
        function ratingShown(): int {
            const bar = frameProbe.firstIn(frameProbe.window().contentItem, i => i.visible && i.blocker !== undefined && typeof i.rated === "function");
            return bar ? bar.rating : -1;
        }
        function rate(stars: int): bool {
            const bar = frameProbe.firstIn(frameProbe.window().contentItem, i => i.visible && i.blocker !== undefined && typeof i.rated === "function");
            bar?.rated(stars);
            return bar !== null;
        }
        function playlistId(name: string): string {
            return PlaylistService.playlists.find(p => p.name === name)?.id ?? "";
        }
        function loadPlaylist(id: string): void {
            PlaylistService.loadPlaylist(id);
        }
        // the playlist's track ids in order, "" while its tracks aren't loaded
        function playlistOrder(id: string): string {
            return (PlaylistService.playlist(id)?.items ?? []).map(t => t.id).join(",");
        }
        // a drag of the first track to the end
        function moveFirstLast(id: string): void {
            const n = PlaylistService.playlist(id).items.length;
            PlaylistService.reorderPlaylist(id, Array.from({
                length: n
            }, (_, i) => (i + 1) % n));
        }
        // whether the settings window's switch labelled `label` is on: 1, 0, or -1 when none shows
        function toggleOn(label: string): int {
            const row = frameProbe.firstIn(settingsLoader.item?.contentItem?.Window.window?.contentItem, i => i.visible && i.text === label && i.checked !== undefined);
            return row ? (row.checked ? 1 : 0) : -1;
        }
        function customize(surface: string): void {
            Router.customize(surface);
        }
        function settings(page: string): void {
            Router.openSettings(page);
        }
        // the open settings sub-page's title in the header, "" on a page's main view
        function settingsSub(): string {
            return settingsLoader.item?.subTitle ?? "";
        }
        // "X Y": the centre of the nth visible item of the settings window labelled `label`, in window coordinates
        function settingsPoint(label: string, n: int): string {
            const hits = [];
            const walk = item => {
                if (!item || !item.visible)
                    return;
                if (item.text === label || item.title === label || item.tooltipText === label)
                    hits.push(item);
                for (const child of item.children)
                    walk(child);
            };
            const root = settingsLoader.item?.contentItem?.Window.window?.contentItem;
            walk(root);
            const hit = hits[n];
            const p = hit ? hit.mapToItem(root, hit.width / 2, hit.height / 2) : null;
            return p ? Math.round(p.x) + " " + Math.round(p.y) : "";
        }
        // the settings window's focused item: the accessible name of it or the nearest item around it that has one
        function settingsFocus(): string {
            for (let item = settingsLoader.item?.focusedItem ?? null; item; item = item.parent) {
                if (item.Accessible.name)
                    return item.Accessible.name;
            }
            return "";
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
        function artColorMode(mode: string): void {
            SettingsData.artColorMode = mode;
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
        // the element types of one zone of a surface in order: ask zoneOrder bar left
        function zoneOrder(surface: string, zone: string): string {
            return UiConfig.surface(surface).zones[zone].map(i => i.type).join(" ");
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
        function nowPlayingPatch(json: string): void {
            UiConfig.setNowPlaying(JSON.parse(json));
        }
        function pageEndInset(px: real): void {
            Router.pageEndInset = px;
        }
        // the colour Theme[name] as #rrggbb
        function themeColor(name: string): string {
            return Qt.color(Theme[name]).toString();
        }
        // the text of the first visible item with that objectName in the main window, "" when none
        function objectText(objectName: string): string {
            const hit = frameProbe.firstIn(frameProbe.window().contentItem, i => i.visible && i.objectName === objectName);
            return hit ? String(hit.text) : "";
        }
        // how many visible items of the main window have that objectName
        function countObjects(objectName: string): int {
            let n = 0;
            const walk = item => {
                if (!item || !item.visible)
                    return;
                if (item.objectName === objectName)
                    n++;
                for (const child of item.children)
                    walk(child);
            };
            walk(frameProbe.window().contentItem);
            return n;
        }
        // one line "X Y W H chosen" per visible Base color swatch of the settings window, in window coordinates
        function swatches(): string {
            const out = [];
            const root = settingsLoader.item?.contentItem?.Window.window?.contentItem;
            const walk = item => {
                if (!item || !item.visible)
                    return;
                if (item.objectName === "swatch") {
                    const p = item.mapToItem(root, 0, 0);
                    out.push([p.x, p.y, item.width, item.height].map(Math.round).join(" ") + " " + item.chosen);
                }
                for (const child of item.children)
                    walk(child);
            };
            walk(root);
            return out.join("\n");
        }
        // moves the focus into the settings window's search field
        function settingsFocusSearch(): void {
            settingsLoader.item?.focusSearch();
        }
        // one line "page title" per result of the settings search
        function settingsResults(): string {
            return (settingsLoader.item?.searchResults ?? []).map(e => e.page + " " + e.title).join("\n");
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

# The dead proxy keeps lyrics providers and artist lookups off the network (localhost, the fake server, bypasses it). Switching the lyrics setting off instead would drop the menu rows the lyrics scenario checks.
env -i LANG=C.UTF-8 USER="$USER" HOME="$W/home" PATH="$W/bin:$repo/core/bin:/usr/bin:/bin" XDG_RUNTIME_DIR="$W/rt" \
    XDG_CONFIG_HOME="$W/config" XDG_STATE_HOME="$W/state" XDG_CACHE_HOME="$W/cache" XDG_DATA_HOME="$W/data" \
    HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 \
    XDG_DATA_DIRS="${XDG_DATA_DIRS:-/usr/share}" QT_QPA_PLATFORMTHEME=qtengine DPLAYER_WINRULES=off DPLAYER_CAST=off DPLAYER_BACKEND="${DPLAYER_BACKEND:-}" DPLAYER_QS_CHILD="${DPLAYER_QS_CHILD:-}" \
    DPLAYER_DISABLE_HOT_RELOAD=1 HYPRLAND_NO_SD_NOTIFY=1 HYPRLAND_NO_SD_VARS=1 HYPRLAND_NO_CRASHREPORTER=1 \
    W="$W" LIB="$here/lib.sh" SCENARIO="$scenario" SERVER="${SERVER:-}" GAP_MAX="${GAP_MAX:-}" SEEN_MAX="${SEEN_MAX:-}" DELAY="${DELAY:-}" SETTLE="${SETTLE:-}" FEEL_LEGS="${FEEL_LEGS:-}" FEEL_SPEED="${FEEL_SPEED:-}" \
    timeout -k 30 "${HEADLESS_TIMEOUT:-420}" dbus-run-session -- bash "$here/session.sh"
status=$?

log="$W/out/ui.log"
errors=$(grep -hE 'TypeError|ReferenceError|Cannot assign|is not a function|Cannot specify anchors for items inside|\.qml[:[][0-9]+.*(rror|nable)' "$log" 2>/dev/null | sort -u)
[ -n "$errors" ] && { echo "QML errors:"; echo "$errors" | cut -c1-200 | head -20; status=1; }
echo "output: $W/out"
ls "$W/out"
exit "$status"
