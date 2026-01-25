# Gas Town WebUI 部署指南

本指南介绍如何在生产环境中部署 Gas Town WebUI，包括认证配置和 Token 管理。

---

## 快速开始

### 1. 生成认证 Token

在**你的本地机器或服务器**上运行：

```bash
# 生成一个 64 字符的强随机 Token
openssl rand -hex 32
```

**示例输出**：
```
3da1d34a4622321f06c87e8531e71a970b39a855d964485f16a1f592930a732d
```

**⚠️ 重要**：
- 将这个 Token 保存到密码管理器（1Password/Bitwarden）
- 这是你访问 WebUI 时需要输入的密码
- **不要提交到 Git 仓库！**

### 2. 创建环境变量文件

在**服务器**上运行：

```bash
# 复制示例文件
sudo cp /home/shisui/laplace/gastown-src/deploy/gastown.env.example /etc/gastown.env

# 编辑文件，填入真实的 Token
sudo nano /etc/gastown.env
```

**填入内容**：
```bash
# WebUI 认证 Token（替换为上面生成的 Token）
GT_WEB_AUTH_TOKEN=3da1d34a4622321f06c87e8531e71a970b39a855d964485f16a1f592930a732d

# 允许远程访问（如果有反向代理）
GT_WEB_ALLOW_REMOTE=1
```

**设置文件权限**（重要！）：
```bash
# 仅 root 可读写，其他用户无权限
sudo chmod 600 /etc/gastown.env

# 验证权限
ls -l /etc/gastown.env
# 应该显示: -rw------- 1 root root ...
```

### 3. 部署 systemd service

```bash
# 复制 service 文件
sudo cp /home/shisui/laplace/gastown-src/deploy/gastown-web.service /etc/systemd/system/

# 重新加载 systemd
sudo systemctl daemon-reload

# 启动服务
sudo systemctl restart gastown-web

# 检查状态
sudo systemctl status gastown-web

# 设置开机自启
sudo systemctl enable gastown-web
```

### 4. 验证部署

```bash
# 测试 1：访问 Dashboard 应该被重定向到 /login
curl -I http://localhost:8080/dashboard

# 应该看到：
# HTTP/1.1 302 Found
# Location: /login

# 测试 2：访问 /login 应该看到登录页面
curl http://localhost:8080/login | grep "Access Token"

# 应该看到包含 "Access Token" 的 HTML
```

### 5. 浏览器登录

1. 访问你的域名（如 `https://gt.ananthe.party`）
2. 如果有反向代理的 Basic Auth，先输入用户名和密码
3. 看到 WebUI Login 页面后，输入你的 Token
4. 成功登录到 Dashboard

---

## Token 管理

### 如何查看当前的 Token

**方法 1：查看环境变量文件**（推荐）
```bash
sudo cat /etc/gastown.env | grep GT_WEB_AUTH_TOKEN
```

**方法 2：查看 service 文件**（如果直接在 service 中设置）
```bash
sudo grep GT_WEB_AUTH_TOKEN /etc/systemd/system/gastown-web.service
```

**方法 3：从运行中的进程查看**
```bash
sudo cat /proc/$(pgrep -f "gt gui")/environ | tr '\0' '\n' | grep GT_WEB_AUTH_TOKEN
```

### Token 什么时候会改变？

| 操作 | Token 会变吗？ | 说明 |
|------|--------------|------|
| 重启服务 | ❌ 不会 | Token 在配置文件中，不受影响 |
| 重新部署代码 | ❌ 不会 | Token 存储在服务器本地，不在代码里 |
| 服务器重启 | ❌ 不会 | systemd 会加载同样的配置 |
| 更新 service 文件 | ⚠️ 可能 | 如果你改了 service 文件但没同步 /etc/gastown.env |
| 手动修改配置 | ✅ 会变 | 只有你手动改了 /etc/gastown.env 才会变 |

### 如何更换 Token

```bash
# 1. 生成新 Token
openssl rand -hex 32

# 2. 编辑配置文件
sudo nano /etc/gastown.env

# 3. 替换 GT_WEB_AUTH_TOKEN 的值

# 4. 重启服务
sudo systemctl restart gastown-web

# 5. 更新密码管理器中的 Token

# 6. 所有已登录的会话会失效，需要用新 Token 重新登录
```

### Token 存储位置

**推荐位置**（按优先级排序）：

1. **密码管理器**（最佳）
   - 1Password / Bitwarden / LastPass
   - 可跨设备同步
   - 有密码生成器和自动填充

2. **服务器上的配置文件**（部署必需）
   - `/etc/gastown.env`
   - 仅 root 可读（`chmod 600`）
   - 不提交到 Git

3. **安全笔记**（备份）
   - 加密的笔记软件（如 Obsidian + 加密）
   - 实体保险柜

**❌ 不要存储的位置**：
- ❌ Git 仓库（即使是私有仓库）
- ❌ 未加密的文本文件
- ❌ 浏览器书签
- ❌ 聊天记录（Slack/Discord/微信）
- ❌ 邮件
- ❌ 云笔记（Notion/Evernote，除非企业版有加密）

---

## 反向代理配置

### 推荐架构

**只使用 WebUI Token 认证**（推荐）：
```
互联网
  ↓
HTTPS (Let's Encrypt)
  ↓
反向代理 (Nginx/Caddy)
  ↓
WebUI Token 认证 ← 【唯一的认证层】
  ↓
Dashboard
```

### Nginx 配置示例

**/etc/nginx/sites-available/gastown**:
```nginx
server {
    listen 443 ssl http2;
    server_name gt.ananthe.party;

    # SSL 证书
    ssl_certificate /etc/letsencrypt/live/gt.ananthe.party/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/gt.ananthe.party/privkey.pem;

    # ❌ 不要在这里设置 auth_basic（使用 WebUI 的 Token 认证）

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        # ⚠️ 重要：让 WebUI 知道这是 HTTPS 请求
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket 支持（用于实时更新）
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # 超时设置
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # 安全头（可选但推荐）
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
}

# HTTP 重定向到 HTTPS
server {
    listen 80;
    server_name gt.ananthe.party;
    return 301 https://$server_name$request_uri;
}
```

**应用配置**：
```bash
# 测试配置
sudo nginx -t

# 重新加载
sudo systemctl reload nginx
```

### Caddy 配置示例

**/etc/caddy/Caddyfile**:
```caddyfile
gt.ananthe.party {
    # Caddy 自动处理 HTTPS（Let's Encrypt）

    # ❌ 不要在这里设置 basicauth（使用 WebUI 的 Token 认证）

    reverse_proxy localhost:8080 {
        # 自动设置 X-Forwarded-* 头
        header_up X-Forwarded-Proto {scheme}
    }

    # 安全头
    header {
        X-Frame-Options "SAMEORIGIN"
        X-Content-Type-Options "nosniff"
        X-XSS-Protection "1; mode=block"
    }
}
```

**应用配置**：
```bash
# 测试配置
sudo caddy validate --config /etc/caddy/Caddyfile

# 重新加载
sudo systemctl reload caddy
```

---

## 常见问题

### Q1: 忘记 Token 了怎么办？

**A**: 查看服务器上的配置文件：
```bash
sudo cat /etc/gastown.env | grep GT_WEB_AUTH_TOKEN
```

如果找不到，可以生成新的并重启服务（会使所有已登录会话失效）。

### Q2: Token 可以短一点吗？

**A**: 可以，但不推荐。Token 越短越容易被暴力破解。

- ✅ 推荐：64 字符（`openssl rand -hex 32`）
- ⚠️ 可接受：32 字符（`openssl rand -hex 16`）
- ❌ 不推荐：16 字符以下

建议使用密码管理器的自动填充功能，长 Token 不影响使用体验。

### Q3: 每次重新部署都要重新登录吗？

**A**: 不需要。只要满足：
1. Token 没变（/etc/gastown.env 没修改）
2. 浏览器 Cookie 没清除
3. 没超过 30 天（Cookie 过期时间）

重新部署（git pull + restart）不会影响已登录的会话。

### Q4: 可以设置多个 Token 吗？

**A**: 当前版本不支持。只能设置一个 Token。

如果需要多用户访问：
- 可以考虑使用同一个 Token
- 或者在反向代理层实现多用户（如 Cloudflare Access）

### Q5: 移动端登录总是失败怎么办？

**A**: 检查以下几点：

1. **Cookie 设置问题**：
   ```bash
   # 检查服务是否正确检测 HTTPS
   curl -I https://gt.ananthe.party/login
   # 应该看到 Set-Cookie: ... Secure; ...
   ```

2. **浏览器隐私设置**：
   - Safari: 设置 → Safari → 阻止跨站跟踪（关闭）
   - Chrome: 设置 → 隐私和安全 → Cookie（允许所有）

3. **使用 Bearer Token**：
   如果 Cookie 不工作，可以使用 API 方式：
   ```javascript
   fetch('https://gt.ananthe.party/api/status', {
       headers: {
           'Authorization': 'Bearer YOUR_TOKEN_HERE'
       }
   })
   ```

### Q6: 如何移除反向代理的 Basic Auth？

**A**: 编辑反向代理配置，删除 auth 相关行：

**Nginx**:
```nginx
# 删除或注释掉这些行：
# auth_basic "Gas Town Admin";
# auth_basic_user_file /etc/nginx/.htpasswd;
```

**Caddy**:
```caddyfile
# 删除或注释掉这个块：
# basicauth / {
#     username $2a$14$...
# }
```

然后重启：
```bash
sudo systemctl reload nginx  # 或 caddy
```

---

## 安全检查清单

部署完成后，确认以下项目：

### 认证配置
- [ ] `GT_WEB_AUTH_TOKEN` 已设置且强度足够（32+ 字符）
- [ ] Token 已保存到密码管理器
- [ ] Token 未提交到 Git 仓库
- [ ] `/etc/gastown.env` 权限为 600（仅 root 可读）

### 网络配置
- [ ] 使用 HTTPS（Let's Encrypt 证书）
- [ ] 反向代理正确设置 `X-Forwarded-Proto`
- [ ] `GT_WEB_ALLOW_REMOTE=1` 已设置（如果有反向代理）
- [ ] 防火墙阻止直接访问 8080 端口（仅允许 localhost）

### 服务配置
- [ ] systemd service 正常运行
- [ ] 服务设置为开机自启
- [ ] 日志正常记录（`journalctl -u gastown-web`）

### 功能测试
- [ ] 访问网站能看到 /login 页面
- [ ] 输入 Token 能成功登录
- [ ] 刷新页面不需要重新登录（Cookie 工作正常）
- [ ] 登出功能正常
- [ ] WebSocket 实时更新正常（Dashboard 自动刷新）

### 定期维护
- [ ] 定期检查访问日志（`journalctl -u gastown-web`）
- [ ] 定期轮换 Token（建议每 3-6 个月）
- [ ] 监控异常登录尝试
- [ ] 定期更新依赖和系统补丁

---

## 故障排查

### 问题 1: 访问网站直接进入 Dashboard，没有登录页面

**诊断**：
```bash
# 检查 Token 是否设置
sudo grep GT_WEB_AUTH_TOKEN /etc/gastown.env

# 检查服务是否加载了环境变量
sudo systemctl show gastown-web | grep GT_WEB_AUTH_TOKEN
```

**解决**：
- 确保 `/etc/gastown.env` 存在且包含 `GT_WEB_AUTH_TOKEN`
- 确保 service 文件包含 `EnvironmentFile=-/etc/gastown.env`
- 重启服务：`sudo systemctl restart gastown-web`

### 问题 2: 刷新页面就要重新登录

**诊断**：
```bash
# 检查浏览器开发者工具 → Application → Cookies
# 应该看到 gt_session 和 gt_csrf Cookie

# 检查 Cookie 的 Secure 标志
curl -I https://gt.ananthe.party/login
```

**解决**：
- 检查反向代理是否设置了 `X-Forwarded-Proto: https`
- 检查浏览器是否阻止了 Cookie（隐私设置）

### 问题 3: 服务启动失败

**诊断**：
```bash
# 查看详细错误
sudo journalctl -u gastown-web -n 50 --no-pager

# 检查配置文件语法
sudo systemd-analyze verify gastown-web.service
```

**常见错误**：
- `GT_WEB_ALLOW_REMOTE=1` 但没有设置 `GT_WEB_AUTH_TOKEN`
- `/etc/gastown.env` 文件格式错误（检查是否有多余空格或引号）

---

## 相关文档

- [认证系统审计报告](./webui-auth-audit-report.md) - 详细的安全分析
- [WebUI 使用指南](./webui-usage-guide.md) - 功能使用说明（TODO）
- [API 文档](./api-reference.md) - API 接口文档（TODO）

---

**更新日期**: 2026-01-24
**维护者**: Gas Town Team
