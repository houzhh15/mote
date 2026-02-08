#!/bin/bash
# 图标转换脚本：将PNG转换为macOS icns格式

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

SOURCE_PNG="$PROJECT_ROOT/.tmp/mote_logo.png"
ICONSET_DIR="$PROJECT_ROOT/.tmp/mote.iconset"
OUTPUT_ICNS="$PROJECT_ROOT/build/darwin/appicon.icns"

echo "=== Mote Icon Converter ==="

# 检查源文件
if [ ! -f "$SOURCE_PNG" ]; then
    echo "错误: 源文件不存在: $SOURCE_PNG"
    exit 1
fi

# 创建临时目录
rm -rf "$ICONSET_DIR"
mkdir -p "$ICONSET_DIR"
mkdir -p "$(dirname "$OUTPUT_ICNS")"

echo "创建1024x1024基础图标..."
sips -z 1024 1024 "$SOURCE_PNG" --out "$ICONSET_DIR/icon_512x512@2x.png" >/dev/null

echo "生成各种尺寸的图标..."
# macOS要求的所有尺寸
sips -z 16 16     "$ICONSET_DIR/icon_512x512@2x.png" --out "$ICONSET_DIR/icon_16x16.png" >/dev/null
sips -z 32 32     "$ICONSET_DIR/icon_512x512@2x.png" --out "$ICONSET_DIR/icon_16x16@2x.png" >/dev/null
sips -z 32 32     "$ICONSET_DIR/icon_512x512@2x.png" --out "$ICONSET_DIR/icon_32x32.png" >/dev/null
sips -z 64 64     "$ICONSET_DIR/icon_512x512@2x.png" --out "$ICONSET_DIR/icon_32x32@2x.png" >/dev/null
sips -z 128 128   "$ICONSET_DIR/icon_512x512@2x.png" --out "$ICONSET_DIR/icon_128x128.png" >/dev/null
sips -z 256 256   "$ICONSET_DIR/icon_512x512@2x.png" --out "$ICONSET_DIR/icon_128x128@2x.png" >/dev/null
sips -z 256 256   "$ICONSET_DIR/icon_512x512@2x.png" --out "$ICONSET_DIR/icon_256x256.png" >/dev/null
sips -z 512 512   "$ICONSET_DIR/icon_512x512@2x.png" --out "$ICONSET_DIR/icon_256x256@2x.png" >/dev/null
sips -z 512 512   "$ICONSET_DIR/icon_512x512@2x.png" --out "$ICONSET_DIR/icon_512x512.png" >/dev/null

echo "转换为icns格式..."
iconutil -c icns "$ICONSET_DIR" -o "$OUTPUT_ICNS"

# 复制到各个位置
echo "复制图标到各个位置..."

# GUI App图标
GUI_ICON="$PROJECT_ROOT/gui/build/bin/Mote.app/Contents/Resources/iconfile.icns"
if [ -d "$(dirname "$GUI_ICON")" ]; then
    cp "$OUTPUT_ICNS" "$GUI_ICON"
    echo "  ✓ GUI App: $GUI_ICON"
fi

# Tray App图标
TRAY_ICON="$PROJECT_ROOT/gui/build/bin/Mote.app/Contents/Helpers/Mote Tray.app/Contents/Resources/iconfile.icns"
if [ -d "$(dirname "$TRAY_ICON")" ]; then
    mkdir -p "$(dirname "$TRAY_ICON")"
    cp "$OUTPUT_ICNS" "$TRAY_ICON"
    echo "  ✓ Tray App: $TRAY_ICON"
fi

# Bubble App图标
BUBBLE_ICON="$PROJECT_ROOT/cmd/mote-bubble/build/bin/mote-bubble.app/Contents/Resources/iconfile.icns"
if [ -d "$(dirname "$BUBBLE_ICON")" ]; then
    cp "$OUTPUT_ICNS" "$BUBBLE_ICON"
    echo "  ✓ Bubble App: $BUBBLE_ICON"
fi

# 同时复制1024x1024的PNG版本（用于wails配置）
PNG_1024="$PROJECT_ROOT/build/darwin/appicon-1024.png"
cp "$ICONSET_DIR/icon_512x512@2x.png" "$PNG_1024"
echo "  ✓ 1024x1024 PNG: $PNG_1024"

# 复制到gui/appicon.png（wails可能用到）
cp "$ICONSET_DIR/icon_512x512@2x.png" "$PROJECT_ROOT/gui/appicon.png"
echo "  ✓ GUI source icon: $PROJECT_ROOT/gui/appicon.png"

# 清理临时文件
rm -rf "$ICONSET_DIR"

echo ""
echo "✅ 图标转换完成！"
echo "   输出文件: $OUTPUT_ICNS"
echo "   尺寸: 1024x1024 (包含所有macOS所需的多种尺寸)"
echo ""
echo "提示: 重新编译应用后，新图标将生效"
