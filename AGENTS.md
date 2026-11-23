# Dux Player

A standalone music player for Linux in the look and feel of [Dux Shell](https://github.com/duxlabs-dev/dux-shell). It plays local libraries and playlists and streams from remote servers ((Open-)Subsonic/Jellyfin for now). Go daemon and CLI plus a QML UI that runs on dux-qt-base (installed `libdux-qt-base.so.0`, loaded by `dplayer qs`) or, as a fallback, on Quickshell (`qs`), built on [dux-qml-common](https://github.com/duxlabs-dev/dux-qml-common) and [dux-go](https://github.com/duxlabs-dev/dux-go) like the other Dux apps.

## Names

| | |
|---|---|
| Binary | `dplayer` |
| App id (desktop entry, icon, D-Bus) | `io.github.conava.DuxPlayer` |
| MPRIS bus name | `org.mpris.MediaPlayer2.dplayer` |
| Config / state / cache dirs | `dplayer` under the XDG base dirs |

## Repo structure

- `core/`: Go module. `cmd/dplayer` is the CLI and daemon entrypoint. `internal/` holds the packages (`player`, `library`, `source/<kind>`, `mpris`, `cast`, `lyrics`, `ipc`, `settings`, `shellembed`, …).
- `quickshell/`: the whole UI. `Common/` (Theme, SettingsData, I18n, Paths, …), `Services/` (the bridge to core and other non-UI logic), `Modules/` (windows and views), `translations/`.
- `dux-qml-common/`: submodule, symlinked in as `quickshell/DCommon`.
- `assets/`: icon, desktop entry, metainfo.

`core` and `quickshell` talk over a unix socket (requests plus an event stream). They share that protocol: changing one side means checking the other.

## Working on this project

Features are specified by the maintainer and mostly implemented with AI agents. Scope and look are the maintainer's decisions.

- **Questions get answers, not edits.** "Could we…", "how would…" means explain and recommend. Edit code when asked to.
- **Build what was asked.** No unrequested features, settings or UI. If a request needs something extra to work, or has a glaring problem, say so before building it.
- **Show anything new-looking before building it.** A new screen or control gets a sketch or a preview of the options first.
- **Never commit or push unless asked.** Never use `sudo`; hand root commands to the maintainer.
- **You may never create an Issue, Discussion, PR or comment yourself** AI is permitted for development but not for communication. All changes have to be owned and understood by the developer.
- **Agents in parallel write files; only the main session commits,** serially, after every agent has stopped. Give each agent the files it owns and name the files that belong to others.
- **Defects and Bugs must be solved** If you encounter a defect or a bug in this project, investigate it, and offer to fix it if the blast radius and effort is small, else raise it to the developer for a separate PR or to create an upstream issue.

### Definition of done

1. `make check` (lint, test, test-race and build) passes.
2. **The developer has tried it by hand.** Hand over one command, say what to look at and what working looks like. A green test suite is not the end.
3. `make run` exercises the change end to end with no extra env vars, flags or wrappers. If it doesn't, fix your code or the script if really necessary.
4. No dead code, no leftover old path, comments pass the comment rules.
5. The README and other docs match the change (features, requirements, commands, settings, online services).

### Testing without touching the live desktop

The developer's desktop session, often Dux Shell, keeps running while you work. Never restart the shell or other session services, never `make install`, and never write to the developer's real config (the shell's or the player's): test runs use sandboxed `XDG_*` dirs unless asked otherwise. Never run a script "to see what it prints" if it touches system state; read it. Never pick PIDs with `pgrep -f <pattern>` (it matches your own shell); start test processes yourself and use `$!`.

For GUI work, use `make headless` before handing anything over. It runs the real daemon, window and mpv (the built-in player when mpv is missing) in a nested Hyprland that is invisible on the desktop, with private `HOME`, `XDG_*` dirs and session bus, null audio and a generated library, then writes screenshots and `ui.log` to a temp folder and prints the path. It fails on QML errors. Read the screenshots yourself: a clean run says nothing about how it looks. Unlike `QT_QPA_PLATFORM=offscreen`, it draws shaders, masks and art.

- `DPLAYER_BACKEND=builtin make headless SCENARIO=scripts/headless/builtin.sh` plays through the built-in player on a private PipeWire with null sinks and no access to real devices: output switching, a removed sink, a killed server and the snackbars. Read `facts.txt` and the screenshots.
- `SERVER=1 make headless …` adds a server library "Server" from a fake (Open)Subsonic server (`scripts/headless/fakesubsonic.py`, three albums, one playlist) whose password sits in a fresh keyring on the private bus; the run waits for its catalog sync. Needs `gnome-keyring-daemon`.
- `DPLAYER_QS_CHILD=1 make headless` runs on upstream `qs` instead of dux-qt-base; run it after touching the window launcher or the `qs` subcommand. Every headless run prints `runtime: dux-qt-base` or `runtime: qs` and fails when the library is installed but the UI didn't load it.
- `make headless SCENARIO=file.sh` runs your own steps; `scripts/headless/lib.sh` lists the helpers (`go`, `expand`, `shot`, `dp`, `rpc`, `click`, `pointer`). Add a helper to the `headless` IPC target in `run.sh` when a scenario needs more than navigation.
- Anything about pointer feel (drag, scroll, hover) goes through `pointer`/`click`. QtTest mouse events are throttled to about 60 a second and hide lag that a real pointer shows.
- `make headless SCENARIO=scripts/headless/typing.sh` types real keys into fields that change the list around them and fails when a character goes missing. Run it after touching a list, its model or a field in a page header.
- `make headless SCENARIO=scripts/headless/corners.sh` walks the settings pages and fails on a row outside a `SettingsGroup` or with the wrong corners for its place on screen. Run it after touching settings rows or groups, and add new pages to it.
- `make headless SCENARIO=scripts/headless/cursors.sh` hovers a text field, a column gap and a button, with and without a focused search field, and fails when the Wayland log lacks their `set_shape`. Run it after adding anything that covers the window: a MouseArea sets an arrow cursor even while disabled.
- `make headless SCENARIO=scripts/headless/navigation.sh` sends Alt+Left, Alt+Right and the side buttons (`pointer b|f`) with and without the full view open and fails on a wrong route. Run it after touching `Router`, the back and forward shortcuts or what opens a page.
- `make headless SCENARIO=scripts/headless/scroll.sh` checks that back and forward restore a page's scroll offset, a new visit starts at the top and a late layout change doesn't pull the view back, on the list pages, Home and an album's track list. Run it after touching a page's scrolling view or `PageScroll`. A new page's scrolling view declares a `PageScroll`.
- `make headless SCENARIO=scripts/headless/startup.sh` restarts the window with a custom page and then a pinned playlist as the first navigation entry and fails when it doesn't open on that item. Run it after touching `Router`'s start page or the navigation entries.
- `make headless SCENARIO=scripts/headless/home.sh` resizes the window across the compact width and switches the header search between field and button, both with the field on, and rescans a changed library, and fails when Home's shelves are rebuilt or Random albums misses the change. Run it after touching `HomePage` or `Shelf`.
- `make headless SCENARIO=scripts/headless/keys.sh` types Space, the arrows, Ctrl+arrows and F with focus on the rail's Settings entry (Tab reaches it, Enter opens Settings, both rails), on a button, a text field, a dialog, a menu and a dropdown, and fails when a key goes to the wrong place. Run it after adding a field, popup or overlay, or touching the window's `Shortcut`s.
- `make headless SCENARIO=scripts/headless/columns.sh` shrinks a track list's columns and fails on a QML warning. Run it after touching column cells or the column options.
- `make headless SCENARIO=scripts/headless/motion.sh` measures frame gaps while the full view morphs and the bar and side panel slide on a 700-track list, tracks their positions frame by frame and compares mid-motion stills to end shots. It also checks what a click on the bar or side panel opens, a vanished track, reduce motion, layout modes, search from the full view and compact overlay focus. It fails on a gap over `GAP_MAX` (80 ms) or a jump. Run it after touching `MainWindow.qml`, the full view or the side panel.
- `make headless SCENARIO=scripts/headless/feel.sh` times every motion: track changes on each player surface, page changes and their direction with Back and Forward, menus and their rows, the details dialog, queue edits, pane, Library and settings tabs, the rail label switch, the floating pill, shelf paging, presses on buttons, tiles, rows, the rail and a button group, the Statistics bars, the favorite heart, a refused field's shake, then the idle window, a hidden window and speed 0. Each leg checks the frame gaps against `GAP_MAX` (80 ms), a mid-motion still against the end shot, that reduce motion lands at once and that the settled window draws nothing for 1.5 s. `FEEL_LEGS="queue press"` runs only those legs and `FEEL_SPEED=1000` runs them at that animation speed; the whole run takes about 7 minutes (`HEADLESS_TIMEOUT=600`). Run it after touching any motion and report its `gaps.txt` before and after.
- `make headless SCENARIO=scripts/headless/switch.sh` clicks every rail entry three times on a library of 1000 albums and fails on a frame gap over 80 ms the first time or 40 ms after that. Run it after touching `MainWindow.qml`'s pages, `Router`, `BrowsePage` or a tile or row.
- `SERVER=1 make headless SCENARIO=scripts/headless/snappy.sh` measures the frame gaps while an album page opens, Home comes back and the full view opens and closes, checks that a track menu and the details dialog show at their settled size from the first frame, and holds the fake server's answers back (`delay` file, 800 ms) to check that a rating, a playlist reorder and a settings switch show at once. It fails on a gap over `GAP_MAX` (80 ms) or a value that arrives after `SEEN_MAX` (400 ms). Run it after touching page or popup building, the full view or an action's optimistic update.
- `make headless SCENARIO=scripts/headless/handle.sh` drags the full view's handle and fails when the window doesn't move or the closed hand doesn't show, then checks that "Top bar" (Settings → Now playing → Full view) brings the header back without the handle. With title bars off it checks that the header is gone, the handle moves the window above every page and in the full view, the rail shows Search as its button and Settings at its bottom (the bar keeps them as entries), the settings show the lock, and the Settings window's own handle and ✕ replace its header. Run it after touching the full view's header, `DSheetHandle`, `SettingsWindow`, the title bars setting or the forced navigation entries.
- `SERVER=1 make headless SCENARIO=scripts/headless/pages.sh` clicks through the pages, Home and navigation features: the playing tile's plate, shelf titles and arrows, the Library entry and its tabs, the letter index, the page header tint (a grey cover stays neutral), the album page side by side, Speed dial and Genre chips, the rail menu button, the title-bars-off rail, the header field on a narrow window, the three Pages style rows and the Playlists page's collection tiles. It fails when a click does nothing or a layout breaks. Run it after touching a shelf, the tile, a page's header or layout, the rail or `DLetterIndex`.
- `make headless SCENARIO=scripts/headless/menus.sh` checks a tile's buttons after its menu closes and under Tab, and that a menu stays at the pointer in fullscreen with a reserved top area. Run it after touching a tile's hover buttons or `DMenu` placement.
- `make headless SCENARIO=scripts/headless/player.sh` checks the floating player (size, place, page ends above it, the ring opening the full view and filling), the seekbar across the bar, the labelled play button's width, the queue header's switch and Clear, the pane tabs with Last used, the blurred background and the blurred lyric lines, and measures the full view's frame gaps (`GAP_MAX`). Run it after touching the bar, the floating player, the full view's panes or background, `QueueList` or `LyricsView`.
- `SERVER=1 make headless SCENARIO=scripts/headless/downloads.sh` adds offline rules against the fake server, deletes a track, deletes and excludes a kept one, lets the next sync pass, brings it back with Download again, and opens the Add rule dialog for a scope that already has the rule. It fails on a wrong copy count or exclusion. Run it after touching the Downloads settings, `DownloadedTracks` or `cache/`.
- `SERVER=1 make headless SCENARIO=scripts/headless/download.sh` opens a server album, clicks Download, then Downloaded and Remove in the dialog, and checks a local album has no button and a server playlist has one. Run it after touching `DownloadButton`, `PlayButtons` or a page's button row.
- `SERVER=1 make headless SCENARIO=scripts/headless/library.sh` opens a library's page, renames it through the Name dialog, switches scrobbling off and opens Downloads from it, and fails when that library's card isn't expanded. Run it after touching `LibrariesSettings`, `LibraryPage` or the library dialogs.
- `SERVER=1 make headless SCENARIO=scripts/headless/addserver.sh` adds a server in the Add library dialog: the address is checked first, a reachable (Open)Subsonic server leads to the sign-in step, an unreachable address shows the error with Continue anyway and Try again, and editing the address goes back to the first step. Run it after touching `ServerSignIn`, `LibraryDialog` or a source's `Probe`.
- `make headless SCENARIO=scripts/headless/appearance.sh` switches the palette style with a base color and fails when Vibrant leaves the theme unchanged, Tonal spot doesn't bring the first colors back exactly or Monochrome has chroma, then clicks a Base color swatch and fails when the seed color doesn't change, and checks that the settings search for "pause" finds the fixed shortcut on General. Run it after touching `Scheme.js`, `StyleSource`, `AppearanceSettings` or the Keyboard shortcuts rows.
- `make headless SCENARIO=scripts/headless/stats.sh` plays one real track past half, seeds earlier days in the history database and fails when the Statistics page's totals differ from `history.stats`, the 30 days range misses a seed or takes the one 40 days back, the bars aren't one per day, a top artist doesn't open its page or clearing the history leaves the totals. Run it after touching `StatisticsPage`, `StatsService` or `core/internal/history`.
- `make headless SCENARIO=scripts/headless/lyrics.sh` plays a fixture with synced lyrics in both styles and checks word fill, line-timed lines, instrumental breaks, follow, reduce motion and the lyrics menu (sources, timing slider, status icons). Run it after touching `LyricsView`, `LyricsLine` or the lyrics menu.
- `make headless SCENARIO=scripts/headless/reorder.sh` drags a row's handle in the Pages editor and fails when the drop lands in the wrong place. Run it after touching `DInstanceRow`, `DListItem` or the Pages settings.
- `make headless SCENARIO=scripts/headless/smoke.sh` opens every main page and the full view with a track playing, and fails on a QML error. Run it after touching the window launcher or the `qs` subcommand.
- `make headless SCENARIO=scripts/headless/readme.sh` regenerates `docs/screenshots/` from a made-up library. Run it when the look changes.
- `make headless-affected` runs only the scenarios that cover the files changed since `BASE=` (default: the merge-base with `origin/master`, plus uncommitted and untracked files), sets `SERVER=1` or `DPLAYER_BACKEND=builtin` where a scenario needs it and prints which it picked and why. The file-to-scenario table is in `scripts/headless/sweep.sh`: add a row when a scenario's line above changes. A change to docs only picks none; a change to the harness picks all.
- `make headless-all` runs every scenario above except `readme.sh`, `JOBS=` at a time (default 4; `motion`, `snappy`, `navigation`, `player` and `feel` run alone first because frame gaps and click timing fail on a loaded machine), and prints a table of result, seconds and output folder. It exits non-zero on any failure. Every run has its own session, so runs don't share sockets or state. `GAP_MAX`, `SEEN_MAX`, `DELAY` and `SETTLE` reach the scenarios.
- Don't use `make run` or `make quit` for tests: `quit` can stop the developer's real player. Real speakers and devices can't be tested here.

## Architecture rules

- **The daemon owns playback.** Playback, the queue, library index, sources and MPRIS live in the Go daemon. The UI is a view: it sends commands and renders events. Closing the window must not stop playback when "keep running" is on.
- **mpv plays the audio** when it is installed, as a child process controlled over its JSON IPC socket; otherwise the built-in engine (`internal/player/engine`) answers the same IPC in-process and plays fewer formats, with no crossfade or speed. Only `internal/player` talks to either. Every command has a timeout; a crashed backend is restarted and the position restored. A feature that needs mpv goes through the capability fields in the player status, never a check for the binary.
- **Sources are plugins of the core.** Each library is an instance of a source kind (`local`, `subsonic`, …). A new kind is one package under `internal/source/` plus one registry line. Required behavior is a small interface; optional features (server playlists, scrobbling, favorites, write access) are separate interfaces checked with a type assertion. The player, IPC and UI never switch on the kind; they ask for capabilities and show only what the source supports. Model: Dux Calendar's `core/internal/calendar/provider.go`. A server that implements `CatalogSync` is read into the same `library.Store` tables as rows with an empty path, and `Manager.Browser` reads from there once a sync is complete; until then browsing is live.
- **Tracks are addressed by library and id,** never by path or URL alone, so playlists and the queue can mix libraries. Local playlists live in the player and can hold tracks from any library; server playlists belong to their server and are edited through it.
- **Credentials go in the keyring,** never in settings, logs, IPC events or crash output. Stream URLs that carry tokens are secrets too.
- **Optional integrations cost nothing when absent.** Cast and anything else optional is detected, never required; when missing, its UI doesn't show and it does no work.
- **Lyrics** come from `dux-go/lyrics` (the shell's engine). The view is our own, close to the shell's (`quickshell/Modules/DDash/Media/LyricsOverlay.qml` in dux-shell) in feel.
- **Background and keybinds.** Every action has a CLI command (`dplayer play`, `dplayer toggle`, …) that talks to the daemon and starts it hidden if it isn't running, so compositor keybinds work with no window open.

## Dux ecosystem

- Shared widgets and theming come from dux-qml-common. Fix a shared widget there, as its own upstream PR, then bump the submodule here (`make update-common`). Never patch a copy locally, never fork a widget into this repo because the shared one is missing a property.
- A widget dux-qml-common doesn't have yet is built here under `quickshell/Widgets/`, in the library's style (`D` prefix, reads only `DC.Style` and its contract), so it can move upstream unchanged. Once upstream merges it, delete the local copy and bump the submodule.
- Shared Go comes from dux-go (`shellapp`, `lyrics`, `paths`, `portal`, …). Use it before writing your own.
- Follow the shell's colors and style the way the sibling apps do (the shell's color file, light/dark, font scale, radius strength, motion settings). The app must also look right without the shell running.
- The app injects its singletons into DCommon at startup (`DC.Style.theme`, `DC.Style.settings`, `DC.I18n.backend`, `DC.Paths.backend`, `DC.Log.backend`, `DC.Host.*`). The stubs in `dux-qml-common/Common/` and `Services/` list every property the library reads; when bumping the submodule, diff them against ours and add what's new.
- Before building something a sibling app or the shell already has (settings pages, media controls, seekbar, art view, about page), read theirs and match it.

## Design: Material 3 Expressive

The whole Dux suite follows Google's Material 3 Expressive (M3E). The app must look like it shipped with the shell.

- **Tokens only.** Every color, size, spacing, radius, font size, elevation and duration comes from Theme or `DC.Style`. A spec value without a token (M3's 48 px touch target) becomes one named readonly property. No literal colors, no hard-coded alphas.
- **Shapes** follow the M3 shape scale and the radius-strength setting (`dux-qml-common/SHAPES.md`). Use the named radius tokens and `Style.fullRadius()` / `Style.buttonRadius()` helpers, never pixel radii.
- **D\* widgets only** (`DButton`, `DIconButton`, `DFab`, `DSlider`, `DListView`, `DFlickable`, `DGridView`, `DDropdown`, `DTabBar`, `DNavigationBar`, …), never raw Qt Controls or ListView/Flickable/ScrollView. Check `DCommon/Widgets` and the sibling apps before building any control.
- **One filled button per section.** Secondary actions are tonal (`secondaryContainer`), dialog dismiss is a text button. A page's primary action is a FAB, bottom center.
- **Forms and detail panes use the contained list style:** every field or fact is a row in one connected group (2 px gaps, large outer radius), like the shell's Settings pages. One label style and one field style per form, no per-field height or font overrides. Collapsible sections show as one card.
- **Content never outgrows its window.** Clamp popups and sheets to the window and scroll instead.
- **Motion** follows the vocabulary below, uses the shared animation tokens and honors reduce-motion. No always-running animations; anything animated stops when hidden.
- **Interaction:** a row click does the row's main action, its switch only toggles. A disabled action says why (a disabled Qt control shows no tooltip, so say it in visible text). Backspace acts like Delete; removing something takes more than a plain right-click. Keyboard: focus order, Enter to submit, Escape to close.
- **Controls never resize when their state changes.** A button whose label switches (Play/Pause, Show more/less) keeps the width of its widest label in the current language: set `reserveText` to the other label.
- **Accessibility and RTL:** custom controls set role, name and checked state; use `Row`/mirrored anchors, never a left-only anchor chain; flip literal arrows.
- A setting that turns a feature off greys out that feature's other settings.

### Motion

Durations are `Theme.expressiveDurations.*` (`DC.Style` inside `Widgets/`), so they follow the animation speed setting. Reduce motion or speed None cuts every motion to its end state.

- **Spatial by size:** small things (buttons, chips, thumbs, a row moving, a tab indicator) on `expressiveFastSpatial`, cards and tiles on `expressiveDefaultSpatial`, large panels (full view, side panel, rail width, sheets, pages, a carousel page) on `normal`/`large` with `emphasized`, `emphasizedDecel` or `standard`.
- **No overshoot** on large panels and shape morphs (radius, cookie, swatch: `expressiveEffects` on `standard`); a card overshoots at most by its own size.
- **Effects:** fades and colour or blur cross-fades on `expressiveEffects`, quick ones (press-in, hover tint, check marks) on `expressiveFastEffects`.
- **Enter and exit:** something appears on `normal` with `emphasizedDecel`, from the first frame its content is ready; it leaves in about half that, `expressiveEffects` with `emphasizedAccel`.
- **Lists** use `Widgets/DListMotion.qml`: add fades and rises by `PlayerMetrics.listRise`, never staggered; a first build staggers by `listStaggerStep` up to `listStaggerMax`; remove fades and slides toward the end edge; displaced and move run on `expressiveFastSpatial` and force opacity and scale to 1.
- **Press** goes through `DPressFeedback` (in on fast effects, out on fast spatial, width-aware scale); hover never scales, it is the state layer.
- **Text** that changes with the track slides through `TextSlide`; pages entered by Back or Forward settle from `pageBackScale`, new pages from `pageEnterScale`.
- **No new `SpringMotion`** without a reason no `Behavior` with a token can meet.

Hygiene, for every animated file:

- Every `Behavior`, `Transition` and `Animation` is off when `Theme.animationsEnabled` is false (`DC.Style.reduceMotion` or speed None in `Widgets/`).
- Nothing animates on first placement: a value set at creation or while hidden lands at once.
- No Behavior on a value that follows a drag or the pointer; a scroll Behavior only for wheel or key steps.
- `alwaysRunToEnd` only on enter and exit sequences, never on a value that can be retargeted.
- No infinite animation, `FrameAnimation` or repeating `Timer` while its item is hidden, the window is minimized, playback is paused (unless it is about playback) or the motion has settled.
- Durations only from tokens, never literal milliseconds; content paces such as lyric scrolling derive from the base duration.
- A layer or effect that exists for a motion (`layer.enabled`, blur) exists only while the motion runs or its result shows.

## Go

- `CGO_ENABLED=0`, always. No cgo and no GPL-3 dependencies. Something that needs C is an external binary.
- The one exception is `make test-race`, which needs cgo for the race detector; nothing built ships with cgo.
- Guard clauses and early returns; switch over if/else chains.
- Every exec and network call has a timeout and bounded output, and none runs while holding a lock. Processes run with argv, never `sh -c` with data in it. Clamp server-supplied delays and sizes.
- Every failure path returns a readable error, including quiet ones (a clean EOF, a peer that hangs up).
- An optional subsystem never blocks startup; a lost race degrades the feature, not the app.
- No library for a simple problem; a well-maintained one for a complex, common problem (Subsonic client, tag reading, SQLite via `modernc.org/sqlite` like Dux Calendar).
- A struct field that's set but never read in production is a missing feature or dead code: use it or delete it.
- Never edit generated code.

## QML

- **Connections vs bound properties.** In per-item components (delegates, rows, pages), no `Connections` for `on<Prop>Changed`: bind to a typed readonly property (never `var`) and handle its change. Keep `Connections` for plain signals and in `Services/`/`Common/` singletons. A bound handler fires once at creation if the initial value differs from the default, so convert only idempotent bodies.
- **Services never import UI** (`Modules/`). The UI holds a reference count on a service; timers, watchers and subscriptions run only while something is visible and stop when it hides.
- **Qt's JS engine lacks ES2019+** (`Array.flat`/`flatMap`, `Object.fromEntries`). Node-based tests pass anyway, so grep for them.
- **A view with a field in its header takes a `RowModel`,** never an array or a count: a new model makes row 0 current, and that row takes the focus from the field. Set `currentIndex: -1` as well.
- **Every settings row is a direct child of a `SettingsGroup`.** The group works out the corners; a row anywhere else has none.
- A `DDropdown` drops a plain binding once the user picks; re-bind resettable ones with a `Binding` element.
- A content-sized item (Column, Row) inside a settings group needs an explicit `width`, or the group renders 0×0.
- Guard transient empty inputs (`Quickshell.screens` and other live lists can be empty for a moment).
- **qmllint with warnings is the real check**, and `make lint` runs it: an unresolved type or an assignment to a property that doesn't exist only warns, yet breaks at runtime, so every `Could not find property` and unresolved type is a blocker. The lint script builds Quickshell's tooling files by itself when they're missing.
- **A popup or overlay that takes keys stops the window's `Shortcut`s.** A `Shortcut` fires before the focused item; `DMenu` does it with `Keys.onShortcutOverride`.
- **Settings search is generated.** `make settings-index` builds `Modules/Settings/SettingsSearchIndex.js` from the settings pages (`quickshell/scripts/settings_index.py`); `make run` regenerates it and `make check` fails when it is stale. Never edit it by hand. A row, card or section label is found when its title is one literal `I18n.tr(…)` call; its description, option and button labels are searchable too. Give a row `keywords: "…"` for common synonyms its text doesn't contain (English, space-separated), and `searchable: false` when it is a status line rather than a setting. A new row type or a page file goes into the script's tables.

## State and persistence

- Derived state must not depend on which refresh runs first. Test the real startup order.
- Pending/requested flags are cleared on every path, in `finally`. An async reply checks it still belongs to the current request.
- Runtime state (paused, a preview value, a typed password) never overwrites saved settings.
- A changed persisted key or format ships a migration. Reading then writing a file keeps what it doesn't understand.
- Values entering settings or the library from outside (imports, servers, playlist files, tags) are checked against their type before they are stored. Imported and fetched data is hostile: reject control characters, cap sizes, never let it reach a shell.

## Strings

- Every user-facing string goes through `I18n.tr("Text", "translator context")`. Search `quickshell/translations/en.json` first and reuse an existing term where it reads right. Strings under `DCommon/` belong to dux-qml-common.
- Every new string costs a translation per language. Labels carry a dropdown; per-option descriptions belong in docs. A label that names a bare parameter (Intensity, Level) gets one short description.
- Toast titles state the result ("Couldn't connect to server"), never a bare feature name or the raw error.
- IPC and CLI replies stay plain English. Never edit translation catalogs by hand.

### Writing UI text

Write like a shipped desktop app (GNOME, KDE, Android settings): correct, concise, neutral. Agent-written text has needed rewriting again and again for being too long; check every new string against these rules before showing it.

- **Labels** are sentence case noun or verb phrases, two to five words: "Download on metered connections", "Play next order". Options are one or two words: "Off", "Now playing", "Whole app".
- **Descriptions are optional.** Leave one out when the label says it. When needed, one sentence of at most about ten words on what the setting does, never two sentences. Fragments are fine: "Shown on keyboard focus.", "Same as the ⋮ menu".
- **Start with the verb or the fact.** "Resets the bar, side panel and both full views." not "The bar, the side panel and both full views return to their defaults." "Holds tracks from any library." not "Can hold tracks from any library."
- **No "you" or "your".** "the music files", not "your music files"; "It can be added again with Add shelf.", not "You can add it again…".
- **Only what the app knows.** Never describe conditions the app can't detect ("when the server sends its plays to the same accounts"). If it is uncertain, say so in a few words: "Plays may count twice if the server also scrobbles."
- **No mechanics or UI tours.** Don't explain how the feature works inside, which other control changes it ("Turn off the switch above…"), or what each button in view does. The control's own label covers that.
- **Dialogs:** the title names the thing or asks the question; supporting text says only what the buttons can't: usually one short sentence, two when each choice has a consequence to name ("A rule keeps this track and may download it again. Exclude creates an exception for this track from all rules."). Button labels are verbs: "Delete", "Keep", "Delete and exclude".
- **Toasts** state the result in a few words ("Removed Favorites", "Couldn't connect to server"); no second sentence explaining side effects.
- **Disabled reasons** are one imperative sentence: "Turn on a server library first."
- Credits name the role in standard terms ("Audio playback", "Tag reading").

Rewrites the maintainer made, for calibration:

| Written | Shipped |
|---|---|
| Drag to set the navigation order; the eye shows or hides a page there. Select an entry to rename it. Edit opens the page in its edit mode. | Drag to reorder. Use the pencil to rename an entry. |
| Highlights the focused control when using the keyboard. | Shown on keyboard focus. |
| The playlist is deleted from %1 and every device that uses it. The tracks stay in their libraries. | Deletes the playlist from %1 on all devices. Tracks stay in their libraries. |
| A Dux Shell theme file with light and dark palettes. Export the current colors to use as a starting point. | Dux Shell theme file with light and dark palettes. |
| Turn off when the server sends its plays to the same accounts. | Plays may count twice if the server also scrobbles. |

## Markdown

One line per paragraph, list item or table row; never hard-wrap prose at a column. Edits then touch one line, and diffs stay readable.

## Comments

Comment only what the code can't say: a non-obvious why, a trap, a workaround. A line or two. Never narrate code, never explain a rejected alternative (that goes in the commit or PR), and never reference anything a reader of the public repo can't open: local notes, plan or task numbers, agent names, paths outside the repo. Put the fact in the comment instead of a pointer to it. Delete paragraph comments you come across.

## Tests

- A test must fail on the broken code; prove it by breaking the guard once. Otherwise don't write it.
- Test what loses data or breaks a user flow: parsers, the IPC protocol, queue and playlist logic, migrations, source adapters against recorded responses. One table per behavior, outcomes only (no call sequences, wording, layout).
- Go tests use testify and run sandboxed: no network, no system services, `t.TempDir()`.
- QML tests call `.js` modules and `Services/`/`Common/` logic directly; never load `Modules/`, never spawn a compositor or the shell.
- Small fixes don't add tests.

## Commits and pull requests

- Commit messages: `area: short lowercase explanation`, e.g. `player: keep position when the queue is reordered`. A body only when the why isn't obvious, in plain words, wrapped at ~72 columns.
- One PR does one thing. A sibling bug gets its own PR; a shared-widget fix goes to dux-qml-common.
- Once a branch is pushed, add commits instead of rewriting it. Reconcile with `master` by merging, not rebasing.
- Before opening a PR, read the whole diff: every removed line must be explained by the PR, every moved call site keeps its old behavior, and `git log -S` anything you remove that might be an earlier fix.
- PR and issue text is drafted in files the developer can edit, not only on the clipboard.

## Common commands

`make` is the interface for everything, locally and in CI:

- `make check`: the gate before calling work done, and what CI runs. It is `make lint` (gofmt, `go vet`, `go mod tidy`, golangci-lint, qmllint with warnings, qmlformat, a stale settings search index; it only reports and never changes files), `make test` (sandboxed Go tests in a new order each run, and the QML logic tests), `make test-race` (the Go tests under the race detector; needs gcc) and `make build` (the release binary with the UI embedded).
- `make headless-affected`: the headless scenarios that cover the changed files. `make headless-all`: every scenario, `JOBS=` at a time.
- `make run`: a fast dev build, then the window from the in-repo QML with hot reload.
- `make run-default`: like `make run`, with new player dirs on every run except the libraries copied from the real `library.db`.
- `make fmt`: format the Go code (gofmt) and the QML (qmlformat, options in `.qmlformat.ini`).
- `make settings-index`: regenerate the settings search index after changing a settings page.

`make help` lists the rest (`install`, `dist`, `i18n-extract`, `update-common`).
