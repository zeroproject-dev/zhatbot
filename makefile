# =====================================================
# zhatBot - Makefile (web + wails desktop)
# =====================================================

APP_NAME    := zhatBot
WEB_DIR     := web
DESKTOP_DIR := desktop
ASSETS_DIR  := $(DESKTOP_DIR)/appassets

WIN_GOOS   := windows
WIN_GOARCH := amd64
WIN_CC     := x86_64-w64-mingw32-gcc

BUN_INSTALL := $(PWD)/$(WEB_DIR)/.bun
BUN_TMPDIR := $(PWD)/$(WEB_DIR)/.bun-tmp

# =====================================================
# Help
# =====================================================

.PHONY: help
help:
	@echo ""
	@echo "Targets:"
	@echo "  make web-build                 → build frontend (bun)"
	@echo "  make desktop-assets            → copy web/build into desktop/appassets (for embed)"
	@echo "  make win-build                 → Windows portable EXE"
	@echo "  make win-build-dev             → Windows EXE (debug/devtools)"
	@echo "  make win-build-dev-legacy      → DevTools + legacy WebView2 loader"
	@echo "  make inspect-web-build         → Check web/build structure"
	@echo "  make inspect-desktop-assets    → Check desktop/appassets structure"
	@echo "  make headless                  → build headless bot only"
	@echo "  make clean                     → remove build artifacts"
	@echo ""

# =====================================================
# Frontend
# =====================================================

.PHONY: web-build
web-build:
	@echo "==> Building frontend (bun)"
	cd $(WEB_DIR) && \
	BUN_INSTALL=$(BUN_INSTALL) \
	BUN_TMPDIR=$(BUN_TMPDIR) \
	bun run build

# =====================================================
# Desktop assets (embed)
# =====================================================

.PHONY: desktop-assets
desktop-assets: web-build
	@echo "==> Syncing frontend build into desktop/appassets"
	rm -rf $(ASSETS_DIR)
	mkdir -p $(ASSETS_DIR)
	cp -a $(WEB_DIR)/build/. $(ASSETS_DIR)/
	@echo "==> Verifying desktop/appassets"
	test -f $(ASSETS_DIR)/index.html
	test -d $(ASSETS_DIR)/_app

# =====================================================
# Windows builds (Wails)
# =====================================================

.PHONY: win-build
win-build: desktop-assets
	@echo "==> Building Windows portable EXE"
	cd $(DESKTOP_DIR) && \
	ZHATBOT_MODE=production \
	GOOS=$(WIN_GOOS) GOARCH=$(WIN_GOARCH) CGO_ENABLED=1 CC=$(WIN_CC) \
	wails build \
		-clean \
		-platform windows/amd64 \
		-o $(APP_NAME).exe

.PHONY: win-build-dev
win-build-dev: desktop-assets
	@echo "==> Building Windows EXE (DevTools ENABLED)"
	cd $(DESKTOP_DIR) && \
	ZHATBOT_MODE=development \
	GOOS=$(WIN_GOOS) GOARCH=$(WIN_GOARCH) CGO_ENABLED=1 CC=$(WIN_CC) \
	wails build \
		-clean \
		-debug \
		-devtools \
		-platform windows/amd64 \
		-o $(APP_NAME)-dev.exe

.PHONY: win-build-dev-legacy
win-build-dev-legacy: desktop-assets
	@echo "==> Building Windows EXE (DevTools + legacy WebView2 loader)"
	cd $(DESKTOP_DIR) && \
	ZHATBOT_MODE=development \
	GOOS=$(WIN_GOOS) GOARCH=$(WIN_GOARCH) CGO_ENABLED=1 CC=$(WIN_CC) \
	wails build \
		-clean \
		-debug \
		-devtools \
		-tags native_webview2loader \
		-platform windows/amd64 \
		-o $(APP_NAME)-dev-legacy.exe

# =====================================================
# Headless bot
# =====================================================

.PHONY: headless
headless:
	@echo "==> Building headless bot"
	go build -o bin/zhatbot ./cmd/bot

# =====================================================
# Clean
# =====================================================

.PHONY: clean
clean:
	@echo "==> Cleaning build artifacts"
	rm -rf $(ASSETS_DIR)
	rm -rf $(DESKTOP_DIR)/build
	rm -rf bin/zhatbot

# =====================================================
# Inspect helpers
# =====================================================

.PHONY: inspect-web-build
inspect-web-build:
	@echo "==> Inspecting web/build"
	@ls -la $(WEB_DIR)/build | head -n 200
	@test -d $(WEB_DIR)/build/_app && \
		echo "OK: web/build/_app exists" || \
		(echo "ERROR: web/build/_app missing" && exit 1)

.PHONY: inspect-desktop-assets
inspect-desktop-assets:
	@echo "==> Inspecting desktop/appassets"
	@ls -la $(ASSETS_DIR) | head -n 200
	@test -d $(ASSETS_DIR)/_app && \
		echo "OK: desktop/appassets/_app exists" || \
		(echo "ERROR: desktop/appassets/_app missing" && exit 1)
