# Dux Player

A standalone music player for Linux in the look and feel of [Dux Shell](https://github.com/duxlabs-dev/dux-shell). It plays local libraries and playlists and streams from remote servers ((Open-)Subsonic/Jellyfin for now). Go daemon and CLI plus a Quickshell (QML) UI, built on [dux-qml-common](https://github.com/duxlabs-dev/dux-qml-common) and [dux-go](https://github.com/duxlabs-dev/dux-go) like the other Dux apps.

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

1. `make check` (lint, test and build) passes.
2. **The developer has tried it by hand.** Hand over one command, say what to look at and what working looks like. A green test suite is not the end.
3. `make run` exercises the change end to end with no extra env vars, flags or wrappers. If it doesn't, fix your code or the script if really necessary.
4. No dead code, no leftover old path, comments pass the comment rules.
5. The README and other docs match the change (features, requirements, commands, settings, online services).

### Testing without touching the live desktop

The developer's desktop session, often Dux Shell, keeps running while you work. Never restart the shell or other session services, never `make install`, and never write to the developer's real config (the shell's or the player's): test runs use sandboxed `XDG_*` dirs unless asked otherwise. Never run a script "to see what it prints" if it touches system state; read it. Never pick PIDs with `pgrep -f <pattern>` (it matches your own shell); start test processes yourself and use `$!`.

## Architecture rules

- **The daemon owns playback.** Playback, the queue, library index, sources and MPRIS live in the Go daemon. The UI is a view: it sends commands and renders events. Closing the window must not stop playback when "keep running" is on.
- **mpv plays the audio**, as a child process controlled over its JSON IPC socket. Only `internal/player` talks to mpv. Every mpv command has a timeout; a crashed mpv is restarted and the position restored.
- **Sources are plugins of the core.** Each library is an instance of a source kind (`local`, `subsonic`, …). A new kind is one package under `internal/source/` plus one registry line. Required behavior is a small interface; optional features (server playlists, scrobbling, favorites, write access) are separate interfaces checked with a type assertion. The player, IPC and UI never switch on the kind; they ask for capabilities and show only what the source supports. Model: Dux Calendar's `core/internal/calendar/provider.go`.
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
- **Motion** uses the shared animation tokens and honors reduce-motion. No always-running animations; anything animated stops when hidden.
- **Interaction:** a row click does the row's main action, its switch only toggles. A disabled action says why (a disabled Qt control shows no tooltip, so say it in visible text). Backspace acts like Delete; removing something takes more than a plain right-click. Keyboard: focus order, Enter to submit, Escape to close.
- **Accessibility and RTL:** custom controls set role, name and checked state; use `Row`/mirrored anchors, never a left-only anchor chain; flip literal arrows.
- A setting that turns a feature off greys out that feature's other settings.

## Go

- `CGO_ENABLED=0`, always. No cgo and no GPL-3 dependencies. Something that needs C is an external binary.
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
- A `DDropdown` drops a plain binding once the user picks; re-bind resettable ones with a `Binding` element.
- A content-sized item (Column, Row) inside a settings group needs an explicit `width`, or the group renders 0×0.
- Guard transient empty inputs (`Quickshell.screens` and other live lists can be empty for a moment).
- **qmllint with warnings is the real check**, and `make lint` runs it: an unresolved type or an assignment to a property that doesn't exist only warns, yet breaks at runtime, so every `Could not find property` and unresolved type is a blocker. The lint script builds Quickshell's tooling files by itself when they're missing.

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
- Write like a shipped desktop app (GNOME, KDE, Android settings): correct, concise, neutral. Labels are sentence case noun or verb phrases. A description is one plain sentence on what the setting changes, only when the label doesn't already say it. No chatty or casual tone, no figurative phrasing, no addressing the user as a friend, no explaining how the app works inside. Credits name the role in standard terms ("Audio playback", "Tag reading").
- IPC and CLI replies stay plain English. Never edit translation catalogs by hand.

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

- `make check`: the gate before calling work done, and what CI runs. It is `make lint` (gofmt, `go vet`, `go mod tidy`, golangci-lint, qmllint with warnings, qmlformat; it only reports and never changes files), `make test` (sandboxed Go tests and the QML logic tests) and `make build` (the release binary with the UI embedded).
- `make run`: a fast dev build, then the window from the in-repo QML with hot reload.
- `make fmt`: format the Go code (gofmt) and the QML (qmlformat, options in `.qmlformat.ini`).

`make help` lists the rest (`install`, `dist`, `i18n-extract`, `update-common`).
