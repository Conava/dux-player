# Contributing

Thanks for helping with Dux Player. It is in early development (0.0.1-alpha), so expect things to move fast.

## Before you start

- Please open an issue or discussion before big changes, rewrites or refactorings.
- Open an issue with a sketch or screenshot before building a new screen or control. Scope and look are the maintainer's call.
- AI tools are welcome for writing code, but you own the result: understand every line you submit. Issues, pull requests and comments are written by you, not by an AI.
- [AGENTS.md](AGENTS.md) is the detailed rule set for AI agents working on the project. You don't need to read it to contribute.

## Setup

```sh
git clone --recursive https://github.com/Conava/dux-player
cd dux-player
make run
```

You need Go, dux-qt-base or Quickshell with Qt 6 (`make lint` needs Quickshell; `DPLAYER_QS_CHILD=1` runs the UI on it), mpv and Node.js (for the QML logic tests). `make check` also needs gcc, for the race detector. The tests that play or measure audio run only when mpv and ffmpeg are installed. `make run-builtin` runs the player with its built-in playback instead of mpv. `make help` lists every target.

## Making a change

- Try to keep PRs small and focused. One PR per feature is preferred, but small changes or bug fixes discovered on the way can be included. A fix to a shared widget goes to [dux-qml-common](https://github.com/duxlabs-dev/dux-qml-common).
- Run `make check` (lint, tests and build, the same as CI) before you open a pull request. Run it as a normal user: root ignores file permissions, which some tests rely on. `make test-stress` repeats the Go tests in a new order each round, to bring out one that fails only sometimes.
- Try the change by hand with `make run`, then use the rest of the app around it to check nothing else broke. Describe the steps to test your change in the PR. Every visual change or addition requires a screenshot.
- The UI follows Material 3 Expressive like the rest of the Dux suite. Use the shared widgets (`DButton`, `DSlider`, …) from [dux-qml-common](https://github.com/duxlabs-dev/dux-qml-common) and take colors, sizes, spacing and radii from `Theme` or `DC.Style`, never fixed values.
- UI text is short and in sentence case: "Download on metered connections", not "Download On Metered Connections". Leave out descriptions the label already says, and don't address the user as "you".
- Go code builds without cgo, and new dependencies must not be GPL-3. Passwords and sign-in tokens go in the keyring, never in settings or logs.
- User-facing text goes through `I18n.tr()`. Run `make i18n-extract` when you add or change strings; never edit the translation files by hand.
- Keep the docs current: a change to features, requirements, commands, settings or online services updates the README and any other affected docs in the same PR.
- Commit messages: `area: short lowercase explanation`, for example `player: keep position when the queue is reordered`.
- Once a branch is pushed, add commits instead of force-pushing, and merge `master` into it instead of rebasing.

## Reporting bugs

Open an issue with what you did, what you expected and what happened, plus `dplayer version` and your distribution. Logs from `dplayer daemon` in a terminal help.
