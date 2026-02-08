# Mote Makefile
#
# 编译分次:
#   1. make build       - CLI + Embed UI (单独编译)
#   2. make build-full  - Wails App + CLI + Embed UI (完整编译)
#
# 支持平台: macOS (arm64/amd64), Windows (amd64)

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -ldflags "-X mote/internal/cli.Version=$(VERSION) -X mote/internal/cli.GitCommit=$(GIT_COMMIT) -X mote/internal/cli.BuildTime=$(BUILD_TIME)"

BUILD_DIR := build/bin
DIST_DIR := dist

.PHONY: all build build-full clean test lint fmt tidy
.PHONY: build-ui build-cli dev help
.PHONY: build-gui build-all package-darwin package-windows gui-dev

# =============================================================================
# 默认目标
# =============================================================================

all: build

help:
	@echo "Mote Build System"
	@echo ""
	@echo "基本命令:"
	@echo "  make build        - 编译 CLI + Embed UI (本地平台)"
	@echo "  make build-full   - 完整编译 Wails App + CLI (本地平台)"
	@echo "  make dev          - 启动开发模式 (Wails dev)"
	@echo "  make clean        - 清除构建产物"
	@echo ""
	@echo "交叉编译:"
	@echo "  make release-macos   - macOS Universal Binary (arm64 + amd64)"
	@echo "  make release-windows - Windows amd64"
	@echo "  make release-all     - 所有平台"
	@echo ""
	@echo "其他:"
	@echo "  make test         - 运行测试"
	@echo "  make lint         - 静态检查"
	@echo "  make fmt          - 格式化代码"

# =============================================================================
# 第一层: CLI + Embed UI
# =============================================================================

# Alias targets for compatibility
build-gui: build-gui-frontend
build-all: build-full
package-darwin: release-macos
package-windows: release-windows
gui-dev: dev

# 构建 UI 组件库和 Web 前端 (输出到 internal/ui/ui/)
build-ui:
	@echo "📦 Building shared-ui..."
	cd shared/ui && npm install --silent && npm run build
	@echo "📦 Building web frontend (embed UI)..."
	cd web/frontend && npm install --silent && npm run build
	@echo "✅ UI build complete"

# 构建 CLI (依赖 embed UI)
build-cli: build-ui
	@echo "🔨 Building CLI..."
	@mkdir -p build
	go build $(LDFLAGS) -o build/mote ./cmd/mote
	@echo "✅ CLI build complete: build/mote"

# 简化命令: 编译 CLI + Embed UI
build: build-cli

# =============================================================================
# 第二层: 完整编译 (Wails App + CLI + Embed UI)
# =============================================================================

# 构建 GUI 前端 (依赖 shared-ui)
build-gui-frontend:
	@echo "📦 Building GUI frontend..."
	cd gui/frontend && npm install --silent && npm run build
	@echo "✅ GUI frontend build complete"

# 构建托盘程序
build-tray:
	@echo "🎨 Converting tray icon..."
	@bash scripts/convert-tray-icon.sh
	@echo "🔨 Building tray..."
	go build $(LDFLAGS) -o build/mote-tray ./cmd/mote-tray

# 完整编译: Wails App + CLI (本地平台)
build-full: build-cli build-gui-frontend build-tray
	@echo "🔨 Building Wails GUI..."
	cd gui && wails build
	@echo "📦 Assembling app bundle..."
	@mkdir -p "gui/build/bin/Mote.app/Contents/Helpers/Mote Tray.app/Contents/MacOS"
	@mkdir -p "gui/build/bin/Mote.app/Contents/Helpers/Mote Tray.app/Contents/Resources"
	@cp build/mote gui/build/bin/Mote.app/Contents/Resources/mote
	@chmod +x gui/build/bin/Mote.app/Contents/Resources/mote
	@cp build/mote-tray "gui/build/bin/Mote.app/Contents/Helpers/Mote Tray.app/Contents/MacOS/mote-tray"
	@chmod +x "gui/build/bin/Mote.app/Contents/Helpers/Mote Tray.app/Contents/MacOS/mote-tray"
	@cp build/darwin/Info-Tray.plist "gui/build/bin/Mote.app/Contents/Helpers/Mote Tray.app/Contents/Info.plist"
	@codesign --force --sign - "gui/build/bin/Mote.app/Contents/Helpers/Mote Tray.app" 2>/dev/null || true
	@codesign --force --sign - "gui/build/bin/Mote.app" 2>/dev/null || true
	@xattr -cr "gui/build/bin/Mote.app" 2>/dev/null || true
	@echo "✅ Full build complete: gui/build/bin/Mote.app"

# =============================================================================
# 开发模式
# =============================================================================

# Wails 开发模式 (热更新)
dev: build-ui
	@echo "🚀 Starting Wails dev mode..."
	@echo "   (shared-ui 已编译，GUI 前端将实时编译)"
	cd gui && wails dev

# =============================================================================
# 交叉编译 - macOS
# =============================================================================

release-macos: build-ui build-gui-frontend
	@echo "🍎 Building macOS Universal Binary..."
	@mkdir -p $(BUILD_DIR)
	
	@# CLI - Universal Binary
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/mote-darwin-arm64 ./cmd/mote
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/mote-darwin-amd64 ./cmd/mote
	lipo -create -output $(BUILD_DIR)/mote-darwin $(BUILD_DIR)/mote-darwin-arm64 $(BUILD_DIR)/mote-darwin-amd64
	
	@# Tray - Universal Binary
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/mote-tray-darwin-arm64 ./cmd/mote-tray
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/mote-tray-darwin-amd64 ./cmd/mote-tray
	lipo -create -output $(BUILD_DIR)/mote-tray-darwin $(BUILD_DIR)/mote-tray-darwin-arm64 $(BUILD_DIR)/mote-tray-darwin-amd64
	
	@# GUI - Universal Binary (Wails 输出固定为 Mote.app，需分别构建后重命名)
	cd gui && wails build -platform darwin/arm64
	cp gui/build/bin/Mote.app/Contents/MacOS/Mote $(BUILD_DIR)/mote-gui-darwin-arm64
	cd gui && wails build -platform darwin/amd64
	cp gui/build/bin/Mote.app/Contents/MacOS/Mote $(BUILD_DIR)/mote-gui-darwin-amd64
	lipo -create -output $(BUILD_DIR)/mote-gui-darwin $(BUILD_DIR)/mote-gui-darwin-arm64 $(BUILD_DIR)/mote-gui-darwin-amd64
	
	@# 打包 App Bundle
	@echo "📦 Creating macOS App Bundle..."
	@rm -rf $(DIST_DIR)/Mote-macOS.app
	@mkdir -p $(DIST_DIR)/Mote-macOS.app/Contents/MacOS
	@mkdir -p $(DIST_DIR)/Mote-macOS.app/Contents/Resources
	@mkdir -p "$(DIST_DIR)/Mote-macOS.app/Contents/Helpers/Mote Tray.app/Contents/MacOS"
	@mkdir -p "$(DIST_DIR)/Mote-macOS.app/Contents/Helpers/Mote Tray.app/Contents/Resources"
	@cp $(BUILD_DIR)/mote-gui-darwin $(DIST_DIR)/Mote-macOS.app/Contents/MacOS/Mote
	@cp $(BUILD_DIR)/mote-darwin $(DIST_DIR)/Mote-macOS.app/Contents/Resources/mote
	@cp $(BUILD_DIR)/mote-tray-darwin "$(DIST_DIR)/Mote-macOS.app/Contents/Helpers/Mote Tray.app/Contents/MacOS/mote-tray"
	@cp build/darwin/Info.plist $(DIST_DIR)/Mote-macOS.app/Contents/
	@cp build/darwin/Info-Tray.plist "$(DIST_DIR)/Mote-macOS.app/Contents/Helpers/Mote Tray.app/Contents/Info.plist"
	@cp gui/build/bin/Mote.app/Contents/Resources/iconfile.icns $(DIST_DIR)/Mote-macOS.app/Contents/Resources/ 2>/dev/null || cp build/darwin/appicon.icns $(DIST_DIR)/Mote-macOS.app/Contents/Resources/iconfile.icns 2>/dev/null || true
	@cp cmd/mote-tray/icon.icns "$(DIST_DIR)/Mote-macOS.app/Contents/Helpers/Mote Tray.app/Contents/Resources/" 2>/dev/null || true
	@codesign --force --deep --sign - "$(DIST_DIR)/Mote-macOS.app" 2>/dev/null || true
	@xattr -cr "$(DIST_DIR)/Mote-macOS.app" 2>/dev/null || true
	
	@echo "✅ macOS release complete: $(DIST_DIR)/Mote-macOS.app"

# =============================================================================
# 交叉编译 - Windows
# =============================================================================

release-windows: build-ui build-gui-frontend
	@echo "🪟 Building Windows amd64..."
	@mkdir -p $(BUILD_DIR)
	@mkdir -p $(DIST_DIR)/Mote-Windows
	
	@# CLI
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/mote-windows-amd64.exe ./cmd/mote
	
	@# Tray
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/mote-tray-windows-amd64.exe ./cmd/mote-tray
	
	@# GUI (需要在 Windows 上或使用 Docker 交叉编译)
	cd gui && wails build -platform windows/amd64 -o Mote.exe || echo "⚠️  Windows GUI 需要在 Windows 上编译或使用 Docker"
	
	@# 打包
	@cp $(BUILD_DIR)/mote-windows-amd64.exe $(DIST_DIR)/Mote-Windows/mote.exe
	@cp $(BUILD_DIR)/mote-tray-windows-amd64.exe $(DIST_DIR)/Mote-Windows/mote-tray.exe
	@cp gui/build/bin/Mote.exe $(DIST_DIR)/Mote-Windows/ 2>/dev/null || true
	
	@echo "✅ Windows release complete: $(DIST_DIR)/Mote-Windows/"

# =============================================================================
# 所有平台
# =============================================================================

release-all: release-macos release-windows
	@echo "✅ All platforms built"
	@echo "   macOS:   $(DIST_DIR)/Mote-macOS.app"
	@echo "   Windows: $(DIST_DIR)/Mote-Windows/"

# =============================================================================
# 工具命令
# =============================================================================

test:
	go test ./... -v -cover

clean:
	rm -rf build/mote build/mote-tray
	rm -rf $(BUILD_DIR)
	rm -rf $(DIST_DIR)
	rm -rf gui/build/bin
	rm -rf internal/ui/ui
	rm -rf gui/frontend/node_modules/.vite
	rm -rf web/frontend/node_modules/.vite

fmt:
	go fmt ./...

lint:
	go vet ./...

tidy:
	go mod tidy

# 快速安装到 GOPATH/bin
install: build
	cp build/mote $(GOPATH)/bin/mote 2>/dev/null || cp build/mote /usr/local/bin/mote
