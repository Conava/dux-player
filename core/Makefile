BINARY_NAME=dplayer
SOURCE_DIR=cmd/dplayer
BUILD_DIR=bin
SHELL_SRC=../quickshell
EMBED_DIR=internal/shellembed/dist

GO=go

BASE_VERSION=$(or $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//'),0.0.1-alpha)
COMMIT_COUNT=$(shell git rev-list --count HEAD 2>/dev/null || echo "0")
COMMIT_HASH=$(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "unknown")
VERSION?=$(BASE_VERSION)+git$(COMMIT_COUNT).$(COMMIT_HASH)
BUILD_TIME?=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT?=$(COMMIT_HASH)

SCROBBLE_PKG=github.com/Conava/dux-player/core/internal/scrobble
BUILD_LDFLAGS=-ldflags='-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.Commit=$(COMMIT) -X $(SCROBBLE_PKG).LastFMAPIKey=$(LASTFM_API_KEY) -X $(SCROBBLE_PKG).LastFMAPISecret=$(LASTFM_API_SECRET)'

.PHONY: all build dev sync-shell clean test cover fmt vet lint deps help

all: build

# tar -h dereferences the DCommon submodule symlink (go:embed rejects
# symlinks); .qmlls.ini is a symlink into the quickshell runtime VFS.
sync-shell:
	@test -f $(SHELL_SRC)/DCommon/Widgets/DIcon.qml || { echo "DCommon missing: run git submodule update --init"; exit 1; }
	@rm -rf $(EMBED_DIR)
	@mkdir -p $(EMBED_DIR)
	@tar -C $(SHELL_SRC) --exclude=.qmlls.ini -chf - . | tar -C $(EMBED_DIR) -xf -
	@rm -rf $(EMBED_DIR)/scripts $(EMBED_DIR)/tests $(EMBED_DIR)/.claude

# Release build: the UI tree is embedded. `dev` builds without it; run the
# UI from the source tree with `dplayer show -c ../quickshell`.
build: sync-shell
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build -tags withshell $(BUILD_LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(SOURCE_DIR)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

dev:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) ./$(SOURCE_DIR)

clean:
	@rm -rf $(BUILD_DIR) $(EMBED_DIR)

test:
	CGO_ENABLED=0 GIT_CONFIG_GLOBAL=/dev/null $(GO) test ./...

cover:
	CGO_ENABLED=0 GIT_CONFIG_GLOBAL=/dev/null $(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

fmt:
	gofmt -s -w cmd internal

vet:
	CGO_ENABLED=0 $(GO) vet ./...

lint: vet
	@out="$$(gofmt -s -l cmd internal)"; [ -z "$$out" ] || { echo "not gofmt -s clean (run make fmt):"; echo "$$out"; exit 1; }

deps:
	$(GO) mod tidy
	$(GO) mod download

help:
	@echo "Targets: build dev sync-shell clean test cover fmt vet lint deps"
