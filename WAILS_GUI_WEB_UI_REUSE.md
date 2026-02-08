# Wails GUI 与 Web UI 前端代码复用方案

> NIEP Agent 桌面应用前端代码复用技术方案分析

## 目录

- [1. 核心设计理念](#1-核心设计理念)
- [2. 架构概览](#2-架构概览)
- [3. 关键实现](#3-关键实现)
- [4. 数据流示例](#4-数据流示例)
- [5. 核心优势](#5-核心优势)
- [6. 特殊处理：图片代理](#6-特殊处理图片代理)
- [7. 与普通 Wails 应用的区别](#7-与普通-wails-应用的区别)
- [8. 总结](#8-总结)

---

## 1. 核心设计理念

NIEP Agent 通过 **APIProxy** 实现了前端代码复用，让 Wails GUI 和 Web UI 使用相同的前端代码。

**关键创新**：
- **统一 API 代理层**：Go 后端封装所有 HTTP 请求
- **抽象接口设计**：`IApiClient` 支持多环境适配
- **自动 Token 管理**：Go 后端处理认证逻辑

---

## 2. 架构概览

```
┌─────────────────────────────────────────────────────────┐
│              前端代码（复用层）                          │
│  ┌──────────────────────────────────────────────────┐  │
│  │ WailsApiClient (Wails)  /  HttpApiClient (Web)   │  │
│  │        implements IApiClient interface           │  │
│  └───────────────────┬──────────────────────────────┘  │
│                      │ 统一接口                         │
│                      │ get/post/put/delete             │
└──────────────────────┼──────────────────────────────────┘
                       │
       ┌───────────────┼───────────────┐
       │               │               │
   Wails GUI        Web UI         Electron
       │               │               │
       ▼               ▼               ▼
┌──────────────┐  ┌──────────┐  ┌──────────┐
│ CallAPI()    │  │ fetch()  │  │ axios()  │
│ Go Backend   │  │ HTTP     │  │ HTTP     │
└──────┬───────┘  └──────────┘  └──────────┘
       │
       ▼
┌─────────────────────────────────┐
│    APIProxy (Go)                │
│  - Token 管理                   │
│  - 自动刷新                     │
│  - 统一错误处理                 │
└────────┬────────────────────────┘
         │
         ▼
    NIEP Server
```

---

## 3. 关键实现

### 3.1 前端统一接口 (TypeScript)

```typescript
// services/apiClient.ts
export interface IApiClient {
  get<T>(path: string, config?: RequestConfig): Promise<ApiResponse<T>>;
  post<T>(path: string, body?: unknown, config?: RequestConfig): Promise<ApiResponse<T>>;
  put<T>(path: string, body?: unknown, config?: RequestConfig): Promise<ApiResponse<T>>;
  delete<T>(path: string, config?: RequestConfig): Promise<ApiResponse<T>>;
}
```

### 3.2 Wails 实现：调用 Go 后端

```typescript
// services/wailsApiClient.ts
import { CallAPI } from '../../wailsjs/go/app/App'; // Wails 自动生成

export class WailsApiClient implements IApiClient {
  async request<T>(method: string, path: string, body?: unknown, config?: RequestConfig) {
    const url = buildUrl(path, config?.params);
    const bodyStr = body ? JSON.stringify(body) : '';
    const headers = config?.headers || {};
    
    // 调用 Go 后端的 CallAPI 方法
    const rawResponse = await CallAPI(
      method.toUpperCase(), 
      url, 
      bodyStr, 
      headers
    );
    
    return parseResponse<T>(rawResponse);
  }
}
```

### 3.3 Go 后端 API Proxy

```go
// internal/agent/network/api_proxy.go
type APIProxy struct {
    httpClient  *http.Client
    secureStore *storage.SecureStore
    baseURL     string
}

// ProxyRequest 统一代理 API 请求
func (p *APIProxy) ProxyRequest(ctx context.Context, req types.APIRequest) types.APIResponse {
    // 1. 获取 access token
    token, err := p.secureStore.GetAccessToken()
    
    // 2. 执行请求
    resp := p.doRequest(ctx, req, token)
    
    // 3. 401 时自动刷新 token 并重试
    if resp.Status == http.StatusUnauthorized {
        p.refreshToken(ctx)
        newToken, _ := p.secureStore.GetAccessToken()
        resp = p.doRequest(ctx, req, newToken)
    }
    
    return resp
}
```

### 3.4 Wails Binding (Go)

```go
// internal/agent/app/app.go
// CallAPI 暴露给前端的统一 API 入口
func (a *App) CallAPI(method string, path string, body string, headers map[string]string) types.APIResponse {
    req := types.APIRequest{
        Method:  method,
        Path:    path,
        Body:    body,
        Headers: headers,
    }
    return a.apiProxy.ProxyRequest(a.ctx, req)
}
```

---

## 4. 数据流示例

### 前端调用

```typescript
// 前端代码（Wails 和 Web 完全一致）
const apiClient = new WailsApiClient(); // or HttpApiClient for Web

const response = await apiClient.get('/api/user/profile');
```

### Wails 执行流程

```
1. WailsApiClient.get()
   ↓
2. CallAPI('GET', '/api/user/profile', '', {})  // Wails JS Bridge
   ↓
3. App.CallAPI() (Go)                           // 后端方法
   ↓
4. apiProxy.ProxyRequest()                      // API 代理
   ↓
5. 自动添加 Bearer Token
   ↓
6. http.Client.Do() → https://server/api/user/profile
   ↓
7. 返回 types.APIResponse
   ↓
8. 返回到前端 (JSON)
```

---

## 5. 核心优势

| 特性 | Wails GUI | Web UI |
|------|-----------|--------|
| **前端代码** | ✅ 完全复用 | ✅ 完全复用 |
| **API 调用** | 通过 Go Proxy | 直接 HTTP |
| **Token 管理** | Go 后端自动处理 | 前端/后端处理 |
| **离线存储** | Go secure store | LocalStorage |
| **跨域问题** | ❌ 无（本地调用） | ⚠️ 需 CORS |

---

## 6. 特殊处理：图片代理

### 问题
Wails Webview 在 HTTPS 上下文中无法加载 HTTP 图片。

### 解决方案

```go
// cmd/agent/main.go - AssetServer Handler
assetHandler := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
    if strings.HasPrefix(req.URL.Path, "/proxy-image") {
        imageURL := req.URL.Query().Get("url");
        
        // Go 后端获取图片
        resp, _ := http.Get(imageURL);
        
        // 转发到前端
        io.Copy(res, resp.Body);
        return;
    }
    // ... 其他静态资源处理
})
```

前端使用：

```html
<img src="/proxy-image?url=http://example.com/avatar.jpg" />
```

或通过 Wails Binding：

```typescript
const { dataBase64, contentType } = await FetchImage(imageURL);
```

---

## 7. 与普通 Wails 应用的区别

| 对比项 | 普通 Wails | NIEP Agent |
|--------|-----------|------------|
| API 调用 | 每个 API 一个 Go 方法 | **统一 CallAPI** |
| 前端实现 | `import { GetUser } from 'wailsjs'` | **抽象接口 IApiClient** |
| 代码复用 | ❌ 无法复用到 Web | ✅ **完全复用** |
| HTTP 客户端 | 前端直接调用 Go | **Go Proxy → Server** |
| Token 管理 | 前端或 Go 混合 | **Go 统一管理** |

---

## 8. 总结

NIEP Agent **没有使用传统的 HTTP Proxy 服务器**，而是：

1. **Go 后端封装 HTTP Client**：`APIProxy` 负责所有 HTTP 请求
2. **统一 Wails Binding**：`CallAPI()` 作为唯一入口
3. **前端接口抽象**：`IApiClient` 实现多环境适配
4. **自动 Token 管理**：Go 后端处理 401 刷新逻辑

这种设计让前端代码可以**无修改地运行在 Wails GUI、Web UI 和未来的 Electron** 等环境中。

---

*文档生成时间：2026-02-03*
*基于 NIEP Agent 代码分析*
