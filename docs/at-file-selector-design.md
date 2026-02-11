# Mote @ 文件选择器设计方案

**灵感来源**: GitHub Copilot CLI 的 `@filename` 文件引用机制  
**设计目标**: 借鉴现有的 `/` 提示词选择器，实现 `@` 触发的文件选择器  
**日期**: 2026-02-10

---

## 🎯 核心理念

**用户体验**：
```
用户输入: "看看这个图片 @"
         ↓ 触发文件选择器（类似 / 的提示词选择器）
         ↓ 输入字符过滤：scre...
         ↓ 选中：screenshot.png
结果输入框: "看看这个图片 @screenshot.png "
```

**与 `/` 提示词选择器的相似性**：

| 特性 | `/` 提示词选择器 | `@` 文件选择器 |
|-----|----------------|--------------|
| 触发字符 | `/` | `@` |
| 数据源 | 用户提示词 + MCP 提示词 | Workspace 文件 + 当前目录文件 |
| 过滤 | 名称、内容模糊匹配 | 文件名、路径模糊匹配 |
| 选择结果 | 插入提示词内容 | 插入 `@filename` 引用 |
| UI 组件 | 弹出列表（绝对定位） | 弹出列表（绝对定位） |

---

## 📋 Copilot CLI 的 @ 机制

### 官方行为

根据搜索结果和官方文档：

```bash
$ copilot
> @utils.py 帮我优化这个文件
> @screenshot.png 这个 UI 有什么问题？
> @src/config.json 生成迁移脚本
```

**关键特性**：

1. ✅ **自动补全**：输入 `@` 后显示文件列表
2. ✅ **模糊匹配**：输入部分文件名过滤列表
3. ✅ **相对/绝对路径**：支持 `./`, `../`, `/` 等路径
4. ✅ **多文件引用**：可以引用多个文件 `@file1.py @file2.js`
5. ✅ **文件类型识别**：图片、代码、文档等不同处理

**底层机制**：

- CLI 读取文件内容
- 自动编码（图片 → base64）
- 添加到 context
- 发送给 LLM

---

## 🎨 Mote 的 @ 文件选择器设计

### 1. 触发逻辑（复用 / 的实现）

查看现有的 `/` 触发逻辑：

```typescript
// ChatPage.tsx:1229-1252
const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
  const newValue = e.target.value;
  setInputValue(newValue);

  // Check if user typed "/" to trigger prompt selector
  if (newValue.endsWith('/')) {
    const beforeSlash = newValue.slice(0, -1);
    if (beforeSlash === '' || beforeSlash.endsWith(' ') || beforeSlash.endsWith('\n')) {
      setPromptSelectorVisible(true);
      setPromptSearchQuery('');
      return;
    }
  }

  // Extract search query after "/"
  const lastSlashIndex = newValue.lastIndexOf('/');
  if (lastSlashIndex !== -1 && promptSelectorVisible) {
    setPromptSearchQuery(newValue.slice(lastSlashIndex + 1));
  } else {
    setPromptSelectorVisible(false);
  }
};
```

**修改为支持 @ 触发**：

```typescript
const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
  const newValue = e.target.value;
  setInputValue(newValue);

  // --- 原有的 / 提示词选择器逻辑 ---
  if (newValue.endsWith('/')) {
    const beforeSlash = newValue.slice(0, -1);
    if (beforeSlash === '' || beforeSlash.endsWith(' ') || beforeSlash.endsWith('\n')) {
      setPromptSelectorVisible(true);
      setPromptSearchQuery('');
      setFileSelectorVisible(false); // 关闭文件选择器
      return;
    }
  }

  const lastSlashIndex = newValue.lastIndexOf('/');
  if (lastSlashIndex !== -1 && promptSelectorVisible) {
    setPromptSearchQuery(newValue.slice(lastSlashIndex + 1));
  } else {
    setPromptSelectorVisible(false);
  }

  // --- 新增的 @ 文件选择器逻辑 ---
  if (newValue.endsWith('@')) {
    const beforeAt = newValue.slice(0, -1);
    if (beforeAt === '' || beforeAt.endsWith(' ') || beforeAt.endsWith('\n')) {
      setFileSelectorVisible(true);
      setFileSearchQuery('');
      setPromptSelectorVisible(false); // 关闭提示词选择器
      return;
    }
  }

  const lastAtIndex = newValue.lastIndexOf('@');
  if (lastAtIndex !== -1 && fileSelectorVisible) {
    setFileSearchQuery(newValue.slice(lastAtIndex + 1));
  } else {
    setFileSelectorVisible(false);
  }
};
```

---

### 2. FileSelector 组件设计

**参考 PromptSelector 的结构**，创建类似的 `FileSelector` 组件：

```typescript
// shared/ui/src/components/FileSelector.tsx

import React, { useState, useEffect } from 'react';
import { List, Typography, Tag, Empty, Spin, Tabs, theme } from 'antd';
import { 
  FileOutlined, 
  FileImageOutlined, 
  FileTextOutlined, 
  FolderOutlined,
  CodeOutlined 
} from '@ant-design/icons';
import { useAPI } from '../context/APIContext';

const { Text } = Typography;

interface WorkspaceFile {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
  children?: WorkspaceFile[];
}

interface FileItem {
  name: string;
  path: string;
  type: 'image' | 'code' | 'text' | 'other';
  size: number;
  icon: React.ReactNode;
}

interface FileSelectorProps {
  visible: boolean;
  searchQuery: string;
  sessionId: string;
  onSelect: (filepath: string) => void;
  onCancel: () => void;
}

export const FileSelector: React.FC<FileSelectorProps> = ({
  visible,
  searchQuery,
  sessionId,
  onSelect,
  onCancel,
}) => {
  const api = useAPI();
  const { token } = theme.useToken();
  const [workspaceFiles, setWorkspaceFiles] = useState<FileItem[]>([]);
  const [recentFiles, setRecentFiles] = useState<FileItem[]>([]);
  const [filteredFiles, setFilteredFiles] = useState<FileItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [activeTab, setActiveTab] = useState<string>('workspace');

  // 文件类型检测
  const detectFileType = (filename: string): 'image' | 'code' | 'text' | 'other' => {
    const ext = filename.split('.').pop()?.toLowerCase();
    const imageExts = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp'];
    const codeExts = ['js', 'ts', 'jsx', 'tsx', 'py', 'go', 'rs', 'java', 'cpp', 'c', 'h'];
    const textExts = ['txt', 'md', 'json', 'yaml', 'yml', 'xml', 'csv'];
    
    if (imageExts.includes(ext!)) return 'image';
    if (codeExts.includes(ext!)) return 'code';
    if (textExts.includes(ext!)) return 'text';
    return 'other';
  };

  // 文件图标
  const getFileIcon = (type: string) => {
    const iconStyle = { fontSize: 16 };
    switch (type) {
      case 'image': return <FileImageOutlined style={{ ...iconStyle, color: '#52c41a' }} />;
      case 'code': return <CodeOutlined style={{ ...iconStyle, color: '#1890ff' }} />;
      case 'text': return <FileTextOutlined style={{ ...iconStyle, color: '#faad14' }} />;
      default: return <FileOutlined style={iconStyle} />;
    }
  };

  // 递归展开文件树
  const flattenFiles = (files: WorkspaceFile[], basePath = ''): FileItem[] => {
    const result: FileItem[] = [];
    
    for (const file of files) {
      if (file.is_dir && file.children) {
        // 递归处理子目录
        result.push(...flattenFiles(file.children, file.path));
      } else if (!file.is_dir) {
        const type = detectFileType(file.name);
        result.push({
          name: file.name,
          path: file.path,
          type,
          size: file.size,
          icon: getFileIcon(type),
        });
      }
    }
    
    return result;
  };

  // 加载 Workspace 文件
  useEffect(() => {
    const loadFiles = async () => {
      if (!visible) return;
      
      setLoading(true);
      try {
        // 1. 加载 Workspace 文件（如果有绑定的 workspace）
        const workspaceData = await api.listWorkspaceFiles?.(sessionId, '/');
        if (workspaceData) {
          const files = flattenFiles(workspaceData);
          setWorkspaceFiles(files);
        }
        
        // 2. 加载最近使用的文件（可选，从 localStorage 或 API）
        // const recent = await api.getRecentFiles?.();
        // setRecentFiles(recent);
      } catch (error) {
        console.error('Failed to load files:', error);
        setWorkspaceFiles([]);
      } finally {
        setLoading(false);
      }
    };
    
    if (visible) {
      loadFiles();
    }
  }, [api, visible, sessionId]);

  // 过滤文件
  useEffect(() => {
    let items: FileItem[] = [];
    
    if (activeTab === 'workspace') {
      items = workspaceFiles;
    } else if (activeTab === 'recent') {
      items = recentFiles;
    }

    if (searchQuery) {
      const query = searchQuery.toLowerCase();
      items = items.filter((f) =>
        f.name.toLowerCase().includes(query) ||
        f.path.toLowerCase().includes(query)
      );
    }

    // 按类型排序：图片 > 代码 > 文本 > 其他
    items.sort((a, b) => {
      const typeOrder = { image: 0, code: 1, text: 2, other: 3 };
      return typeOrder[a.type] - typeOrder[b.type];
    });

    setFilteredFiles(items);
  }, [searchQuery, workspaceFiles, recentFiles, activeTab]);

  const handleFileClick = (file: FileItem) => {
    // 插入文件引用到输入框
    onSelect(file.path);
    onCancel();
  };

  if (!visible) return null;

  const tabItems = [
    { key: 'workspace', label: <span style={{ fontSize: 13 }}>{`工作区 (${workspaceFiles.length})`}</span> },
    { key: 'recent', label: <span style={{ fontSize: 13 }}>{`最近 (${recentFiles.length})`}</span> },
  ];

  return (
    <div
      style={{
        position: 'absolute',
        bottom: '100%',
        left: 0,
        right: 0,
        maxHeight: 350,
        overflowY: 'auto',
        background: token.colorBgContainer,
        border: `1px solid ${token.colorBorderSecondary}`,
        borderRadius: 8,
        boxShadow: '0 -4px 12px rgba(0, 0, 0, 0.1)',
        zIndex: 100,
        marginBottom: 8,
      }}
      onClick={(e) => e.stopPropagation()}
    >
      <div style={{ padding: '4px 12px 0', borderBottom: `1px solid ${token.colorBorderSecondary}` }}>
        <Tabs 
          activeKey={activeTab} 
          onChange={setActiveTab}
          size="small"
          items={tabItems}
        />
      </div>

      {loading ? (
        <div style={{ padding: 24, textAlign: 'center' }}>
          <Spin size="small" />
        </div>
      ) : filteredFiles.length === 0 ? (
        <Empty
          description="无匹配的文件"
          style={{ padding: 24 }}
          imageStyle={{ height: 40 }}
        />
      ) : (
        <List
          size="small"
          dataSource={filteredFiles}
          renderItem={(file) => (
            <List.Item
              onClick={() => handleFileClick(file)}
              style={{
                cursor: 'pointer',
                padding: '8px 12px',
              }}
              className="file-selector-item"
            >
              <List.Item.Meta
                style={{ textAlign: 'left' }}
                avatar={file.icon}
                title={
                  <span style={{ fontSize: 13 }}>
                    @{file.name}
                    <Tag 
                      color={file.type === 'image' ? 'green' : file.type === 'code' ? 'blue' : 'default'} 
                      style={{ marginLeft: 8, fontSize: 11 }}
                    >
                      {file.type}
                    </Tag>
                  </span>
                }
                description={
                  <Text
                    ellipsis
                    type="secondary"
                    style={{ fontSize: 12, display: 'block', textAlign: 'left' }}
                  >
                    {file.path} · {(file.size / 1024).toFixed(1)} KB
                  </Text>
                }
              />
            </List.Item>
          )}
        />
      )}

      <style>{`
        .file-selector-item:hover {
          background: ${token.colorBgTextHover};
        }
      `}</style>
    </div>
  );
};
```

---

### 3. ChatPage 集成

```typescript
// ChatPage.tsx 中的状态
const [fileSelectorVisible, setFileSelectorVisible] = useState(false);
const [fileSearchQuery, setFileSearchQuery] = useState('');

// 文件选择处理
const handleFileSelect = (filepath: string) => {
  // 在输入框中替换 @ 后的内容为文件路径
  const lastAtIndex = inputValue.lastIndexOf('@');
  const before = inputValue.slice(0, lastAtIndex);
  const after = ''; // 清除 @ 后的搜索内容
  
  // 插入 @filepath 并添加空格
  setInputValue(`${before}@${filepath} `);
  setFileSelectorVisible(false);
};

// 渲染文件选择器
<FileSelector
  visible={fileSelectorVisible}
  searchQuery={fileSearchQuery}
  sessionId={sessionId}
  onSelect={handleFileSelect}
  onCancel={() => {
    setFileSelectorVisible(false);
    setFileSearchQuery('');
  }}
/>
```

---

### 4. 后端处理文件引用

```go
// api/v1/chat.go

// 解析消息中的文件引用
func parseFileReferences(message string) (cleanMessage string, fileRefs []string) {
    // 正则匹配 @filepath
    re := regexp.MustCompile(`@([^\s]+)`)
    matches := re.FindAllStringSubmatch(message, -1)
    
    for _, match := range matches {
        if len(match) > 1 {
            fileRefs = append(fileRefs, match[1])
        }
    }
    
    // 移除 @filepath，保留纯文本消息（可选）
    cleanMessage = re.ReplaceAllString(message, "")
    cleanMessage = strings.TrimSpace(cleanMessage)
    
    return cleanMessage, fileRefs
}

// 在 handleChat 中使用
func (r *Router) handleChat(w http.ResponseWriter, req *http.Request) {
    // ... 解析 request
    
    // 解析文件引用
    cleanMessage, fileRefs := parseFileReferences(chatReq.Message)
    
    // 构造附件
    var attachments []provider.Attachment
    for _, filepath := range fileRefs {
        // 读取文件
        data, err := os.ReadFile(filepath)
        if err != nil {
            // 错误处理
            continue
        }
        
        // 检测文件类型
        mimeType := detectMimeType(filepath)
        attachments = append(attachments, provider.Attachment{
            Type:     detectAttachmentType(mimeType),
            Filepath: filepath,
            MimeType: mimeType,
            Data:     string(data),
            Filename: filepath.Base(filepath),
            Size:     len(data),
        })
    }
    
    // 发送给 Runner
    runnerReq := runner.Request{
        Message:     cleanMessage,
        Attachments: attachments,
        // ...
    }
}
```

---

## 🎯 交互流程示例

### 场景 1: 引用单个图片

```
步骤 1: 用户输入 "@"
┌────────────────────────────────────┐
│ @                                  │← 光标在这里
│                            [发送 ▶] │
└────────────────────────────────────┘
       ↓ 弹出文件选择器
┌────────────────────────────────────┐
│ [工作区 (23)] [最近 (5)]            │
├────────────────────────────────────┤
│ 📷 screenshot.png (image)         │
│    workspace/assets/screenshot.png │
│ 📄 config.json (text)             │
│    workspace/config.json           │
│ 💻 main.go (code)                 │
│    workspace/src/main.go           │
└────────────────────────────────────┘

步骤 2: 用户输入 "scre" 过滤
┌────────────────────────────────────┐
│ @scre                              │
│                            [发送 ▶] │
└────────────────────────────────────┘
       ↓ 只显示匹配的文件
┌────────────────────────────────────┐
│ [工作区 (1)] [最近 (0)]             │
├────────────────────────────────────┤
│ 📷 screenshot.png (image)         │← 选中这个
│    workspace/assets/screenshot.png │
└────────────────────────────────────┘

步骤 3: 点击文件或按 Enter
┌────────────────────────────────────┐
│ @workspace/assets/screenshot.png   │← 自动填充
│ 这个 UI 设计有什么问题？           │
│                            [发送 ▶] │
└────────────────────────────────────┘
```

---

### 场景 2: 引用多个文件

```
用户输入: "@src/main.go @src/utils.go 重构这两个文件"
         ↓ 后端解析
文件引用: ["src/main.go", "src/utils.go"]
消息文本: "重构这两个文件"
         ↓ 读取文件
附件:    [Attachment{type: code, data: "..."}, Attachment{type: code, data: "..."}]
         ↓ 发送给 Provider
```

---

## 📊 功能对比

| 功能 | `/` 提示词选择器 | `@` 文件选择器 |
|-----|----------------|--------------|
| **触发** | 输入 `/` | 输入 `@` |
| **数据源** | API: getPrompts, getMCPPrompts | API: listWorkspaceFiles |
| **过滤** | 名称、内容 | 文件名、路径 |
| **选择** | 插入提示词文本 | 插入 `@filepath` |
| **分类** | 用户/MCP | 工作区/最近 |
| **图标** | FileTextOutlined, ApiOutlined | FileImageOutlined, CodeOutlined |
| **参数** | MCP 提示词需要参数 | 无需参数 |

---

## ✅ 实施清单

### Phase 1: 基础实现（2 天）

```
[ ] 1. 创建 FileSelector 组件（复制 PromptSelector 并修改）
[ ] 2. 修改 ChatPage 的 handleInputChange，添加 @ 触发逻辑
[ ] 3. 添加状态：fileSelectorVisible, fileSearchQuery
[ ] 4. 集成 Workspace API（listWorkspaceFiles）
[ ] 5. 文件类型检测和图标显示
[ ] 6. 点击文件插入 @filepath 到输入框
```

### Phase 2: 后端支持（1 天）

```
[ ] 1. API 层解析 @filepath 引用
[ ] 2. 读取文件内容
[ ] 3. 构造 Attachment 对象
[ ] 4. 传递给 Runner → Provider
```

### Phase 3: 增强功能（1-2 天，可选）

```
[ ] 1. 最近使用文件列表（localStorage 缓存）
[ ] 2. 图片预览缩略图
[ ] 3. 文件大小限制提示
[ ] 4. 支持相对路径（./、../）
[ ] 5. 键盘快捷键（Ctrl+@）
[ ] 6. 多文件选择（Ctrl+点击）
```

---

## 🎨 UI 细节

### 文件类型图标和颜色

```typescript
const fileTypeConfig = {
  image: { icon: <FileImageOutlined />, color: '#52c41a' },
  code: { icon: <CodeOutlined />, color: '#1890ff' },
  text: { icon: <FileTextOutlined />, color: '#faad14' },
  pdf: { icon: <FilePdfOutlined />, color: '#ff4d4f' },
  archive: { icon: <FileZipOutlined />, color: '#722ed1' },
  other: { icon: <FileOutlined />, color: '#8c8c8c' },
};
```

### 键盘支持

```typescript
// 在 FileSelector 中添加键盘导航
const handleKeyDown = (e: React.KeyboardEvent) => {
  if (e.key === 'ArrowDown') {
    // 选择下一个文件
    setSelectedIndex((prev) => Math.min(prev + 1, filteredFiles.length - 1));
    e.preventDefault();
  } else if (e.key === 'ArrowUp') {
    // 选择上一个文件
    setSelectedIndex((prev) => Math.max(prev - 1, 0));
    e.preventDefault();
  } else if (e.key === 'Enter') {
    // 选择当前文件
    handleFileClick(filteredFiles[selectedIndex]);
    e.preventDefault();
  } else if (e.key === 'Escape') {
    // 关闭选择器
    onCancel();
    e.preventDefault();
  }
};
```

---

## 💡 最佳实践

### 1. 文件路径处理

```typescript
// 优先使用相对于 Workspace 的路径
const normalizeFilePath = (filepath: string, workspaceRoot: string) => {
  if (filepath.startsWith(workspaceRoot)) {
    return `./${filepath.slice(workspaceRoot.length)}`;
  }
  return filepath;
};
```

### 2. 文件大小限制

```typescript
// 超过 10MB 的文件给出警告
const MAX_FILE_SIZE = 10 * 1024 * 1024;

if (file.size > MAX_FILE_SIZE) {
  message.warning(`文件 ${file.name} 过大 (${(file.size / 1024 / 1024).toFixed(1)} MB)，可能导致性能问题`);
}
```

### 3. 错误处理

```typescript
// 文件不存在或无权限
try {
  const data = await os.ReadFile(filepath);
} catch (err) {
  if (err.code === 'ENOENT') {
    message.error(`文件不存在: ${filepath}`);
  } else if (err.code === 'EACCES') {
    message.error(`无权限读取文件: ${filepath}`);
  } else {
    message.error(`读取文件失败: ${err.message}`);
  }
}
```

---

## 📝 总结

### ✅ 你的观点完全正确

1. **借鉴 Copilot CLI 的 `@` 机制** - 符合用户习惯
2. **复用 `/` 提示词选择器的设计** - 统一的交互体验
3. **避免手动输入路径** - 提供文件浏览和过滤

### 🎯 核心优势

- ✅ **用户友好**: 类似 IDE 的体验（VS Code 的 `@` 引用）
- ✅ **实现简单**: 复用现有的 PromptSelector 逻辑
- ✅ **性能优秀**: 只传递路径，后端处理文件读取
- ✅ **扩展性强**: 可添加文件预览、最近文件等功能

### 🚀 预估工作量

- **基础实现**: 2-3 天
- **完整功能**: 4-5 天（包含增强功能）

比完整的"文件上传"方案减少了 **70% 的工作量**！
