# Mote 多模态支持方案修正 - 基于本地 Agent 特性

**原方案问题**: 按照 Web Chat 的思路设计了文件上传功能，忽略了 Mote 是本地 Agent 的本质  
**修正日期**: 2026-02-10  
**核心观点**: Mote 无需上传文件，应充分利用本地文件系统访问能力

---

## 🎯 关键洞察

### ❌ 错误假设（原方案）

将 Mote 等同于 **Web Chat 应用**（ChatGPT、Claude Web 等）：

```
用户: [上传图片] 分析这个 UI
→ 前端读取文件 → base64 编码 → 发送到后端 → 发送给 Provider
```

**问题**：
- ❌ 文件已经在本地，为什么要"上传"？
- ❌ base64 编码浪费 CPU 和内存
- ❌ 数据冗余（本地文件 + 消息中的 base64）
- ❌ 不符合 Mote 的定位

---

### ✅ 正确理解（修正后）

Mote 是**本地运行的 Agent**，拥有文件系统访问权限：

```
用户: 看看这个图片 ./screenshot.png 有什么问题？
→ 后端读取本地文件 → 编码 → 发送给 Provider（如需要）
或
→ AI 理解文件路径 → 调用 read_file 工具 → 返回内容
```

**优势**：
- ✅ 无需前端文件选择器
- ✅ 无需前端读取和编码
- ✅ 直接利用现有的 `read_file` 工具
- ✅ 符合本地 Agent 的定位

---

## 📋 Mote 现有能力

### 已有的文件访问工具

Mote 已经内置了强大的文件系统工具：

```go
// internal/tools/builtin/
1. read_file   - 读取文件内容（支持行范围）
2. write_file  - 写入文件
3. edit_file   - 编辑文件（搜索替换）
4. list_dir    - 列出目录内容
5. shell       - 执行 Shell 命令
6. http        - HTTP 请求
```

**read_file 工具特性**：
- ✅ 支持任意路径
- ✅ 支持行范围读取（大文件优化）
- ✅ 最大 10MB 限制
- ✅ 自动截断过大内容

---

## 🔍 Provider 支持调查

### 1. Copilot CLI (@文件引用)

根据最新调研，**Copilot CLI 原生支持文件引用**：

#### 交互模式中的 `@` 语法

```bash
$ copilot
> @screenshot.png 这个 UI 设计有什么问题？
> @src/index.js 帮我优化这段代码
```

- ✅ **支持 `@filename` 引用本地文件**
- ✅ **支持图片、代码、文档等多种文件**
- ✅ **自动处理文件权限**（`/allow-all` 或逐文件审批）

#### ACP 协议中的 image 资源类型

```json
{
  "type": "image",
  "path": "/absolute/path/to/screenshot.png"
}
```

**关键发现**：
- ✅ **ACP 支持 `type: "image"` 和 `path` 字段**
- ✅ **无需 base64 编码，直接传文件路径**
- ✅ **CLI 内部处理文件读取和编码**

---

### 2. Copilot ACP 协议实现

查看 Mote 代码中的 ACP 类型定义：

```go
// internal/provider/copilot/acp_types.go:164-170
type PromptContent struct {
    Type string `json:"type"` // "text", "image", etc.
    Text string `json:"text,omitempty"`
    // For image content:
    // MimeType string `json:"mimeType,omitempty"`
    // Data     string `json:"data,omitempty"`
}
```

**现状**：
- ⚠️ `MimeType` 和 `Data` 字段被注释掉
- ⚠️ 没有 `Path` 字段
- ⚠️ 实现不完整

**修正方案**：

```go
type PromptContent struct {
    Type     string `json:"type"`     // "text", "image"
    Text     string `json:"text,omitempty"`
    
    // Option A: 传文件路径（推荐）
    Path     string `json:"path,omitempty"`      // 本地文件路径
    
    // Option B: 传 base64 数据（兼容）
    MimeType string `json:"mimeType,omitempty"`  // "image/png"
    Data     string `json:"data,omitempty"`      // base64 编码
}
```

**推荐使用 Option A**：
- ✅ 符合 CLI 的设计理念
- ✅ 性能更好（CLI 内部优化）
- ✅ 无需 Mote 手动编码

---

### 3. Ollama (本地模型)

Ollama Vision API 要求 **base64 编码的图片数据**：

```json
{
  "model": "llava:7b",
  "messages": [{
    "role": "user",
    "content": "Describe this image",
    "images": ["base64EncodedData"]  // 必须是 base64
  }]
}
```

**对于 Ollama，Mote 需要**：
- 读取本地文件 → base64 编码 → 发送给 Ollama
- 但这个逻辑在**后端完成**，不需要前端参与

---

## 🎨 用户交互设计（修正后）

### 方案 A: 纯文本路径引用（最简单）

```
用户输入:
┌────────────────────────────────────────┐
│ 看看这个图片 ./screenshot.png 有什么问题│
│                                         │
│                                 [发送 ▶]│
└────────────────────────────────────────┘
```

**实现**：
- 用户直接在输入框中输入文件路径
- Mote 后端检测路径 → 读取文件 → 根据 Provider 处理
- **零前端改动**

**优势**：
- ✅ 实现最简单
- ✅ 符合命令行工具风格
- ✅ 无需 UI 组件

**劣势**：
- ❌ 用户体验不够友好（需要手动输入路径）
- ❌ 无法预览图片

---

### 方案 B: 文件选择器 + 路径引用（推荐）

```
输入区域:
┌────────────────────────────────────────┐
│ 📎 选择文件                              │
│  → ./screenshot.png (已添加)       [x]  │
├────────────────────────────────────────┤
│ 这个 UI 设计有什么问题？                │
│                                         │
│ [📁 浏览文件]                    [发送 ▶]│
└────────────────────────────────────────┘
```

**实现**：
1. 用户点击"浏览文件"按钮
2. 前端文件选择器（`<input type="file">`）
3. **仅读取文件路径**（不读取内容）
4. 将路径添加到消息中：`[File: ./screenshot.png] 用户消息`
5. 后端解析文件路径 → 读取并处理

**优势**：
- ✅ 用户友好（点击选择文件）
- ✅ 前端只处理路径，无需读取文件内容
- ✅ 可以添加文件预览（缩略图）

**前端逻辑**：

```typescript
// 文件选择处理
const handleFileSelect = (file: File) => {
  // 关键：只获取文件路径，不读取内容
  const filePath = file.path || file.name; // Electron/Wails 提供 path
  
  // 添加到附件列表
  setAttachments([...attachments, {
    path: filePath,
    name: file.name,
    type: file.type,
    size: file.size
  }]);
};

// 发送消息时
const handleSend = () => {
  const messageText = `${attachments.map(a => `[File: ${a.path}]`).join(' ')} ${inputValue}`;
  
  // 发送到后端，attachments 中只有路径信息
  api.chat({
    session_id: sessionId,
    message: messageText,
    attachments: attachments.map(a => ({ path: a.path, type: a.type }))
  });
};
```

---

### 方案 C: 工作区文件浏览器（最佳）

利用 Mote 已有的 **Workspace 功能**：

```
Chat 页面:
┌────────────────┬───────────────────────┐
│ Workspace      │ 对话区域               │
│ ├─ src/        │ User: 分析这个文件     │
│ │  ├─ app.js   │ AI: 正在读取...       │
│ │  └─ style.css│                       │
│ ├─ docs/       │                       │
│ │  └─ api.md   │                       │
│ └─ assets/     │                       │
│    ├─ logo.png │ [输入框]              │
│    └─ ui.png   │ [📁 从工作区选择]      │
└────────────────┴───────────────────────┘
```

**交互流程**：
1. 用户点击"从工作区选择"
2. 打开侧边栏显示文件树
3. 用户点击文件 → 插入路径到输入框
4. 发送时，后端读取文件

**优势**：
- ✅ 最符合 Mote 的定位（本地 Agent）
- ✅ 与现有 Workspace 功能集成
- ✅ 可视化文件浏览

---

## 🏗️ 后端实现方案

### 核心逻辑：路径检测 + 文件读取

```go
// api/v1/types.go - API 层
type ChatRequest struct {
    SessionID   string            `json:"session_id,omitempty"`
    Message     string            `json:"message"`  // 可能包含 [File: path]
    Attachments []AttachmentRef   `json:"attachments,omitempty"` // 文件路径引用
    Model       string            `json:"model,omitempty"`
}

type AttachmentRef struct {
    Path     string `json:"path"`      // 本地文件路径
    Type     string `json:"type"`      // "image", "text"
}

// internal/runner/runner.go - Runner 层
func (r *Runner) processAttachments(attachments []AttachmentRef) ([]provider.Attachment, error) {
    var processed []provider.Attachment
    
    for _, ref := range attachments {
        // 读取本地文件
        data, err := os.ReadFile(ref.Path)
        if err != nil {
            return nil, fmt.Errorf("failed to read %s: %w", ref.Path, err)
        }
        
        // 根据文件类型处理
        if strings.HasPrefix(ref.Type, "image/") {
            // 图片 → base64 编码
            encoded := base64.StdEncoding.EncodeToString(data)
            processed = append(processed, provider.Attachment{
                Type:     "image",
                MimeType: ref.Type,
                Data:     encoded,
                Filename: filepath.Base(ref.Path),
            })
        } else {
            // 文本文件 → 直接读取
            processed = append(processed, provider.Attachment{
                Type:     "file",
                MimeType: ref.Type,
                Data:     string(data),
                Filename: filepath.Base(ref.Path),
            })
        }
    }
    
    return processed, nil
}

// internal/provider/copilot/acp_provider.go - ACP Provider
func (p *ACPProvider) buildPromptContent(req provider.ChatRequest) []PromptContent {
    contents := []PromptContent{
        {Type: "text", Text: req.Messages[len(req.Messages)-1].Content},
    }
    
    // 添加附件（使用文件路径，让 CLI 处理）
    for _, att := range req.Attachments {
        if att.Type == "image" {
            contents = append(contents, PromptContent{
                Type: "image",
                Path: att.Filepath, // 传递文件路径，不是 base64
            })
        }
    }
    
    return contents
}
```

---

## 📊 方案对比

| 方案 | 用户体验 | 前端复杂度 | 后端复杂度 | 性能 | 推荐度 |
|-----|---------|----------|----------|-----|--------|
| **方案 A: 纯文本路径** | ⭐⭐ | 🟢 无需改动 | 🟢 路径解析 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| **方案 B: 文件选择器** | ⭐⭐⭐⭐ | 🟡 文件选择器 | 🟡 路径 + 读取 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| **方案 C: 工作区集成** | ⭐⭐⭐⭐⭐ | 🟡 集成现有 UI | 🟢 利用现有 API | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

---

## ✅ 最终推荐方案

### 短期 MVP（1-2 天）：方案 A

**实现**：
1. 用户在输入框中输入：`看看这个图片 ./screenshot.png`
2. 后端正则匹配文件路径
3. 读取文件 → 根据 Provider 处理（Copilot 传路径，Ollama 传 base64）

**优势**：
- ✅ 最快实现
- ✅ 零前端改动
- ✅ 验证可行性

---

### 长期完善（3-5 天）：方案 C

**实现**：
1. 在 Chat 页面添加"从工作区选择"按钮
2. 点击后显示文件树（复用 Workspace API）
3. 用户点击文件 → 插入路径到输入框
4. 可选：显示文件缩略图预览

**优势**：
- ✅ 最佳用户体验
- ✅ 与 Mote 定位一致
- ✅ 复用现有功能

---

## 🔧 需要修改的代码

### 1. ACP 协议类型定义

```go
// internal/provider/copilot/acp_types.go
type PromptContent struct {
    Type     string `json:"type"`     // "text", "image"
    Text     string `json:"text,omitempty"`
    Path     string `json:"path,omitempty"`      // 新增：文件路径
    MimeType string `json:"mimeType,omitempty"`  // 可选：MIME 类型
    Data     string `json:"data,omitempty"`      // 可选：base64 数据
}
```

### 2. Provider 消息结构

```go
// internal/provider/types.go
type Message struct {
    Role        string       `json:"role"`
    Content     string       `json:"content"`
    Attachments []Attachment `json:"attachments,omitempty"` // 新增
    ToolCalls   []ToolCall   `json:"tool_calls,omitempty"`
    ToolCallID  string       `json:"tool_call_id,omitempty"`
}

type Attachment struct {
    Type     string `json:"type"`      // "image", "file"
    Filepath string `json:"filepath"`  // 本地文件路径
    MimeType string `json:"mime_type"`
    Data     string `json:"data,omitempty"` // 可选：已读取的数据
    Filename string `json:"filename,omitempty"`
    Size     int    `json:"size,omitempty"`
}
```

### 3. Runner 处理逻辑

```go
// internal/runner/runner.go
// 在发送给 Provider 前，读取附件文件并编码
func (r *Runner) prepareAttachments(attachments []Attachment) error {
    for i := range attachments {
        if attachments[i].Filepath != "" && attachments[i].Data == "" {
            // 读取文件
            data, err := os.ReadFile(attachments[i].Filepath)
            if err != nil {
                return err
            }
            // 图片编码为 base64
            if attachments[i].Type == "image" {
                attachments[i].Data = base64.StdEncoding.EncodeToString(data)
            } else {
                attachments[i].Data = string(data)
            }
        }
    }
    return nil
}
```

---

## ⚡ 实施路线图

### Phase 1: ACP 路径引用支持（1 天）

```
[ ] 1. 启用 PromptContent 的 Path 字段
[ ] 2. 在 ACPProvider 中构造 image 类型的 PromptContent
[ ] 3. 测试：copilot CLI 是否正确处理文件路径
```

### Phase 2: 后端文件读取逻辑（1 天）

```
[ ] 1. 扩展 provider.Message 添加 Attachments
[ ] 2. Runner 中添加文件读取 + 编码逻辑
[ ] 3. Ollama Provider 适配（base64）
[ ] 4. 单元测试
```

### Phase 3: 前端路径输入支持（1 天）

```
[ ] 1. API 层添加 AttachmentRef 支持
[ ] 2. 前端检测消息中的文件路径（可选）
[ ] 3. 简单的路径验证
```

### Phase 4: 工作区集成（2-3 天，可选）

```
[ ] 1. Chat 页面添加"从工作区选择"按钮
[ ] 2. 文件树侧边栏 UI
[ ] 3. 文件选择 → 插入路径
[ ] 4. 图片预览缩略图
```

---

## 📝 总结

### 核心观点确认

✅ **你的观点完全正确**：

1. **Mote 无需"上传"文件** - 因为是本地 Agent，文件已经在本地
2. **只需传递文件路径** - Copilot CLI 支持 `type: "image", path: "..."` 格式
3. **后端读取并编码** - 前端无需处理文件内容
4. **充分利用现有能力** - Workspace、read_file 工具等

### 关键差异

| 传统 Web Chat | Mote 本地 Agent |
|--------------|---------------|
| 必须上传文件到服务器 | 文件已在本地 |
| 前端读取 + base64 编码 | 后端读取或传路径 |
| 数据存储在云端 | 数据保留在本地 |
| 无法访问本地文件系统 | 完整文件系统权限 |

### 下一步

1. ✅ **验证 ACP 协议**: 实验测试 `type: "image", path: "..."` 是否有效
2. ⬜ **选择实施方案**: 短期用方案 A，长期用方案 C
3. ⬜ **修改代码**: 主要在 ACP 类型定义和 Provider 适配层
4. ⬜ **测试验证**: 本地文件 → Copilot CLI → 正确响应

**估计工作量**: 2-4 天（核心功能），比原方案减少 60% 的工作量！
