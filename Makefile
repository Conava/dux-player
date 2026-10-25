# Root Makefile for Dux Player. The Go core builds in core/.

BINARY_NAME=dplayer
APP_ID=com.marlonkranz.player
CORE_DIR=core
BUILD_DIR=$(CORE_DIR)/bin
SHELL_DIR=quickshell
ASSETS_DIR=assets

PREFIX ?= /usr/local
DESTDIR ?=
BIN_DIR=$(PREFIX)/bin
DATA_DIR=$(PREFIX)/share
LICENSE_DIR ?= $(DATA_DIR)/licenses/dux-player

# TagLib (LGPL-2.1) is compiled into the binary as WebAssembly, so its license ships with it.
TAGLIB_LICENSE = $(firstword $(wildcard $(CORE_DIR)/vendor/go.senan.xyz/taglib/LICENSE) \
	$(shell cd $(CORE_DIR) && go list -m -f '{{.Dir}}' go.senan.xyz/taglib 2>/dev/null)/LICENSE)

.PHONY: all build dev run clean test test-go test-qml lint lint-go lint-qml fmt vet install uninstall quit \
	update-common i18n-extract help

all: build

build:
	@$(MAKE) -C $(CORE_DIR) build

dev:
	@$(MAKE) -C $(CORE_DIR) dev

# Opens the window from the source tree, so QML edits hot-reload. A daemon left over from an older build is
# stopped first, or the new window would attach to the old code.
run: dev quit
	$(BUILD_DIR)/$(BINARY_NAME) show -c $(CURDIR)/$(SHELL_DIR)

# Stops the running player: daemon, window, mpv and its PipeWire children.
quit:
	@$(BUILD_DIR)/$(BINARY_NAME) quit 2>/dev/null || true

clean:
	@$(MAKE) -C $(CORE_DIR) clean

test: test-go test-qml

test-go:
	@$(MAKE) -C $(CORE_DIR) test

test-qml:
	@for t in $(SHELL_DIR)/tests/*.test.js; do node "$$t" || exit 1; done

lint: lint-go lint-qml

lint-go:
	@$(MAKE) -C $(CORE_DIR) lint

# Every .qml with warnings on; builds the Quickshell tooling VFS first when it is missing or stale.
lint-qml:
	@./$(SHELL_DIR)/scripts/qmllint-entrypoints.sh

fmt:
	@$(MAKE) -C $(CORE_DIR) fmt

vet:
	@$(MAKE) -C $(CORE_DIR) vet

install:
	@test -f $(BUILD_DIR)/$(BINARY_NAME) || { echo "$(BUILD_DIR)/$(BINARY_NAME) not found; run 'make build' first"; exit 1; }
	install -Dm755 $(BUILD_DIR)/$(BINARY_NAME) $(DESTDIR)$(BIN_DIR)/$(BINARY_NAME)
	install -Dm644 $(ASSETS_DIR)/$(APP_ID).desktop $(DESTDIR)$(DATA_DIR)/applications/$(APP_ID).desktop
	install -Dm644 $(ASSETS_DIR)/$(APP_ID).svg $(DESTDIR)$(DATA_DIR)/icons/hicolor/scalable/apps/$(APP_ID).svg
	install -Dm644 $(ASSETS_DIR)/$(APP_ID).metainfo.xml $(DESTDIR)$(DATA_DIR)/metainfo/$(APP_ID).metainfo.xml
	install -Dm644 LICENSE $(DESTDIR)$(LICENSE_DIR)/LICENSE
	install -Dm644 $(TAGLIB_LICENSE) $(DESTDIR)$(LICENSE_DIR)/LICENSE.taglib
	@test -n "$(DESTDIR)" || { gtk-update-icon-cache -q $(DATA_DIR)/icons/hicolor; update-desktop-database -q $(DATA_DIR)/applications; } 2>/dev/null || true

uninstall:
	rm -f $(DESTDIR)$(BIN_DIR)/$(BINARY_NAME) \
		$(DESTDIR)$(DATA_DIR)/applications/$(APP_ID).desktop \
		$(DESTDIR)$(DATA_DIR)/icons/hicolor/scalable/apps/$(APP_ID).svg \
		$(DESTDIR)$(DATA_DIR)/metainfo/$(APP_ID).metainfo.xml \
		$(DESTDIR)$(LICENSE_DIR)/LICENSE $(DESTDIR)$(LICENSE_DIR)/LICENSE.taglib
	-rmdir $(DESTDIR)$(LICENSE_DIR) 2>/dev/null

update-common:
	git submodule update --remote --merge dux-qml-common

# Regenerates quickshell/translations/en.json (and template.json) from the I18n.tr() calls.
i18n-extract:
	@python3 $(SHELL_DIR)/translations/extract_translations.py

help:
	@echo "Develop:"
	@echo "  run            - Dev build, then open the window from quickshell/ with hot reload"
	@echo "  quit           - Stop the running player (daemon, window, mpv)"
	@echo "  dev            - Fast build without the embedded UI ($(BUILD_DIR)/$(BINARY_NAME))"
	@echo "  build          - Release build with the UI embedded; VERSION= overrides git describe"
	@echo "  clean"
	@echo ""
	@echo "Check (CI runs lint, test and build):"
	@echo "  lint           - lint-go (gofmt -s, go vet) and lint-qml (qmllint with warnings; needs qs and qmllint)"
	@echo "  test           - test-go (sandboxed Go tests) and test-qml (node logic tests)"
	@echo "  fmt / vet"
	@echo ""
	@echo "Install (PREFIX=$(PREFIX), DESTDIR for staging):"
	@echo "  install        - Binary, desktop entry, icon, metainfo, licenses (run make build first)"
	@echo "  uninstall"
	@echo ""
	@echo "Other:"
	@echo "  update-common  - Bump the dux-qml-common submodule"
	@echo "  i18n-extract   - Regenerate translations/en.json from I18n.tr() calls"
