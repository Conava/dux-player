# Contributing

Thanks for helping with Dux Player. It is in early development (0.0.1-alpha), so expect things to move fast.

## Before you start

- Please open an issue or discussion before big changes, rewrites or refactorings.
- [AGENTS.md](AGENTS.md) holds the project rules for code, design, strings, tests and commits. They apply to people and AI agents alike; read them before your first change.

## Setup

```sh
git clone --recursive https://github.com/Conava/dux-player
cd dux-player
make run
```

You need Go, Quickshell with Qt 6, mpv and Node.js (for the QML logic tests). `make help` lists every target.

## Making a change

- Try to keep PRs small and focused. One PR per feature is preferred, but small changes or bug fixes discovered on the way can be included. A fix to a shared widget goes to [dux-qml-common](https://github.com/duxlabs-dev/dux-qml-common).
- Run `make check` (lint, tests and build, the same as CI) before you open a pull request.
- Try the change by hand with `make run` and describe any steps necessary to test your change in the PR. Every visual change or addition requires a screenshot.
- User-facing text goes through `I18n.tr()`. Run `make i18n-extract` when you add or change strings; never edit the translation files by hand.
- Keep the docs current: a change to features, requirements, commands, settings or online services updates the README and any other affected docs in the same PR.
- Commit messages: `area: short lowercase explanation`, for example `player: keep position when the queue is reordered`.

## Reporting bugs

Open an issue with what you did, what you expected and what happened, plus `dplayer version` and your distribution. Logs from `dplayer daemon` in a terminal help.
