#!/bin/bash
# 托盘图标转换脚本：将PNG转换为macOS系统托盘图标

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

SOURCE_PNG="$PROJECT_ROOT/.tmp/mote_tray.png"
ICONSET_DIR="$PROJECT_ROOT/.tmp/tray.iconset"
OUTPUT_PNG="$PROJECT_ROOT/cmd/mote-tray/icon.png"

echo "=== Mote Tray Icon Converter ==="

# 检查源文件
if [ ! -f "$SOURCE_PNG" ]; then
    echo "错误: 源文件不存在: $SOURCE_PNG"
    exit 1
fi

# 创建临时目录
rm -rf "$ICONSET_DIR"
mkdir -p "$ICONSET_DIR"

echo "处理系统托盘图标..."

# macOS 托盘图标通常使用 Template Image 格式
# 最佳实践：
# - 使用黑白轮廓（或透明背景）
# - 尺寸: 16x16 和 32x32（支持 Retina）
# - 命名为 *Template.png 会自动适配暗色模式

# 生成标准尺寸
echo "  生成 16x16..."
sips -z 16 16 "$SOURCE_PNG" --out "$ICONSET_DIR/icon_16x16.png" >/dev/null

echo "  生成 32x32 (Retina)..."
sips -z 32 32 "$SOURCE_PNG" --out "$ICONSET_DIR/icon_16x16@2x.png" >/dev/null

echo "  生成 32x32..."
sips -z 32 32 "$SOURCE_PNG" --out "$ICONSET_DIR/icon_32x32.png" >/dev/null

echo "  生成 64x64 (Retina)..."
sips -z 64 64 "$SOURCE_PNG" --out "$ICONSET_DIR/icon_32x32@2x.png" >/dev/null

# 对于 Go systray，使用单一的 PNG 文件（32x32 足够）
echo "  复制到托盘程序目录..."
cp "$ICONSET_DIR/icon_32x32@2x.png" "$OUTPUT_PNG"

# 可选：如果需要 icns 格式（用于某些情况）
TRAY_ICNS="$PROJECT_ROOT/cmd/mote-tray/icon.icns"
if command -v iconutil &> /dev/null; then
    # 为完整的 iconset 添加更多尺寸
    sips -z 128 128 "$SOURCE_PNG" --out "$ICONSET_DIR/icon_128x128.png" >/dev/null
    sips -z 256 256 "$SOURCE_PNG" --out "$ICONSET_DIR/icon_128x128@2x.png" >/dev/null
    sips -z 256 256 "$SOURCE_PNG" --out "$ICONSET_DIR/icon_256x256.png" >/dev/null
    sips -z 512 512 "$SOURCE_PNG" --out "$ICONSET_DIR/icon_256x256@2x.png" >/dev/null
    sips -z 512 512 "$SOURCE_PNG" --out "$ICONSET_DIR/icon_512x512.png" >/dev/null
    sips -z 1024 1024 "$SOURCE_PNG" --out "$ICONSET_DIR/icon_512x512@2x.png" >/dev/null
    
    echo "  生成 icns 格式..."
    iconutil -c icns "$ICONSET_DIR" -o "$TRAY_ICNS"
    echo "  ✓ icns: $TRAY_ICNS"
fi

# 清理临时文件
rm -rf "$ICONSET_DIR"

echo ""
echo "✅ 托盘图标转换完成！"
echo "   输出文件: $OUTPUT_PNG"
echo "   尺寸: 64x64 (适合高分辨率显示)"
echo ""
echo "提示: 重新编译托盘程序后，新图标将生效"
echo "      运行: make build-tray"
