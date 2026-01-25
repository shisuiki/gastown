# Gas Town WebUI 认证系统审计报告

**审计日期**: 2026-01-24
**审计范围**: `gastown/internal/web/` 目录下所有认证相关代码
**审计人员**: Claude Code

---

## 执行摘要

Gas Town WebUI 当前实现了一套**基于 Token 的单一认证机制**，结合 Cookie 会话和 CSRF 防护。系统**并未实现 OAuth 或多种认证方式**，而是采用统一的环境变量 Token 认证方案。

**核心发现**：
- ❌ 存在多个安全风险点（Token 明文传输、无速率限制、无会话撤销）
- ⚠️ 移动端认证问题源于 Cookie 策略与 HTTPS 配置不匹配
- ⚠️ Dashboard 刷新重复认证与 Cookie 设置有关
- ✅ CSRF 防护实现正确
- ✅ Localhost 限制机制完善

---

## 1. 认证方式分析

### 1.1 当前实现的认证方式

**结论：仅实现了一种认证方式 - Token 认证**

#### 1.1.1 Token 认证（唯一方式）

**环境变量配置**：
```bash
# 启用认证（必需）
GT_WEB_AUTH_TOKEN="your-secret-token"

# 允许远程访问（可选，需同时设置 TOKEN）
GT_WEB_ALLOW_REMOTE=1
```

**支持的认证格式**：

| 方式 | 格式 | 使用场景 | 实现位置 |
|------|------|----------|---------|
| HTTP Header | `Authorization: Bearer <token>` | API 调用、脚本访问 | gui.go:244-247 |
| Cookie Session | `Cookie: gt_session=<SHA256(token)>` | 浏览器持久登录 | gui.go:251-254 |
| URL 参数 | `GET /login?token=<token>` | 移动端快捷登录 | gui.go:314-333 |
| 表单提交 | `POST /login {token: ...}` | 传统登录表单 | gui.go:292-311 |

**认证流程图**：
```
┌─────────────────────────────────────────────────────────┐
│                    请求到达                              │
└───────────────────┬─────────────────────────────────────┘
                    ↓
        ┌───────────────────────┐
        │ 是 /login, /logout    │ ──→ YES ──→ 放行
        │ 或 /static/* ?        │
        └───────────┬───────────┘
                    ↓ NO
        ┌───────────────────────┐
        │ GT_WEB_AUTH_TOKEN     │ ──→ NO ──→ 检查 Localhost
        │ 已配置?               │           限制
        └───────────┬───────────┘
                    ↓ YES
        ┌───────────────────────────────────┐
        │ 检查认证:                          │
        │ 1. Authorization: Bearer <token>  │
        │ 2. Cookie: gt_session=SHA256(...)│
        └───────────┬───────────────────────┘
                    ↓
            ┌───────────┐
            │ 认证通过? │ ──→ NO ──┐
            └─────┬─────┘           │
                  ↓ YES             ↓
        ┌──────────────────┐  ┌──────────────┐
        │ 是页面请求?      │  │ 页面请求?    │
        │ (Accept: html)   │  │              │
        └─────┬────────────┘  └──────┬───────┘
              ↓ NO                   ↓
        ┌──────────────┐      ┌──────────────┐
        │ POST/PUT/    │      │ 302 重定向到 │
        │ PATCH/DELETE?│      │ /login       │
        └─────┬────────┘      └──────────────┘
              ↓ YES                  ↓
        ┌──────────────┐      ┌──────────────┐
        │ 验证 CSRF    │      │ 401          │
        │ Token        │      │ Unauthorized │
        └─────┬────────┘      └──────────────┘
              ↓
        [ 路由处理 ]
```

### 1.2 未实现的认证方式

经过全面审计，**以下认证方式均未实现**：

- ❌ **OAuth 2.0 / OpenID Connect**（无外部身份提供者集成）
- ❌ **HTTP Basic Auth**（无 `WWW-Authenticate` 响应头）
- ❌ **多因素认证 (MFA)**
- ❌ **JWT Token 自签发**（仅使用固定环境变量 Token）
- ❌ **密码认证**（代码中存在 `allowPasswordAuth` 字段但标注为"仅用于本地开发"且未实现）

### 1.3 "浏览器弹窗要求账号密码"问题溯源

**问题现象**：用户报告有时会遇到浏览器原生的认证弹窗

**调查结果**：

1. **WebUI 代码本身不会触发浏览器弹窗**
   - 未找到任何 `WWW-Authenticate` 响应头设置
   - 未找到任何 HTTP 401 Basic Auth 挑战
   - 认证失败时：
     - 页面请求 → 302 重定向到 `/login`（gui.go:213）
     - API 请求 → 401 纯文本响应（gui.go:218）

2. **可能的真实原因**：

   **情况 A：反向代理配置**
   ```nginx
   # 用户的 nginx/Caddy 可能配置了 Basic Auth
   location / {
       auth_basic "Gas Town Admin";
       auth_basic_user_file /etc/nginx/.htpasswd;
       proxy_pass http://localhost:8080;
   }
   ```

   **情况 B：浏览器缓存的认证凭据**
   - 浏览器可能记住了之前的 Basic Auth 凭据
   - 在某个配置变更前的版本可能有过 Basic Auth

   **情况 C：Account 登录流程混淆**
   - `handler_accounts.go:428` 创建 tmux 会话运行 `claude --dangerously-skip-permissions`
   - 这个命令会启动 Claude CLI，可能触发 Claude 自己的认证流程
   - 用户可能将此认证流程误认为 WebUI 的认证

**建议**：
1. 检查反向代理配置文件（nginx.conf, Caddyfile）
2. 清除浏览器缓存和已保存密码
3. 确认 `/api/accounts/login/start` 触发的 Claude CLI 登录流程

---

## 2. 认证实现详细分析

### 2.1 Token 生成与存储

#### 2.1.1 环境变量 Token（主 Token）

**读取位置**: `gui.go:44`
```go
authConfig = struct {
    token       string
    allowRemote bool
}{
    token:       os.Getenv("GT_WEB_AUTH_TOKEN"),
    allowRemote: os.Getenv("GT_WEB_ALLOW_REMOTE") == "1",
}
```

**安全特性**：
- ✅ 存储在进程环境变量中（不在代码中硬编码）
- ✅ 启动时校验：`allowRemote=1` 必须配套 `token`（gui.go:55-56）
- ⚠️ **风险**：环境变量可被同用户的其他进程读取（/proc/PID/environ）

#### 2.1.2 Session Cookie Token（派生 Token）

**生成算法**: `gui.go:283-288`
```go
func generateSessionToken(token string) string {
    hash := sha256.Sum256([]byte("gt-session:" + token))
    return hex.EncodeToString(hash[:])
}
```

**安全特性**：
- ✅ 使用 SHA256 单向哈希（Cookie 中不存储原始 Token）
- ✅ 加盐（固定前缀 `"gt-session:"`）
- ⚠️ **缺陷**：盐值固定且硬编码，所有用户共享同一盐值

**Cookie 属性**: `gui.go:296-304`
```go
http.SetCookie(w, &http.Cookie{
    Name:     "gt_session",
    Value:    generateSessionToken(token),
    Path:     "/",
    HttpOnly: true,           // ✅ 防止 JavaScript 访问
    Secure:   r.TLS != nil,   // ⚠️ 仅在 HTTPS 时设置
    SameSite: http.SameSiteLaxMode,  // ✅ CSRF 防护
    MaxAge:   86400 * 30,     // ⚠️ 30 天有效期
})
```

**问题分析**：
| 属性 | 配置 | 问题 | 影响 |
|------|------|------|------|
| `Secure` | 动态（仅 HTTPS） | HTTP 环境下 Cookie 可被网络监听 | 🔴 高风险 |
| `MaxAge` | 30 天 | 无主动撤销机制 | 🟡 中风险 |
| `Path` | `/` | 全站共享 | ✅ 正常 |
| `SameSite` | Lax | POST 跨站请求会携带 Cookie | 🟡 中风险 |

### 2.2 CSRF 防护机制

#### 2.2.1 CSRF Token 实现

**生成**: `csrf.go:60-66`
```go
func newCSRFToken() (string, error) {
    secret := make([]byte, 32)
    if _, err := rand.Read(secret); err != nil {
        return "", err
    }
    return hex.EncodeToString(secret), nil
}
```

**验证**: `csrf.go:42-58`
```go
func validateCSRF(r *http.Request) bool {
    cookie, err := r.Cookie("gt_csrf")
    if err != nil || cookie.Value == "" {
        return false
    }

    header := r.Header.Get("X-CSRF-Token")
    if header == "" {
        return false
    }

    // ✅ 使用恒定时间比较防止时序攻击
    return subtle.ConstantTimeCompare(
        []byte(header),
        []byte(cookie.Value)
    ) == 1
}
```

**客户端自动注入**: `gastown.js:299-352`
```javascript
window.fetch = (input, init = {}) => {
    // ... 解析请求 ...

    if (isStateChangingMethod(method) && isSameOriginUrl(url)) {
        const token = getCSRFToken();
        if (token && !headers.has('X-CSRF-Token')) {
            headers.set('X-CSRF-Token', token);
        }
    }

    return originalFetch(input, finalInit);
};
```

**安全评估**：
- ✅ 使用加密安全随机数生成器（`crypto/rand`）
- ✅ Double Submit Cookie 模式正确实现
- ✅ 恒定时间比较防止时序攻击
- ⚠️ **问题**：CSRF Cookie 的 `HttpOnly=false`（csrf.go:35）
  - 允许 JavaScript 读取 Cookie
  - XSS 攻击可窃取 CSRF Token
  - **建议**：改为服务端在 HTML 中注入 Token（如 `<meta>` 标签）

### 2.3 Localhost 限制机制

**实现**: `gui.go:592-627`

**支持的 Localhost 判定**：
1. IPv4 回环：`127.0.0.1`
2. IPv6 回环：`::1`
3. 域名：`localhost`
4. 任意回环 IP（`net.IP.IsLoopback()`）
5. 代理模式：信任 `X-Forwarded-For` 的第一个 IP（仅在 `allowRemote=1` 时）

**安全问题**：
```go
// gui.go:612-623
if authConfig.allowRemote {
    forwarded := r.Header.Get("X-Forwarded-For")
    if forwarded != "" {
        parts := strings.Split(forwarded, ",")
        forwardedIP := net.ParseIP(strings.TrimSpace(parts[0]))
        if forwardedIP != nil && forwardedIP.IsLoopback() {
            return true
        }
    }
}
```

**风险分析**：
- ⚠️ **X-Forwarded-For 伪造风险**
  - 当 `allowRemote=1` 时，信任客户端提供的 `X-Forwarded-For` 头
  - 攻击者可伪造：`X-Forwarded-For: 127.0.0.1` 绕过限制
  - **建议**：仅在确认有可信代理时启用（如 nginx 配置了 `proxy_set_header`）

---

## 3. Dashboard 刷新重复认证问题分析

### 3.1 问题现象

用户报告：在 Dashboard 界面，每次刷新（F5）都会重新要求认证

### 3.2 可能的根本原因

#### 原因 1：Cookie 的 Secure 标志问题

**代码位置**: `gui.go:301`
```go
Secure: r.TLS != nil,  // 仅在 HTTPS 请求时设置 Secure=true
```

**问题场景**：
```
场景 A：HTTP 环境（本地开发）
┌──────────────────────────────────────────────────────┐
│ 1. 用户通过 HTTP 访问: http://localhost:8080/login  │
│ 2. 登录成功，设置 Cookie: Secure=false              │
│ 3. Cookie 正常工作 ✅                                │
└──────────────────────────────────────────────────────┘

场景 B：HTTPS 环境（生产）
┌──────────────────────────────────────────────────────┐
│ 1. 用户通过 HTTPS 访问: https://example.com/login   │
│ 2. 登录成功，设置 Cookie: Secure=true               │
│ 3. Cookie 正常工作 ✅                                │
└──────────────────────────────────────────────────────┘

场景 C：混合环境（问题场景）⚠️
┌──────────────────────────────────────────────────────┐
│ 1. 反向代理: HTTPS → WebUI: HTTP                    │
│ 2. r.TLS = nil (WebUI 看到的是 HTTP)               │
│ 3. 设置 Cookie: Secure=false                        │
│ 4. 浏览器因外层是 HTTPS，拒绝接受 Secure=false     │
│ 5. Cookie 未设置 ❌                                 │
│ 6. 刷新时需要重新登录                               │
└──────────────────────────────────────────────────────┘
```

**诊断命令**：
```bash
# 检查 Cookie 是否正确设置
curl -i http://localhost:8080/login -d "token=YOUR_TOKEN"

# 应看到 Set-Cookie 响应头
Set-Cookie: gt_session=...; Path=/; HttpOnly; SameSite=Lax; Max-Age=2592000
```

#### 原因 2：浏览器隐私设置

**移动端 Safari/Chrome 的隐私保护**：
- 默认阻止跨站 Cookie
- 私密浏览模式阻止所有 Cookie
- "阻止所有 Cookie"设置

**验证方法**：
1. 打开浏览器开发者工具 → Application → Cookies
2. 检查 `gt_session` Cookie 是否存在
3. 检查 `gt_csrf` Cookie 是否存在

#### 原因 3：Cookie Domain 冲突

**当前实现**: 未显式设置 `Domain` 属性
```go
http.SetCookie(w, &http.Cookie{
    Name:     "gt_session",
    // Domain: "",  // 未设置，默认为当前主机
    ...
})
```

**问题场景**：
- 用户先访问 `http://192.168.1.100:8080`（设置 Cookie for 192.168.1.100）
- 后访问 `http://localhost:8080`（Cookie 不匹配）
- 需要重新登录

#### 原因 4：移动端浏览器的后台清理

**iOS Safari / Android Chrome**：
- 内存不足时清除非活动标签页的 Cookie
- 后台一段时间后自动清理会话数据

### 3.3 解决方案建议

#### 解决方案 1：修复 Secure 标志检测

**代码改进**：
```go
// 检查是否通过 HTTPS 访问（考虑反向代理）
func isTLS(r *http.Request) bool {
    // 直接 TLS 连接
    if r.TLS != nil {
        return true
    }

    // 通过反向代理的 HTTPS
    if r.Header.Get("X-Forwarded-Proto") == "https" {
        return true
    }
    if r.Header.Get("X-Forwarded-Ssl") == "on" {
        return true
    }

    return false
}

// 使用改进的检测
http.SetCookie(w, &http.Cookie{
    Secure: isTLS(r),
    ...
})
```

#### 解决方案 2：增加调试日志

```go
func (h *GUIHandler) isAuthenticated(r *http.Request) bool {
    // 检查 Bearer Token
    auth := r.Header.Get("Authorization")
    if auth == "Bearer "+authConfig.token {
        log.Printf("Auth: Bearer token valid for %s", r.RemoteAddr)
        return true
    }

    // 检查 Session Cookie
    cookie, err := r.Cookie(sessionCookieName)
    if err != nil {
        log.Printf("Auth: No session cookie for %s: %v", r.RemoteAddr, err)
        return false
    }

    expected := generateSessionToken(authConfig.token)
    if cookie.Value == expected {
        log.Printf("Auth: Session cookie valid for %s", r.RemoteAddr)
        return true
    }

    log.Printf("Auth: Session cookie mismatch for %s", r.RemoteAddr)
    return false
}
```

#### 解决方案 3：支持 Bearer Token 持久化

**为移动端提供 Bearer Token 存储机制**：
```javascript
// 登录时存储 Token 到 localStorage
localStorage.setItem('gt_bearer_token', token);

// 每次请求自动附加
fetch(url, {
    headers: {
        'Authorization': 'Bearer ' + localStorage.getItem('gt_bearer_token')
    }
});
```

---

## 4. 移动端认证问题分析

### 4.1 移动登录链接机制

**实现位置**: `gui.go:524-588`

**功能**：
1. 用户在登录页面输入 Token
2. JavaScript 生成包含 Token 的 URL：`https://example.com/login?token=SECRET`
3. 支持复制链接或原生分享（`navigator.share()`）

**安全风险** 🔴：

| 风险点 | 描述 | 危害等级 |
|--------|------|---------|
| URL 明文 Token | Token 出现在 URL 中 | 高 |
| 浏览器历史记录 | Token 被保存在浏览器历史 | 高 |
| 服务器访问日志 | Token 被记录在 access.log | 高 |
| 代理日志 | 反向代理/CDN 记录完整 URL | 高 |
| 浏览器分享功能 | Token 可能被分享到第三方应用 | 中 |

**代码示例**（问题代码）：
```javascript
// gui.go:550-552
const url = new URL(window.location.href);
url.searchParams.set('token', token);  // ❌ Token 暴露在 URL
linkInput.value = url.toString();
```

**攻击场景**：
```
1. 用户生成移动登录链接:
   https://gastown.example.com/login?token=super-secret-token-12345

2. 用户通过某个 IM 应用分享链接给自己的手机

3. IM 服务器记录了这个 URL（包含 Token）

4. 攻击者入侵 IM 服务器，获取所有包含 token= 的 URL

5. 攻击者使用窃取的 Token 访问系统
```

### 4.2 移动端 Cookie 问题

**问题 1：iOS Safari 的智能防跟踪**
- 阻止跨站 Cookie（即使 SameSite=Lax）
- 7 天后过期第三方 Cookie

**问题 2：Android Chrome 的第三方 Cookie 限制**
- 默认阻止第三方 Cookie
- 隐私模式下阻止所有 Cookie

**问题 3：移动浏览器的后台清理**
- 后台 15 分钟后清除非活动标签页数据
- 低内存时优先清理 Cookie

### 4.3 改进建议

#### 建议 1：使用一次性登录码（OTC - One-Time Code）

**替代移动登录链接的实现**：

```go
// 生成一次性登录码
type LoginCode struct {
    Code      string
    ExpiresAt time.Time
    Used      bool
}

var loginCodes = sync.Map{}

func generateLoginCode() string {
    code := randomString(8)  // 如: GT-ABC123
    loginCodes.Store(code, LoginCode{
        Code:      code,
        ExpiresAt: time.Now().Add(5 * time.Minute),
        Used:      false,
    })
    return code
}

// 使用登录码登录
func (h *GUIHandler) handleLoginWithCode(w http.ResponseWriter, r *http.Request) {
    code := r.URL.Query().Get("code")

    val, ok := loginCodes.Load(code)
    if !ok {
        http.Error(w, "Invalid code", 400)
        return
    }

    loginCode := val.(LoginCode)
    if loginCode.Used || time.Now().After(loginCode.ExpiresAt) {
        http.Error(w, "Code expired", 400)
        return
    }

    // 标记为已使用
    loginCode.Used = true
    loginCodes.Store(code, loginCode)

    // 设置 Session Cookie
    setSessionCookie(w, r)
    http.Redirect(w, r, "/", 302)
}
```

**优势**：
- ✅ 一次性使用，无法重放
- ✅ 短期有效（5 分钟）
- ✅ 代码简短，易于手动输入（8 位）
- ✅ 日志泄露影响小

#### 建议 2：实现 QR 码登录

**流程**：
```
Desktop                          Mobile
   │                               │
   │  1. 生成 QR 码                │
   │     (包含一次性 session_id)   │
   │                               │
   │  2. 显示 QR 码                │
   ├──────────────────────────────>│
   │                               │  3. 扫描 QR 码
   │                               │
   │  4. 输入 Token                │
   │<───────────────────────────────│
   │                               │
   │  5. WebSocket 推送登录状态    │
   │                               │
   │  6. 桌面端自动登录            │
```

---

## 5. 安全风险评估

### 5.1 高危风险 🔴

| 风险 ID | 风险描述 | 影响 | 位置 | 建议 |
|---------|---------|------|------|------|
| SEC-001 | URL 参数传递 Token | Token 泄露到日志/历史记录 | gui.go:314-333 | 禁用 URL Token，改用一次性登录码 |
| SEC-002 | 无速率限制 | 暴力破解 Token | gui.go:292-311 | 实现登录失败锁定机制 |
| SEC-003 | CSRF Cookie 可被 JS 读取 | XSS 攻击窃取 CSRF Token | csrf.go:35 | 改为服务端注入 Token 到 HTML |
| SEC-004 | X-Forwarded-For 信任问题 | 绕过 Localhost 限制 | gui.go:612-623 | 仅在明确配置时信任代理头 |
| SEC-005 | 无会话撤销机制 | Token 泄露后无法撤销 | gui.go:303 | 实现会话管理接口 |

### 5.2 中危风险 🟡

| 风险 ID | 风险描述 | 影响 | 位置 | 建议 |
|---------|---------|------|------|------|
| SEC-101 | 会话有效期过长（30 天） | 增加 Token 泄露窗口期 | gui.go:303 | 缩短至 7 天，提供"记住我"选项 |
| SEC-102 | 固定盐值 | 彩虹表攻击（理论风险） | gui.go:286 | 使用每用户随机盐 |
| SEC-103 | 无登录审计日志 | 无法追溯安全事件 | - | 记录所有认证尝试 |
| SEC-104 | SameSite=Lax | CSRF 风险（POST 跨站） | gui.go:302 | 改为 SameSite=Strict |
| SEC-105 | Secure 标志检测不完善 | 反向代理环境 Cookie 失效 | gui.go:301 | 检查 X-Forwarded-Proto |

### 5.3 低危风险 🟢

| 风险 ID | 风险描述 | 影响 | 位置 | 建议 |
|---------|---------|------|------|------|
| SEC-201 | 无密码复杂度要求 | N/A（当前无密码认证） | - | 如实现密码认证，需强制复杂度 |
| SEC-202 | 无多因素认证 | 单点失效风险 | - | 考虑 TOTP/WebAuthn |
| SEC-203 | 环境变量 Token 可被读取 | 同用户进程可读 /proc | - | 考虑文件存储 Token |

### 5.4 安全加固优先级

**Phase 1（立即修复）**：
1. 禁用 URL Token 传递（SEC-001）
2. 实现登录速率限制（SEC-002）
3. 修复 Secure 标志检测（SEC-105）

**Phase 2（短期改进）**：
1. 实现一次性登录码（移动端）
2. 添加会话管理接口（列表/撤销）
3. 实现登录审计日志

**Phase 3（长期规划）**：
1. 支持 OAuth 2.0（Google/GitHub 登录）
2. 实现 WebAuthn（硬件密钥/生物识别）
3. 支持基于角色的访问控制（RBAC）

---

## 6. 认证流程混乱问题分析

### 6.1 用户困惑的根源

根据审计发现，认证流程混乱主要源于**两个独立的认证系统混在一起**：

#### 系统 1：WebUI 认证（Gas Town 自身）
- Token 认证（环境变量）
- 登录页面：`/login`
- Session Cookie：`gt_session`

#### 系统 2：Claude CLI 认证（Anthropic）
- **代码位置**: `handler_accounts.go:428`
- **触发时机**: 点击 Account 页面的"Login"按钮
- **执行命令**: `claude --dangerously-skip-permissions`
- **认证方式**: Claude CLI 的 OAuth 流程（浏览器弹窗）

```go
// handler_accounts.go:428
cmd, cancel := command("tmux", "new-session", "-d", "-s", sessionID,
    "-c", configDir,
    "env", "CLAUDE_CONFIG_DIR="+configDir,
    "claude", "--dangerously-skip-permissions")  // ← 这里触发 Claude 认证
```

**用户体验问题**：
1. 用户登录 WebUI（输入 GT_WEB_AUTH_TOKEN）
2. 进入 Account 页面，点击"Login"（期望是切换账户）
3. **突然弹出浏览器窗口要求 Anthropic 账号密码**
4. 用户困惑：我不是已经登录了吗？为什么还要登录？

### 6.2 Account 登录流程详解

**完整流程图**：
```
用户点击 "Login" 按钮
         ↓
  POST /api/accounts/login/start
         ↓
  创建 tmux 会话: gt-login-<handle>
         ↓
  执行命令: claude --dangerously-skip-permissions
         ↓
  Claude CLI 检测到无凭据
         ↓
  启动 OAuth 流程: 打开浏览器 → claude.ai
         ↓
  【浏览器弹窗】要求输入 Anthropic 账号密码
         ↓
  用户登录 Anthropic 账户
         ↓
  Claude CLI 保存凭据到 ~/.claude-accounts/<handle>/.credentials.json
         ↓
  前端轮询检测 .credentials.json 文件
         ↓
  显示 "Logged In" 状态
```

**代码证据**：
```go
// handler_accounts.go:328-338
func accountHasCredentials(configDir string) bool {
    credPath := filepath.Join(configDir, ".credentials.json")
    info, err := os.Stat(credPath)
    if err != nil || info.IsDir() {
        return false
    }
    return info.Size() > 0  // 检查文件存在且非空
}
```

### 6.3 改进建议

#### 建议 1：明确区分两种认证

**UI 改进**：
```html
<!-- Account 页面 -->
<div class="auth-section">
    <h3>Gas Town WebUI Access</h3>
    <p class="status">✅ Authenticated as Admin (Token Auth)</p>
    <button onclick="logout()">Logout from WebUI</button>
</div>

<div class="auth-section">
    <h3>Anthropic Claude Account</h3>
    <p class="status">
        {{ if .LoggedIn }}
            ✅ Logged in as {{ .Email }}
        {{ else }}
            ❌ Not logged in
        {{ end }}
    </p>
    <button onclick="loginClaude()">
        {{ if .LoggedIn }}
            Switch Claude Account
        {{ else }}
            Login to Claude
        {{ end }}
    </button>
    <p class="hint">
        This will open a browser window to authenticate with Anthropic.
        You will need your Claude.ai credentials.
    </p>
</div>
```

#### 建议 2：预警提示

**在点击前显示确认对话框**：
```javascript
function loginClaude(handle) {
    if (!confirm(
        'This will open a new browser window to log in to your Anthropic Claude account.\n\n' +
        'This is separate from your Gas Town WebUI login.\n\n' +
        'Continue?'
    )) {
        return;
    }

    // 执行登录...
}
```

#### 建议 3：文档说明

**在 docs/ 添加认证指南**：
```markdown
# Gas Town 认证指南

## 两种独立的认证系统

### 1. WebUI 访问认证
- **用途**: 控制谁可以访问 Gas Town WebUI
- **配置**: 设置 `GT_WEB_AUTH_TOKEN` 环境变量
- **登录位置**: /login 页面
- **凭据**: 自定义 Token（任意字符串）

### 2. Claude CLI 账户认证
- **用途**: 关联你的 Anthropic Claude 账户
- **配置**: Account 页面点击 "Login"
- **登录位置**: 浏览器弹窗 → claude.ai
- **凭据**: Anthropic 账户邮箱和密码

## 常见混淆

Q: 我已经登录 WebUI 了，为什么还要输入账号密码？
A: WebUI 登录和 Claude 账户登录是两个独立的系统。
   WebUI 登录让你访问界面，Claude 登录让系统能调用 AI。
```

---

## 7. 代码质量评估

### 7.1 优点 ✅

1. **CSRF 防护实现正确**
   - 使用加密安全随机数
   - Double Submit Cookie 模式
   - 恒定时间比较

2. **Session Token 哈希存储**
   - 不在 Cookie 中存储原始 Token
   - 使用 SHA256 单向哈希

3. **Localhost 限制完善**
   - 支持多种回环地址表示
   - 正确处理 IPv4/IPv6

4. **认证中间件清晰**
   - Fail-closed 策略（默认拒绝）
   - 明确的路由豁免逻辑

### 7.2 缺陷 ❌

1. **缺少速率限制**
   - 无登录尝试次数限制
   - 无 IP 封禁机制

2. **缺少会话管理**
   - 无法列出活动会话
   - 无法撤销特定会话
   - 无会话活动记录

3. **缺少审计日志**
   - 认证成功/失败不记录
   - 无法追溯安全事件

4. **错误处理不统一**
   - 有时返回 401，有时重定向
   - 错误信息不够详细

### 7.3 代码改进建议

#### 改进 1：实现速率限制

```go
// rate_limit.go
type RateLimiter struct {
    attempts sync.Map  // key: IP, value: []time.Time
}

func (rl *RateLimiter) CheckLogin(ip string) error {
    val, _ := rl.attempts.LoadOrStore(ip, []time.Time{})
    attempts := val.([]time.Time)

    // 清除 15 分钟前的记录
    now := time.Now()
    cutoff := now.Add(-15 * time.Minute)
    attempts = filterAfter(attempts, cutoff)

    // 检查是否超过 5 次
    if len(attempts) >= 5 {
        return errors.New("too many login attempts, try again in 15 minutes")
    }

    // 记录本次尝试
    attempts = append(attempts, now)
    rl.attempts.Store(ip, attempts)

    return nil
}
```

#### 改进 2：实现会话管理

```go
// session.go
type SessionManager struct {
    sessions sync.Map  // key: sessionID, value: SessionInfo
}

type SessionInfo struct {
    ID        string
    UserAgent string
    IP        string
    CreatedAt time.Time
    LastSeen  time.Time
}

func (sm *SessionManager) CreateSession(r *http.Request) string {
    sessionID := generateSessionID()
    sm.sessions.Store(sessionID, SessionInfo{
        ID:        sessionID,
        UserAgent: r.Header.Get("User-Agent"),
        IP:        r.RemoteAddr,
        CreatedAt: time.Now(),
        LastSeen:  time.Now(),
    })
    return sessionID
}

func (sm *SessionManager) ListSessions() []SessionInfo {
    var sessions []SessionInfo
    sm.sessions.Range(func(key, value interface{}) bool {
        sessions = append(sessions, value.(SessionInfo))
        return true
    })
    return sessions
}

func (sm *SessionManager) RevokeSession(sessionID string) {
    sm.sessions.Delete(sessionID)
}
```

#### 改进 3：实现审计日志

```go
// audit.go
type AuditLogger struct {
    file *os.File
}

func (al *AuditLogger) LogAuth(event string, ip string, success bool) {
    log := fmt.Sprintf("[%s] %s from %s: %v\n",
        time.Now().Format(time.RFC3339),
        event, ip, success)

    al.file.WriteString(log)
}

// 使用示例
func (h *GUIHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
    token := r.FormValue("token")
    ip := r.RemoteAddr

    if token == authConfig.token {
        h.auditLogger.LogAuth("LOGIN", ip, true)
        // ...
    } else {
        h.auditLogger.LogAuth("LOGIN_FAILED", ip, false)
        // ...
    }
}
```

---

## 8. 总结与建议

### 8.1 核心问题总结

| 问题 | 根本原因 | 严重程度 | 建议优先级 |
|------|---------|---------|-----------|
| 浏览器弹窗认证 | Claude CLI OAuth 与 WebUI 认证混淆 | 低（体验问题） | P2 |
| Dashboard 刷新重认证 | Cookie Secure 标志检测不完善 | 中（功能缺陷） | P1 |
| 移动端认证失败 | URL Token 不安全 + Cookie 兼容性 | 高（安全+体验） | P1 |
| 认证整体混乱 | 缺少文档和 UI 提示 | 中（体验问题） | P2 |

### 8.2 立即行动项（P0 - 本周完成）

1. **禁用 URL Token 传递**
   - 移除 `GET /login?token=<token>` 功能
   - 实现一次性登录码机制

2. **修复 Secure Cookie 检测**
   - 检查 `X-Forwarded-Proto` 头
   - 支持反向代理环境

3. **添加登录速率限制**
   - 每 IP 每 15 分钟最多 5 次尝试
   - 失败后显示剩余锁定时间

### 8.3 短期改进项（P1 - 本月完成）

1. **实现会话管理界面**
   - 列出所有活动会话
   - 支持远程撤销会话
   - 显示会话详情（IP、设备、最后活动）

2. **优化 Account 认证体验**
   - 明确区分 WebUI 和 Claude 认证
   - 添加登录前确认提示
   - 显示当前登录的 Claude 账户

3. **添加审计日志**
   - 记录所有登录尝试
   - 记录会话创建/撤销
   - 支持日志导出

### 8.4 长期规划项（P2 - 下季度）

1. **支持多种认证方式**
   - OAuth 2.0（Google/GitHub）
   - SAML 2.0（企业 SSO）
   - WebAuthn（硬件密钥）

2. **实现 RBAC**
   - 管理员、开发者、访客角色
   - 基于角色的页面访问控制
   - API 权限细分

3. **安全加固**
   - 实现 Content Security Policy
   - 添加 Subresource Integrity
   - 支持 HSTS

### 8.5 文档改进建议

创建以下文档：

1. **docs/auth-guide.md** - 用户认证指南
   - 两种认证系统的区别
   - 如何设置 GT_WEB_AUTH_TOKEN
   - 如何登录 Claude 账户
   - 常见问题解答

2. **docs/deployment-guide.md** - 部署指南
   - 反向代理配置示例（nginx/Caddy）
   - HTTPS 证书配置
   - 安全最佳实践

3. **docs/security.md** - 安全策略文档
   - 威胁模型分析
   - 安全配置清单
   - 应急响应流程

---

## 附录 A：认证相关文件清单

### 核心认证文件（按重要性排序）

1. **gui.go** (628 行)
   - 主认证中间件（ServeHTTP）
   - Token 验证逻辑
   - 登录/登出处理
   - Session Cookie 管理
   - Localhost 限制检查

2. **csrf.go** (67 行)
   - CSRF Token 生成
   - CSRF 验证逻辑
   - Cookie 管理

3. **handler_accounts.go** (438 行)
   - Claude CLI 账户管理
   - Tmux 登录会话创建
   - 凭据检测逻辑

4. **security.go** (47 行)
   - Same-origin 请求检测
   - Host/Port 规范化

5. **static/js/gastown.js** (812 行)
   - CSRF Token 客户端注入
   - Cookie 读取工具函数
   - Fetch API 拦截

### 相关模板文件

1. **templates/account.html** (405 行)
   - 账户管理界面
   - Claude 登录按钮

2. **templates/base.html** (47 行)
   - 导航栏（包含登出链接）

### 测试文件

1. **gui_test.go** (253 行)
   - 认证中间件测试
   - 不安全配置拒绝测试

---

## 附录 B：环境变量配置参考

```bash
# ============================================================================
# Gas Town WebUI 认证配置
# ============================================================================

# 必需：设置认证 Token（任意字符串，建议 32+ 字符）
# 推荐生成方法: openssl rand -hex 32
export GT_WEB_AUTH_TOKEN="your-secure-random-token-here"

# 可选：允许非 Localhost 访问（必须同时设置 TOKEN）
# 警告：仅在有反向代理保护时启用
export GT_WEB_ALLOW_REMOTE=1

# ============================================================================
# 反向代理配置示例（nginx）
# ============================================================================

# /etc/nginx/sites-available/gastown
server {
    listen 443 ssl http2;
    server_name gastown.example.com;

    ssl_certificate /etc/letsencrypt/live/gastown.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/gastown.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;  # ← 重要：修复 Secure Cookie

        # WebSocket 支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

---

## 附录 C：安全检查清单

使用此清单验证部署的安全性：

### 基础安全

- [ ] GT_WEB_AUTH_TOKEN 已设置且强度足够（32+ 字符）
- [ ] Token 未提交到 Git 仓库
- [ ] 生产环境使用 HTTPS
- [ ] 反向代理正确设置 X-Forwarded-Proto
- [ ] GT_WEB_ALLOW_REMOTE=1 仅在必要时启用

### Cookie 安全

- [ ] 浏览器 DevTools 检查 Cookie 存在
- [ ] gt_session Cookie 的 Secure 标志正确（HTTPS 时为 true）
- [ ] gt_session Cookie 的 HttpOnly 为 true
- [ ] gt_csrf Cookie 正确生成

### 网络安全

- [ ] 防火墙阻止直接访问 8080 端口（仅允许 localhost）
- [ ] 反向代理配置 rate limiting
- [ ] 访问日志不包含 URL 参数（避免 Token 泄露）

### 审计与监控

- [ ] 定期检查访问日志
- [ ] 监控异常登录尝试
- [ ] 定期轮换 GT_WEB_AUTH_TOKEN

---

**报告结束**

如有疑问或需要进一步的技术支持，请查阅：
- 源代码: `gastown/internal/web/`
- 问题追踪: GitHub Issues
- 技术文档: `docs/`
