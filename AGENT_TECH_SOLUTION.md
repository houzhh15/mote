# GUI + Tray + Bubble 桌面应用技术方案

> 基于 NIEP Agent 代码反向工程生成的标准技术方案，用于快速复用开发类似架构的桌面应用程序。

## 目录

- [1. 架构概述](#1-架构概述)
- [2. 进程间通信（IPC）](#2-进程间通信ipc)
- [3. 如何打包](#3-如何打包)
- [4. 如何避免重复 Dock 图标](#4-如何避免重复-dock-图标)
- [5. 托盘进程实现](#5-托盘进程实现)
- [6. 气泡通知进程实现](#6-气泡通知进程实现)
- [7. 其他技术要点](#7-其他技术要点)

---

## 1. 架构概述

### 1.1 三进程架构模式

```
┌─────────────────────────────────────────────────────────────────┐
│                        用户交互层                                │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐    │
│  │   Tray 托盘     │  │  Bubble 气泡    │  │  Main GUI      │    │
│  │  (systray库)   │  │  (Wails窗口)   │  │  (Wails窗口)   │    │
│  └───────┬────────┘  └───────┬────────┘  └───────┬────────┘    │
│          │                   │                   │              │
└──────────┼───────────────────┼───────────────────┼──────────────┘
           │                   │                   │
           └──────────┬────────┴────────┬──────────┘
                      │                 │
            ┌─────────▼─────────────────▼─────────┐
            │          IPC 通信层                  │
            │    (Unix Domain Socket / Named Pipe) │
            └─────────────────┬───────────────────┘
                              │
            ┌─────────────────▼───────────────────┐
            │         主进程 (Agent)               │
            │  ┌───────────┐  ┌───────────────┐   │
            │  │ IPC Server │  │ Process Manager│   │
            │  └───────────┘  └───────────────┘   │
            │  ┌───────────┐  ┌───────────────┐   │
            │  │ WebSocket │  │ Notification   │   │
            │  │ Client    │  │ Dispatcher     │   │
            │  └───────────┘  └───────────────┘   │
            └─────────────────────────────────────┘
```

### 1.2 进程职责划分

| 进程 | 可执行文件 | 职责 | 技术栈 |
|------|-----------|------|--------|
| **主进程 (Agent)** | `agent` | GUI窗口、业务逻辑、子进程管理、IPC服务端 | Wails v2 |
| **托盘进程 (Tray)** | `agent-tray` | 系统托盘图标、右键菜单、状态显示 | systray 库 |
| **气泡进程 (Bubble)** | `agent-bubble` | 桌面通知气泡、队列管理 | Wails v2 |

### 1.3 为什么采用多进程架构？

1. **macOS Dock 图标隔离**：macOS 每个 GUI 进程默认显示 Dock 图标，独立进程可通过 `LSUIElement` 配置隐藏
2. **进程稳定性**：子进程崩溃不影响主进程，可自动重启恢复
3. **资源隔离**：托盘和气泡进程内存占用小，独立运行
4. **职责分离**：各进程专注单一功能，便于维护

---

## 2. 进程间通信（IPC）

### 2.1 通信协议设计

采用 **Unix Domain Socket (macOS/Linux)** 或 **Named Pipe (Windows)** 实现进程间通信。

#### 2.1.1 消息帧格式

```
+----------------+------------------------+
|  长度头 (4字节) |  JSON 消息体           |
|  Big Endian    |  UTF-8 编码            |
+----------------+------------------------+
```

```go
// 消息编码
func EncodeMessage(msg *IPCMessage) ([]byte, error) {
    payload, err := json.Marshal(msg)
    if err != nil {
        return nil, err
    }
    // 4字节长度头(大端序) + JSON负载
    frame := make([]byte, 4+len(payload))
    binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
    copy(frame[4:], payload)
    return frame, nil
}
```

#### 2.1.2 消息结构定义

```go
// IPCMessage 进程间通信消息结构
type IPCMessage struct {
    Type      string      `json:"type"`      // 消息类型
    Source    string      `json:"source"`    // 消息来源: main/tray/bubble
    Payload   interface{} `json:"payload"`   // 消息负载
    Timestamp int64       `json:"timestamp"` // 时间戳(Unix毫秒)
}
```

### 2.2 消息类型定义

```go
const (
    // MsgTypeRegister 子进程注册身份
    MsgTypeRegister = "register"
    // MsgTypeStatusUpdate 主进程更新托盘状态
    MsgTypeStatusUpdate = "status_update"
    // MsgTypeShowNotification 主进程发送通知到气泡
    MsgTypeShowNotification = "show_notification"
    // MsgTypeCloseNotification 主进程关闭指定气泡
    MsgTypeCloseNotification = "close_notification"
    // MsgTypeAction 子进程发送用户操作
    MsgTypeAction = "action"
    // MsgTypeExit 主进程发送退出命令
    MsgTypeExit = "exit"
    // MsgTypeFolderStats 文件夹统计数据
    MsgTypeFolderStats = "folder_stats"
    // MsgTypeLanguageUpdate 语言切换通知
    MsgTypeLanguageUpdate = "language_update"
)
```

### 2.3 IPC Server 实现（主进程）

```go
// IPCServer IPC服务端
type IPCServer struct {
    listener    net.Listener
    connections map[string]net.Conn // key: source (tray/bubble)
    handlers    map[string]MessageHandler
    mu          sync.RWMutex
}

// Start 启动 IPC 服务
func (s *IPCServer) Start(path string) error {
    // macOS/Linux: Unix Domain Socket
    // Windows: Named Pipe
    var err error
    if runtime.GOOS == "windows" {
        s.listener, err = winio.ListenPipe(path, nil)
    } else {
        os.Remove(path) // 清理残留
        s.listener, err = net.Listen("unix", path)
    }
    // 启动接受连接循环...
}

// Send 发送消息到指定客户端（异步，防止阻塞）
func (s *IPCServer) Send(target string, msg *IPCMessage) error {
    go func() {
        conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
        conn.Write(data)
    }()
    return nil
}
```

### 2.4 IPC Client 实现（子进程）

```go
// IPCClient IPC客户端
type IPCClient struct {
    conn      net.Conn
    handler   MessageHandler
    source    string // 客户端身份: tray/bubble
}

// Connect 连接到主进程
func (c *IPCClient) Connect(path string) error {
    var err error
    if runtime.GOOS == "windows" {
        c.conn, err = winio.DialPipe(path, nil)
    } else {
        c.conn, err = net.Dial("unix", path)
    }
    // 发送注册消息
    return c.sendRegister()
}
```

### 2.5 IPC 路径约定

```go
func getDefaultIPCPath() string {
    if runtime.GOOS == "windows" {
        return `\\.\pipe\myapp-agent-ipc`
    }
    return "/tmp/myapp-agent-ipc.sock"
}
```

### 2.6 消息流示例

```
                 主进程                     托盘进程
                   │                          │
                   │◄───── MsgTypeRegister ───│  (子进程启动后注册)
                   │                          │
                   │─── MsgTypeStatusUpdate ─►│  (状态变化通知托盘)
                   │                          │
                   │◄───── MsgTypeAction ─────│  (用户点击菜单)
                   │                          │
                   │─────── MsgTypeExit ─────►│  (主进程退出通知)
                   │                          │
```

---

## 3. 如何打包

### 3.1 macOS App Bundle 结构

```
MyApp.app/
├── Contents/
│   ├── Info.plist              # 主应用配置
│   ├── MacOS/
│   │   └── agent               # 主应用可执行文件
│   ├── Resources/
│   │   ├── icon.icns           # 应用图标
│   │   └── ...
│   └── Helpers/                # 辅助应用目录
│       ├── MyApp Tray.app/     # 托盘辅助应用
│       │   └── Contents/
│       │       ├── Info.plist  # LSUIElement=true
│       │       └── MacOS/
│       │           └── agent-tray
│       └── MyApp Bubble.app/   # 气泡辅助应用
│           └── Contents/
│               ├── Info.plist  # LSUIElement=true
│               └── MacOS/
│                   └── agent-bubble
```

### 3.2 Makefile 打包脚本

```makefile
# 构建完整 macOS Agent 套件
build-agent-full-macos: build-agent-macos build-agent-tray-macos build-agent-bubble-macos
	@echo '创建 Helper App bundles...'
	# 创建 Helpers 目录
	@mkdir -p cmd/agent/build/bin/myapp.app/Contents/Helpers
	
	# 创建 Tray Helper App
	@mkdir -p "cmd/agent/build/bin/myapp.app/Contents/Helpers/MyApp Tray.app/Contents/MacOS"
	@cp -f bin/agent-tray "cmd/agent/build/bin/myapp.app/Contents/Helpers/MyApp Tray.app/Contents/MacOS/"
	@echo '创建 Info.plist (LSUIElement=true)...'
	
	# 创建 Bubble Helper App  
	@mkdir -p "cmd/agent/build/bin/myapp.app/Contents/Helpers/MyApp Bubble.app/Contents/MacOS"
	@cp -f bin/agent-bubble "cmd/agent/build/bin/myapp.app/Contents/Helpers/MyApp Bubble.app/Contents/MacOS/"
	
	# 签名
	@codesign --force --sign - --deep cmd/agent/build/bin/myapp.app
	@xattr -cr cmd/agent/build/bin/myapp.app
```

### 3.3 Windows 打包结构

```
MyApp/
├── myapp.exe           # 主应用
├── agent-tray.exe      # 托盘进程
├── agent-bubble.exe    # 气泡进程
└── resources/          # 资源文件
```

### 3.4 子进程路径发现

```go
// getTrayExePath 获取托盘进程可执行文件路径
func (m *TrayManager) getTrayExePath() (string, error) {
    // 获取当前可执行文件路径
    exePath, err := os.Executable()
    if err != nil {
        return "", err
    }
    
    exeDir := filepath.Dir(exePath)
    
    if runtime.GOOS == "darwin" {
        // macOS: 查找 Helpers 目录下的 .app bundle
        // /path/to/MyApp.app/Contents/MacOS/agent
        // -> /path/to/MyApp.app/Contents/Helpers/MyApp Tray.app/Contents/MacOS/agent-tray
        contentsDir := filepath.Dir(exeDir) // Contents
        helpersDir := filepath.Join(contentsDir, "Helpers", "MyApp Tray.app", "Contents", "MacOS")
        return filepath.Join(helpersDir, "agent-tray"), nil
    }
    
    // Windows/Linux: 同目录
    return filepath.Join(exeDir, "agent-tray"+getExeSuffix()), nil
}
```

---

## 4. 如何避免重复 Dock 图标

### 4.1 macOS 方案：LSUIElement + 代码隐藏

#### 4.1.1 Info.plist 配置（关键）

每个辅助应用的 `Info.plist` 必须包含：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>agent-tray</string>
    <key>CFBundleIdentifier</key>
    <string>com.myapp.agent.tray</string>
    <key>CFBundleName</key>
    <string>MyApp Tray</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleVersion</key>
    <string>1.0.0</string>
    <!-- 关键配置：隐藏 Dock 图标 -->
    <key>LSUIElement</key>
    <true/>
    <!-- 可选：标记为后台应用 -->
    <key>LSBackgroundOnly</key>
    <true/>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
```

#### 4.1.2 Wails 应用代码隐藏（Bubble 进程）

对于使用 Wails 的气泡进程，由于 Wails 初始化会重置 `ActivationPolicy`，需要使用 CGO 延迟隐藏：

```go
//go:build darwin && wails
// +build darwin,wails

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#import <dispatch/dispatch.h>

// 隐藏 Dock 图标（立即执行）
void hideDockIcon() {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}

// 隐藏 Dock 图标（延迟执行）
// 在 Wails 初始化完成后生效
void hideDockIconDeferred() {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        // 再次延迟，确保彻底隐藏
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(100 * NSEC_PER_MSEC)), 
            dispatch_get_main_queue(), ^{
            [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        });
    });
}
*/
import "C"

// HideDockIcon 隐藏 Dock 图标
func HideDockIcon() {
    C.hideDockIcon()
}

// HideDockIconDeferred 延迟隐藏 Dock 图标
func HideDockIconDeferred() {
    C.hideDockIconDeferred()
}
```

在 Wails 应用启动时调用：

```go
wailsOptions := &options.App{
    OnStartup: func(ctx context.Context) {
        if runtime.GOOS == "darwin" {
            HideDockIconDeferred()
        }
        app.startup(ctx)
    },
    Mac: &mac.Options{
        // Info.plist 的 LSUIElement 配置是主要方案
    },
}
```

### 4.2 Windows 方案

Windows 托盘应用默认不显示任务栏图标，无需特殊处理。

---

## 5. 托盘进程实现

### 5.1 技术选型

使用 [github.com/getlantern/systray](https://github.com/getlantern/systray) 库，跨平台支持。

### 5.2 核心结构

```go
// TrayApp 托盘应用主体
type TrayApp struct {
    currentStatus Status
    ipcClient     *ipc.IPCClient
    iconLoader    *IconLoader
    icons         map[IconType][]byte
    menuItems     *MenuItems
    i18n          *I18n
    blinker       *Blinker       // 图标闪烁控制
}

// MenuItems 菜单项集合
type MenuItems struct {
    NotificationsRoot    *systray.MenuItem
    PendingReview        *systray.MenuItem
    ConfidentialOutbound *systray.MenuItem
    ShowWindow           *systray.MenuItem
    Quit                 *systray.MenuItem
}
```

### 5.3 图标管理

```go
//go:embed icons/*.png icons/*.ico
var embeddedIcons embed.FS

type IconType string

const (
    IconNormal  IconType = "normal"   // 正常状态
    IconWarning IconType = "warning"  // 警告状态
    IconBadge   IconType = "badge"    // 带红点（有未读）
)

// 加载图标时优先 Retina 版本
func (l *IconLoader) LoadIcon(iconType IconType) ([]byte, error) {
    if runtime.GOOS == "darwin" {
        // 优先加载 @2x 版本
        filename := fmt.Sprintf("%s_2x.png", iconType)
        if data, err := l.embedFS.ReadFile("icons/" + filename); err == nil {
            return data, nil
        }
    }
    // 回退普通版本
}
```

### 5.4 图标闪烁（有新通知）

```go
// Blinker 托盘图标闪烁控制器
type Blinker struct {
    isBlinking bool
    ticker     *time.Ticker
    normalIcon []byte
    badgeIcon  []byte
    interval   time.Duration // 默认 500ms
}

func (b *Blinker) Start() {
    b.ticker = time.NewTicker(b.interval)
    go func() {
        showBadge := true
        for range b.ticker.C {
            if showBadge {
                systray.SetIcon(b.badgeIcon)
            } else {
                systray.SetIcon(b.normalIcon)
            }
            showBadge = !showBadge
        }
    }()
}
```

### 5.5 多语言支持

```go
type Language string

const (
    LangZhCN Language = "zh-CN"
    LangJaJP Language = "ja-JP"
    LangEnUS Language = "en-US"
)

type I18n struct {
    currentLang  Language
    translations map[Language]map[MenuKey]string
}

// 带数量的翻译
func (i *I18n) TWithCount(key MenuKey, count int) string {
    base := i.T(key)
    if count > 0 {
        return fmt.Sprintf("%s (%d)", base, count)
    }
    return base
}
```

---

## 6. 气泡通知进程实现

### 6.1 技术选型

使用 Wails v2 创建透明无边框窗口，实现自定义气泡样式。

### 6.2 Wails 配置

```go
wailsOptions := &options.App{
    Title:         "Notification",
    Width:         400,
    Height:        195,
    DisableResize: true,
    Frameless:     true,
    StartHidden:   true,      // 初始隐藏
    AlwaysOnTop:   true,      // 始终置顶
    BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0}, // 透明
    Mac: &mac.Options{
        WebviewIsTransparent: true,
        WindowIsTranslucent:  true,
    },
    Windows: &windows.Options{
        WebviewIsTransparent: true,
        WindowIsTranslucent:  true,
    },
}
```

### 6.3 通知队列管理

```go
// QueueManager 队列管理器
type QueueManager struct {
    app          *BubbleApp
    currentNotif *BubbleNotification // 当前显示的
    queue        []*BubbleNotification // 等待队列
    timer        *time.Timer
    timerPaused  bool
    remainingMs  int
}

// EnqueueNotification 加入队列
func (qm *QueueManager) EnqueueNotification(notif *BubbleNotification) {
    qm.queue = append(qm.queue, notif)
    
    if !qm.windowVisible || qm.currentNotif == nil {
        qm.showNextNotification()
    } else {
        // 更新显示累积数量
        qm.updateBubbleContent(len(qm.queue) + 1)
    }
}
```

### 6.4 窗口定位（右下角）

```go
// positionWindow 定位气泡窗口到屏幕右下角
func (a *BubbleApp) positionWindow() {
    screens, err := wailsruntime.ScreenGetAll(a.ctx)
    if err != nil || len(screens) == 0 {
        return
    }
    
    primary := screens[0]
    width, height := 400, 195
    padding := 20
    
    x := primary.Size.Width - width - padding
    y := primary.Size.Height - height - padding - 50 // 避开任务栏
    
    wailsruntime.WindowSetPosition(a.ctx, x, y)
}
```

### 6.5 鼠标悬停暂停

```go
// 前端事件
wailsruntime.EventsOn(ctx, "bubble:mouse-enter", func(...) {
    qm.pauseTimer()
})

wailsruntime.EventsOn(ctx, "bubble:mouse-leave", func(...) {
    qm.resumeTimer()
})

func (qm *QueueManager) pauseTimer() {
    if qm.timer != nil && !qm.timerPaused {
        qm.timer.Stop()
        qm.timerPaused = true
        qm.remainingMs = /* 计算剩余时间 */
    }
}
```

---

## 7. 其他技术要点

### 7.1 子进程生命周期管理

```go
// TrayManager 托盘进程管理器
type TrayManager struct {
    process      *os.Process
    cmd          *exec.Cmd
    ipcServer    *ipc.IPCServer
    restartCount int
    maxRestarts  int // 最大重启次数，如 3 次
}

// monitor 监控子进程状态
func (m *TrayManager) monitor(ctx context.Context) {
    for {
        err := m.cmd.Wait()
        if !m.running {
            return // 正常退出
        }
        
        // 异常退出，尝试重启
        if m.restartCount < m.maxRestarts {
            m.restartCount++
            time.Sleep(5 * time.Second)
            m.startProcess()
        }
    }
}

// Stop 停止子进程
func (m *TrayManager) Stop() error {
    // 1. 发送 IPC 退出消息
    m.ipcServer.SendSync(ipc.SourceTray, exitMsg)
    
    // 2. 异步等待进程退出
    go func() {
        for i := 0; i < 30; i++ { // 最多等 3 秒
            if proc.Signal(nil) != nil {
                return // 进程已退出
            }
            time.Sleep(100 * time.Millisecond)
        }
        proc.Kill() // 超时强制终止
    }()
}
```

### 7.2 优雅退出流程

```
用户点击退出
     │
     ▼
主进程收到 MsgTypeAction(quit)
     │
     ├──► 发送 MsgTypeExit 到 Tray ──► Tray 调用 systray.Quit()
     │
     ├──► 发送 MsgTypeExit 到 Bubble ──► Bubble 调用 wails.Quit()
     │
     ▼
主进程等待子进程退出（超时 3 秒）
     │
     ▼
主进程调用 OnBeforeClose() / OnShutdown()
     │
     ▼
程序退出
```

### 7.3 主进程与前端通信

```go
// 主进程向前端发送事件
wailsruntime.EventsEmit(ctx, "notification:new", data)

// 前端监听事件
runtime.EventsOn("notification:new", callback)

// 前端调用后端方法
app.SomeMethod(args)
```

### 7.4 日志管理

```go
// 子进程日志重定向到文件
logFile := filepath.Join("logs", "agent-tray.log")
logF, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
cmd.Stdout = logF
cmd.Stderr = logF
```

### 7.5 资源嵌入

```go
// 托盘图标嵌入
//go:embed icons/*.png icons/*.ico
var embeddedIcons embed.FS

// 主应用前端资源嵌入
//go:embed all:frontend/dist
var assets embed.FS
```

### 7.6 跨平台编译

```makefile
# macOS Universal Binary
build-macos-universal:
	wails build -platform darwin/arm64
	wails build -platform darwin/amd64
	lipo -create -output bin/app bin/app-arm64 bin/app-amd64

# Windows (跨平台编译)
build-windows:
	wails build -platform windows/amd64
```

---

## 附录

### A. 目录结构建议

```
project/
├── cmd/
│   ├── agent/              # 主应用
│   │   ├── main.go
│   │   ├── build/          # Wails 构建配置
│   │   ├── frontend/       # 前端代码
│   │   └── wails.json
│   ├── agent-tray/         # 托盘进程
│   │   ├── main.go
│   │   └── build/
│   └── agent-bubble/       # 气泡进程
│       ├── main_wails.go
│       ├── queue.go
│       ├── events.go
│       ├── dock_darwin.go  # macOS Dock 隐藏
│       ├── dock_other.go   # 其他平台 stub
│       ├── build/
│       └── frontend/
├── internal/
│   └── agent/
│       ├── app/            # 主应用逻辑
│       │   ├── app.go
│       │   ├── extension.go
│       │   ├── tray_manager.go
│       │   └── bubble_manager.go
│       ├── ipc/            # IPC 通信
│       │   ├── protocol.go
│       │   ├── client.go
│       │   ├── server.go
│       │   ├── client_darwin.go
│       │   └── server_windows.go
│       ├── tray/           # 托盘业务逻辑
│       │   ├── manager.go
│       │   ├── menu.go
│       │   ├── icons.go
│       │   ├── blinker.go
│       │   ├── i18n.go
│       │   └── icons/      # 嵌入的图标
│       └── notification/   # 通知类型
│           ├── types.go
│           └── dispatcher.go
├── bin/                    # 构建输出
├── scripts/
│   └── package-agent-macos.sh
└── Makefile
```

### B. 关键依赖

```go
// go.mod
require (
    github.com/wailsapp/wails/v2 v2.x.x
    github.com/getlantern/systray v1.x.x
    github.com/Microsoft/go-winio v0.x.x  // Windows Named Pipe
)
```

### C. 常见问题

| 问题 | 解决方案 |
|------|----------|
| macOS 多个 Dock 图标 | 确保 Helper App 的 Info.plist 包含 `LSUIElement=true` |
| Wails 初始化后 Dock 图标重现 | 使用 CGO 延迟调用 `setActivationPolicy` |
| 子进程无法启动 | 检查可执行文件路径，确保签名正确 |
| IPC 连接失败 | 检查 socket 文件权限，确保主进程先启动 |
| Windows 托盘图标不显示 | 确保 ICO 文件格式正确（多尺寸） |

---

*文档生成时间：2026-02-02*
*基于 NIEP Agent 代码反向工程*
