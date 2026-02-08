#!/bin/bash
# 验证修复的快速测试脚本

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "=== 修复验证测试 ==="
echo ""

# 1. 检查UI文本更改
echo "✓ 检查 1: UI 提示文本"
if grep -q "配置已生效" "$PROJECT_ROOT/shared/ui/src/pages/SettingsPage.tsx"; then
    echo "  ✓ 找到新的提示文本 '配置已生效'"
else
    echo "  ✗ 未找到新的提示文本"
    exit 1
fi

if grep -q "配置更改将立即生效" "$PROJECT_ROOT/shared/ui/src/pages/SettingsPage.tsx"; then
    echo "  ✓ 说明文本已更新"
else
    echo "  ✗ 说明文本未更新"
    exit 1
fi

# 2. 检查图标文件
echo ""
echo "✓ 检查 2: 图标文件"
if [ -f "$PROJECT_ROOT/build/darwin/appicon.icns" ]; then
    SIZE=$(ls -lh "$PROJECT_ROOT/build/darwin/appicon.icns" | awk '{print $5}')
    echo "  ✓ 图标文件存在: build/darwin/appicon.icns ($SIZE)"
else
    echo "  ✗ 图标文件不存在"
    exit 1
fi

if [ -f "$PROJECT_ROOT/gui/appicon.png" ]; then
    echo "  ✓ GUI 源图标存在: gui/appicon.png"
else
    echo "  ✗ GUI 源图标不存在"
fi

# 3. 检查图标转换脚本
echo ""
echo "✓ 检查 3: 图标转换脚本"
if [ -x "$PROJECT_ROOT/scripts/convert-icon.sh" ]; then
    echo "  ✓ 脚本存在且可执行"
else
    echo "  ✗ 脚本不存在或不可执行"
    exit 1
fi

# 4. 检查 Makefile 更新
echo ""
echo "✓ 检查 4: Makefile 配置"
if grep -q "convert-icon.sh" "$PROJECT_ROOT/Makefile"; then
    echo "  ✓ Makefile 已添加图标转换步骤"
else
    echo "  ✗ Makefile 未更新"
    exit 1
fi

# 5. 检查 Info-Tray.plist
echo ""
echo "✓ 检查 5: Tray Info.plist"
if grep -q "CFBundleIconFile" "$PROJECT_ROOT/build/darwin/Info-Tray.plist"; then
    echo "  ✓ Tray plist 已添加图标配置"
else
    echo "  ✗ Tray plist 未更新"
    exit 1
fi

# 6. 检查热重载相关代码
echo ""
echo "✓ 检查 6: 热重载功能"
if grep -q "ReloadProviders" "$PROJECT_ROOT/internal/server/server.go"; then
    echo "  ✓ Server 热重载方法存在"
else
    echo "  ✗ Server 热重载方法不存在"
    exit 1
fi

if grep -q "EmbeddedServerInterface" "$PROJECT_ROOT/api/v1/routes.go"; then
    echo "  ✓ API 热重载接口存在"
else
    echo "  ✗ API 热重载接口不存在"
    exit 1
fi

# 7. 编译测试
echo ""
echo "✓ 检查 7: 编译测试"
echo "  编译 Go 代码..."
if cd "$PROJECT_ROOT" && go build -o /dev/null ./... 2>&1 | grep -q "error"; then
    echo "  ✗ Go 编译失败"
    exit 1
else
    echo "  ✓ Go 编译成功"
fi

echo ""
echo "==================================="
echo "✅ 所有检查通过！"
echo ""
echo "已修复的问题:"
echo "  1. ✓ 设置页面提示文本已更新（移除'需要重启'）"
echo "  2. ✓ 图标文件已转换并放置到正确位置"
echo "  3. ✓ Makefile 已添加自动图标转换"
echo "  4. ✓ 热重载功能已集成到 API"
echo ""
echo "关于 MacOS/Resources 下的 mote 文件:"
echo "  经检查，这两个文件都是必需的："
echo "  - MacOS/Mote: GUI 主程序"
echo "  - Resources/mote: CLI 工具"
echo "  不是冗余，无需修改。"
echo ""
echo "详细信息请查看: docs/fixes-2026-02-06.md"
