#!/bin/bash
# macOS 打包脚本
# 将 Wails 构建产物打包为可发布的 .app

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_ROOT/build"
GUI_BUILD_DIR="$PROJECT_ROOT/gui/build/bin"
APP_NAME="Mote"
VERSION="${VERSION:-1.0.0}"

echo "=== Mote macOS Package Script ==="
echo "Version: $VERSION"

# Check if Wails build exists
if [ ! -d "$GUI_BUILD_DIR" ]; then
    echo "Error: GUI build directory not found. Run 'make build-gui' first."
    exit 1
fi

# Find the .app bundle
APP_BUNDLE=$(find "$GUI_BUILD_DIR" -name "*.app" -type d | head -1)
if [ -z "$APP_BUNDLE" ]; then
    echo "Error: .app bundle not found in $GUI_BUILD_DIR"
    exit 1
fi

echo "Found app bundle: $APP_BUNDLE"

# Create output directory
OUTPUT_DIR="$BUILD_DIR/darwin"
mkdir -p "$OUTPUT_DIR"

# Copy app bundle to output
cp -R "$APP_BUNDLE" "$OUTPUT_DIR/"
FINAL_APP="$OUTPUT_DIR/$APP_NAME.app"

# Ensure Resources directory exists
RESOURCES_DIR="$FINAL_APP/Contents/Resources"
mkdir -p "$RESOURCES_DIR"

# Copy mote CLI binary to Resources
if [ -f "$BUILD_DIR/mote" ]; then
    cp "$BUILD_DIR/mote" "$RESOURCES_DIR/"
    chmod +x "$RESOURCES_DIR/mote"
    echo "Copied mote CLI to Resources"
fi

# Copy mote-tray binary to Resources
if [ -f "$BUILD_DIR/mote-tray" ]; then
    cp "$BUILD_DIR/mote-tray" "$RESOURCES_DIR/"
    chmod +x "$RESOURCES_DIR/mote-tray"
    echo "Copied mote-tray to Resources"
fi

# Update Info.plist version
INFO_PLIST="$FINAL_APP/Contents/Info.plist"
if [ -f "$INFO_PLIST" ]; then
    /usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $VERSION" "$INFO_PLIST" 2>/dev/null || true
    /usr/libexec/PlistBuddy -c "Set :CFBundleVersion $VERSION" "$INFO_PLIST" 2>/dev/null || true
    echo "Updated Info.plist with version $VERSION"
fi

# Create symlink installation script
cat > "$OUTPUT_DIR/install-symlink.sh" << 'INSTALL_SCRIPT'
#!/bin/bash
# Install mote CLI symlink to /usr/local/bin

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_PATH="$SCRIPT_DIR/Mote.app"
MOTE_PATH="$APP_PATH/Contents/Resources/mote"
SYMLINK_PATH="/usr/local/bin/mote"

if [ ! -f "$MOTE_PATH" ]; then
    echo "Error: mote binary not found in app bundle"
    exit 1
fi

echo "Creating symlink: $SYMLINK_PATH -> $MOTE_PATH"

# Remove existing symlink
if [ -L "$SYMLINK_PATH" ]; then
    sudo rm "$SYMLINK_PATH"
fi

# Create new symlink
sudo ln -s "$MOTE_PATH" "$SYMLINK_PATH"

echo "Done! You can now use 'mote' command from terminal."
INSTALL_SCRIPT
chmod +x "$OUTPUT_DIR/install-symlink.sh"

# Create DMG (optional, requires create-dmg)
if command -v create-dmg &> /dev/null; then
    DMG_NAME="Mote-$VERSION-macOS.dmg"
    echo "Creating DMG: $DMG_NAME"
    create-dmg \
        --volname "Mote" \
        --window-pos 200 120 \
        --window-size 600 400 \
        --icon-size 100 \
        --app-drop-link 450 185 \
        "$OUTPUT_DIR/$DMG_NAME" \
        "$FINAL_APP" \
        2>/dev/null || echo "DMG creation skipped (create-dmg not configured)"
fi

# Create ZIP archive
ZIP_NAME="Mote-$VERSION-macOS.zip"
echo "Creating ZIP: $ZIP_NAME"
cd "$OUTPUT_DIR"
zip -r "$ZIP_NAME" "$APP_NAME.app" "install-symlink.sh"

echo ""
echo "=== Package Complete ==="
echo "Output: $OUTPUT_DIR"
ls -la "$OUTPUT_DIR"
