# Runner 模块化重构方案

## 📊 现状确认（已验证）

### 核心数据
- **文件**：`internal/runner/runner.go` - **2397 行**
- **方法数**：**51 个** `func (r *Runner)` 方法
- **核心循环**：2 个重复循环（`runLoopCore` + `runACPMode`，各 200+ 行）
- **文件大小**：~80KB

### 已验证的架构问题

#### ❌ 问题 1：过度集中（God Object）
`runner.go` 混合了 **10+ 种职责**：

```
当前职责混杂：
├── Provider 路由 (GetProvider, SetProviderPool, 73 lines)
├── Tool 执行 (executeToolsWithSession, 200+ lines)
├── 消息构建 (buildMessages, 120+ lines)
├── 上下文压缩 (inline compaction 逻辑)
├── 暂停控制 (pauseController 调用)
├── MCP 集成 (PreprocessMCPInput)
├── Skill/Hook 执行 (triggerHook)
├── Policy 审批 (policyExecutor)
├── Channel 系统 (handleChannelMessage, 80+ lines)
├── Token 追踪 (UpdateTokens, shouldRunMemoryFlush)
└── 核心循环 (runLoopCore/runACPMode, 400+ lines)
```

#### ❌ 问题 2：重复代码严重
`runLoopCore` vs `runACPMode` **高度重复**：

| 重复逻辑 | 行数 | DRY 违反 |
|---------|------|---------|
| 添加用户消息 | L831, L1203 | ✓ |
| 构建消息 | L824, L1206 | ✓ |
| 压缩历史 | L880-922, L1220-1253 | ✓ |
| 触发 Hook | L799-805, 未见但逻辑应相同 | ✓ |
| 发送事件 | 散落各处 | ✓ |

#### ❌ 问题 3：状态管理混乱
`runLoopCore` 内部状态变量：

```go
// Lines 766-1195 (430 行循环中的状态)
consecutiveToolErrors := 0        // 错误追踪
contextRetried := false           // 重试 flag
transientRetries := 0             // 临时错误重试
useChat := false                  // 模式切换
var totalUsage Usage              // Token 统计
var resp *provider.ChatResponse   // 临时响应
```

**问题**：
- 作用域过大（430 行可见）
- 状态转换逻辑分散
- 难以追踪和调试

#### ❌ 问题 4：缺少抽象层
当前直接调用链：

```
runLoopCore
  → buildMessages (inline 120 lines)
  → callProviderWith (inline 250 lines)
  → executeToolsWithSession (inline 200 lines)
```

**应该抽象的组件** | **状态** | **影响** |
|-----------------|---------|---------|
| `LoopOrchestrator` | ❌ 不存在 | 循环逻辑无法复用 |
| `MessageBuilder` | ❌ 仅 buildMessages 方法 | 难以自定义消息构建策略 |
| `ToolExecutor` | ❌ 仅 executeToolsWithSession 方法 | 无法拦截执行前后 |
| `ProviderGateway` | ❌ 仅 callProviderWith 方法 | 重试逻辑硬编码 |
| `StopConditionChecker` | ❌ 不存在 | 停止条件散落各处 |
| `RetryPolicy` | ❌ 不存在 | 硬编码在循环内 |

#### ❌ 问题 5：错误处理硬编码
重试逻辑示例（Lines 948-975）：

```go
// 上下文溢出重试
if provider.IsContextWindowExceeded(err) && !contextRetried {
    contextRetried = true
    compacted := r.compactor.CompactWithFallback(ctx, messages, prov)
    continue
}

// 临时错误重试
if provider.IsRetryable(err) && transientRetries < maxTransientRetries {
    transientRetries++
    backoff := time.Duration(transientRetries*10) * time.Second
    time.Sleep(backoff)
    continue
}
```

**不可配置**：
- 重试次数（`maxTransientRetries = 2`）
- Backoff 策略（`transientRetries*10`）
- 错误判断逻辑

#### ✅ 积极方面
已有部分模块化：

```
internal/runner/
├── history.go           (HistoryManager, 123 lines) ✅
├── pause_controller.go  (PauseController) ✅
├── config.go            (Config) ✅
├── event.go             (Event types) ✅
├── mcp_preprocess.go    (MCP 预处理) ✅
└── errors.go            (错误定义) ✅
```

---

## 🎯 重构目标

### 验收标准
1. **可维护性**：单文件不超过 **800 行**
2. **可测试性**：每个组件可独立 mock 测试
3. **可扩展性**：新功能不修改核心循环
4. **可读性**：职责清晰，依赖明确
5. **兼容性**：**向后兼容**现有 API

---

## 🏗️ 重构方案

### 阶段 0：准备工作（1 天）
**任务**：
- [x] 确认现有测试覆盖率（`runner_test.go`, `runner_m04_test.go`, `runner_hooks_test.go`）
- [ ] 创建集成测试基线（保证重构不破坏功能）
- [ ] 设置重构分支 `refactor/runner-modular`

**验收**：
- 所有现有测试通过
- 有完整的端到端测试覆盖

---

### 阶段 1：提取核心抽象（3 天）

#### 1.1 创建 `LoopOrchestrator` 接口
**文件**：`internal/runner/orchestrator/orchestrator.go`

```go
package orchestrator

import (
    "context"
    "mote/internal/provider"
    "mote/internal/runner"
)

// Orchestrator 控制 Agent 循环的执行流程
type Orchestrator interface {
    // Run 执行完整的 Agent 循环
    Run(ctx context.Context, request RunRequest) (<-chan runner.Event, error)
}

// RunRequest 封装运行请求的所有参数
type RunRequest struct {
    SessionID    string
    UserInput    string
    Attachments  []provider.Attachment
    Provider     provider.Provider
}

// Config 控制循环行为
type Config struct {
    MaxIterations       int
    StopConditions      []StopCondition
    RetryPolicy         RetryPolicy
    ErrorHandler        ErrorHandler
    ProgressReporter    ProgressReporter
}
```

**实现**：
- `StandardOrchestrator`：标准工具调用循环（替代 `runLoopCore`）
- `ACPOrchestrator`：ACP 提供商循环（替代 `runACPMode`）

#### 1.2 创建 `MessageBuilder` 组件
**文件**：`internal/runner/message/builder.go`

```go
package message

// Builder 负责构建 LLM 请求消息
type Builder interface {
    // BuildMessages 构建完整的消息列表
    BuildMessages(ctx context.Context, request BuildRequest) ([]provider.Message, error)
}

// BuildRequest 封装构建请求
type BuildRequest struct {
    SessionID     string
    UserInput     string
    CachedSession *scheduler.CachedSession
    SystemPrompt  string
}

// StandardBuilder 实现标准消息构建逻辑
type StandardBuilder struct {
    systemPrompt    *prompt.SystemPromptBuilder
    skillManager    *skills.Manager
    contextManager  *context.Manager
}
```

**职责**：
- 系统提示词构建
- 历史消息加载
- Skill 注入
- 上下文管理器集成

#### 1.3 创建 `ToolExecutor` 组件
**文件**：`internal/runner/executor/tool_executor.go`

```go
package executor

// ToolExecutor 执行工具调用并返回结果
type ToolExecutor interface {
    // Execute 执行工具调用
    Execute(ctx context.Context, request ExecuteRequest) ExecuteResponse
}

// ExecuteRequest 封装执行请求
type ExecuteRequest struct {
    ToolCalls  []provider.ToolCall
    SessionID  string
    AgentID    string
}

// ExecuteResponse 封装执行结果
type ExecuteResponse struct {
    Results    []provider.Message
    ErrorCount int
}

// StandardExecutor 标准工具执行器
type StandardExecutor struct {
    registry        *tools.Registry
    policyExecutor  policy.PolicyChecker
    approvalManager approval.ApprovalHandler
    eventEmitter    EventEmitter
    heartbeatInterval time.Duration
}
```

**职责**：
- 工具调用解析
- 策略检查
- 审批流程
- 结果收集
- 心跳保活

#### 1.4 创建 `RetryPolicy` 组件
**文件**：`internal/runner/retry/policy.go`

```go
package retry

// Policy 定义重试策略
type Policy interface {
    // ShouldRetry 判断是否应该重试
    ShouldRetry(ctx context.Context, err error, attempt int) (bool, time.Duration)
    
    // Reset 重置重试状态
    Reset()
}

// CompositePolicy 组合多个重试策略
type CompositePolicy struct {
    policies []Policy
}

// ContextWindowPolicy 上下文窗口溢出重试
type ContextWindowPolicy struct {
    maxRetries int
    compactor  *compaction.Compactor
}

// TransientErrorPolicy 临时错误重试
type TransientErrorPolicy struct {
    maxRetries      int
    backoffStrategy BackoffStrategy
}

// BackoffStrategy 退避策略
type BackoffStrategy interface {
    NextBackoff(attempt int) time.Duration
}

// ExponentialBackoff 指数退避（当前：10s, 20s, 30s）
type ExponentialBackoff struct {
    BaseDelay  time.Duration
    Multiplier float64
}
```

**优势**：
- 重试逻辑可配置
- 可独立测试
- 可添加新策略（如 Circuit Breaker）

#### 1.5 创建 `StopCondition` 接口
**文件**：`internal/runner/stop/condition.go`

```go
package stop

// Condition 定义停止条件
type Condition interface {
    // ShouldStop 判断是否应该停止循环
    ShouldStop(ctx context.Context, state LoopState) (bool, string)
}

// LoopState 封装循环状态
type LoopState struct {
    Iteration           int
    ConsecutiveErrors   int
    LastResponse        *provider.ChatResponse
    TotalTokens         int64
}

// MaxIterationCondition 最大迭代次数
type MaxIterationCondition struct {
    maxIterations int
}

// ConsecutiveErrorCondition 连续错误次数
type ConsecutiveErrorCondition struct {
    maxErrors int
}

// FinishReasonCondition 完成原因（stop/length）
type FinishReasonCondition struct{}

// CompositeCondition 组合多个停止条件
type CompositeCondition struct {
    conditions []Condition
}
```

---

### 阶段 2：重构核心循环（5 天）

#### 2.1 实现 `StandardOrchestrator`
**文件**：`internal/runner/orchestrator/standard.go`

**核心逻辑**（替代 `runLoopCore` Lines 766-1195）：

```go
func (o *StandardOrchestrator) Run(ctx context.Context, req RunRequest) (<-chan Event, error) {
    events := make(chan Event, 100)
    
    go func() {
        defer close(events)
        
        // 初始化循环状态
        state := &loopState{
            iteration:          0,
            consecutiveErrors:  0,
            retryPolicy:        o.retryPolicy,
            stopConditions:     o.stopConditions,
        }
        
        for {
            // 检查停止条件
            if should, reason := o.checkStopConditions(state); should {
                events <- NewDoneEvent(reason)
                return
            }
            
            // 1. 构建消息
            messages, err := o.messageBuilder.BuildMessages(ctx, BuildRequest{
                SessionID:     req.SessionID,
                UserInput:     req.UserInput,
                CachedSession: req.CachedSession,
            })
            if err != nil {
                events <- NewErrorEvent(err)
                return
            }
            
            // 2. 压缩历史（如果需要）
            messages = o.compressIfNeeded(ctx, messages, req.Provider)
            
            // 3. 调用 LLM（带重试）
            resp, err := o.callWithRetry(ctx, req.Provider, messages, events, state)
            if err != nil {
                events <- NewErrorEvent(err)
                return
            }
            
            // 4. 处理响应
            if resp.FinishReason == "stop" || len(resp.ToolCalls) == 0 {
                events <- NewContentEvent(resp.Content)
                events <- NewDoneEvent("complete")
                return
            }
            
            // 5. 执行工具
            results := o.toolExecutor.Execute(ctx, ExecuteRequest{
                ToolCalls: resp.ToolCalls,
                SessionID: req.SessionID,
            })
            
            // 6. 更新状态
            state.iteration++
            state.consecutiveErrors = results.ErrorCount
            if results.ErrorCount > 0 {
                state.totalConsecutiveErrors += results.ErrorCount
            } else {
                state.totalConsecutiveErrors = 0
            }
        }
    }()
    
    return events, nil
}
```

**优势**：
- 清晰的步骤分离
- 可测试的状态管理
- 策略模式（RetryPolicy, StopCondition）

#### 2.2 实现 `ACPOrchestrator`
**文件**：`internal/runner/orchestrator/acp.go`

**逻辑**（替代 `runACPMode` Lines 1197-1428）：

```go
func (o *ACPOrchestrator) Run(ctx context.Context, req RunRequest) (<-chan Event, error) {
    // ACP 提供商处理内部工具循环，只需单次调用
    events := make(chan Event, 100)
    
    go func() {
        defer close(events)
        
        // 1. 构建消息（包含历史）
        messages, err := o.messageBuilder.BuildMessages(ctx, BuildRequest{...})
        // ...
        
        // 2. 压缩历史（与 Standard 共享逻辑）
        messages = o.compressIfNeeded(ctx, messages, req.Provider)
        
        // 3. 单次 LLM 调用
        resp, err := o.callProvider(ctx, req.Provider, messages, events)
        // ...
        
        events <- NewDoneEvent("complete")
    }()
    
    return events, nil
}
```

**重用**：
- `MessageBuilder`（与 StandardOrchestrator 共享）
- 压缩逻辑（提取为方法）
- 错误处理

#### 2.3 修改 `Runner.Run()`
**文件**：`internal/runner/runner.go`（大幅简化）

**变更**：

```go
// 当前：Line 587-765 (178 lines)
func (r *Runner) Run(ctx context.Context, sessionID, userInput string, attachments []provider.Attachment) (<-chan Event, error) {
    // ... 170+ lines of setup ...
    go func() {
        r.runLoop(ctx, cached, sessionID, userInput, attachments, prov, events)
    }()
}

// 重构后：~50 lines
func (r *Runner) Run(ctx context.Context, sessionID, userInput string, attachments []provider.Attachment) (<-chan Event, error) {
    // 1. 选择 Orchestrator
    orchestrator := r.selectOrchestrator(prov)
    
    // 2. 构建请求
    request := orchestrator.RunRequest{
        SessionID:   sessionID,
        UserInput:   userInput,
        Attachments: attachments,
        Provider:    prov,
    }
    
    // 3. 委托给 Orchestrator
    return orchestrator.Run(ctx, request)
}
```

---

### 阶段 3：移除重复代码（2 天）

#### 3.1 提取共享逻辑
创建 **`internal/runner/orchestrator/common.go`**：

```go
// compressIfNeeded 共享压缩逻辑
func compressIfNeeded(ctx context.Context, messages []provider.Message, prov provider.Provider, compactor *compaction.Compactor) []provider.Message {
    if compactor != nil && compactor.NeedsCompaction(messages) {
        return compactor.CompactWithFallback(ctx, messages, prov)
    }
    return messages
}

// addUserMessage 共享消息添加逻辑
func addUserMessage(sessionManager *scheduler.SessionManager, sessionID, userInput string) error {
    _, err := sessionManager.AddMessage(sessionID, provider.RoleUser, userInput, nil, "")
    return err
}
```

#### 3.2 删除冗余代码
- 删除 `runLoopCore`（Lines 766-1195，430 行）
- 删除 `runACPMode`（Lines 1197-1428，232 行）
- **总减少**：~662 行

---

### 阶段 4：优化可测试性（2 天）

#### 4.1 依赖注入重构
修改 **`Runner` 结构**：

```go
// 当前：Lines 43-95 (53 lines)
type Runner struct {
    provider     provider.Provider
    providerPool *provider.Pool
    registry     *tools.Registry
    // ... 20+ fields
}

// 重构后：简化为组件引用
type Runner struct {
    // Core components
    orchestratorFactory *orchestrator.Factory
    messageBuilder      *message.Builder
    toolExecutor        *executor.ToolExecutor
    
    // Legacy dependencies (逐步迁移)
    sessions     *scheduler.SessionManager
    registry     *tools.Registry
    // ...
}
```

#### 4.2 创建测试工具
**文件**：`internal/runner/orchestrator/orchestrator_test.go`

```go
func TestStandardOrchestrator_Run(t *testing.T) {
    // Mock dependencies
    mockBuilder := &mockMessageBuilder{...}
    mockExecutor := &mockToolExecutor{...}
    mockProvider := &mockProvider{...}
    
    orch := &StandardOrchestrator{
        messageBuilder: mockBuilder,
        toolExecutor:   mockExecutor,
        retryPolicy:    NewDefaultRetryPolicy(),
        stopConditions: []StopCondition{
            NewMaxIterationCondition(10),
        },
    }
    
    events, err := orch.Run(context.Background(), RunRequest{...})
    // 断言 events 顺序和内容
}
```

**覆盖率目标**：85%+

---

### 阶段 5：扩展性增强（3 天）

#### 5.1 添加中间件机制
**文件**：`internal/runner/middleware/middleware.go`

```go
// Middleware 拦截器接口
type Middleware interface {
    // BeforeCall LLM 调用前
    BeforeCall(ctx context.Context, messages []provider.Message) error
    
    // AfterCall LLM 调用后
    AfterCall(ctx context.Context, response *provider.ChatResponse) error
    
    // OnError 错误处理
    OnError(ctx context.Context, err error) error
}

// Chain 中间件链
type Chain struct {
    middlewares []Middleware
}

// 示例：日志中间件
type LoggingMiddleware struct{}

// 示例：指标收集中间件
type MetricsMiddleware struct {
    prometheus *prometheus.Registry
}
```

**应用场景**：
- 请求/响应日志
- 指标收集
- Rate limiting
- 缓存

#### 5.2 插件化停止条件
允许用户自定义停止条件：

```go
// 用户自定义：Token 预算停止
type TokenBudgetCondition struct {
    maxTokens int64
}

func (c *TokenBudgetCondition) ShouldStop(ctx context.Context, state LoopState) (bool, string) {
    if state.TotalTokens > c.maxTokens {
        return true, "token budget exceeded"
    }
    return false, ""
}
```

#### 5.3 事件钩子扩展
创建 **`EventHook` 接口**：

```go
type EventHook interface {
    OnEvent(ctx context.Context, event Event) error
}

// 应用：实时保存到数据库
type PersistenceHook struct {
    db *storage.Storage
}
```

---

### 阶段 6：文档和迁移（2 天）

#### 6.1 架构文档
创建 **`docs/RUNNER_ARCHITECTURE.md`**：

```markdown
# Runner 架构说明

## 组件图
```
Runner
├── OrchestratorFactory → StandardOrchestrator
│                      └→ ACPOrchestrator
├── MessageBuilder → StandardBuilder
├── ToolExecutor → StandardExecutor
├── RetryPolicy → CompositePolicy
│                 ├→ ContextWindowPolicy
│                 └→ TransientErrorPolicy
└── StopCondition → MaxIterationCondition
                  └→ ConsecutiveErrorCondition
```

## 执行流程
1. `Runner.Run()` → 选择 Orchestrator
2. `Orchestrator.Run()` → 循环执行
   - BuildMessages
   - CompressIfNeeded
   - CallProvider (with RetryPolicy)
   - ExecuteTools
   - CheckStopConditions
3. Events → 流式返回
```

#### 6.2 迁移指南
创建 **`docs/RUNNER_MIGRATION.md`**：

```markdown
# 迁移指南

## 向后兼容性
所有现有 API 保持不变：
- `Runner.Run()` ✅
- `Runner.SetProvider()` ✅
- `Runner.SetCompactor()` ✅

## 新特性
### 自定义重试策略
```go
customRetry := retry.NewTransientErrorPolicy(
    retry.WithMaxRetries(5),
    retry.WithBackoff(retry.ExponentialBackoff{
        BaseDelay: 5 * time.Second,
        Multiplier: 2.0,
    }),
)
runner.SetRetryPolicy(customRetry)
```

### 自定义停止条件
```go
runner.SetStopConditions([]stop.Condition{
    stop.NewMaxIterationCondition(20),
    stop.NewTokenBudgetCondition(100000),
})
```
```

---

## 📦 最终文件结构

```
internal/runner/
├── runner.go              (300 lines, 核心入口)
├── history.go             (123 lines, 已存在)
├── config.go              (113 lines, 已存在)
├── event.go               (已存在)
├── pause_controller.go    (已存在)
├── mcp_preprocess.go      (已存在)
│
├── orchestrator/
│   ├── orchestrator.go    (接口定义, 100 lines)
│   ├── standard.go        (StandardOrchestrator, 300 lines)
│   ├── acp.go             (ACPOrchestrator, 200 lines)
│   ├── common.go          (共享逻辑, 150 lines)
│   ├── factory.go         (工厂模式, 50 lines)
│   └── orchestrator_test.go
│
├── message/
│   ├── builder.go         (接口 + StandardBuilder, 200 lines)
│   ├── system_prompt.go   (提示词构建逻辑, 100 lines)
│   └── builder_test.go
│
├── executor/
│   ├── tool_executor.go   (接口 + StandardExecutor, 250 lines)
│   ├── heartbeat.go       (心跳逻辑, 50 lines)
│   └── executor_test.go
│
├── retry/
│   ├── policy.go          (接口定义, 80 lines)
│   ├── context_window.go  (上下文窗口重试, 100 lines)
│   ├── transient.go       (临时错误重试, 100 lines)
│   ├── backoff.go         (退避策略, 80 lines)
│   └── retry_test.go
│
├── stop/
│   ├── condition.go       (接口定义, 60 lines)
│   ├── max_iteration.go   (最大迭代, 40 lines)
│   ├── consecutive_error.go (连续错误, 50 lines)
│   ├── finish_reason.go   (完成原因, 30 lines)
│   └── stop_test.go
│
└── middleware/
    ├── middleware.go      (接口定义, 60 lines)
    ├── logging.go         (日志中间件, 80 lines)
    ├── metrics.go         (指标收集, 100 lines)
    └── middleware_test.go
```

**总行数对比**：
- **当前**：`runner.go` 2397 行
- **重构后**：最大文件 ~300 行

---

## 🎯 优先级排序

### P0（必须）- 2 周
- **阶段 1**：提取核心抽象（MessageBuilder, ToolExecutor, RetryPolicy）
- **阶段 2**：重构核心循环（StandardOrchestrator, ACPOrchestrator）
- **阶段 3**：移除重复代码（DRY）

### P1（重要）- 1 周
- **阶段 4**：优化可测试性（依赖注入，Mock）
- **阶段 6**：文档和迁移指南

### P2（可选）- 1 周
- **阶段 5**：扩展性增强（中间件，插件化）

---

## ✅ 成功指标

| 指标 | 当前 | 目标 |
|------|------|------|
| 最大文件行数 | 2397 | <800 |
| 重复代码行数 | ~662 | 0 |
| 单元测试覆盖率 | ~60% | >85% |
| 平均圈复杂度 | 25+ | <10 |
| 新功能添加时间 | 2 天 | <4 小时 |

---

## 🚨 风险与缓解

### 风险 1：破坏现有功能
**缓解**：
- 保持 `Runner.Run()` API 不变
- 先创建新组件，再迁移
- 每阶段后运行全量集成测试

### 风险 2：性能下降
**缓解**：
- 基准测试（Benchmark）对比
- 避免不必要的内存分配
- 保持现有的流式处理

### 风险 3：团队学习曲线
**缓解**：
- 详细的架构文档
- 代码示例和迁移指南
- Code Review 培训

---

## 📝 执行计划

### Week 1-2
- [ ] 阶段 0：准备工作（测试基线）
- [ ] 阶段 1.1-1.3：创建 Orchestrator, MessageBuilder, ToolExecutor

### Week 3-4
- [ ] 阶段 1.4-1.5：创建 RetryPolicy, StopCondition
- [ ] 阶段 2.1-2.2：实现 StandardOrchestrator, ACPOrchestrator

### Week 5
- [ ] 阶段 2.3：修改 Runner.Run()
- [ ] 阶段 3：移除重复代码

### Week 6
- [ ] 阶段 4：优化可测试性
- [ ] 阶段 6：文档和迁移指南

---

## 💡 后续优化方向

1. **分布式执行**：支持多机工具执行
2. **流式工具调用**：工具结果流式返回
3. **断点续传**：循环中间状态持久化
4. **并行工具调用**：同时执行多个独立工具
5. **智能压缩**：基于 LLM 的上下文总结

---

## 📚 参考资料

- [LangChain Agent 架构](https://python.langchain.com/docs/modules/agents/)
- [Clean Architecture by Robert C. Martin](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [Go Project Layout](https://github.com/golang-standards/project-layout)

---

**作者**：架构重构团队  
**日期**：2026-02-20  
**版本**：v1.0
