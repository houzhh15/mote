# Mote

> 🚧 **开发中** - 一个开箱即用的 Local AI Agent 运行时

Mote 是一个轻量级、基于 Go 语言构建的本地 AI Agent 运行时。它提供 CLI、Web UI 和桌面应用三种交互方式，支持多模型切换、工具调用、记忆系统和定时任务等核心能力。

## ✅ 已完成功能

### 核心能力
- **多模型支持**: 通过 GitHub Copilot API 支持 OpenAI、Anthropic、Google、xAI 等模型，同时支持本地 Ollama
- **Agent 模式**: ask（问答）、edit（编辑）、agent（自主执行）、plan（规划）
- **工具系统**: 内置 shell、文件读写、HTTP 请求等工具，支持 JS 自定义工具
- **MCP 协议**: 完整支持 Model Context Protocol，可连接外部 MCP 服务器
- **记忆系统**: 基于 sqlite-vec 向量搜索，支持语义检索和自动捕获/召回
- **Skill 技能包**: 通过 manifest.json 或 SKILL.md 扩展 Agent 能力
- **定时任务**: 标准 Cron 表达式调度，支持 prompt、tool、script 类型
- **安全策略**: 工具调用黑白名单、危险操作审批机制
- **多代理委托**: 主代理可将任务分发给专用子代理，支持深度控制、循环检测和审计追踪

### 交互方式
- **CLI**: 完整命令行工具 (`mote chat`, `mote serve` 等)
- **Web UI**: 内嵌 Web 界面，支持会话管理和实时对话
- **桌面应用**: 基于 Wails 的 GUI 应用 (macOS/Windows)

## 🚧 开发中 / 不完善

- **Channel 系统**: iMessage、Reminders、Notes 集成（框架已有，功能未完善）
- **Hooks 系统**: 消息前后置钩子（基础框架完成，扩展性待增强）
- **多轮规划**: Plan 模式的复杂任务分解与执行
- **GUI 稳定性**: 桌面应用部分功能仍在调试
- **文档完善**: API 文档和使用指南待补充

---

## 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                        User Interface                           │
│  ┌─────────┐    ┌─────────────┐    ┌─────────────────────────┐ │
│  │   CLI   │    │   Web UI    │    │   Desktop (Wails GUI)   │ │
│  └────┬────┘    └──────┬──────┘    └────────────┬────────────┘ │
└───────┼────────────────┼───────────────────────┼───────────────┘
        │                │                       │
        ▼                ▼                       ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Gateway Server (HTTP)                      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │  REST API    │  │  WebSocket   │  │   Middleware Chain   │  │
│  │  (api/v1/)   │  │  (实时通信)   │  │ (CORS/Rate/Logging) │  │
│  └──────────────┘  └──────────────┘  └──────────────────────┘  │
└─────────────────────────────┬───────────────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────────────┐
│                        Core Runtime                             │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                      Runner (核心执行引擎)                │   │
│  │  • 消息处理与流式响应    • Tool Calling 循环             │   │
│  │  • 多模型动态切换        • 上下文压缩 (Compaction)        │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              │                                  │
│  ┌───────────┬───────────────┼───────────────┬──────────────┐  │
│  ▼           ▼               ▼               ▼              ▼  │
│ ┌─────┐  ┌────────┐   ┌──────────┐   ┌────────┐  ┌────────┐   │
│ │Tool │  │ Memory │   │ Provider │   │ Skills │  │ Policy │   │
│ │Reg. │  │ Index  │   │  Pool    │   │ Mgr.   │  │ Exec.  │   │
│ └─────┘  └────────┘   └──────────┘   └────────┘  └────────┘   │
│                                                                 │
│  ┌───────────┬────────────────┬───────────────┬─────────────┐  │
│  │  Hooks    │  MCP Client    │  Cron Sched.  │   JS VM     │  │
│  │  Manager  │  Manager       │  (定时任务)    │  (Goja)     │  │
│  └───────────┴────────────────┴───────────────┴─────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────────────┐
│                      Storage Layer                              │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │              SQLite + sqlite-vec (向量扩展)               │  │
│  │  • Sessions / Messages    • Memory Vectors               │  │
│  │  • Cron Jobs / History    • KV Store                     │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

---

## 核心模块算法逻辑

### 1. Runner - Agent 执行引擎

Runner 是整个系统的核心，实现了 ReAct 风格的 Agent 循环：

```
用户输入 → 构建 Prompt → LLM 推理 → Tool Call? 
                                    ↓ Yes
                              执行工具 → 结果注入 → 继续推理
                                    ↓ No
                              返回最终响应
```

**关键特性**：
- 支持流式输出 (SSE)
- 动态模型切换（通过 MultiProviderPool）
- 上下文压缩（Compaction）防止超长对话溢出
- Policy 检查危险操作

### 2. Memory - 语义记忆系统

基于向量相似度的记忆检索：

```
存储: 文本 → Embedding API → 向量 → sqlite-vec
检索: Query → Embedding → 向量相似度搜索 → Top-K 结果
```

**混合搜索策略**：
- 向量相似度 (余弦距离)
- FTS5 全文检索
- 加权融合排序

### 3. Compaction - 上下文压缩

当对话 Token 超过阈值时自动触发：

```
历史消息 → 分块 → LLM 摘要 → 压缩后的上下文 + 保留最近 N 条
```

### 4. Skills - 技能扩展

通过 `manifest.json` 定义技能：
- 自动注入 System Prompt
- 注册自定义工具
- 条件触发 (Gating)

### 5. MCP Client - 外部工具集成

实现 Model Context Protocol 客户端：
- 支持 stdio 和 HTTP 两种传输
- 动态发现和调用外部 MCP 服务器的工具

---

## 目录结构

```
mote/
├── cmd/                    # 入口程序
│   ├── mote/              # CLI 主程序
│   ├── mote-bubble/       # Wails Bubble 窗口
│   └── mote-tray/         # 系统托盘程序
│
├── api/v1/                # REST API 定义
│   ├── routes.go          # 路由注册
│   ├── chat.go            # 对话接口
│   ├── memory.go          # 记忆接口
│   ├── mcp.go             # MCP 管理接口
│   └── ...
│
├── internal/              # 核心实现 (私有)
│   ├── runner/            # Agent 执行引擎
│   │   └── runner.go      # 核心循环、Tool Calling
│   ├── provider/          # LLM Provider 抽象
│   │   ├── copilot/       # GitHub Copilot 实现
│   │   └── ollama/        # Ollama 本地模型
│   ├── memory/            # 记忆系统
│   │   ├── index.go       # 向量索引与检索
│   │   ├── capture.go     # 自动捕获
│   │   └── recall.go      # 自动召回
│   ├── tools/             # 工具系统
│   │   ├── registry.go    # 工具注册表
│   │   └── builtin/       # 内置工具
│   ├── skills/            # 技能管理
│   │   ├── manager.go     # 技能加载与激活
│   │   └── manifest.go    # manifest.json 解析
│   ├── mcp/               # MCP 协议实现
│   │   ├── client/        # MCP 客户端
│   │   └── server/        # MCP 服务端
│   ├── cron/              # 定时任务
│   │   ├── scheduler.go   # Cron 调度器
│   │   └── executor.go    # 任务执行器
│   ├── hooks/             # 钩子系统
│   ├── policy/            # 安全策略
│   ├── compaction/        # 上下文压缩
│   ├── jsvm/              # JS 运行时 (Goja)
│   ├── gateway/           # HTTP 服务器
│   ├── storage/           # 数据持久化
│   ├── config/            # 配置管理
│   ├── cli/               # CLI 命令实现
│   └── ...
│
├── gui/                   # Wails 桌面应用
│   ├── app.go             # 应用生命周期
│   └── frontend/          # Vue.js 前端
│
├── shared/ui/             # 共享 UI 组件库
├── web/frontend/          # 内嵌 Web 前端
├── pkg/                   # 公共库
├── docs/api/              # OpenAPI 文档
├── scripts/               # 构建脚本
└── Makefile               # 构建入口
```

---

## 快速开始

```bash
# 构建
make build

# 启动服务
./build/mote serve

# 或直接对话
./build/mote chat "你好"
```

配置文件位于 `~/.mote/config.yaml`，数据存储于 `~/.mote/data.db`。

---

## Multi-Agent Delegate (多代理委托)

Mote 支持将任务委托给专用子代理执行。每个子代理可以有独立的模型、工具集和系统提示词。

### 配置

在 `~/.mote/config.yaml` 中添加代理定义：

```yaml
delegate:
  enabled: true
  max_depth: 3

agents:
  researcher:
    description: "Research specialist"
    model: "gpt-4o"
    tools: ["read_file", "grep", "http", "!write_file"]
    max_depth: 2
    timeout: "3m"

  coder:
    description: "Code implementation specialist"
    model: "claude-sonnet-4-20250514"
    tools: ["read_file", "write_file", "grep", "patch"]
    timeout: "5m"
```

详细配置示例参见 `examples/multi-agent.yaml`。

### CLI 管理

```bash
# 列出所有代理
mote delegate list

# 查看代理详情
mote delegate show researcher
```

### 工具过滤

支持灵活的工具过滤语法：
- `tool_name` — 包含指定工具
- `!tool_name` — 排除指定工具
- `prefix_*` — glob 通配符匹配（如 `mcp_*` 包含所有 MCP 工具）
- `*` — 包含所有工具

### API

```
GET  /api/v1/agents                          # 列出所有代理
POST /api/v1/agents                          # 添加代理
GET  /api/v1/agents/{name}                   # 获取代理详情
PUT  /api/v1/agents/{name}                   # 更新代理
DELETE /api/v1/agents/{name}                 # 删除代理
GET  /api/v1/sessions/{id}/delegations       # 查询会话的委托记录
GET  /api/v1/delegations/{id}                # 查询单个委托记录
```

---

## License

MIT
