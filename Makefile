# Root Makefile for Dux Player. The Go core builds in core/.

BINARY_NAME=dplayer
DEV_BINARY=dplayer-dev
APP_ID=io.github.conava.DuxPlayer
CORE_DIR=core
BUILD_DIR=$(CORE_DIR)/bin
SHELL_DIR=quickshell
ASSETS_DIR=assets
QMLFORMAT ?= /usr/lib/qt6/bin/qmlformat
# find doesn't follow the DCommon symlink, so the submodule is left out
QML_FILES = $(shell find $(SHELL_DIR) -name '*.qml')

PREFIX ?= /usr/local
DESTDIR ?=
BIN_DIR=$(PREFIX)/bin
DATA_DIR=$(PREFIX)/share
LICENSE_DIR ?= $(DATA_DIR)/licenses/dux-player

.PHONY: all build dev run run-builtin headless headless-all headless-affected deps clean check test test-go test-qml test-race test-stress lint lint-go lint-qml lint-qml-format lint-settings-index fmt vet install uninstall quit \
	dist update-common i18n-extract settings-index help

all: build

build:
	@$(MAKE) -C $(CORE_DIR) build

dev:
	@$(MAKE) -C $(CORE_DIR) dev

# Opens the window from the source tree, so QML edits hot-reload. A daemon left over from an older build is
# stopped first, or the new window would attach to the old code.
run: deps settings-index dev quit
	$(BUILD_DIR)/$(DEV_BINARY) show -c $(CURDIR)/$(SHELL_DIR)

# Like run, with the built-in player playing even though mpv is installed.
run-builtin: deps dev quit
	DPLAYER_BACKEND=builtin $(BUILD_DIR)/$(DEV_BINARY) show -c $(CURDIR)/$(SHELL_DIR)

# The real app in a nested Hyprland, invisible on the desktop: scripts/headless/run.sh explains it. SCENARIO= picks one.
headless: deps dev
	@scripts/headless/run.sh $(SCENARIO)

# Every scenario, JOBS= at a time (default 4); a table of results at the end.
headless-all: deps dev
	@scripts/headless/sweep.sh all

# Only the scenarios that cover the files changed since BASE= (default: merge-base with origin/master).
headless-affected: deps dev
	@scripts/headless/sweep.sh affected $(BASE)

# What run needs; install only warns about the runtime part, since a staged or packaged install runs elsewhere.
RUNTIME_DEPS = qs
deps:
	@for c in go $(RUNTIME_DEPS); do command -v $$c >/dev/null || { echo "$$c not found; see Requirements in the README"; exit 1; }; done
	@test -f $(SHELL_DIR)/DCommon/Widgets/DIcon.qml || { echo "dux-qml-common is missing: run git submodule update --init"; exit 1; }

# Stops the running player: daemon, window, mpv and its PipeWire children.
quit:
	@{ $(BUILD_DIR)/$(DEV_BINARY) quit || dplayer quit; } 2>/dev/null || true

clean:
	@$(MAKE) -C $(CORE_DIR) clean

# What CI runs.
check: lint test test-race build

test: test-go test-qml

test-go:
	@$(MAKE) -C $(CORE_DIR) test

test-qml:
	@for t in $(SHELL_DIR)/tests/*.test.js; do node "$$t" || exit 1; done

# The Go tests under the race detector; needs a C compiler (gcc).
test-race:
	@$(MAKE) -C $(CORE_DIR) test-race

# The Go tests COUNT times (default 20), each round in a new order.
test-stress:
	@$(MAKE) -C $(CORE_DIR) test-stress

lint: lint-go lint-qml lint-qml-format lint-settings-index

lint-go:
	@$(MAKE) -C $(CORE_DIR) lint

# Every .qml with warnings on; builds the Quickshell tooling VFS first when it is missing or stale.
lint-qml:
	@./$(SHELL_DIR)/scripts/qmllint-entrypoints.sh

# Options come from .qmlformat.ini, which editors read too.
lint-qml-format:
	@command -v $(QMLFORMAT) >/dev/null || { echo "qmlformat (Qt 6) not found; set QMLFORMAT="; exit 127; }
	@out="$$(for f in $(QML_FILES); do $(QMLFORMAT) "$$f" | cmp -s - "$$f" || echo "$$f"; done)"; \
		[ -z "$$out" ] || { echo "not qmlformat clean (run make fmt):"; echo "$$out"; exit 1; }

# The settings search index is generated from the settings pages; a stale one fails the check.
lint-settings-index:
	@python3 $(SHELL_DIR)/scripts/settings_index.py --check

fmt:
	@$(MAKE) -C $(CORE_DIR) fmt
	@$(QMLFORMAT) -i $(QML_FILES)

vet:
	@$(MAKE) -C $(CORE_DIR) vet

install:
	@test -f $(BUILD_DIR)/$(BINARY_NAME) || { echo "$(BUILD_DIR)/$(BINARY_NAME) not found; run 'make build' first"; exit 1; }
	install -Dm755 $(BUILD_DIR)/$(BINARY_NAME) $(DESTDIR)$(BIN_DIR)/$(BINARY_NAME)
	install -Dm644 $(ASSETS_DIR)/$(APP_ID).desktop $(DESTDIR)$(DATA_DIR)/applications/$(APP_ID).desktop
	install -Dm644 $(ASSETS_DIR)/$(APP_ID).svg $(DESTDIR)$(DATA_DIR)/icons/hicolor/scalable/apps/$(APP_ID).svg
	install -Dm644 $(ASSETS_DIR)/$(APP_ID).metainfo.xml $(DESTDIR)$(DATA_DIR)/metainfo/$(APP_ID).metainfo.xml
	install -Dm644 LICENSE $(DESTDIR)$(LICENSE_DIR)/LICENSE
	scripts/third-party-licenses.sh > $(DESTDIR)$(LICENSE_DIR)/THIRD_PARTY_LICENSES
	@test -n "$(DESTDIR)" || for c in $(RUNTIME_DEPS); do command -v $$c >/dev/null || echo "warning: $$c not found; dplayer needs it to run"; done
	@test -n "$(DESTDIR)" || { gtk-update-icon-cache -q $(DATA_DIR)/icons/hicolor; update-desktop-database -q $(DATA_DIR)/applications; } 2>/dev/null || true

uninstall:
	rm -f $(DESTDIR)$(BIN_DIR)/$(BINARY_NAME) \
		$(DESTDIR)$(DATA_DIR)/applications/$(APP_ID).desktop \
		$(DESTDIR)$(DATA_DIR)/icons/hicolor/scalable/apps/$(APP_ID).svg \
		$(DESTDIR)$(DATA_DIR)/metainfo/$(APP_ID).metainfo.xml \
		$(DESTDIR)$(LICENSE_DIR)/LICENSE $(DESTDIR)$(LICENSE_DIR)/THIRD_PARTY_LICENSES
	-rmdir $(DESTDIR)$(LICENSE_DIR) 2>/dev/null
	@test -n "$(DESTDIR)" || { gtk-update-icon-cache -q $(DATA_DIR)/icons/hicolor; update-desktop-database -q $(DATA_DIR)/applications; } 2>/dev/null || true

# Release artifacts in dist/: one `make install` tree per architecture, a
# source tarball that builds offline (submodule and vendored Go modules included), and SHA256SUMS.
# Asset names follow `uname -m`.
DIST_ARCHES = amd64:x86_64 arm64:aarch64
dist:
	@test -n "$(VERSION)" || { echo "set VERSION, e.g. make dist VERSION=0.1.0"; exit 1; }
	@rm -rf dist stage-*
	@mkdir -p dist
	@for arch in $(DIST_ARCHES); do \
		GOARCH=$${arch%%:*} $(MAKE) build VERSION="$(VERSION)" $(if $(COMMIT),COMMIT="$(COMMIT)") || exit 1; \
		$(MAKE) install DESTDIR="$(CURDIR)/stage-$${arch##*:}" PREFIX=/usr || exit 1; \
		tar -C "stage-$${arch##*:}" -czf "dist/dplayer-$(VERSION)-$${arch##*:}.tar.gz" usr || exit 1; \
	done
	cd $(CORE_DIR) && go mod vendor
	{ git ls-files --recurse-submodules; find $(CORE_DIR)/vendor -type f; } | \
		tar -czf "dist/dux-player-$(VERSION).tar.gz" --transform "s,^,dux-player-$(VERSION)/,S" -T -
	@rm -rf $(CORE_DIR)/vendor stage-*
	cd dist && sha256sum * > SHA256SUMS

update-common:
	git submodule update --remote --merge dux-qml-common

# Regenerates quickshell/Modules/Settings/SettingsSearchIndex.js from the settings pages.
settings-index:
	@python3 $(SHELL_DIR)/scripts/settings_index.py

# Regenerates quickshell/translations/en.json (and template.json) from the I18n.tr() calls.
i18n-extract:
	@python3 $(SHELL_DIR)/translations/extract_translations.py

help:
	@echo "Develop:"
	@echo "  run            - Dev build, then open the window from quickshell/ with hot reload"
	@echo "  run-builtin    - Same, playing with the built-in player instead of mpv"
	@echo "  headless       - Run the app in a sandboxed nested compositor and take screenshots; SCENARIO=file.sh"
	@echo "  headless-all   - Run every headless scenario, JOBS= at a time (default 4)"
	@echo "  headless-affected - Run the headless scenarios that cover the changed files; BASE= picks the base"
	@echo "  quit           - Stop the running player (daemon, window, mpv)"
	@echo "  deps           - Check that go and qs are installed (run does this first)"
	@echo "  dev            - Fast build without the embedded UI ($(BUILD_DIR)/$(DEV_BINARY))"
	@echo "  build          - Release build with the UI embedded; VERSION= overrides git describe"
	@echo "  clean"
	@echo ""
	@echo "Check:"
	@echo "  check          - lint, test, test-race and build: what CI runs"
	@echo "  lint           - lint-go (gofmt -s, go vet, go mod tidy, golangci-lint), lint-qml (qmllint with warnings; needs qs), lint-qml-format (qmlformat) and lint-settings-index"
	@echo "  test           - test-go (sandboxed Go tests, in a new order each run) and test-qml (node logic tests)"
	@echo "  test-race      - The Go tests under the race detector (needs gcc)"
	@echo "  test-stress    - The Go tests COUNT times (default 20), to catch one that fails only sometimes"
	@echo "  fmt            - gofmt -s the Go code and qmlformat the QML"
	@echo "  vet"
	@echo ""
	@echo "Install (PREFIX=$(PREFIX), DESTDIR for staging):"
	@echo "  install        - Binary, desktop entry, icon, metainfo, licenses (after make build; PREFIX=~/.local for one user)"
	@echo "  uninstall"
	@echo ""
	@echo "Release:"
	@echo "  dist           - Release tarballs and checksums in dist/ (VERSION= required)"
	@echo ""
	@echo "Other:"
	@echo "  update-common  - Bump the dux-qml-common submodule"
	@echo "  i18n-extract   - Regenerate translations/en.json from I18n.tr() calls"
	@echo "  settings-index - Regenerate the settings search index from the settings pages (run does this too)"
