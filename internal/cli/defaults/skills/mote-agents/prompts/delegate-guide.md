# Multi-Agent Delegation Guide

## Quick Reference（速查）

| 概念 | 说明 |
|------|------|
| `delegate(agent, prompt)` | 调用子 agent 执行任务 |
| Standard Agent | 单轮 LLM 调用，适合简单任务 |
| PDA Agent (`steps`) | 多步编排流水线，支持路由、递归、嵌套 |
| **`agent_ref` 步骤** | **静态**引用 — agent 名称在配置时固定 |
| **`route` 步骤** | **动态**选择 — LLM 运行时决定调用哪个 agent |
| `_end` | 路由特殊目标，终止 PDA 执行 |
| 自递归 | route 分支指向自身 agent，需设 `max_recursion` |

> **关键区别**: `agent_ref` 的 `agent` 字段是配置时固定的字符串，不支持运行时动态替换。如果需要根据中间结果动态选择不同 agent，**必须使用 `route` 步骤**。

## Overview

Mote supports delegating tasks to specialized sub-agents via the `delegate` tool. Each sub-agent runs in its own isolated session with its own system prompt, model, and tool set.

Agents can operate in two modes:
- **Standard mode** — A single LLM call with the agent's system prompt and tools (default)
- **PDA mode** — A structured multi-step orchestration pipeline using a pushdown automaton engine. Steps are executed sequentially with frame-local context isolation and support for agent composition, routing, and recursion.

You can dynamically manage agents at runtime using the `manage_agents` tool.

## Agent Configuration

### 方式一：`agents.yaml` 集中配置

Agents 存储在 `~/.mote/agents.yaml` (独立配置文件，优先级高于 `config.yaml` 中的 agents 段)。

```yaml
# ~/.mote/agents.yaml
agents:
  code-reviewer:
    description: "Reviews code for correctness and style"
    system_prompt: "You are an expert code reviewer..."
    model: ""
    tools: ["read_file", "list_dir"]
    max_depth: 1
    timeout: "5m"
```

> **向下兼容**: 如果 `agents.yaml` 不存在，系统会从 `config.yaml` 的 `agents` 段读取。首次通过 UI 或 API 修改 agent 后，所有 agents 会自动迁移到 `agents.yaml`。

### 方式二：`agents/` 目录分文件配置（推荐 LLM 使用）

在 `~/.mote/agents/` 目录下放置 `.yaml` 或 `.yml` 文件。每个文件会被自动加载，优先级高于 `agents.yaml`（同名 agent 会覆盖）。

这是 **LLM 创建和管理 agent 的推荐方式**，因为：
- 使用 `write_file` 工具即可完成，无需学习额外 API
- 每个 agent 独立一个文件，便于管理和版本控制
- YAML 是声明式格式，LLM 生成准确率更高
- 不受"新 agent 需要重启会话"的限制：下次 `Load` 时自动发现

#### 格式 A：标准格式（一个文件多个 agent）

文件内容与 `agents.yaml` 格式完全一致，使用 `agents:` 顶级 key：

```yaml
# ~/.mote/agents/review-team.yaml
agents:
  code-reviewer:
    description: "代码审查专家"
    system_prompt: "你是一位代码审查专家..."
    model: "gpt-4o"
    tools: ["read_file", "list_dir", "grep"]
    max_depth: 1
    timeout: "5m"
  
  security-reviewer:
    description: "安全审计专家"
    system_prompt: "你是一位安全审计专家..."
    tools: ["read_file", "grep"]
    timeout: "3m"
```

#### 格式 B：单 agent 格式（一个文件一个 agent，文件名即名称）

文件内容直接是 AgentConfig 字段，**文件名（去掉扩展名）作为 agent 名称**：

```yaml
# ~/.mote/agents/code-reviewer.yaml
# → agent 名称自动为 "code-reviewer"
description: "代码审查专家"
system_prompt: |
  你是一位资深代码审查专家。你的职责是：
  - 检查代码正确性、性能和风格
  - 识别潜在 bug 和安全隐患
  - 给出具体的改进建议和示例代码
model: "gpt-4o"
tools:
  - read_file
  - list_dir
  - grep
max_depth: 1
timeout: "5m"
temperature: 0.2
```

#### 格式 B 的 PDA 编排示例

```yaml
# ~/.mote/agents/code-pipeline.yaml
# → agent 名称自动为 "code-pipeline"
description: "自动化代码审查流水线"
system_prompt: "你是代码审查流水线编排器。"
timeout: "15m"
max_recursion: 3
steps:
  - type: prompt
    label: "分析代码"
    content: "阅读并分析目标代码的结构，列出所有函数及其用途。"
  
  - type: agent_ref
    label: "安全审查"
    agent: "security-reviewer"
    content: "重点检查 SQL 注入和 XSS 漏洞"
  
  - type: route
    label: "决定下一步"
    prompt: "根据分析结果，需要重构还是仅报告？只回复 'refactor' 或 'report'"
    branches:
      refactor: "refactor-agent"
      report: "_end"
      _default: "_end"
  
  - type: prompt
    label: "总结报告"
    content: "综合所有审查结果，输出完整报告。"
```

#### LLM 创建 agent 的推荐步骤

1. 确定 agent 用途，构思 `description` 和 `system_prompt`
2. 使用 `write_file` 工具直接写入文件：
   ```
   write_file(path="~/.mote/agents/my-agent.yaml", content="description: ...\nsystem_prompt: |\n  ...\n")
   ```
3. 重启会话或服务后，agent 自动可用
4. 如果需要 PDA 编排，添加 `steps` 字段

> **目录不存在时**：系统自动忽略，不影响正常运行。LLM 写入前应先确保目录存在（使用 `shell` 工具 `mkdir -p ~/.mote/agents`）。

### 加载优先级

配置加载按以下优先级（从低到高，后者覆盖前者的同名 agent）：

1. `config.yaml` 中的 `agents` 字段
2. `agents.yaml` 独立文件（完全替换 config.yaml 中的 agents）
3. `agents/` 目录中的各 YAML 文件（**合并**到已有 agents，同名则覆盖）

### Configuration fields

| Field | Type | Description |
|-------|------|-------------|
| `description` | string | Agent 描述，用于选择和路由 |
| `system_prompt` | string | Agent 的系统提示词 |
| `provider` | string | LLM Provider (空=继承主 runner) |
| `model` | string | LLM 模型名 (空=继承主 runner) |
| `tools` | string[] | 可用工具列表 (空=继承所有工具) |
| `tags` | string[] | 分类标签，用于 UI 分组和过滤 |
| `stealth` | bool | 隐身模式：为 true 时该 Agent 不会注入到主 Agent 的系统提示词中，节省 token。但仍可通过 delegate 工具调用 |
| `entry_point` | bool | 入口模式：为 true 时在 @ 引用列表中作为“入口”优先展示，通常一个团队只需 1 个入口 |
| `max_depth` | int | 该Agent还能继续向下委派的层数 (默认 0=继承全局设定)。设为 1 表示只能再委派一层，设为 2 表示两层，以此类推 |
| `max_iterations` | int | 最大迭代次数 |
| `max_tokens` | int | 最大输出 token 数 (0=继承主 runner) |
| `temperature` | float | 采样温度 |
| `timeout` | string | 超时时间（默认无超时）。可设为 "5m"/"30m" 等限制执行时长，特殊值 "0"/"none"/"infinite" 也表示无超时 |
| `enabled` | bool* | 是否启用 (nil=true，向下兼容) |
| `steps` | Step[] | PDA 编排步骤 (见下方) |
| `max_recursion` | int | PDA 自递归最大次数 (默认 0=不允许) |

### 隐身与入口属性用法

**`stealth`（隐身）**：当一个团队包含大量子 Agent 时，将非入口的 Agent 设为隐身可以避免它们的信息被注入到主 Agent 的系统提示词中，从而节省大量 token。隐身的 Agent 仍然可以被 PDA 编排和 delegate 工具调用。

**`entry_point`（入口）**：将团队的顶层 Agent 设为入口，它会在用户通过 @ 引用选择 Agent 时优先展示。一个团队通常只需要 1 个入口。

**典型用法**：
```yaml
agents:
  团队入口:
    entry_point: true          # ← 入口，@ 引用中优先展示
    description: "团队总调度"
    steps: [...]               # PDA 编排
  内部工作者A:
    stealth: true              # ← 隐身，不注入系统提示词
    description: "专业子任务"
  内部工作者B:
    stealth: true
    description: "专业子任务"
```

> **重要**：如果在 PDA 编排中引用现有的 Agent（而非新建），不要改变现有 Agent 的 `stealth` / `entry_point` 设置。

## Managing Agents

有两种方式管理 agents：工具调用（`manage_agents`）和文件写入（`write_file` → `agents/` 目录）。

### 方式对比

| 维度 | `manage_agents` 工具 | `write_file` → `agents/` 目录 |
|------|---------------------|------------------------------|
| **操作** | 调用专用工具，传入 JSON config | 写入 YAML 文件到目录 |
| **格式** | JSON（嵌套对象参数） | YAML（声明式，适合多行文本） |
| **生效时机** | 立即持久化到 `agents.yaml`，但 delegate 可用列表需重启会话 | 下次 Load 时自动发现 |
| **适合 LLM** | 中等 — JSON 嵌套深，`system_prompt` 多行文本需转义 | **高** — YAML 天然支持多行文本 `\|` 语法 |
| **适合 UI/API** | **高** — 结构化 CRUD 操作 | 低 — 需操作文件系统 |
| **版本控制** | 所有 agent 混在一个文件中 | 每个文件独立，易于 git 管理 |
| **删除 agent** | `manage_agents(action="delete")` | 删除文件即可 |
| **复杂编排** | JSON 中编写 steps 可读性差 | YAML 编写 steps 可读性好 |

### **推荐**

- **LLM 自主创建 agent → 优先用 `write_file` + `agents/` 目录**
  - YAML 格式对 LLM 更友好，尤其是包含 `system_prompt` 多行文本和 `steps` 编排时
  - 避免 JSON 中处理换行符转义的问题
  - 文件名即 agent 名称，直观清晰

- **运行时动态增删改查 → 用 `manage_agents` 工具**
  - 适合快速实验性修改
  - 适合 UI 集成

### 方式 A：使用 `manage_agents` 工具

Use the `manage_agents` tool to create, update, delete, or list agents.

### List all agents

```
manage_agents(action="list")
```

### Create a standard agent

```
manage_agents(action="create", name="code-reviewer", config={
  "description": "Reviews code for correctness, performance, and style",
  "system_prompt": "You are an expert code reviewer. Analyze code for bugs, performance issues, and style violations.",
  "model": "",
  "tools": ["read_file", "list_dir", "shell"],
  "max_depth": 1,
  "timeout": "5m"
})
```

### Create a PDA agent (structured steps)

PDA agents define an ordered sequence of steps. When delegated to, the PDA engine executes steps one by one, maintaining isolated context per frame.

```
manage_agents(action="create", name="code-pipeline", config={
  "description": "Automated code review pipeline",
  "system_prompt": "You are a code review pipeline orchestrator.",
  "timeout": "15m",
  "steps": [
    {"type": "prompt", "label": "Analyze code", "content": "分析代码结构"},
    {"type": "agent_ref", "label": "Security review", "agent": "security-checker"},
    {"type": "route", "label": "Decide", "prompt": "refactor or report?",
     "branches": {"refactor": "refactor-agent", "report": "_end"}},
    {"type": "prompt", "label": "Summarize", "content": "输出审查报告"}
  ],
  "max_recursion": 2
})
```

> **注意**: JSON 中编写 `steps` 可读性较差（尤其 `system_prompt` 多行文本需转义）。推荐使用方式 B（YAML 文件）编写 PDA agent。

### Update / Delete / Enable / Disable

```
manage_agents(action="update", name="code-reviewer", config={...updated config...})
manage_agents(action="delete", name="code-reviewer")
manage_agents(action="update", name="code-reviewer", config={...existing config..., "enabled": false})
```

> **Note:** Agent changes are persisted immediately to `agents.yaml`。但 `delegate` 工具的可用列表在会话启动时构建——新 agent 在下次会话或服务重启后可用。

### 方式 B：使用 `write_file` 写入 `agents/` 目录（推荐 LLM 使用）

直接用 `write_file` 工具将 YAML 文件写入 `~/.mote/agents/` 目录。

#### 创建标准 agent

```
# 先确保目录存在
shell(command="mkdir -p ~/.mote/agents")

# 写入 agent 配置文件（文件名 = agent 名称）
write_file(path="~/.mote/agents/code-reviewer.yaml", content="description: \"代码审查专家，检查正确性、性能和风格\"\nsystem_prompt: |\n  你是一位资深代码审查专家。你的职责是：\n  - 检查代码正确性、性能和风格\n  - 识别潜在 bug 和安全隐患\n  - 给出具体的改进建议\nmodel: \"gpt-4o\"\ntools:\n  - read_file\n  - list_dir\n  - grep\nmax_depth: 1\ntimeout: \"5m\"\ntemperature: 0.2\n")
```

#### 创建 PDA 编排 agent

```
write_file(path="~/.mote/agents/review-pipeline.yaml", content="description: \"自动化代码审查流水线\"\nsystem_prompt: \"你是代码审查流水线编排器。\"\ntimeout: \"15m\"\nmax_recursion: 3\nsteps:\n  - type: prompt\n    label: \"分析代码\"\n    content: \"阅读并分析目标代码结构。\"\n  - type: agent_ref\n    label: \"安全审查\"\n    agent: \"security-reviewer\"\n  - type: route\n    label: \"决定下一步\"\n    prompt: \"需要重构还是仅报告？只回复 refactor 或 report\"\n    branches:\n      refactor: \"refactor-agent\"\n      report: \"_end\"\n      _default: \"_end\"\n  - type: prompt\n    label: \"总结报告\"\n    content: \"综合所有审查结果，输出完整报告。\"\n")
```

#### 创建多 agent 团队

使用标准格式（`agents:` 顶级 key）在一个文件中定义多个 agent：

```
shell(command="mkdir -p ~/.mote/agents")
write_file(path="~/.mote/agents/my-team.yaml", content="agents:\n  agent-a:\n    description: \"...\"\n    system_prompt: \"...\"\n  agent-b:\n    description: \"...\"\n    system_prompt: \"...\"\n")
```

> 完整的多 agent 团队 + 路由编排示例见文末 **Complete PDA Example** 小节。

#### 管理文件

```
shell(command="ls ~/.mote/agents/")              # 查看已有 agent 文件
shell(command="rm ~/.mote/agents/my-agent.yaml")  # 删除 agent
```

#### 重新加载 agents（热加载）

写入文件后，需要调用 reload API 让系统加载新配置：

```
# 使用 shell 调用 reload 端点
shell(command="curl -s -X POST http://localhost:18788/api/v1/agents/reload")
# 返回: {"status":"ok","count":5}
```

`POST /api/v1/agents/reload` 会从 `agents.yaml` 和 `agents/` 目录重新读取所有配置，无需重启服务。

用户也可以在 Web UI 的 **Agents 页面** 点击 **「重新加载」** 按钮触发同样的操作。

#### 验证 agents/ 目录文件

写入后可调用验证 API 检查语法和格式：

```
shell(command="curl -s http://localhost:18788/api/v1/agents/validate-dir | python3 -m json.tool")
```

返回每个文件的解析状态：
- `valid: true/false` — 是否成功解析
- `format: "multi" / "single"` — 使用的格式（标准格式 / 单 agent 格式）
- `agents: [...]` — 解析出的 agent 名称列表
- `error: "..."` — 错误信息（语法错误、格式不识别等）

推荐工作流：
1. `write_file` 写入 YAML 文件
2. 调用 `validate-dir` API 确认无语法错误
3. 调用 `reload` API 加载新配置
4. 使用 `delegate_task` 测试新 agent

## PDA Step Types

### Step type reference

| Type | Description | Required fields | Optional fields |
|------|-------------|-----------------|-----------------|
| `prompt` | LLM 执行指定提示词 | `content` — 提示词文本 | `label` |
| `agent_ref` | **静态**委托给另一个 agent | `agent` — 目标 agent 名（固定值） | `label`, `content` (注入上下文) |
| `route` | **动态**路由 — LLM 运行时选择目标 agent | `prompt` — 路由问题; `branches` — 关键词→目标映射 | `label`, `content` (注入上下文) |

### ⚠️ 上下文传递行为（route vs agent_ref 的关键区别）

`route` 和 `agent_ref` 除了「静态 vs 动态」目标选择的区别外，还有一个**更关键的区别：子 Agent 能看到的上下文范围不同**。

| 维度 | `agent_ref` | `route` |
|------|-------------|---------|
| **子 Agent 初始上下文** | 🆕 **全新上下文** — 仅包含 `[用户任务描述]\n{step.content + previousResult}` | 📋 **继承父帧完整上下文** — 拷贝父 Agent 当前帧的全部对话历史 |
| **子 Agent 能看到的信息** | 只能看到当前步骤的 `content` 和上一步结果 | 能看到父 Agent 从第一步到当前步的所有对话、所有中间结果 |
| **目标选择** | 配置时固定（静态绑定） | 运行时 LLM 决定（动态选择） |
| **适合场景** | 独立子任务，不需要知道前因后果 | 需要看到讨论过程、需要增量修改、需要验收反馈循环 |

> **这意味着什么？**
> - 用 `agent_ref` 调用子 Agent 时，子 Agent **完全不知道**父 Agent 之前的对话内容。它只收到步骤配置中写死的 `content` 文本和上一步的结果摘要。
> - 用 `route` 调用子 Agent 时，子 Agent **能看到**父 Agent 帧中积累的完整上下文，包括之前步骤产生的所有对话、验收意见、工具调用结果等。

#### 典型错误模式：验收重做循环用了 agent_ref

```yaml
# ❌ 错误：工作→验收→重做 循环中用 agent_ref
steps:
  - type: agent_ref
    agent: worker        # 工作 Agent 用 agent_ref 调用
    content: "请完成任务"
  - type: agent_ref  
    agent: reviewer       # 验收 Agent 用 agent_ref 调用
    content: "请审查成果"
  - type: route
    prompt: "通过还是重做？"
    branches:
      通过: _end
      重做: my-scheduler   # 自递归
```

**问题**：当验收不通过触发重做时，worker Agent 再次被 agent_ref 调用，它拿到的是全新上下文 —— **完全看不到验收 Agent 指出的问题**，只能把所有工作从头做一遍，无法针对性修改。

```yaml
# ✅ 正确：用 route 调用工作/验收 Agent，确保上下文传递
steps:
  - type: route
    prompt: "开始工作，输出一个词：工作"
    content: |
      请完成任务。
      如果上下文中有验收反馈意见，请针对反馈问题重点修改。
    branches:
      工作: worker
      _default: worker
  - type: route
    prompt: "开始验收，输出一个词：验收"
    content: "请审查成果，逐条检查。"
    branches:
      验收: reviewer
      _default: reviewer
  - type: route
    prompt: "通过还是重做？"
    branches:
      通过: _end
      重做: my-scheduler
```

**效果**：重做时，worker Agent 通过 route 调用，继承了父帧上下文，**能看到验收 Agent 之前指出的所有问题**，可以做针对性修改。

### 何时用哪种步骤？

| 场景 | 推荐步骤 | 原因 |
|------|---------|------|
| 独立子任务，无需知道前因后果 | `agent_ref` | 子 Agent 获得干净上下文，不受父帧干扰 |
| 子 Agent 需要看到父帧之前的讨论、结果 | **`route`** | route 调用会继承父帧完整上下文 |
| 工作→验收→重做 迭代循环 | **`route`** | 重做时子Agent需要看到验收反馈才能针对性修复 |
| 根据中间结果选择不同 agent | **`route`** | 需要 LLM 运行时判断和动态分支 |
| 条件分支（继续/结束、A方案/B方案） | **`route`** | 用 `branches` 映射不同路径 |
| 多轮循环（讨论、迭代改进） | **`route`** + 自递归 | 分支指向自身 agent |
| 最终成果汇总（不需要父帧讨论历史） | `agent_ref` | 汇总 Agent 从外部数据源获取信息，不需要父帧上下文 |
| 纯 LLM 计算（分析、生成、总结等） | `prompt` | 无需委托其他 agent |

> **⚠️ 常见误区**: `agent_ref` 的 `agent` 字段是**配置时写死的字符串**，引擎不做任何模板替换或变量展开。如果你需要"根据上一步结果动态决定调用哪个 agent"，那就**必须用 `route` 步骤**，通过 `branches` 映射来实现。

### prompt step

最基本的步骤类型。LLM 以 agent 的系统提示词为基础，执行 `content` 中的提示词。

```yaml
steps:
  - type: prompt
    label: "分析代码"
    content: "请分析这段代码的结构和潜在问题。"
```

### agent_ref step（静态绑定）

委托给另一个已配置的 agent 执行。**`agent` 字段是固定值，不支持运行时动态替换。** 目标 agent 可以是 Standard 或 PDA agent（支持嵌套编排）。

`content` 字段可选，用于向目标 agent 注入额外上下文信息（追加到委托 prompt 之后）。

```yaml
steps:
  - type: agent_ref
    label: "安全审查"
    agent: "security-checker"
    content: "重点检查 SQL 注入和 XSS 漏洞"  # 可选：注入额外上下文
```

### route step（动态选择 — PDA 核心控制流）

**`route` 是 PDA 中实现动态 Agent 选择的唯一步骤类型。** 引擎在运行时调用 LLM 评估 `prompt`，将输出与 `branches` 中的关键词匹配，从而选择不同的目标 agent 执行。

这是区别于 `agent_ref` 的关键特性：`agent_ref` 的目标 agent 在配置时固定，而 `route` 的目标 agent 在运行时由 LLM 决定。

```yaml
steps:
  - type: route
    label: "选择评审专家"
    prompt: "根据代码类型选择合适的评审专家。回复：security、performance、或 style"
    content: "当前代码分析结果：..."  # 可选：注入路由上下文
    branches:
      security: "security-reviewer"     # LLM 输出含 "security" → 委托 security-reviewer
      performance: "perf-reviewer"      # LLM 输出含 "performance" → 委托 perf-reviewer  
      style: "style-reviewer"           # LLM 输出含 "style" → 委托 style-reviewer
      _default: "general-reviewer"      # 以上都不匹配 → 委托 general-reviewer
```

#### 路由匹配优先级

LLM 输出与 branches 的匹配按以下优先级进行：

1. **Key 精确匹配** — LLM 输出 == branch key
2. **Key 子串匹配** — LLM 输出包含 branch key
3. **Value 精确匹配** — LLM 输出 == branch value (agent name)
4. **Value 子串匹配** — LLM 输出包含 branch value
5. **`_default`** — 以上均不匹配时使用 `_default` 分支

> **提示**: 为获得最佳匹配效果，route prompt 应明确要求 LLM 只输出一个关键词。

#### 特殊分支目标

| Target | Description |
|--------|-------------|
| `_end` | 立即终止 PDA 执行，跳转到最后一个步骤（通常是总结步骤） |
| `<自身agent名>` | 自递归 — 重置当前帧从步骤 0 重新开始（需设置 `max_recursion`） |
| `<其他agent名>` | 委托给已配置的 agent 执行 |

#### `_end` 终止标记

`_end` 是一个特殊的分支值，用于在满足条件时结束 PDA 循环：

```yaml
steps:
  - type: prompt
    label: "主持讨论"
    content: "继续讨论当前话题..."
  - type: route
    label: "是否继续"
    prompt: "讨论是否充分？回复'继续'或'结束'"
    branches:
      继续: "my-agent"      # 自递归，继续讨论
      结束: "_end"           # 终止循环，跳到最后步骤
      _default: "my-agent"  # 默认继续
  - type: prompt
    label: "总结"
    content: "总结以上全部讨论内容。"
```

当路由选择 `_end` 时，PDA 引擎直接跳转到最后一个步骤（index = len(steps)-1）执行。

### 自递归 (Self-Recursion)

一个 PDA agent 可以在 route 分支中引用自身，实现循环执行模式。引擎使用帧重置而非栈压入——当前帧的 StepIndex 重置为 0，从头开始执行所有步骤。

**必须设置 `max_recursion`** 以防止无限循环。

```yaml
# 一个多轮对话/讨论 agent
name: "roundtable"
max_recursion: 10
steps:
  - type: prompt
    label: "主持人发言"
    content: "作为主持人，引导讨论方向..."
  - type: route
    label: "选择嘉宾"
    prompt: "选择下一位发言的嘉宾"
    branches:
      expert-a: "expert-a"
      expert-b: "expert-b"
      _default: "expert-a"
  - type: route
    label: "是否继续"
    prompt: "讨论是否充分？"
    branches:
      继续: "roundtable"     # ← 自递归：重置当前帧
      结束: "_end"            # ← 终止循环
      _default: "roundtable" # 默认继续
  - type: prompt
    label: "总结"
    content: "总结全部讨论内容。"
```

每次自递归时 `recursionCount` 加 1，达到 `max_recursion` 后引擎自动终止。

## When to Delegate

Use delegation when:

- **Specialized expertise** is needed (e.g., a code-review agent, a research agent, a testing agent)
- **Parallel workstreams** can be broken into independent sub-tasks
- **Different models** are better suited for specific tasks (e.g., a smaller/faster model for simple lookups)
- **Isolation** is desired — sub-agent errors won't pollute the main conversation context
- **Multi-step workflows** need structured orchestration (use PDA agents with `steps`)

Do NOT delegate when:

- The task is simple and can be done directly
- The task requires deep context from the current conversation (sub-agents start fresh)
- Delegation overhead would exceed the time saved

### When to use PDA vs Standard agents

Use **Standard agents** for:
- Single-round tasks that can be completed in one LLM interaction
- Tasks that rely heavily on tool-calling loops (the orchestrator handles iteration)

Use **PDA agents** (with `steps`) for:
- Multi-stage pipelines where each stage has a distinct purpose
- Workflows that compose multiple agents in sequence
- Conditional branching based on intermediate results (route steps)
- Multi-round discussions or iterative refinement loops (self-recursion)
- Tasks where you want explicit progress tracking and checkpoint/resume capability

## How to Delegate

Use the `delegate` tool with:

1. **`agent`** — Name of the configured sub-agent to invoke
2. **`prompt`** — Clear, self-contained task description with all necessary context

### Example (standard agent)

```
delegate(agent="code-reviewer", prompt="Review the following Go function for correctness, performance, and idiomatic style:\n\nfunc process(items []Item) {...}")
```

### Example (PDA agent)

```
delegate(agent="code-pipeline", prompt="Review all Go files in the src/ directory for security issues and code quality.")
```

The PDA engine will execute each step sequentially, showing real-time progress in the UI.

## PDA Features

### Checkpoint & Resume

PDA execution state is automatically checkpointed at key points (after each step completion, frame push/pop). If execution is interrupted (error, timeout, server restart), the checkpoint is saved to the session metadata.

On the next interaction with the same session, the LLM will see a `pda_control` tool allowing it to either:
- **Continue** from the last checkpoint
- **Restart** the PDA from scratch

### Progress Tracking

During PDA execution, the UI displays:
- Step-by-step progress with completion status (pending → running → completed/failed)
- Current agent name and model
- Token consumption
- Call stack breadcrumb (when sub-agents invoke other PDA agents)

### Frame-local Context Isolation

Each PDA stack frame maintains its own LLM context。上下文隔离行为取决于步骤类型：

- **`agent_ref` 步骤**：子 Agent 以**全新上下文**运行，仅包含 `[用户任务描述]\n{step.content + previousResult}`。子 Agent 看不到父帧的任何对话历史。
- **`route` 步骤**：子 Agent **继承父帧完整上下文**（通过 `copy(parentFrame.Context)` 实现）。子 Agent 能看到父帧从第一步开始积累的所有对话、工具调用结果、验收意见等。
- 两种方式的**返回行为一致**：子 Agent 执行完成后，结果以 `[agent result]` 格式注入回父帧的上下文。

### Route Decision Suppression

路由步骤的 LLM 输出（如"继续""结束"等关键词）不会出现在用户可见的消息历史中——引擎内部处理这些决策，只展示实际内容步骤的输出。

## Best Practices

1. **Be explicit in the task description** — Sub-agents have no access to your conversation history. Include all relevant code, file paths, and requirements in the task field.

2. **Choose the right agent** — Match the sub-agent's specialization to the task. Check available agents and their descriptions before delegating.

3. **Keep tasks focused** — One clear objective per delegation. Avoid compound tasks like "review and then refactor" — split them into two delegations or use a PDA agent with steps.

4. **Handle results** — The delegation result is returned as text. Summarize or act on the result for the user rather than passing it through verbatim.

5. **Respect depth limits** — Sub-agents can themselves delegate (if configured), but there is a maximum depth limit. Design your agent hierarchy to stay within bounds.

6. **Error handling** — If a delegation fails or times out, inform the user and consider retrying or handling the task directly.

7. **PDA step design** — Keep each step focused on a single concern. Use `label` fields so users can track progress. Put expensive/slow operations in separate steps.

8. **Route prompt 要精确** — 路由步骤的 prompt 应该明确要求 LLM 只返回一个关键词，避免输出完整句子导致匹配失败。推荐格式：`"你是一个决策判断器，只需要输出一个词：X 或 Y"`

9. **善用 `_default` 分支** — 为 route 步骤设置 `_default` 分支，确保即使 LLM 输出不匹配任何关键词，流程也能继续。

10. **设置合理的 `max_recursion`** — 自递归 agent 必须设置 `max_recursion`。建议根据预期循环次数设置（通常 5-15），过大可能导致上下文窗口溢出。

11. **利用 `content` 字段注入上下文** — `agent_ref` 和 `route` 步骤支持 `content` 字段，可以向目标 agent 或路由 LLM 注入动态上下文，无需修改 agent 配置。

12. **理解上下文传递差异** — `agent_ref` 给子 Agent 全新上下文（仅含步骤 content + 上一步结果），`route` 给子 Agent 完整父帧上下文。在**工作→验收→重做**循环中，务必用 `route` 调用工作和验收 Agent，否则重做时工作 Agent 看不到验收反馈，只能从头重做而非针对性修改。

13. **route 可以固定目标** — `route` 步骤的 branches 可以所有分支都指向同一个 Agent（相当于「固定目标 + 继承上下文」），此时 route 的作用不是动态选择，而是确保上下文传递。这在迭代修改场景中非常有用。

## Complete PDA Example: Multi-Round Discussion

以下是一个完整的多轮圆桌讨论 PDA 配置示例：

```yaml
agents:
  话题讨论:
    description: "多轮圆桌讨论主持人"
    system_prompt: |
      你是一位讨论主持人。每轮讨论：
      1. 简要回顾上轮观点
      2. 提出新的讨论角度
      3. 引导下一位嘉宾发言
    timeout: "0"
    max_recursion: 10
    steps:
      - type: prompt
        label: "主持人" 
        content: |
          根据讨论历史，提出新的讨论角度。
          注意：不要做总结，只是引导讨论方向。

      - type: route
        label: "选择嘉宾"
        prompt: |
          你是一个选择器，选择下一位发言的嘉宾。
          只回复嘉宾名称。
        branches:
          历史专家: "历史专家"
          技术专家: "技术专家"
          教育专家: "教育专家"
          _default: "历史专家"

      - type: route
        label: "继续讨论"
        prompt: |
          你是一个决策判断器，只需要输出一个词。
          已有足够多元的观点了吗？
          回复"继续"或"结束"。
        branches:
          继续: "话题讨论"
          结束: "_end"
          _default: "话题讨论"

      - type: prompt
        label: "总结"
        content: "综合所有嘉宾观点，输出完整的讨论总结。"

  历史专家:
    description: "从历史角度分析问题"
    system_prompt: "你是一位历史学者，善于从历史角度分析当下问题..."

  技术专家:
    description: "从技术角度分析问题" 
    system_prompt: "你是一位技术专家，善于从技术发展角度分析问题..."

  教育专家:
    description: "从教育角度分析问题"
    system_prompt: "你是一位教育学者，善于从教育角度分析社会问题..."
```

执行流程：
1. 主持人发言 (prompt step)
2. 选择嘉宾 (route → agent_ref)
3. 嘉宾发言 (被选中的 agent 执行)
4. 判断是否继续 (route → 自递归 or _end)
5. 重复 1-4 直到选择"结束"或达到 max_recursion
6. 执行总结步骤 (最后一个 prompt step)
