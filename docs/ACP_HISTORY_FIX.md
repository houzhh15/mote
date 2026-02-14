# ACP 历史修复说明

## 修复内容

修复了 GitHub Copilot ACP Provider 在 mote 重启后丢失历史对话的问题。

## 问题描述

**修复前:**
- 使用 Copilot ACP 时，mote 重启后无法访问历史对话
- AI 只能看到重启后的新对话
- 用户要求"总结历史对话"时，AI 会说看不到历史

**根本原因:**
- `runACPMode` 只发送当前消息，没有加载历史
- 缺少与非 ACP 模式相同的压缩机制

## 修复后的行为

✅ **与非 ACP 模式完全一致:**
- 重启后自动加载完整历史对话
- 使用智能压缩机制（Compactor）控制 token 消耗
- 历史过长时自动生成摘要
- 保留最近的完整对话

## 技术细节

### 修改文件
- `internal/runner/runner.go` - `runACPMode()` 方法

### 关键改进
1. 使用 `buildMessages()` 加载完整历史（与非 ACP 一致）
2. 添加压缩逻辑：
   ```go
   if r.compactor != nil {
       if r.compactor.NeedsCompaction(messages) {
           compacted := r.compactor.CompactWithFallback(ctx, messages)
           messages = compacted
       }
   }
   ```
3. 正确处理 system message 与 skills 的合并

### 测试验证
- `internal/runner/runner_acp_history_test.go`
- 包含历史加载和压缩测试
- 所有测试通过 ✅

## 压缩机制说明

### 触发条件
- 消息总 token 数超过 `MaxContextTokens * TriggerThreshold`
- 默认：超过 80,000 tokens 时触发

### 压缩策略
1. **保留最近的消息** (默认最近 20 条)
2. **总结旧消息** (使用 LLM 生成摘要)
3. **构建新的历史**:
   ```
   [System Message]
   [历史摘要: "用户询问了...，讨论了...，得出结论..."]
   [最近的完整对话]
   [当前消息]
   ```

### 日志示例
```
INFO runACPMode: built messages with history sessionID=xxx messageCount=103 cachedMessageCount=101
INFO runACPMode: compacting messages sessionID=xxx beforeCount=103
INFO runACPMode: compaction done sessionID=xxx afterCount=100
INFO runACPMode: starting ACP execution sessionID=xxx finalMessageCount=100
```

## 使用建议

### 1. 查看日志
```bash
# 检查历史是否被正确加载
tail -f mote.log | grep "runACPMode"
```

关键日志指标：
- `messageCount`: 加载的消息数（应该 > 1）
- `cachedMessageCount`: 数据库中的历史消息数
- `compaction done`: 压缩执行成功

### 2. 验证修复
测试场景：
```
1. 在 mote 中创建一个对话
2. 发送几条消息："你好" → "介绍一下你自己" → "记住我喜欢Python"
3. 关闭 mote
4. 重新启动 mote
5. 打开该对话
6. 发送: "总结一下我们之前的对话"
7. ✅ AI 应该能够回忆起之前的内容
```

### 3. 调整压缩参数（可选）

如果需要调整压缩行为，修改 `internal/server/server.go`:

```go
compactorConfig := compaction.CompactionConfig{
    MaxContextTokens: 100000,  // 最大 context 大小
    TriggerThreshold: 0.8,     // 80% 时触发压缩
    KeepRecentCount:  20,      // 保留最近 20 条消息
    SummaryMaxTokens: 500,     // 摘要最大长度
}
```

## 对比

| 特性 | 修复前 | 修复后 |
|------|--------|--------|
| 历史加载 | ❌ 不加载 | ✅ 完整加载 |
| 压缩机制 | ❌ 无 | ✅ 智能压缩 |
| Token 控制 | ❌ 可能超限 | ✅ 自动控制 |
| 与非ACP一致性 | ❌ 不同 | ✅ 完全一致 |
| 测试覆盖 | ❌ 无 | ✅ 完整测试 |

## 相关文档
- `docs/CONTEXT_HANDLING_ANALYSIS.md` - 详细的 context 处理分析
- `internal/runner/runner_acp_history_test.go` - 测试用例

## 兼容性
- ✅ 完全向后兼容
- ✅ 不影响其他 Provider (Ollama, OpenAI 等)
- ✅ 不需要迁移数据库
- ✅ 不需要修改配置文件
