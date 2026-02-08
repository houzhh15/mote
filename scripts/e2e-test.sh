#!/bin/bash
set -e

# E2E 测试脚本

echo "=== Mote E2E Tests ==="

# 设置测试环境
TEST_HOME=$(mktemp -d)
export HOME=$TEST_HOME
MOTE_BIN="./mote"

echo "Test home: $TEST_HOME"

# 确保二进制文件存在
if [ ! -f "$MOTE_BIN" ]; then
    echo "Building mote..."
    make build
fi

# 测试 1: version 命令
echo ""
echo "Test 1: mote version"
$MOTE_BIN version
if [ $? -ne 0 ]; then
    echo "FAIL: version command failed"
    exit 1
fi
echo "PASS: version command"

# 测试 2: version --json
echo ""
echo "Test 2: mote version --json"
$MOTE_BIN version --json | grep -q '"version"'
if [ $? -ne 0 ]; then
    echo "FAIL: version --json failed"
    exit 1
fi
echo "PASS: version --json"

# 测试 3: init 命令
echo ""
echo "Test 3: mote init"
$MOTE_BIN init
if [ $? -ne 0 ]; then
    echo "FAIL: init command failed"
    exit 1
fi
if [ ! -f "$TEST_HOME/.mote/config.yaml" ]; then
    echo "FAIL: config.yaml not created"
    exit 1
fi
if [ ! -f "$TEST_HOME/.mote/data.db" ]; then
    echo "FAIL: data.db not created"
    exit 1
fi
echo "PASS: init command"

# 测试 4: config list
echo ""
echo "Test 4: mote config list"
$MOTE_BIN config list | grep -q "gateway.port"
if [ $? -ne 0 ]; then
    echo "FAIL: config list failed"
    exit 1
fi
echo "PASS: config list"

# 测试 5: config get
echo ""
echo "Test 5: mote config get"
PORT=$($MOTE_BIN config get gateway.port)
if [ "$PORT" != "8080" ]; then
    echo "FAIL: config get returned wrong value: $PORT"
    exit 1
fi
echo "PASS: config get"

# 测试 6: config set
echo ""
echo "Test 6: mote config set"
$MOTE_BIN config set gateway.port 9090
NEW_PORT=$($MOTE_BIN config get gateway.port)
if [ "$NEW_PORT" != "9090" ]; then
    echo "FAIL: config set failed, value: $NEW_PORT"
    exit 1
fi
echo "PASS: config set"

# 测试 7: config path
echo ""
echo "Test 7: mote config path"
CONFIG_PATH=$($MOTE_BIN config path)
if [ ! -f "$CONFIG_PATH" ]; then
    echo "FAIL: config path returned non-existent path: $CONFIG_PATH"
    exit 1
fi
echo "PASS: config path"

# 测试 8: 数据库表结构验证
echo ""
echo "Test 8: database schema"
TABLES=$(sqlite3 "$TEST_HOME/.mote/data.db" ".tables")
for table in sessions messages kv_store _migrations; do
    if [[ ! "$TABLES" =~ "$table" ]]; then
        echo "FAIL: table $table not found"
        exit 1
    fi
done
echo "PASS: database schema"

# 测试 9: 二进制文件大小
echo ""
echo "Test 9: binary size"
SIZE=$(stat -f%z "$MOTE_BIN" 2>/dev/null || stat -c%s "$MOTE_BIN")
SIZE_MB=$((SIZE / 1024 / 1024))
if [ $SIZE_MB -gt 50 ]; then
    echo "FAIL: binary too large: ${SIZE_MB}MB"
    exit 1
fi
echo "PASS: binary size (${SIZE_MB}MB)"

# 测试 10: 执行时间
echo ""
echo "Test 10: execution time"
START=$(date +%s%3N)
$MOTE_BIN version > /dev/null
END=$(date +%s%3N)
DURATION=$((END - START))
if [ $DURATION -gt 500 ]; then
    echo "WARN: version command slow: ${DURATION}ms"
else
    echo "PASS: execution time (${DURATION}ms)"
fi

# 清理
rm -rf "$TEST_HOME"

echo ""
echo "=== All E2E tests passed! ==="
