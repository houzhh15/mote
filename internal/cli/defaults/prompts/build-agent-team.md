---
name: 构建层级Agent团队
description: 根据用户描述的目标场景，生成完整的树状PDA Agent编排YAML——通过层级化Agent对任务逐层抽象和分解，适用于知识咨询、调研分析、本地知识库、工程协作等多种场景
type: user
arguments:
  - name: scenario
    description: 目标场景的自然语言描述（如"企业税务咨询"、"竞品调研与分析"、"本地Markdown知识库问答"、"全栈Web项目开发"、"论文选题与文献综述"等）
    required: true
  - name: knowledge_source
    description: 底层Agent的知识/数据来源方式（可选：embedded/内嵌知识, file/读取本地文件, http/联网查询, shell/执行命令, mixed/混合方式），默认mixed
    required: false
  - name: depth
    description: Agent层级深度（2-4层，默认3层）
    required: false
  - name: leaf_count
    description: 期望的叶子节点（执行Agent）数量范围（如"6-10"），默认根据场景自动决定
    required: false
priority: 10
enabled: true
---

# 任务：构建层级化 Agent 团队

你是一位 mote PDA Agent 编排专家。用户需要你为以下场景构建一个**完整的树状 Agent 团队**：

> {{scenario}}

## 核心设计理念

**不同于 RAG 的向量检索，这套体系通过层级化的 Agent 对任务逐层抽象和分解：**
- **顶层Agent**：不了解细节，但深谙任务分配——它理解问题的类别和结构，决定该交给哪个分支处理
- **中间层Agent**（PDA编排节点）：理解本领域的任务分解方式，将任务继续向下路由或编排
- **底层Agent**（叶子节点）：不理解全局，但掌握最终的具体知识、数据获取能力或执行能力

这种架构适用于多种场景——知识咨询、调研分析、本地知识库问答、复杂工程协作、内容生产流水线等。关键在于：**每一层只做自己擅长的事**。

## 输出要求

**不要只输出 YAML 文本**——你需要使用工具实际创建 agent 配置文件并使其生效。

### 执行方式（必须遵守）

1. 使用 `shell` 工具执行 `mkdir -p ~/.mote/agents`
2. 使用 `write_file` 工具将完整的 YAML 写入 `~/.mote/agents/<团队名>.yaml`
3. 使用 `shell` 工具调用 `curl -s -X POST http://localhost:18788/api/v1/agents/reload` 热加载
4. 使用 `shell` 工具调用 `curl -s http://localhost:18788/api/v1/agents/validate-dir | python3 -m json.tool` 验证

### ⚠️ PDA Agent vs 普通 Agent 的核心区别

这是最关键的概念——**必须正确区分**，否则无法形成层级结构：

| | PDA Agent（编排节点） | 普通 Agent（叶子节点） |
|---|---|---|
| **角色** | 调度员/协调者，不直接干活 | 执行者，掌握知识或操作工具 |
| **必须有** | `steps` 字段 + `max_recursion` 字段 | 无 `steps` 字段 |
| **用在哪层** | 顶层入口、中间层 | 最底层叶子节点 |
| **steps 中包含** | `prompt`(分析) → `route`(路由) → `prompt`(汇总) | — |
| **tools 字段** | 通常不设置（不直接用工具） | 设置具体工具如 read_file, shell 等 |

**判断规则**：如果一个 Agent 需要把任务交给其他 Agent 处理，它就**必须是 PDA Agent**（必须有 `steps`）。

```yaml
# ✅ 正确：PDA Agent（有 steps，能路由到子Agent）
入口Agent:
    description: 总调度
    system_prompt: 你是调度员...
    steps:                          # ← 这个字段使它成为 PDA Agent
        - type: prompt
          content: 分析问题
          label: 分析
        - type: route
          prompt: "选择分支：A, B"
          content: 请处理这个任务
          branches:
              A: 子AgentA
              B: 子AgentB
              _default: 子AgentA
          label: 路由
        - type: prompt
          content: 汇总结果
          label: 汇总
    max_recursion: 3                # ← PDA Agent 必须设置
    max_depth: 5

# ✅ 正确：普通 Agent（叶子节点，没有 steps）
子AgentA:
    description: 执行具体任务
    system_prompt: 你是专家...
    tools: [read_file, list_dir]
    max_depth: 2

# ❌ 错误：想做路由但没有 steps → 变成普通Agent，无法路由！
错误的入口:
    description: 总调度
    system_prompt: 你负责分析问题并分发给子Agent...
    max_depth: 5
    # 没有 steps！这只是个普通Agent，无法路由到其他Agent
```

## Mote Agent YAML 语法规范

### 基本结构

```yaml
agents:
    Agent名称:
        description: 一句话描述，用于delegate工具展示
        entry_point: true   # 入口Agent设为true，在@引用中优先展示
        stealth: false       # 隐身: true时不注入到系统提示词，节省token
        system_prompt: |
            系统提示词内容
        tags:
            - 标签
        # PDA编排Agent必须有 steps + max_recursion
        steps:
            - type: prompt
              content: "LLM执行的具体指令"
              label: 步骤标签
            - type: route
              prompt: "路由判断指令"
              content: "被选中的子Agent收到的任务指令"
              branches:
                  分支键1: 目标Agent名称
                  分支键2: 目标Agent名称
                  _default: 默认Agent名称
              label: 路由标签
            - type: agent_ref
              agent: 被引用的Agent名称
              content: "传递给子Agent的任务指令"
              label: 引用标签
        max_recursion: 10
        max_depth: 5
```

### 四种Step类型

| 类型 | 用途 | 关键字段 | 上下文行为 |
|------|------|----------|-----------|
| `prompt` | 让LLM执行具体指令（终结符） | `content`: 指令内容 | 在当前帧上下文中执行 |
| `route` | LLM路由判断后选择分支Agent | `prompt`: 路由提示词, `branches`: 分支映射, `content`: 子Agent任务指令 | **子Agent继承父帧完整上下文** |
| `agent_ref` | 直接引用并调用另一个Agent | `agent`: 目标Agent名称, `content`: 任务指令 | **子Agent获得全新上下文**（仅含content+上一步结果） |
| `exec` | 无Steps的Agent走完整循环 | `agent`: 目标Agent名称 | 同agent_ref |

> **⚠️ route vs agent_ref 的关键区别不仅是"动态vs静态"，更重要的是上下文传递：**
> - `agent_ref`：子Agent以**全新上下文**运行，只收到 `content` 字段文本 + 上一步结果（`[上一步结果参考]`），**完全看不到**父Agent之前的对话历史
> - `route`：子Agent**继承父帧的完整上下文**，能看到父Agent从第一步到当前步的所有对话、中间结果、验收意见等
>
> **实际影响**：在"工作→验收→重做"循环中，如果用 `agent_ref` 调用工作Agent，重做时工作Agent看不到验收反馈，只能从头重做。必须用 `route` 才能让工作Agent看到验收意见并做针对性修改。

### Route 分支规则
- `branches` 的 key 是呈现给LLM选择的选项文本
- `branches` 的 value 是实际目标Agent名称
- `_default` 是兜底分支，**必须设置**
- `_end` 是特殊值，表示结束当前路由不展开
- 自递归：value指向自身Agent名称时，StepIndex重置实现循环

### 叶子Agent（普通Agent）
```yaml
    叶子Agent名:
        description: 具体职责描述
        system_prompt: |
            掌握的知识内容、角色定义或执行指令
        tools:
            - read_file    # 读取本地文件
            - write_file   # 写入文件
            - edit_file    # 编辑文件
            - http         # 联网HTTP请求
            - shell        # 执行Shell命令（代码编译、脚本运行等）
            - list_dir     # 列出目录
        max_depth: 2
```

### 关键配置说明
- `entry_point`: 设为 `true` 的 Agent 会在 @ 引用中作为“入口”优先展示。**通常一个团队只需要1个入口**（顶层Agent）
- `stealth`: 设为 `true` 时，该 Agent 的信息不会注入到主 Agent 的系统提示词，节省 token。**非入口的新建 Agent 都应设为隐身**
- `max_depth`: 该Agent还能继续向下委派的层数（0=继承全局设定）
- `max_recursion`: PDA自递归的最大次数（route指向自身时的循环上限）
- `tools`: 叶子Agent可用的工具列表，可选 `read_file`, `write_file`, `edit_file`, `list_dir`, `shell`, `http`
- `tags`: 标签分组，便于管理

> **重要规则：如果引用现有的 Agent（即已存在的 Agent），不要改变它们的 `stealth` 和 `entry_point` 设置。只对新创建的 Agent 设置这些属性。**

## 适用场景与架构模式

不同场景下，层级 Agent 团队的架构侧重点不同。请根据用户的场景描述，选择最合适的模式或组合多种模式：

### 模式A：知识咨询体系
**适用**：税务咨询、医疗问诊、法律咨询、技术答疑等
- 顶层：问题分类 → 路由到领域
- 中间层：领域内问题分解
- 叶子层：持有知识或查询数据

### 模式B：调研与分析流水线
**适用**：竞品分析、市场调研、文献综述、尽职调查等
- 顶层：调研任务分解（维度拆分）→ 同时调度多个维度
- 中间层：单维度深入分析编排
- 叶子层：信息采集（联网搜索、读取报告文件等）

### 模式C：本地知识库
**适用**：项目文档问答、内部Wiki查询、Markdown笔记库、代码仓库理解等
- 顶层：理解问题意图 → 路由到知识分区
- 中间层：可选（大型知识库需要分区索引）
- 叶子层：挂载 `read_file` + `list_dir`，系统提示词声明目录结构和文件约定

### 模式D：工程协作体系
**适用**：全栈开发、微服务开发、DevOps流水线、测试体系等
- 顶层：任务分解（需求→设计→实现→测试→部署）
- 中间层：各阶段的子任务编排
- 叶子层：实际执行Agent（代码编写、测试运行、部署脚本）挂载 `shell` + `write_file` + `edit_file`

### 模式E：内容生产流水线
**适用**：文章写作、报告生成、翻译流程、课程制作等
- 顶层：内容规划（大纲/结构设计）→ 按章节分发
- 中间层：章节编排（素材收集→初稿→审校）
- 叶子层：写作Agent、审校Agent、配图Agent等

## 层级架构设计规则

### 1. 顶层入口Agent（根节点，**必须是PDA Agent — 必须有 `steps`**）

**职责**：
- 系统提示词中描述自己是该场景的「总调度员」或「总指挥」
- 第一步（prompt）：分析用户输入——识别任务类别、意图、所需资源
- 第二步（route）：根据分析结果路由到对应的中间层Agent
- 第三步（prompt）：汇总子Agent的成果，形成最终输出（报告/代码/方案等）
- 设置 `max_recursion` 用于支持迭代（route可包含"需要补充信息"的分支指回自身）

**必须包含的字段**：
```yaml
steps:           # ← 没有这个字段就不是PDA Agent！
    - type: prompt
      ...
    - type: route
      ...
    - type: prompt
      ...
max_recursion: 3 # ← PDA Agent 必须设置
```

**关键设计**：
- route 的 `prompt` 必须明确列出所有可选分支，要求LLM只输出分支键之一
- route 的 `content` 应包含通用任务指令，如"请基于上下文中的任务描述，以你的专业视角完成工作"
- 顶层不应持有具体知识或执行能力，它只理解任务的结构和分发逻辑

### 2. 中间层Agent（分支节点，**必须是PDA Agent — 必须有 `steps`**）

**职责**：
- 系统提示词中描述自己是某子领域的「协调者」或「编排者」
- 第一步（prompt）：将任务在子领域内进一步分解
- 第二步（route / agent_ref）：路由到具体的叶子Agent执行
- 第三步（prompt）：整合叶子Agent的结果，形成子领域成果
- 可以设置 `max_recursion` 实现子领域内的迭代深入

**必须包含的字段**：
```yaml
steps:           # ← 中间层也必须有 steps！
    - type: prompt
      ...
    - type: route   # 或 agent_ref
      ...
    - type: prompt
      ...
max_recursion: 3 # ← 必须设置
```

**关键设计**：
- 中间层的 route `content` 字段应精炼任务描述，避免传递过多上层上下文（Token优化）
- 中间层系统提示词应声明自己理解的分解维度
- 对于需要**迭代修改**的场景（工作→验收→重做循环），必须用 `route` 调用子Agent，确保验收反馈能通过上下文传递给工作Agent
- 对于**独立子任务**（最终汇总、不需要知道上下文的执行），可以用 `agent_ref`（子Agent获得干净上下文）
- `route` 步骤的 branches 可以所有分支指向同一个Agent（固定目标+继承上下文），这不是误用，而是确保上下文传递的正确做法

### 3. 底层Agent（叶子节点，**普通Agent — 没有 `steps`**）

**职责**：掌握最终知识、获取数据、或执行具体操作

**不需要 `steps` 字段**——叶子Agent接收任务后直接用自己的 system_prompt + tools 完成工作。

**能力来源设计**（根据 `{{knowledge_source}}` 参数）：
- **embedded（内嵌知识）**：将核心知识点直接写在 `system_prompt` 中，适合稳定且少量的知识
- **file（读取文件）**：`tools: [read_file, list_dir]`，系统提示词中指明文件路径和格式
- **http（联网查询）**：`tools: [http]`，系统提示词中指明API端点或搜索策略
- **shell（执行命令）**：`tools: [shell]`，系统提示词中指明可用命令和执行规范
- **mixed（混合方式）**：系统提示词嵌入基础知识 + 工具获取最新/详细数据或执行操作

**关键设计**：
- 叶子Agent的系统提示词**必须明确**自己的能力边界——能做什么、不能做什么
- 如果使用文件，提示词中给出路径约定（如 `.mote/knowledge/` 或项目目录结构）
- 如果使用HTTP，提示词中给出目标URL模式或API使用说明
- 如果使用Shell，提示词中给出命令规范和安全约束（禁止rm -rf等）
- 输出格式应结构化，方便上层Agent整合

## 健壮性设计要求

1. **每个 route 必须有 `_default` 分支**，指向一个通用兜底Agent或 `_end`
2. **总结/整合步骤必须在route之后**，确保子Agent结果被汇总
3. **叶子Agent在超出能力范围时应明确说明**，而非编造
4. **中间层应有max_recursion限制**（建议3-5），防止无限循环
5. **顶层的最终prompt步骤负责质量把关**——检测矛盾、补充caveat、统一输出格式
6. **对于有副作用的操作**（写文件、执行命令），叶子Agent的提示词应包含安全约束

## 完整示例参考

### 示例1：知识咨询体系（税务）

```yaml
agents:
    税务_入口:
        description: 企业税务咨询总入口，分析问题类别并分发
        entry_point: true
        system_prompt: |
            你是企业税务咨询的总调度员。你不直接回答税务细节问题，
            你的职责是理解用户问题属于哪个税务子领域，然后交给对应的专业团队处理。
            你最终负责汇总各团队的分析结果，给出完整、连贯的咨询报告。
        tags:
            - 入口
        max_depth: 5
        steps:
            - type: prompt
              content: |
                  分析用户的问题：
                  1. 识别问题涉及的税种（增值税/企业所得税/个人所得税/其他）
                  2. 判断问题的性质（申报/筹划/争议/政策咨询）
                  3. 用一句话概括核心问题
              label: 问题分析
            - type: route
              prompt: |
                  根据你对用户问题的分析，选择最合适的处理团队。
                  只输出以下选项之一：增值税, 所得税, 综合税务
              content: |
                  你收到了一个税务咨询问题，请基于上下文中的用户问题和前序分析，
                  从你的专业角度进行深入分析。
              branches:
                  增值税: 税务_增值税分析
                  所得税: 税务_所得税分析
                  综合税务: 税务_综合分析
                  _default: 税务_综合分析
              label: 领域路由
            - type: prompt
              content: |
                  现在请汇总上面专业团队的分析结果，输出最终咨询报告：
                  - 问题概述
                  - 专业分析
                  - 建议方案
                  - 风险提示
              label: 汇总报告
        max_recursion: 3

    税务_增值税分析:
        description: 增值税领域的问题分解和路由
        stealth: true
        system_prompt: |
            你是增值税领域的分析协调者。你理解增值税的分类体系，
            能将增值税问题分解为：税率适用、进项抵扣、申报实务、优惠政策等子问题。
        tags:
            - 中间层
        max_depth: 4
        steps:
            - type: prompt
              content: 分析当前增值税问题的具体方面。
              label: 问题分解
            - type: route
              prompt: "选择需要查询的知识领域：税率, 抵扣, 优惠"
              content: 请回答上下文中关于增值税的具体问题。
              branches:
                  税率: 税务_增值税税率
                  抵扣: 税务_进项抵扣
                  优惠: 税务_增值税优惠
                  _default: 税务_增值税税率
              label: 细分路由
            - type: prompt
              content: 基于专项知识Agent的回答，形成增值税分析报告。
              label: 汇总
        max_recursion: 3

    税务_增值税税率:
        description: 掌握增值税税率适用规则
        stealth: true
        system_prompt: |
            你是增值税税率专家，掌握以下知识：
            - 13%税率：销售货物、加工修理修配劳务、有形动产租赁
            - 9%税率：交通运输、邮政、基础电信、建筑、不动产租赁等
            - 6%税率：金融、现代服务、生活服务、增值电信等
            超出范围请明确说明。
        tags: [叶子节点, 知识内嵌]
        max_depth: 2

    税务_进项抵扣:
        description: 掌握增值税进项抵扣规则
        stealth: true
        system_prompt: |
            你是增值税进项抵扣专家，负责查阅本地知识文件回答问题。
            知识文件位于 .mote/knowledge/tax/ 目录下。
        tools: [read_file, list_dir]
        tags: [叶子节点, 文件知识]
        max_depth: 2

    税务_增值税优惠:
        description: 查询最新增值税优惠政策
        stealth: true
        system_prompt: |
            你是增值税优惠政策专家。由于政策经常更新，请使用HTTP工具查询最新动态。
        tools: [http]
        tags: [叶子节点, 联网查询]
        max_depth: 2

    # ...所得税中间层和叶子节点结构同上，此处省略...

    税务_所得税分析:
        description: 所得税领域分析协调
        stealth: true
        system_prompt: |
            你是所得税领域的分析协调者。
        tags: [中间层]
        max_depth: 4
        steps:
            - type: prompt
              content: 分析所得税问题的具体方面。
              label: 问题分解
            - type: route
              prompt: "选择子领域：企业所得税, 个人所得税"
              content: 请回答上下文中的所得税问题。
              branches:
                  企业所得税: 税务_企业所得税
                  个人所得税: 税务_个人所得税
                  _default: 税务_企业所得税
              label: 路由
            - type: prompt
              content: 形成所得税分析报告。
              label: 汇总
        max_recursion: 3

    税务_企业所得税:
        description: 企业所得税知识
        stealth: true
        system_prompt: 你是企业所得税专家...
        tags: [叶子节点]
        max_depth: 2

    税务_个人所得税:
        description: 个人所得税知识
        stealth: true
        system_prompt: 你是个人所得税专家...
        tags: [叶子节点]
        max_depth: 2

    税务_综合分析:
        description: 兜底综合税务分析        stealth: true        system_prompt: 处理无法明确归类的复杂问题。
        tools: [http, read_file]
        tags: [叶子节点, 兜底]
        max_depth: 2
```

### 示例2：本地知识库问答（Markdown笔记）

```yaml
agents:
    笔记_入口:
        description: Markdown笔记库问答入口
        entry_point: true
        system_prompt: |
            你是本地Markdown笔记库的问答助手。笔记分为以下目录：
            - tech/: 技术笔记
            - reading/: 读书笔记
            - work/: 工作日志
            你负责判断用户问题属于哪个分区，并交给对应Agent去查找。
        tags: [入口]
        max_depth: 4
        steps:
            - type: prompt
              content: 分析用户的问题，判断它最可能存在于哪个笔记分区。
              label: 意图识别
            - type: route
              prompt: "选择笔记分区：技术, 读书, 工作, 全局搜索"
              content: 在你负责的笔记目录中查找与问题相关的内容并回答。
              branches:
                  技术: 笔记_技术区
                  读书: 笔记_读书区
                  工作: 笔记_工作区
                  全局搜索: 笔记_全局搜索
                  _default: 笔记_全局搜索
              label: 分区路由
            - type: prompt
              content: 基于查找结果，整理成清晰的回答。标注信息来源的文件名。
              label: 整合回答
        max_recursion: 2

    笔记_技术区:
        description: 搜索tech/目录下的技术笔记
        stealth: true
        system_prompt: |
            你负责在 notes/tech/ 目录中查找信息。
            先用 list_dir 了解文件列表，再用 read_file 读取相关文件。
            回答时引用文件名作为来源。找不到就明确说明。
        tools: [read_file, list_dir]
        tags: [叶子节点]
        max_depth: 2

    笔记_读书区:
        description: 搜索reading/目录下的读书笔记
        stealth: true
        system_prompt: |
            你负责在 notes/reading/ 目录中查找信息。操作方式同上。
        tools: [read_file, list_dir]
        tags: [叶子节点]
        max_depth: 2

    笔记_工作区:
        description: 搜索work/目录下的工作日志
        stealth: true
        system_prompt: |
            你负责在 notes/work/ 目录中查找信息。操作方式同上。
        tools: [read_file, list_dir]
        tags: [叶子节点]
        max_depth: 2

    笔记_全局搜索:
        description: 跨目录搜索笔记
        stealth: true
        system_prompt: |
            你负责在整个 notes/ 目录树中搜索信息。
            使用 shell 的 grep 命令高效全文搜索，再用 read_file 读取匹配文件。
        tools: [shell, read_file, list_dir]
        tags: [叶子节点, 兜底]
        max_depth: 2
```

### 示例3：工程协作体系（全栈开发）

```yaml
agents:
    工程_入口:
        description: 全栈开发任务调度入口
        entry_point: true
        system_prompt: |
            你是全栈开发项目的技术总监。你负责将开发需求拆解为
            前端、后端、数据库、测试等子任务，并协调各团队完成。
            你不写代码，只做任务分解和结果整合。
        tags: [入口]
        max_depth: 5
        steps:
            - type: prompt
              content: |
                  分析开发需求：
                  1. 涉及哪些技术栈（前端/后端/数据库/DevOps）
                  2. 任务之间的依赖关系
                  3. 建议的执行顺序
              label: 需求分析
            - type: route
              prompt: "选择首要任务：后端, 前端, 数据库, 测试"
              content: 请根据上下文中的需求描述和技术分析，完成你负责的部分。
              branches:
                  后端: 工程_后端编排
                  前端: 工程_前端编排
                  数据库: 工程_数据库
                  测试: 工程_测试
                  _default: 工程_后端编排
              label: 任务路由
            - type: prompt
              content: 整合各团队的交付成果，输出完整的实施报告和待办事项。
              label: 成果整合
        max_recursion: 5

    工程_后端编排:
        description: 后端开发子任务编排
        stealth: true
        system_prompt: |
            你是后端技术负责人，负责将后端任务拆解为：
            API设计、业务逻辑实现、数据模型定义等子任务。
            协调代码编写和审查Agent，根据审查反馈迭代修改。
        tags: [中间层]
        max_depth: 4
        steps:
            - type: prompt
              content: 将后端任务拆解为具体的实现步骤。
              label: 后端任务分解
            - type: route
              prompt: |
                现在需要编写代码。输出一个词：编写
              content: |
                根据任务分解结果，编写后端代码。
                如果上下文中有代码审查反馈，请针对反馈问题重点修改，而非从头重写。
              branches:
                编写: 工程_代码编写
                _default: 工程_代码编写
              label: 编写代码
            - type: route
              prompt: |
                现在需要审查代码。输出一个词：审查
              content: 审查上一步生成的代码，指出所有问题。
              branches:
                审查: 工程_代码审查
                _default: 工程_代码审查
              label: 代码审查
            - type: route
              prompt: |
                根据代码审查结果判断：
                如果审查通过没有问题，回复：通过
                如果有需要修改的问题，回复：重做
                只输出一个词
              branches:
                通过: _end
                重做: 工程_后端编排
                _default: _end
              label: 审查路由
            - type: prompt
              content: 基于编写和审查结果，总结后端交付成果。
              label: 后端总结
        max_recursion: 3

    工程_代码编写:
        description: 编写代码的执行Agent
        stealth: true
        system_prompt: |
            你是一位经验丰富的后端开发者。你负责根据指令编写代码。
            使用 write_file 创建新文件，使用 edit_file 修改现有文件。
            代码应遵循项目现有的风格和规范。
        tools: [read_file, write_file, edit_file, list_dir, shell]
        tags: [叶子节点, 执行]
        max_depth: 2

    工程_代码审查:
        description: 代码审查Agent
        stealth: true
        system_prompt: |
            你是一位严格的代码审查者。查看代码文件，指出：
            - 逻辑错误和边界条件
            - 安全隐患
            - 性能问题
            - 风格不一致
            只指出问题，不修改文件。
        tools: [read_file, list_dir]
        tags: [叶子节点]
        max_depth: 2

    工程_前端编排:
        description: 前端开发子任务编排
        stealth: true
        system_prompt: 你是前端技术负责人...
        tags: [中间层]
        max_depth: 4
        steps:
            - type: prompt
              content: 将前端任务拆解为组件设计和实现步骤。
              label: 前端任务分解
            - type: agent_ref
              agent: 工程_代码编写
              content: 根据任务分解编写前端代码。
              label: 编写前端
            - type: prompt
              content: 总结前端交付成果。
              label: 前端总结
        max_recursion: 2

    工程_数据库:
        description: 数据库设计与迁移
        stealth: true
        system_prompt: |
            你负责数据库Schema设计和SQL编写。
            使用 write_file 生成迁移文件，使用 shell 执行迁移测试。
            安全约束：禁止 DROP DATABASE，禁止在非测试库执行破坏性操作。
        tools: [write_file, read_file, shell]
        tags: [叶子节点, 执行]
        max_depth: 2

    工程_测试:
        description: 编写和运行测试
        stealth: true
        system_prompt: |
            你负责为项目编写和运行测试。
            使用 write_file 创建测试文件，使用 shell 运行测试命令。
            报告测试覆盖率和失败用例。
        tools: [write_file, read_file, shell, list_dir]
        tags: [叶子节点, 执行]
        max_depth: 2
```

## 现在请执行

请为「{{scenario}}」场景构建一个完整的层级Agent团队。**你必须实际创建文件，不能只输出YAML文本。**

### Step 1：设计

1. 选择适用的架构模式（参考上方A-E模式或自由组合），简要说明设计思路
2. 画出树状结构（用缩进文本表示），在每个节点后标注 `[PDA]` 或 `[普通]`：
   ```
   入口Agent [PDA] ← 必须有 steps + route
   ├── 子领域A [PDA] ← 必须有 steps + route/agent_ref
   │   ├── 叶子1 [普通] ← 无 steps，有 tools
   │   └── 叶子2 [普通]
   └── 子领域B [PDA]
       ├── 叶子3 [普通]
       └── 叶子4 [普通]
   ```

### Step 2：自查清单（写文件前逐项确认）

- [ ] 标注为 `[PDA]` 的每个Agent都有 `steps` 字段和 `max_recursion` 字段
- [ ] 标注为 `[普通]` 的Agent没有 `steps` 字段
- [ ] 每个 `route` 步骤的 `branches` 中所有 value（Agent名称）都在YAML中定义了
- [ ] 每个 `agent_ref` 步骤的 `agent` 值都在YAML中定义了
- [ ] 每个 `route` 都有 `_default` 分支
- [ ] 叶子Agent的 system_prompt 包含实质性内容（知识/工具使用说明）
- [ ] 有副作用的工具（shell, write_file, edit_file）仅赋予叶子Agent
- [ ] 命名使用统一前缀
- [ ] **仅顶层入口Agent设置 `entry_point: true`**（通常只需1个入口）
- [ ] **所有非入口的新建Agent设置 `stealth: true`**（不污染主Agent系统提示词）
- [ ] **引用的现有Agent不修改其 `stealth` / `entry_point` 设置**
- [ ] **工作→验收→重做循环使用 `route`（非 `agent_ref`）** 确保验收反馈能通过上下文传递
- [ ] 仅对不需要父帧上下文的独立子任务使用 `agent_ref`（如最终汇总）

### Step 3：创建文件并生效

按以下顺序执行操作：

```
# 1. 确保目录存在
shell(command="mkdir -p ~/.mote/agents")

# 2. 写入YAML文件（一个文件包含整个团队）
write_file(path="~/.mote/agents/<场景名>.yaml", content="agents:\n  ...")

# 3. 验证语法
shell(command="curl -s http://localhost:18788/api/v1/agents/validate-dir | python3 -m json.tool")

# 4. 热加载使其立即可用
shell(command="curl -s -X POST http://localhost:18788/api/v1/agents/reload")
```

### 约束条件

- 知识来源方式遵循用户指定的 `{{knowledge_source}}` 策略（默认mixed）
- 层级深度约为 {{depth}} 层（默认3层）
- 叶子节点数量在 {{leaf_count}} 个左右（默认根据场景合理规划）
