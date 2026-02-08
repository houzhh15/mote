#!/bin/bash
# 优化macOS Dock图标脚本
# 根据Apple Human Interface Guidelines处理图标
# https://developer.apple.com/design/human-interface-guidelines/app-icons

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

SOURCE_PNG="${1:-$PROJECT_ROOT/.tmp/appicon.png}"
OUTPUT_PNG="$PROJECT_ROOT/.tmp/appicon_optimized.png"
ICONSET_DIR="$PROJECT_ROOT/.tmp/mote.iconset"
OUTPUT_ICNS="$PROJECT_ROOT/build/darwin/appicon.icns"

echo "=== Mote Icon Optimizer for macOS Dock ==="
echo "Source: $SOURCE_PNG"

# 检查源文件
if [ ! -f "$SOURCE_PNG" ]; then
    echo "错误: 源文件不存在: $SOURCE_PNG"
    echo "用法: $0 [source_png_path]"
    exit 1
fi

# 检查是否有 ImageMagick
if command -v magick &> /dev/null; then
    IM_CMD="magick"
elif command -v convert &> /dev/null; then
    IM_CMD="convert"
else
    echo "警告: ImageMagick 未安装，将使用 sips 进行基本处理"
    IM_CMD=""
fi

mkdir -p "$(dirname "$OUTPUT_PNG")"

if [ -n "$IM_CMD" ]; then
    echo "使用 ImageMagick 优化图标..."
    
    # 1. 首先裁剪掉图标周围的白色/透明边框
    # 2. 将内容放置在 1024x1024 画布的中心
    # 3. 按照 Apple 规范，图标内容应占据约 80% 的区域（824x824）
    
    # 裁剪透明/白色边框，获取实际内容
    $IM_CMD "$SOURCE_PNG" -trim +repage /tmp/mote_trimmed.png
    
    # 获取裁剪后的尺寸
    TRIM_SIZE=$($IM_CMD /tmp/mote_trimmed.png -format "%wx%h" info:)
    echo "裁剪后内容尺寸: $TRIM_SIZE"
    
    # 将图标内容调整到 824x824（1024的80%），保持比例
    $IM_CMD /tmp/mote_trimmed.png \
        -resize 824x824 \
        -gravity center \
        -background transparent \
        -extent 1024x1024 \
        "$OUTPUT_PNG"
    
    echo "✓ 已生成优化后的图标: $OUTPUT_PNG"
    
    # 清理临时文件
    rm -f /tmp/mote_trimmed.png
else
    echo "使用 sips 进行基本处理..."
    # 简单地调整到 1024x1024
    sips -z 1024 1024 "$SOURCE_PNG" --out "$OUTPUT_PNG" >/dev/null
fi

# 创建 iconset
rm -rf "$ICONSET_DIR"
mkdir -p "$ICONSET_DIR"
mkdir -p "$(dirname "$OUTPUT_ICNS")"

echo "生成各种尺寸的图标..."
# macOS要求的所有尺寸
sips -z 1024 1024 "$OUTPUT_PNG" --out "$ICONSET_DIR/icon_512x512@2x.png" >/dev/null
sips -z 16 16     "$OUTPUT_PNG" --out "$ICONSET_DIR/icon_16x16.png" >/dev/null
sips -z 32 32     "$OUTPUT_PNG" --out "$ICONSET_DIR/icon_16x16@2x.png" >/dev/null
sips -z 32 32     "$OUTPUT_PNG" --out "$ICONSET_DIR/icon_32x32.png" >/dev/null
sips -z 64 64     "$OUTPUT_PNG" --out "$ICONSET_DIR/icon_32x32@2x.png" >/dev/null
sips -z 128 128   "$OUTPUT_PNG" --out "$ICONSET_DIR/icon_128x128.png" >/dev/null
sips -z 256 256   "$OUTPUT_PNG" --out "$ICONSET_DIR/icon_128x128@2x.png" >/dev/null
sips -z 256 256   "$OUTPUT_PNG" --out "$ICONSET_DIR/icon_256x256.png" >/dev/null
sips -z 512 512   "$OUTPUT_PNG" --out "$ICONSET_DIR/icon_256x256@2x.png" >/dev/null
sips -z 512 512   "$OUTPUT_PNG" --out "$ICONSET_DIR/icon_512x512.png" >/dev/null

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

# 复制1024x1024的PNG版本（用于wails配置）
PNG_1024="$PROJECT_ROOT/build/darwin/appicon-1024.png"
cp "$ICONSET_DIR/icon_512x512@2x.png" "$PNG_1024"
echo "  ✓ 1024x1024 PNG: $PNG_1024"

# 复制到gui/appicon.png（wails使用）
cp "$ICONSET_DIR/icon_512x512@2x.png" "$PROJECT_ROOT/gui/appicon.png"
echo "  ✓ GUI source icon: $PROJECT_ROOT/gui/appicon.png"

# 清理临时文件
rm -rf "$ICONSET_DIR"

echo ""
echo "✅ 图标优化完成！"
echo "   输出文件: $OUTPUT_ICNS"
echo ""
echo "macOS Dock 图标规范说明:"
echo "  - 画布尺寸: 1024x1024"
echo "  - 图标内容: 824x824 (居中)"
echo "  - 边距: 约100px (10%)"
echo ""
echo "提示: 重新编译应用后，新图标将生效"
echo "      如果图标仍不理想，建议使用专业设计工具重新制作符合规范的图标"
