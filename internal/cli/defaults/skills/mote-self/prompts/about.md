# About Mote

Mote 是一个轻量级、自进化的 AI Agent 运行时，基于 Go 语言构建。

## 核心能力

### 多模型支持 (GitHub Copilot)
通过 GitHub Copilot API 支持多个 LLM 厂商的模型：

**免费模型 (包含在订阅中)**:
- OpenAI: gpt-4.1, gpt-4.1-mini, gpt-5-mini, o3-mini
- Anthropic: claude-haiku-3.5
- Google: gemini-2.0-flash
- xAI: grok-code-fast-1

**付费模型 (需要 Premium 请求配额)**:
- OpenAI: gpt-5 (5x), o1 (10x), o1-pro (50x), o3 (25x)
- Anthropic: claude-sonnet-4 (1x), claude-opus-4 (10x)
- Google: gemini-2.5-pro (5x), gemini-3-flash (1x)
- xAI: grok-3 (5x)

### Agent 模式
- **ask**: 简单问答模式 - 直接回答问题
- **edit**: 代码编辑模式 - 专注于代码修改
- **agent**: 自主 Agent 模式（默认）- 多步骤任务执行
- **plan**: 规划模式 - 生成执行计划

### 内置工具
- `shell`: 执行系统命令
- `read_file`: 读取文件内容
- `write_file`: 写入文件
- `list_dir`: 列出目录内容
- `http`: 发送 HTTP 请求

### 扩展系统
- **Skills**: 通过技能包扩展 Agent 能力
- **JS Tools**: 使用 JavaScript 编写自定义工具
- **MCP**: 支持 Model Context Protocol 协议

### 记忆系统
- 基于 sqlite-vec 的向量搜索
- 对话历史自动索引
- 语义相似度检索

### 定时任务
- 标准 Cron 表达式
- 支持 prompt、tool、script 类型任务

## 配置管理

### 配置文件位置
`~/.mote/config.yaml`

### 常用配置项
| 配置键 | 说明 | 默认值 |
|--------|------|--------|
| `copilot.model` | 当前使用的模型 | gpt-4.1 |
| `copilot.mode` | Agent 运行模式 | agent |
| `gateway.port` | 服务端口 | 18788 |
| `memory.enabled` | 启用记忆系统 | true |
| `cron.enabled` | 启用定时任务 | true |

### CLI 命令
```bash
mote config list              # 查看所有配置
mote config get <key>         # 获取配置值
mote config set <key> <value> # 设置配置值
mote config models            # 查看支持的模型
mote config models --free     # 只看免费模型
mote mode get                 # 查看当前模式
mote mode set <mode>          # 切换模式
```

## 目录结构

```
~/.mote/
├── config.yaml     # 配置文件
├── data.db         # SQLite 数据库
├── skills/         # 技能目录
│   └── mote-self/  # 自管理技能
├── tools/          # JS 工具目录
└── ui/             # 自定义 UI
```

## 自管理工具

此 Skill 提供以下工具用于管理 Mote 自身：

| 工具 | 功能 |
|------|------|
| `mote_get_config` | 获取当前所有配置 |
| `mote_set_config` | 修改配置（限白名单） |
| `mote_list_models` | 列出所有支持的模型 |
| `mote_switch_model` | 切换当前使用的模型 |
| `mote_get_version` | 获取版本信息 |
| `mote_workspace_get` | 获取当前会话绑定的工作区目录路径和名称 |
| `mote_workspace_list` | 列出所有绑定的工作区 |
| `mote_workspace_bind` | 绑定工作区目录到当前会话 |
| `mote_workspace_unbind` | 解除工作区绑定 |
| `mote_workspace_files` | 列出工作区中的文件 |

## 工作区使用说明

**重要**: 当用户询问"工作区"、"工作目录"、"项目目录"时，应使用 `mote_workspace_get` 工具获取绑定的工作区路径，而不是使用 shell 命令执行 pwd。

工作区是用户绑定到会话的项目目录，与 shell 的当前工作目录（通常是 /）不同。

## 常见问题

**Q: 如何切换模型？**
A: 使用 `mote_switch_model` 工具或运行 `mote config set copilot.model <model-id>`

**Q: 哪些模型是免费的？**
A: gpt-4.1, gpt-4.1-mini, gpt-5-mini, o3-mini, claude-haiku-3.5, gemini-2.0-flash, grok-code-fast-1

**Q: 如何查看当前模型？**
A: 使用 `mote_get_config` 或运行 `mote config get copilot.model`

**Q: 如何查看当前工作区？**
A: 使用 `mote_workspace_get` 工具获取当前会话绑定的工作区目录

## 自定义 Skill 开发指南

### Skill 目录结构
```
my-skill/
├── manifest.json    # 必须：技能元数据和工具定义
├── SKILL.md         # 可选：技能文档/说明
├── handler.js       # JS 工具处理脚本
├── tools/           # 可把 JS 文件放在子目录
└── hooks/           # 钩子脚本
```

### manifest.json 结构
```json
{
  "id": "my-skill",
  "name": "My Custom Skill",
  "version": "1.0.0",
  "description": "描述技能功能",
  "author": "作者",
  "config": {
    "base_url": "https://api.example.com",
    "api_token": "your-token-here",
    "timeout_ms": 5000,
    "verify_ssl": true
  },
  "tools": [
    {
      "name": "my_tool",
      "description": "工具描述",
      "handler": "handler.js#myFunction",
      "parameters": {
        "type": "object",
        "properties": {
          "param1": { "type": "string", "description": "参数说明" }
        },
        "required": ["param1"]
      }
    }
  ],
  "prompts": [
    { "name": "usage_guide", "file": "prompts/usage.md" }
  ]
}
```

### JS 脚本编写规范

**关键规则**：
1. **使用 `var` 而非 `let/const`** — goja VM 池复用时 `let/const` 会重复声明报错
2. **同步调用** — `mote.http` 等所有 Host API 是同步的，**不要使用 async/await**
3. **读取 skill 配置** — manifest.json 中的 `config` 字段会自动注入为全局变量 `SKILL_CONFIG`

**读取配置示例**（推荐写法）：
```javascript
// SKILL_CONFIG 由 Mote 运行时自动注入，来自 manifest.json 的 config 字段
// 不需要手动读取 manifest.json 文件

var BASE_URL = SKILL_CONFIG.base_url || 'https://default.example.com';
var API_TOKEN = SKILL_CONFIG.api_token || '';
var VERIFY_SSL = SKILL_CONFIG.verify_ssl !== false;

function queryAPI(args) {
  try {
    var opts = {
      headers: { 'Authorization': 'Bearer ' + API_TOKEN },
      insecure: !VERIFY_SSL   // 跳过自签名证书验证
    };
    var response = mote.http.get(BASE_URL + '/api/data?q=' + args.query, opts);
    if (response.status !== 200) {
      return { error: 'Request failed: ' + response.body };
    }
    return JSON.parse(response.body);
  } catch (err) {
    return { error: err.message || String(err) };
  }
}
```

### Host API 参考

**mote.http** — HTTP 请求（同步）
```javascript
// GET 请求
var resp = mote.http.get(url, options);
// POST 请求
var resp = mote.http.post(url, bodyObject, options);
// PUT 请求
var resp = mote.http.put(url, bodyObject, options);
// DELETE 请求
var resp = mote.http.delete(url, options);

// options 参数：
// {
//   headers: { "Authorization": "Bearer xxx" },
//   timeout: 5000,     // 超时毫秒数
//   insecure: true     // 跳过 TLS 证书验证（用于自签名证书）
// }

// 响应对象：
// resp.status  — HTTP 状态码
// resp.body    — 响应体字符串
// resp.headers — 响应头对象
// resp.json()  — 解析 JSON
// resp.text()  — 获取文本
```

**mote.kv** — 键值存储
```javascript
mote.kv.set("key", "value");
var val = mote.kv.get("key");
mote.kv.delete("key");
var keys = mote.kv.list("prefix");
```

**mote.fs** — 文件系统（受限于 ~/.mote/ 和 /tmp）
```javascript
var content = mote.fs.read("/path/to/file");
mote.fs.write("/path/to/file", "content");
var exists = mote.fs.exists("/path/to/file");
```

**mote.log** — 日志
```javascript
mote.log.info("message");
mote.log.warn("warning");
mote.log.error("error");
```

**mote.context** — 执行上下文
```javascript
var sessionId = mote.context.session_id;
var agentId = mote.context.agent_id;
var scriptName = mote.context.script_name;
```
