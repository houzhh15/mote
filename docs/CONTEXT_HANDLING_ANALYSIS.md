# Mote Context 处理机制分析

## 概述

本文档分析 Mote 的 context 处理机制,特别关注程序关闭后重新打开,通过 session 历史进入 chat 时的行为。

## Context 加载流程

### 1. Session 管理架构

Mote 使用三层架构管理 session 和消息历史:

```
┌─────────────────┐
│  WebSocket API  │
└────────┬────────┘
         │
┌────────▼────────┐
│     Runner      │  ← 执行 Agent 逻辑
└────────┬────────┘
         │
┌────────▼────────────┐
│  SessionManager     │  ← 内存缓存层 (LRU)
│  - Cache (Map)      │
│  - Eviction         │
└────────┬────────────┘
         │
┌────────▼────────────┐
│   Storage (SQLite)  │  ← 持久化层
│  - sessions 表      │
│  - messages 表      │
└─────────────────────┘
```

### 2. 关键代码路径

#### 2.1 WebSocket 接收消息
```go
// internal/gateway/server.go:298
func (s *Server) handleWebSocketChat(sessionID, message string) {
    // 调用 Runner.Run
    events, err := s.agentRunner.Run(ctx, sessionID, message)
}
```

#### 2.2 Runner 执行
```go
// internal/runner/runner.go:591
func (r *Runner) Run(ctx context.Context, sessionID, userInput string) {
    go func() {
        r.runLoop(ctx, sessionID, userInput, attachments, events)
    }()
}
```

#### 2.3 加载 Session
```go
// internal/runner/runner.go:708-710
func (r *Runner) runLoop(...) {
    // 获取或创建 session
    cached, err := r.sessions.GetOrCreate(sessionID, nil)
}
```

#### 2.4 SessionManager 加载
```go
// internal/scheduler/session_manager.go:83-112
func (m *SessionManager) GetOrCreate(sessionID string, metadata) {
    // 1. 先尝试从缓存获取
    cached, err := m.Get(sessionID)
    if err == nil {
        return cached, nil  // ✅ 缓存命中,直接返回
    }
    
    // 2. 缓存未命中,创建新 session
    session, err := m.db.CreateSessionWithID(sessionID, metadata)
}

func (m *SessionManager) Get(sessionID string) {
    // 1. 检查内存缓存
    if cached, ok := m.cache[sessionID]; ok {
        return cached, nil  // ✅ 返回缓存的 session 和历史消息
    }
    
    // 2. 从数据库加载
    session, err := m.db.GetSession(sessionID)
    messages, err := m.db.GetMessages(sessionID, 0)  // ✅ 加载所有历史消息
    
    // 3. 放入缓存
    cached = &CachedSession{
        Session:  session,
        Messages: messages,  // ✅ 历史消息被缓存
    }
    m.cache[sessionID] = cached
}
```

#### 2.5 构建 Context
```go
// internal/runner/runner.go:1226-1290
func (r *Runner) buildMessages(ctx, cached, userInput) {
    var messages []provider.Message
    
    // 1. 构建 system prompt
    messages = append(messages, provider.Message{
        Role:    provider.RoleSystem,
        Content: sysPromptContent,
    })
    
    // 2. 添加历史消息
    for _, msg := range cached.Messages {  // ✅ 遍历缓存的历史消息
        provMsg := provider.Message{
            Role:       msg.Role,
            Content:    msg.Content,
            ToolCallID: msg.ToolCallID,
        }
        messages = append(messages, provMsg)
    }
    
    // 3. 添加当前用户输入
    messages = append(messages, provider.Message{
        Role:    provider.RoleUser,
        Content: userInput,
    })
    
    return messages, nil
}
```

## 重启后的行为

### 场景:Mote 程序关闭后重新打开

#### 情况 1:Session 在缓存中 (程序未关闭)
- ✅ 直接从内存缓存读取
- ✅ 历史对话会加入 context
- 性能:极快 (内存访问)

#### 情况 2:Session 不在缓存中 (程序重启后)
- ✅ 从 SQLite 数据库加载 session
- ✅ 从数据库加载**所有**历史消息 (`GetMessages(sessionID, 0)`)
- ✅ 历史对话**完全**加入 context
- ✅ 加载后放入内存缓存
- 性能:稍慢 (数据库 I/O),但可接受

### 完整流程示例

```
用户操作: 打开历史 session "abc123" 并发送消息 "继续上次的讨论"

1. WebSocket 接收: sessionID="abc123", message="继续上次的讨论"
   ↓
2. Runner.Run(sessionID="abc123")
   ↓
3. SessionManager.GetOrCreate("abc123")
   ├─ 检查缓存: cache["abc123"] → nil (程序重启后缓存为空)
   ├─ 从数据库加载: db.GetSession("abc123") ✅
   ├─ 加载历史消息: db.GetMessages("abc123", 0)
   │  → 返回 [msg1, msg2, msg3, ..., msgN]  ✅ 所有历史消息
   └─ 放入缓存: cache["abc123"] = CachedSession{Messages: [...]}
   ↓
4. buildMessages()
   ├─ System Prompt
   ├─ msg1 (user: "你好")                     ← 历史消息 1
   ├─ msg2 (assistant: "你好!有什么可以帮助你?") ← 历史消息 2
   ├─ msg3 (user: "介绍一下量子计算")           ← 历史消息 3
   ├─ msg4 (assistant: "量子计算是...")        ← 历史消息 4
   └─ msg5 (user: "继续上次的讨论")            ← 当前输入
   ↓
5. 发送到 LLM Provider (完整 context)
   ↓
6. 流式返回响应
```

## 合理性分析

### ✅ 优点

1. **用户体验连贯**
   - 重启后能无缝恢复对话上下文
   - 符合用户期望:历史对话应该被"记住"
   - 支持长期对话管理

2. **设计合理**
   - 双层存储:内存缓存 + 持久化数据库
   - LRU 缓存策略:限制内存使用
   - 懒加载:按需从数据库恢复

3. **性能优化**
   - 热数据在内存中,访问快速
   - 冷数据在数据库中,按需加载
   - 缓存淘汰机制防止内存溢出

### ⚠️ 潜在问题

1. **Token 消耗**
   - 问题:每次请求都携带**完整历史**
   - 影响:长对话会导致巨大的 token 消耗和成本
   - 现有缓解措施:
     ```go
     // internal/runner/runner.go:834-848
     // 历史压缩机制
     if r.compactor != nil {
         if r.compactor.NeedsCompaction(messages) {
             compacted := r.compactor.CompactWithFallback(ctx, messages)
             messages = compacted
         }
     } else if compressed, changed := r.history.Compress(messages); changed {
         messages = compressed
     }
     ```

2. **Context Window 限制**
   - 问题:LLM 有 context 长度限制 (如 GPT-4: 128k tokens)
   - 现有缓解:HistoryManager 实现了压缩逻辑
     - `maxMessages`: 限制消息数量 (默认 100)
     - `maxTokens`: 限制总 token 数 (默认 100,000)
     - 压缩策略:保留 system messages + 最近的消息

3. **内存占用**
   - 问题:长对话占用大量内存
   - 现有缓解:LRU 缓存淘汰 (`maxSize: 100`)

4. **启动时无历史**
   - 问题:首次加载时缓存为空,需要数据库 I/O
   - 影响:轻微延迟 (通常 <100ms)
   - 不是大问题:后续请求会使用缓存

## 优化建议

### 1. 智能上下文窗口管理 (推荐)

```go
// 建议:根据对话长度动态调整加载深度
func (m *SessionManager) GetMessagesWithLimit(sessionID string, maxMessages int) {
    // 只加载最近 N 条消息,而不是全部
    messages, err := m.db.GetMessages(sessionID, maxMessages)
}
```

### 2. 摘要机制 (长期优化)

```go
// 对历史对话生成摘要,用摘要替代旧消息
type SessionSummary struct {
    SessionID string
    Summary   string  // "用户询问了量子计算的基本概念..."
    FromMsgID string
    ToMsgID   string
}
```

### 3. 分层加载 (增量优化)

```
首次请求:加载最近 20 条消息
如果需要:按需加载更早的消息 (用户主动请求"查看更早的对话")
```

### 4. 向量搜索 (高级优化)

```go
// 根据当前问题,语义搜索相关的历史片段
relevantHistory := vectorDB.SearchSimilar(userInput, k=5)
context = systemPrompt + relevantHistory + currentInput
```

## ✅ 修复状态 (2026-02-13)

**问题已修复！** 

### 修复内容

修改了 `internal/runner/runner.go` 中的 `runACPMode` 方法，使其：
1. ✅ 使用 `buildMessages()` 加载完整的历史对话
2. ✅ 使用与非 ACP 相同的智能压缩机制（Compactor + HistoryManager）
3. ✅ 正确地将 skills 注入到 system message 中
4. ✅ 添加了详细的日志用于调试

### 测试验证

创建了专门的测试文件 `internal/runner/runner_acp_history_test.go`：

**TestRunACPMode_HistoryLoading**:
```
✅ 验证历史消息被正确加载
✅ 5条历史消息 → 7条完整消息（含 system message）
✅ 历史 context 被包含在请求中
```

**TestRunACPMode_HistoryCompression**:
```
✅ 验证压缩机制正常工作
✅ 103条消息 → 100条压缩后的消息
✅ 不会超过 token 限制
```

### 修复后的行为

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| ACP + 不重启 | ✅ 正常（ACP session 持续） | ✅ 正常（更好，有压缩） |
| ACP + 重启 | ❌ 丢失历史 | ✅ 完整历史 |
| 长对话 (>100条) | ❌ 无压缩 | ✅ 智能压缩 |
| Token 消耗 | ⚠️ 可能超限 | ✅ 受控 |

### 使用示例

重启 mote 后：
```
用户: 请总结一下我们之前的对话
AI: ✅ 能够看到并总结重启前的历史对话

（之前会说："我看不到之前的对话历史"）
```

---

## 🚨 实际测试发现的严重问题（已修复）

### 问题描述

**用户反馈:**
1. 关闭 mote 后重新打开
2. 进入历史 session 的 chat
3. 要求 AI 总结历史对话
4. **AI 只能总结重启后的对话，无法识别重启前的历史消息**
5. UI 上可以看到历史对话（说明数据在数据库中）
6. 但 AI 无法识别（说明历史消息未加入 context）

### 根本原因：ACP Provider 的设计缺陷

经过深入分析，发现 **ACP Provider (Copilot ACP)** 存在严重的设计问题：

#### 问题 1: 只使用最后一条消息

```go
// internal/provider/copilot/acp_provider.go:1034
func (p *ACPProvider) buildPromptWithAttachments(messages []provider.Message, attachments []provider.Attachment) []PromptContent {
    // ...
    
    // 1. 找到 system message 和最后一条 user message
    var systemMessage string
    var userMessage string
    for _, msg := range messages {
        if msg.Role == provider.RoleSystem && msg.Content != "" {
            systemMessage = msg.Content
        }
    }
    // 🚨 问题：只取最后一条 user message！
    for i := len(messages) - 1; i >= 0; i-- {
        if messages[i].Role == provider.RoleUser {
            userMessage = messages[i].Content
            break  // 只取一条就 break 了！
        }
    }
    
    // 2. 构建 prompt：system + 最后的 user message
    var combinedText string
    if systemMessage != "" {
        combinedText = systemMessage + "\n\n---\n\n" + userMessage
    } else {
        combinedText = userMessage
    }
    // ...
}
```

**结果:** 
- ✅ system prompt 被包含
- ❌ **所有历史对话被丢弃**
- ✅ 只有当前的 user input 被发送

#### 问题 2: ACP 协议的限制

ACP (Agent Communication Protocol) 是 GitHub Copilot CLI 的内部协议，它的设计理念是：

1. **每个 prompt 是独立的**
2. **依赖 ACP session 来维护对话历史**
3. **不期望客户端传递完整的 message history**

```go
// internal/provider/copilot/acp_provider.go:244
func (p *ACPProvider) ensureSession(ctx context.Context, conversationID string, requestedModel string) error {
    // 检查是否已有 ACP session
    if acpSID, ok := p.sessionMap.Load(conversationID); ok {
        // 复用 ACP session
        p.sessionID = acpSID.(string)
        return nil
    }
    
    // 创建新的 ACP session
    // ...
}
```

**问题在于:**
- ✅ 如果 ACP session 持续存在，Copilot CLI 会维护历史
- ❌ **如果 mote 重启，ACPProvider 也重启，sessionMap 清空**
- ❌ **新的 Stream 调用会创建新的 ACP session**
- ❌ **Copilot CLI 的新 session 没有历史信息**

### 为什么其他 Provider 没问题？

对比 REST API Provider（如 Ollama、OpenAI）：

```go
// internal/provider/ollama/provider.go (假设)
func (p *Provider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
    // ✅ 直接使用 req.Messages，包含完整历史
    resp, err := http.Post("/api/chat", json.Marshal(map[string]interface{}{
        "model": req.Model,
        "messages": req.Messages,  // ✅ 完整历史
    }))
}
```

REST API providers 将完整的 messages 数组发送给 LLM，所以历史对话能正常工作。

### 影响范围

| Provider | 历史加载 | 重启后恢复 |
|----------|---------|-----------|
| **Copilot ACP** | ❌ 只发送最后一条消息 | ❌ 丢失历史 |
| Copilot REST | ✅ 完整历史 | ✅ 正常 |
| Ollama | ✅ 完整历史 | ✅ 正常 |
| OpenAI | ✅ 完整历史 | ✅ 正常 |
| Claude | ✅ 完整历史 | ✅ 正常 |

### 解决方案

#### 方案 1: 修复 ACP Provider (推荐)

在发送到 Copilot CLI 之前，将历史消息合并到 prompt 中：

```go
// internal/provider/copilot/acp_provider.go
func (p *ACPProvider) buildPromptWithAttachments(messages []provider.Message, attachments []provider.Attachment) []PromptContent {
    var promptContent []PromptContent
    
    // 1. System message
    var systemMessage string
    for _, msg := range messages {
        if msg.Role == provider.RoleSystem {
            systemMessage = msg.Content
            break
        }
    }
    
    // 2. 构建历史对话摘要
    var historyBuilder strings.Builder
    for i, msg := range messages {
        if msg.Role == provider.RoleSystem {
            continue  // 已处理
        }
        
        // 格式化历史消息
        switch msg.Role {
        case provider.RoleUser:
            historyBuilder.WriteString(fmt.Sprintf("\n[User]: %s\n", msg.Content))
        case provider.RoleAssistant:
            historyBuilder.WriteString(fmt.Sprintf("\n[Assistant]: %s\n", msg.Content))
        case provider.RoleTool:
            // 可选：包含工具调用结果
            historyBuilder.WriteString(fmt.Sprintf("\n[Tool Result]: %s\n", msg.Content))
        }
    }
    
    // 3. 获取当前用户输入（最后一条）
    var currentInput string
    for i := len(messages) - 1; i >= 0; i-- {
        if messages[i].Role == provider.RoleUser {
            currentInput = messages[i].Content
            break
        }
    }
    
    // 4. 合并为完整 prompt
    var fullPrompt strings.Builder
    if systemMessage != "" {
        fullPrompt.WriteString(systemMessage)
        fullPrompt.WriteString("\n\n---\n\n")
    }
    if historyBuilder.Len() > 0 {
        fullPrompt.WriteString("## Conversation History:\n")
        fullPrompt.WriteString(historyBuilder.String())
        fullPrompt.WriteString("\n---\n\n")
    }
    fullPrompt.WriteString("## Current Input:\n")
    fullPrompt.WriteString(currentInput)
    
    promptContent = append(promptContent, PromptContent{
        Type: "text",
        Text: fullPrompt.String(),
    })
    
    // 5. 添加 attachments
    // ...
    
    return promptContent
}
```

#### 方案 2: ACP Session 持久化

将 ACP sessionMap 保存到数据库，重启时恢复：

```go
// internal/provider/copilot/acp_provider.go
type ACPSessionInfo struct {
    ConversationID string
    ACPSessionID   string
    Model          string
    CreatedAt      time.Time
}

func (p *ACPProvider) SaveSessionMapping(convID, acpSessionID, model string) error {
    // 保存到数据库
    return p.db.SaveACPSession(&ACPSessionInfo{
        ConversationID: convID,
        ACPSessionID:   acpSessionID,
        Model:          model,
        CreatedAt:      time.Now(),
    })
}

func (p *ACPProvider) RestoreSessionMappings() error {
    // 从数据库恢复
    sessions, err := p.db.ListACPSessions()
    if err != nil {
        return err
    }
    for _, s := range sessions {
        p.sessionMap.Store(s.ConversationID, s.ACPSessionID)
        p.sessionModelMap.Store(s.ConversationID, s.Model)
        p.reverseSessionMap.Store(s.ACPSessionID, s.ConversationID)
    }
    return nil
}
```

**问题:** Copilot CLI 进程重启后，旧的 ACP session ID 会失效，所以这个方案不可行。

#### 方案 3: 切换到 Copilot REST API (临时方案)

如果使用 Copilot REST API 而不是 ACP：

```yaml
# config.yaml
provider:
  name: copilot
  mode: rest  # 而不是 acp
  api_base: https://api.githubcopilot.com
```

REST API 会正确处理 messages 数组。

## 结论（更新）

### Mote 的 Context 处理是否合理?

**修正后的评价: ⚠️ 部分合理，但 ACP Provider 有严重缺陷**

**设计合理的部分:**
1. ✅ Runner 和 SessionManager 正确加载历史
2. ✅ 双层存储架构设计良好
3. ✅ 历史压缩机制正常工作
4. ✅ LRU 缓存防止内存溢出

**ACP Provider 的严重问题:**
1. ❌ **只发送最后一条消息，丢弃所有历史对话**
2. ❌ **重启后创建新 ACP session，无法恢复历史**
3. ❌ **用户体验严重受损**

**受影响的场景:**
- ❌ 使用 Copilot ACP + 重启 mote → **无法获得历史 context**
- ✅ 使用 Copilot REST / Ollama / OpenAI → **历史 context 正常**
- ⚠️ 使用 Copilot ACP + 不重启 → **历史 context 正常**（ACP session 持续存在）

**优先级:**
1. 🔴 **立即修复:** 修改 `buildPromptWithAttachments` 以包含历史对话
2. 🟡 **中期优化:** 改进历史压缩算法
3. 🟢 **长期规划:** 考虑统一 Provider 接口的语义

## 代码示例:验证历史加载

```go
// 测试:验证重启后历史是否加载
func TestHistoryLoadingAfterRestart(t *testing.T) {
    // 1. 创建 session 并添加消息
    db, _ := storage.Open("test.db")
    manager := scheduler.NewSessionManager(db, 100)
    
    session, _ := manager.Create(nil)
    manager.AddMessage(session.ID, "user", "第一条消息", nil, "")
    manager.AddMessage(session.ID, "assistant", "回复1", nil, "")
    
    // 2. 模拟程序重启:清空缓存
    manager.Clear()
    
    // 3. 重新加载 session
    reloaded, _ := manager.Get(session.ID)
    
    // 4. 验证历史消息是否被加载
    assert.Equal(t, 2, len(reloaded.Messages))
    assert.Equal(t, "第一条消息", reloaded.Messages[0].Content)
    assert.Equal(t, "回复1", reloaded.Messages[1].Content)
}
```

## 参考代码位置

- Session 管理:`internal/scheduler/session_manager.go`
- 历史加载:`internal/runner/runner.go:1226` (buildMessages)
- 历史压缩:`internal/runner/history.go`
- Runner 主循环:`internal/runner/runner.go:708` (runLoop)
- 数据库操作:`internal/storage/message.go:142` (GetMessages)
