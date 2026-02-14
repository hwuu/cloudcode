# OpenCode 阿里云一键部署设计文档

## 目录

- [1. 背景与目标](#1-背景与目标)
  - [1.1 问题](#11-问题)
  - [1.2 启发](#12-启发)
  - [1.3 目标](#13-目标)
  - [1.4 非目标](#14-非目标)
- [2. 总体设计](#2-总体设计)
- [3. 设计决策](#3-设计决策)
  - [3.1 云服务选型：为什么选 ECS 而非 SAE](#31-云服务选型为什么选-ecs-而非-sae)
  - [3.2 认证方案：为什么选 Authelia](#32-认证方案为什么选-authelia)
  - [3.3 反向代理：为什么选 Caddy 而非 Nginx](#33-反向代理为什么选-caddy-而非-nginx)
  - [3.4 认证方式：为什么选两步认证而非 Basic Auth](#34-认证方式为什么选两步认证而非-basic-auth)
- [4. 架构设计](#4-架构设计)
  - [4.1 核心分层](#41-核心分层)
  - [4.2 网络架构](#42-网络架构)
  - [4.3 安全架构](#43-安全架构)
- [5. 组件设计](#5-组件设计)
  - [5.1 Deploy Script（部署脚本）](#51-deploy-script部署脚本)
  - [5.2 Docker Compose（容器编排）](#52-docker-compose容器编排)
  - [5.3 Caddy（反向代理 + HTTPS）](#53-caddy反向代理--https)
  - [5.4 Authelia（认证网关）](#54-authelia认证网关)
  - [5.5 OpenCode（AI 编程助手）](#55-opencodeai-编程助手)
- [6. 用户体验流程](#6-用户体验流程)
  - [6.1 部署流程](#61-部署流程)
  - [6.2 首次登录（注册 Passkey）](#62-首次登录注册-passkey)
  - [6.3 日常使用](#63-日常使用)
- [7. 成本估算](#7-成本估算)
- [8. 实现规划](#8-实现规划)
  - [8.1 目录结构](#81-目录结构)
  - [8.2 实现步骤](#82-实现步骤)
  - [8.3 测试要点](#83-测试要点)
- [9. 运维与备份](#9-运维与备份)
- [参考文献](#参考文献)

---

## 1. 背景与目标

### 1.1 问题

在手机或其他移动设备上使用 OpenCode Web UI 时，面临以下安全风险：

| 风险 | 说明 |
|------|------|
| **HTTP 明文传输** | Basic Auth 密码、API Key 通过 HTTP 传输，易被中间人攻击截获 |
| **VM 端口暴露** | SSH (22)、OpenCode (4096) 端口对公网开放，增加攻击面 |
| **缺乏强认证** | Basic Auth 仅一层密码保护，无 2FA 等强认证机制 |
| **无 HTTPS** | 浏览器安全警告，移动端体验差 |

### 1.2 启发

[opencode-cloud](https://github.com/pRizz/opencode-cloud) 项目提供了优秀的实践：

- **两步认证**：WebAuthn/FIDO2 作为第二因素，手机原生支持
- **Docker sandbox**：容器隔离，不污染宿主机
- **一键部署脚本**：自动化资源创建和配置

其局限：

- 仅支持 AWS/Railway/DigitalOcean，不支持阿里云
- 依赖 fork 版本 opencode，可能与 upstream 有差距

### 1.3 目标

构建一个一键式部署工具 **cloudcode**，将 OpenCode 部署到阿里云海外地域：

- **安全访问**：HTTPS + 两步认证（密码 + Passkey），杜绝明文传输
- **端口隔离**：OpenCode 容器不直接暴露，通过反向代理访问
- **一键部署**：自动化创建 ECS、配置 Docker、部署应用
- **低成本**：月费用控制在 $30 以内

### 1.4 非目标

- **不做多用户**：单信任域，仅支持单一用户
- **不做高可用**：单实例部署，无负载均衡
- **不做定时启停**：24/7 运行，不节省闲时成本
- **不做多地域**：固定新加坡地域
- **不做生产级监控**：基础日志收集即可

---

## 2. 总体设计

核心思路：**单机 ECS + Docker Compose + Authelia 两步认证（密码 + Passkey）**。

```
+----------------------------------------------------------------------+
|                              用户浏览器                               |
|                           (HTTPS Only)                               |
+---------------------------------+------------------------------------+
                                  |
                                  v
+----------------------------------------------------------------------+
|                          阿里云 ECS (新加坡)                          |
|                        Ubuntu 24.04 (2C4G, 24/7)                     |
|  +----------------------------------------------------------------+  |
|  |                      Docker Compose                            |  |
|  |                                                                |  |
|  |  +----------------+     +------------------+                   |  |
|  |  |     Caddy      |     |     Authelia     |                   |  |
|  |  |  (Reverse      |---->|   (Auth Gateway) |                   |  |
|  |  |   Proxy)       |     |   • 1FA: 密码    |                   |  |
|  |  |  • Auto HTTPS  |     |   • 2FA: Passkey |                   |  |
|  |  |  • Port: 80/443|     |   • Port: 9091   |                   |  |
|  |  +-------+--------+     +--------+---------+                   |  |
|  |          |                       |                              |  |
|  |          |  Authenticated        |                              |  |
|  |          v                       v                              |  |
|  |  +----------------------------------------------------------------+  |
|  |  |                    OpenCode Container                          |  |
|  |  |  • Port: 4096 (localhost only)                                 |  |
|  |  |  • Workspace: Docker Volume                                    |  |
|  |  +----------------------------------------------------------------+  |
|  |                                                                |  |
|  |  +------------------------------------------------------------+  |  |
|  |  |                    Docker Volumes                           |  |  |
|  |  |  • opencode-workspace (工作区持久化)                         |  |  |
|  |  |  • caddy_data (SSL 证书)                                    |  |  |
|  |  |  • authelia_data (用户/Passkey 注册信息)                    |  |  |
|  |  +------------------------------------------------------------+  |  |
|  +----------------------------------------------------------------+  |
+----------------------------------------------------------------------+
                                  |
                                  v
                        +------------------+
                        |   EIP (公网 IP)   |
                        |   Access via     |
                        |   HTTPS          |
                        +------------------+
```

关键设计决策：

| 决策 | 选择 | 核心理由 | 详见 |
|------|------|----------|------|
| 云服务 | ECS（非 SAE） | 成本低 5 倍 | [3.1](#31-云服务选型为什么选-ecs-而非-sae) |
| 认证组件 | Authelia | 轻量、Passkey 原生支持 | [3.2](#32-认证方案为什么选-authelia) |
| 反向代理 | Caddy | 自动 HTTPS，配置简单 | [3.3](#33-反向代理为什么选-caddy-而非-nginx) |
| 认证方式 | 两步认证（密码 + Passkey） | 比 Basic Auth 更安全 | [3.4](#34-认证方式为什么选两步认证而非-basic-auth) |

---

## 3. 设计决策

### 3.1 云服务选型：为什么选 ECS 而非 SAE

| 维度 | ECS | SAE |
|------|-----|-----|
| 月费用 (2C4G 24/7) | ~$20 | ~$110 |
| HTTPS 配置 | 需自行配置 Caddy | 自动 HTTPS |
| 运维复杂度 | 中（需管理 VM） | 低（托管服务） |
| 持久化存储 | Docker Volume（简单） | NAS（需配置） |
| 冷启动 | 无 | ~30s |

**决策**：选择 ECS。

**理由**：
1. 成本优势明显（$20 vs $110）
2. 单机部署，运维复杂度可接受
3. 更灵活，可自行定制环境

**代价**：
1. 需要手动配置 HTTPS（但 Caddy 自动化程度高）
2. 需要 5-10 分钟初始部署时间

### 3.2 认证方案：为什么选 Authelia

| 维度 | Authelia | Authentik | OAuth2 Proxy |
|------|----------|-----------|--------------|
| 内存占用 | ~50MB | ~200MB | ~30MB |
| Passkey 支持 | ✅ 原生 | ✅ 原生 | ❌ 依赖外部 IdP |
| 配置复杂度 | 中 | 高 | 低 |
| 用户管理 | 配置文件 | Web UI | 外部 IdP |
| 多应用 SSO | 支持 | 支持 | 支持 |

**决策**：选择 Authelia。

**理由**：
1. Passkey 原生支持，无需外部 IdP
2. 资源占用低，适合单机部署
3. 配置文件管理用户，符合单用户场景

**代价**：
1. 添加用户需要修改配置文件并重启容器
2. 无 Web UI 管理界面
3. **Passkey 是 2FA，不是一级认证**（需要先输入密码）

### 3.3 反向代理：为什么选 Caddy 而非 Nginx

| 维度 | Caddy | Nginx |
|------|-------|-------|
| HTTPS 自动化 | Let's Encrypt 自动申请/续期 | 需 certbot 手动配置 |
| 配置文件 | 极简（3-5 行） | 复杂（20+ 行） |
| 性能 | 高 | 极高 |
| 社区资源 | 较少 | 丰富 |

**决策**：选择 Caddy。

**理由**：
1. 自动 HTTPS 是核心需求，Caddy 开箱即用
2. 配置简单，减少脚本复杂度

**代价**：
1. 高级配置（如 Lua 扩展）不如 Nginx 灵活
2. 社区文档少于 Nginx

**关于 nip.io 的说明**：

nip.io 是公共域名服务，所有用户共享 Let's Encrypt 的速率限制（每个注册域名每周 50 张证书）。
- **推荐**：使用自有域名
- **备选**：使用 nip.io（可能因速率限制导致证书签发失败）
- **兜底**：Caddy 自签名证书（浏览器警告但加密）

### 3.4 认证方式：为什么选两步认证而非 Basic Auth

| 维度 | 两步认证 (密码 + Passkey) | Basic Auth | OAuth |
|------|--------------------------|------------|-------|
| 手机支持 | 原生（面容/指纹） | 需输入密码 | 需跳转 |
| 安全性 | 高（密码 + 第二因素） | 低（仅密码） | 高 |
| 抗钓鱼 | 高（域名绑定） | 无 | 中 |
| 配置复杂度 | 中 | 低 | 高 |

**决策**：选择两步认证。

**理由**：
1. 比 Basic Auth 更安全（需要两个因素）
2. Passkey 作为第二因素，手机原生支持
3. 面容/指纹验证体验好

**代价**：
1. 需要先输入密码，再进行 Passkey 验证
2. 首次使用需要注册 Passkey

**重要说明**：Authelia 的 Passkey 是 **2FA（第二因素）**，不是一级认证。
实际流程是：
1. 输入用户名
2. 输入密码（1FA）
3. 使用 Passkey 验证（2FA）
4. 进入应用

---

## 4. 架构设计

### 4.1 核心分层

```
+----------------------------------------------------------------------+
|                           cloudcode 架构                             |
|                                                                      |
|  +----------------------------------------------------------------+  |
|  |  Access Layer (访问层)                                         |  |
|  |  +----------------------------------------------------------+  |  |
|  |  |  Caddy (Reverse Proxy + HTTPS)                           |  |  |
|  |  |  • 自动 HTTPS (Let's Encrypt)                             |  |  |
|  |  |  • 反向代理到 Authelia                                    |  |  |
|  |  +----------------------------------------------------------+  |  |
|  +----------------------------------------------------------------+  |
|                                                                      |
|  +----------------------------------------------------------------+  |
|  |  Auth Layer (认证层)                                          |  |
|  |  +----------------------------------------------------------+  |  |
|  |  |  Authelia (Auth Gateway)                                  |  |  |
|  |  |  • 1FA: 用户名 + 密码                                     |  |  |
|  |  |  • 2FA: Passkey/WebAuthn                                  |  |  |
|  |  |  • Session 管理                                           |  |  |
|  |  +----------------------------------------------------------+  |  |
|  +----------------------------------------------------------------+  |
|                                                                      |
|  +----------------------------------------------------------------+  |
|  |  Application Layer (应用层)                                   |  |
|  |  +----------------------------------------------------------+  |  |
|  |  |  OpenCode Container                                       |  |  |
|  |  |  • AI 编程助手 Web UI                                     |  |  |
|  |  |  • 仅监听 localhost:4096                                  |  |  |
|  |  +----------------------------------------------------------+  |  |
|  +----------------------------------------------------------------+  |
|                                                                      |
|  +----------------------------------------------------------------+  |
|  |  Storage Layer (存储层)                                       |  |
|  |  +----------------------------------------------------------+  |  |
|  |  |  Docker Volumes                                           |  |  |
|  |  |  • opencode-workspace: 工作区文件                         |  |  |
|  |  |  • opencode-config: OpenCode 配置                         |  |  |
|  |  |  • authelia-data: 用户/Passkey 信息                       |  |  |
|  |  |  • caddy-data: SSL 证书                                    |  |  |
|  |  +----------------------------------------------------------+  |  |
|  +----------------------------------------------------------------+  |
+----------------------------------------------------------------------+
```

### 4.2 网络架构

```
+----------------------------------------------------------------------+
|                            网络流量图                                 |
+----------------------------------------------------------------------+

    Internet
        |
        | HTTPS (443) / HTTP (80 for ACME)
        v
+---------------+
|     EIP       |  <--- 公网 IP (如 47.123.45.67)
+---------------+
        |
        | DNAT to ECS private IP
        v
+---------------+
|  ECS Instance |
|  Ubuntu 24.04 |
|  +---------+  |
|  | Caddy   |  |  <--- 端口 80 (HTTP-01 ACME) + 443 (HTTPS)
|  | :80,:443|  |
|  +----+----+  |
|       |       |
|       | reverse_proxy
|       v       |
|  +---------+  |
|  |Authelia |  |  <--- 端口 9091 (内部)
|  | :9091   |  |
|  +----+----+  |
|       |       |
|       | forward after auth
|       v       |
|  +---------+  |
|  |OpenCode |  |  <--- 端口 4096 (localhost only)
|  | :4096   |  |
|  +---------+  |
+---------------+

安全组入站规则:
+----------+----------+------------------+
| Protocol | Port     | Source           |
+----------+----------+------------------+
| TCP      | 22       | 用户 IP (建议限制)|
| TCP      | 80       | 0.0.0.0/0        |
| TCP      | 443      | 0.0.0.0/0        |
+----------+----------+------------------+

注: 
- 80 端口用于 Let's Encrypt HTTP-01 ACME challenge
- 4096 端口不对外开放，仅 localhost 访问
```

### 4.3 安全架构

```
+----------------------------------------------------------------------+
|                            安全层次图                                 |
+----------------------------------------------------------------------+

Layer 4: Application Security
+------------------------------------------------------------------+
|  OpenCode 内部安全                                               |
|  • API Key 环境变量注入（不写入镜像）                             |
|  • Docker Volume 隔离工作区                                       |
+------------------------------------------------------------------+

Layer 3: Authentication
+------------------------------------------------------------------+
|  Authelia 认证网关                                               |
|  • 1FA: 用户名 + 密码                                            |
|  • 2FA: Passkey (WebAuthn/FIDO2)                                 |
|  • TOTP 备用认证                                                 |
|  • Session 超时自动登出                                          |
+------------------------------------------------------------------+

Layer 2: Transport Security
+------------------------------------------------------------------+
|  Caddy HTTPS                                                     |
|  • TLS 1.3 强制加密                                              |
|  • Let's Encrypt 自动证书                                        |
|  • HSTS 头部                                                     |
+------------------------------------------------------------------+

Layer 1: Network Security
+------------------------------------------------------------------+
|  阿里云安全组                                                    |
|  • 仅开放 22/80/443 端口                                         |
|  • SSH 建议限制用户 IP                                           |
|  • OpenCode 无公网端口暴露                                       |
+------------------------------------------------------------------+
```

---

## 5. 组件设计

### 5.1 Deploy Script（部署脚本）

#### 5.1.1 模块结构

```
cloudcode/
├── deploy.sh                  # 主部署脚本（入口）
├── destroy.sh                 # 清理脚本
├── lib/
│   ├── alicloud.sh           # 阿里云 API 封装
│   ├── docker.sh             # Docker 部署封装
│   ├── utils.sh              # 工具函数
│   └── config.sh             # 配置管理
├── templates/
│   ├── docker-compose.yml.j2 # Docker Compose 模板
│   ├── Caddyfile.j2          # Caddy 配置模板
│   ├── authelia/
│   │   ├── config.yml.j2     # Authelia 主配置
│   │   └── users_database.yml.j2  # 用户数据库
│   └── Dockerfile.opencode   # OpenCode 镜像构建
├── state/
│   └── state.json            # 部署状态文件 (应加入 .gitignore)
└── .gitignore                # 忽略敏感文件
```

#### 5.1.2 核心函数

```bash
# lib/alicloud.sh
alicloud_auth()               # 阿里云认证检查
check_existing_resources()    # 检测已有资源
create_vpc()                  # 创建 VPC
create_vswitch()              # 创建交换机
create_security_group()       # 创建安全组
create_ecs()                  # 创建 ECS 实例
allocate_eip()                # 分配 EIP
bind_eip()                    # 绑定 EIP 到 ECS
destroy_all_resources()       # 释放所有资源

# lib/docker.sh
install_docker()              # 安装 Docker
deploy_compose()              # 部署 Docker Compose 栈
configure_authelia()          # 配置 Authelia 用户

# lib/utils.sh
log_info()                    # 信息日志
log_error()                   # 错误日志
confirm()                     # 交互确认
save_state()                  # 保存部署状态
load_state()                  # 读取部署状态
generate_session_secret()     # 生成 Authelia session secret
hash_password()               # 生成 Argon2id 密码哈希
```

#### 5.1.3 状态文件格式

```json
{
  "version": "1.0",
  "created_at": "2026-02-14T10:30:00Z",
  "region": "ap-southeast-1",
  "os_image": "ubuntu_24_04_x64",
  "resources": {
    "vpc": { "id": "vpc-xxx", "cidr": "192.168.0.0/16" },
    "vswitch": { "id": "vsw-xxx", "zone": "ap-southeast-1a" },
    "security_group": { "id": "sg-xxx" },
    "ecs": {
      "id": "i-xxx",
      "instance_type": "ecs.e-c1m2.large",
      "public_ip": "47.x.x.x",
      "private_ip": "192.168.1.100"
    },
    "eip": { "id": "eip-xxx", "ip": "47.x.x.x" },
    "ssh_key_pair": { "name": "cloudcode-ssh-key", "private_key_path": "~/.ssh/cloudcode" }
  },
  "cloudcode": {
    "username": "admin",
    "domain": "opencode.example.com"
  }
}
```

**安全提醒**：`state.json` 包含敏感信息，应加入 `.gitignore`，不要提交到版本控制。

#### 5.1.4 SSH 密钥管理

部署脚本会自动管理 SSH 密钥：

1. **自动创建**：脚本在本地 `~/.ssh/` 目录创建密钥对（如果不存在）
2. **上传公钥**：创建 ECS 时将公钥注入到实例
3. **记录路径**：私钥路径保存在 `state.json` 中
4. **权限设置**：私钥权限设为 600

```
~/.ssh/cloudcode          # 私钥 (chmod 600)
~/.ssh/cloudcode.pub      # 公钥
```

### 5.2 Docker Compose（容器编排）

```yaml
services:
  caddy:
    image: caddy:2-alpine
    container_name: caddy
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      - authelia
      - opencode
    networks:
      - cloudcode-net

  authelia:
    image: authelia/authelia:4.38
    container_name: authelia
    restart: unless-stopped
    volumes:
      - ./authelia:/config
    environment:
      - TZ=Asia/Singapore
    expose:
      - 9091
    networks:
      - cloudcode-net

  opencode:
    build:
      context: .
      dockerfile: Dockerfile.opencode
    container_name: opencode
    restart: unless-stopped
    volumes:
      - opencode_workspace:/home/opencode/workspace
      - opencode_config:/home/opencode/.config/opencode
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - OPENAI_BASE_URL=${OPENAI_BASE_URL:-}
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY:-}
    expose:
      - 4096
    command: ["opencode", "web", "--hostname", "0.0.0.0", "--port", "4096"]
    networks:
      - cloudcode-net

volumes:
  caddy_data:
  caddy_config:
  opencode_workspace:
  opencode_config:

networks:
  cloudcode-net:
    driver: bridge
```

### 5.3 Caddy（反向代理 + HTTPS）

```caddyfile
# Caddyfile.j2
{{ domain }} {
    # Authelia 认证网关
    forward_auth authelia:9091 {
        uri /api/verify?rd=https://{{ domain }}/auth
        copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
    }

    # 认证通过后转发到 OpenCode
    reverse_proxy opencode:4096 {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }

    log {
        output file /var/log/caddy/access.log
    }
}

{{ domain }} {
    # Authelia 登录页面路由
    handle /auth/* {
        reverse_proxy authelia:9091
    }
}
```

### 5.4 Authelia（认证网关）

```yaml
# authelia/config.yml.j2 (Authelia 4.38+ 格式)
server:
  host: 0.0.0.0
  port: 9091

log:
  level: info

authentication_backend:
  file:
    path: /config/users_database.yml
    password:
      algorithm: argon2id
      iterations: 1
      salt_length: 16
      parallelism: 8
      memory: 64

session:
  name: authelia_session
  secret: {{ session_secret }}
  expiration: 12h
  inactivity: 30m
  cookies:
    - domain: {{ domain }}
      name: authelia_session
      same_site: lax

storage:
  local:
    path: /config/db.sqlite3

webauthn:
  display_name: CloudCode
  attestation_conveyance_preference: indirect
  user_verification: preferred
  timeout:
    enable: true
    seconds: 60

totp:
  issuer: CloudCode
  period: 30
  skew: 1

access_control:
  default_policy: two_factor
  rules:
    - domain: {{ domain }}
      policy: two_factor

notifier:
  filesystem:
    filename: /config/notification.txt
```

```yaml
# authelia/users_database.yml.j2
users:
  {{ username }}:
    displayname: "{{ username }}"
    password: "{{ hashed_password }}"
    email: "{{ email }}"
    groups:
      - admins
```

### 5.5 OpenCode（AI 编程助手）

```dockerfile
# Dockerfile.opencode
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    git \
    openssh-client \
    sudo \
    nodejs \
    npm \
    python3 \
    python3-pip \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/bash opencode \
    && echo "opencode ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/opencode \
    && chmod 0440 /etc/sudoers.d/opencode

RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | \
    dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | \
    tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y gh \
    && rm -rf /var/lib/apt/lists/*

USER opencode
WORKDIR /home/opencode

# 安装 OpenCode (固定版本号，避免不可控)
ARG OPENCODE_VERSION=latest
RUN if [ "$OPENCODE_VERSION" = "latest" ]; then \
        curl -fsSL https://opencode.ai/install | bash; \
    else \
        curl -fsSL -o /tmp/opencode "https://github.com/opencode-ai/opencode/releases/download/v${OPENCODE_VERSION}/opencode-linux-x64" \
        && chmod +x /tmp/opencode \
        && sudo mv /tmp/opencode /usr/local/bin/; \
    fi

RUN mkdir -p /home/opencode/.ssh \
    && touch /home/opencode/.ssh/known_hosts \
    && ssh-keyscan -T 5 github.com 2>/dev/null >> /home/opencode/.ssh/known_hosts || true

RUN mkdir -p /home/opencode/workspace

VOLUME ["/home/opencode/workspace"]
VOLUME ["/home/opencode/.config/opencode"]

EXPOSE 4096

ENTRYPOINT ["opencode", "web", "--hostname", "0.0.0.0", "--port", "4096"]
```

---

## 6. 用户体验流程

### 6.1 部署流程

```
+----------+     check CLI     +-----------+     create VPC     +-----------+
|  Start   | ----------------> |  Config   | ----------------> |  Create   |
|          |                   |  Check    |                   |  VPC/VSwitch
+----------+                   +-----------+                   +-----------+
                                                                    |
                                                                    v
+-----------+     install Docker     +-----------+     deploy compose
|  Config   | <-------------------- |  SSH to   | <----------------+
|  Authelia |                       |  ECS      |
+-----------+                       +-----------+
      |
      | output access info
      v
+-----------+
|  Done     |
|  Output:  |
|  • URL    |
|  • User   |
+-----------+
```

**交互流程示例：**

```bash
$ ./deploy.sh

🚀 CloudCode 阿里云一键部署工具

[1/6] 检查依赖环境...
✓ aliyun CLI 已安装
✓ jq 已安装
✓ ssh 已安装

[2/6] 配置阿里云认证:
检测到 ALICLOUD_ACCESS_KEY 环境变量，使用现有配置
✓ 阿里云认证成功

[3/6] 配置访问信息:
请输入域名 (推荐使用自有域名，留空使用 nip.io): 
✓ 将使用 nip.io: <eip>.nip.io
  ⚠️  注意: nip.io 是公共服务，可能因 Let's Encrypt 速率限制导致证书签发失败
请输入管理员用户名 [admin]: 
请输入管理员密码: ****
请确认管理员密码: ****

[4/6] 配置 OpenCode:
请选择 AI 模型提供商:
  1) OpenAI
  2) Anthropic
  3) 自定义
选择 [1]: 1
请输入 OpenAI API Key: sk-xxx
请输入 Base URL [回车使用默认]: 

[5/6] 创建云资源 (预计 5-10 分钟):
✓ 创建 SSH 密钥对 (~/.ssh/cloudcode)
✓ 创建 VPC (vpc-xxx)
✓ 创建交换机 (vsw-xxx)
✓ 创建安全组 (sg-xxx) - 开放 22/80/443
✓ 创建 ECS 实例 (i-xxx) - Ubuntu 24.04, 规格: ecs.e-c1m2.large
✓ 分配 EIP (eip-xxx) - IP: 47.123.45.67
✓ 绑定 EIP 到 ECS

[6/6] 部署应用 (预计 3-5 分钟):
✓ SSH 连接 ECS 成功
✓ 安装 Docker 和 Docker Compose
✓ 创建目录结构
✓ 部署 Docker Compose 栈
✓ 等待服务启动...

─────────────────────────────────────────────────────────────
✅ 部署完成！

📱 访问地址: https://47.123.45.67.nip.io
👤 用户名: admin
🔑 密码: <你设置的密码>

⚠️ 认证流程: 用户名 + 密码 → Passkey 验证
⚠️ 首次登录后建议注册 Passkey 作为第二因素！

💡 提示:
   - SSH 访问: ssh -i ~/.ssh/cloudcode root@47.123.45.67
   - 清理资源: ./destroy.sh
─────────────────────────────────────────────────────────────
```

### 6.2 首次登录（注册 Passkey）

```
Step 1: 访问服务
─────────────────────────────────────────────────────────────────────
  用户打开 https://47.123.45.67.nip.io
        |
        v
+-------------------------------------------+
|  Authelia Login Page                       |
|  Username: [admin          ]              |
|  Password: [********        ]              |
|  [ Sign In ]                              |
+-------------------------------------------+

Step 2: 输入密码（1FA）
─────────────────────────────────────────────────────────────────────
  输入用户名/密码，点击 Sign In
        |
        v
+-------------------------------------------+
|  Two-Factor Authentication                 |
|  选择第二因素验证方式:                      |
|  [ 🔑 Passkey ]  [ 📱 TOTP ]              |
+-------------------------------------------+

Step 3: Passkey 验证（2FA）
─────────────────────────────────────────────────────────────────────
  点击 Passkey
        |
        v
+-------------------------------------------+
|  首次使用: 注册 Passkey                    |
|  浏览器 Passkey 弹窗                        |
|  [使用面容 ID] [使用 Touch ID] [使用安全密钥]|
+-------------------------------------------+

Step 4: 验证成功
─────────────────────────────────────────────────────────────────────
  面容/指纹验证成功
        |
        v
+-------------------------------------------+
|  OpenCode Web UI                          |
|  ✓ 已登录                                  |
|  ✓ Passkey 已注册                          |
|  ✓ 下次登录: 密码 → Passkey                |
+-------------------------------------------+
```

### 6.3 日常使用

```
Step 1: 访问服务
─────────────────────────────────────────────────────────────────────
  用户打开 https://47.123.45.67.nip.io
        |
        v
+-------------------------------------------+
|  Authelia Login Page                       |
|  Username: [admin          ]              |
|  Password: [********        ]              |
|  [ Sign In ]                              |
+-------------------------------------------+

Step 2: 输入密码（1FA）
─────────────────────────────────────────────────────────────────────
  输入用户名/密码，点击 Sign In
        |
        v
+-------------------------------------------+
|  Two-Factor Authentication                 |
|  检测到已注册 Passkey                       |
|  [ 🔑 Use Passkey ]                       |
+-------------------------------------------+

Step 3: Passkey 验证（2FA）
─────────────────────────────────────────────────────────────────────
  点击 Use Passkey
        |
        v
+-------------------------------------------+
|  手机原生验证                               |
|  (Face ID / Touch ID / 指纹)               |
+-------------------------------------------+

Step 4: 进入应用
─────────────────────────────────────────────────────────────────────
  验证成功，自动跳转
        |
        v
+-------------------------------------------+
|  OpenCode Web UI                          |
|  ✓ 两步验证登录成功                        |
+-------------------------------------------+
```

---

## 7. 成本估算

### 按量付费（月度）

| 资源项 | 规格 | 单价 | 月费用 (USD) | 备注 |
|--------|------|------|-------------|------|
| ECS | ecs.e-c1m2.large (2C4G) | ~$0.02/h | ~$15 | 新加坡地域，Ubuntu 24.04 |
| EIP | 1Mbps 带宽 | ~$0.003/h | ~$2 | 按带宽计费 |
| 流量 | 按量 | $0.8/GB | ~$2 | 预估 2.5GB |
| **总计** | | | **~$19/月** | **约 ¥140/月** |

### 成本优化建议

| 优化方式 | 节省幅度 | 实现方式 |
|----------|----------|----------|
| 抢占式实例 | 70-90% | 设置自动竞价，但可能被回收 |
| 停止不用时 | 按实际使用 | 手动停止 ECS，仅保留 EIP 费用 |
| 降低带宽 | 20-50% | 改用按流量计费（适合低频访问） |

### 与其他方案对比

| 方案 | 月费用 | HTTPS | 两步认证 | 运维复杂度 |
|------|--------|-------|---------|-----------|
| **cloudcode (本方案)** | ~$19 | ✅ 自动 | ✅ | 中 |
| SAE 托管 | ~$110 | ✅ 自动 | 需额外配置 | 低 |
| 原设计（无认证） | ~$15 | ❌ 无 | ❌ | 中 |

---

## 8. 实现规划

### 8.1 目录结构

```
cloudcode/
├── README.md                    # 项目说明
├── .gitignore                   # 忽略敏感文件
├── deploy.sh                    # 主部署脚本
├── destroy.sh                   # 清理脚本
├── lib/
│   ├── alicloud.sh             # 阿里云 API 封装
│   ├── docker.sh               # Docker 部署封装
│   ├── utils.sh                # 工具函数
│   └── config.sh               # 配置管理
├── templates/
│   ├── docker-compose.yml.j2   # Docker Compose 模板
│   ├── Caddyfile.j2            # Caddy 配置模板
│   ├── Dockerfile.opencode     # OpenCode 镜像构建
│   ├── env.j2                  # 环境变量模板
│   └── authelia/
│       ├── config.yml.j2       # Authelia 主配置
│       └── users_database.yml.j2  # 用户数据库
├── state/
│   └── .gitkeep                # 状态文件目录
├── tests/
│   └── test_deploy.sh          # 部署测试脚本
└── docs/
    └── design-oc.md            # 本设计文档
```

### 8.2 实现步骤

| 步骤 | 任务 | 依赖 | 验证方式 |
|------|------|------|----------|
| 1 | 实现基础脚本框架 | 无 | `./deploy.sh --help` 正常输出 |
| 2 | 实现阿里云资源创建 | 步骤 1 | 能创建 ECS 并获取 EIP |
| 3 | 实现 SSH 远程部署 | 步骤 2 | 能远程安装 Docker |
| 4 | 编写 Docker Compose 模板 | 步骤 3 | 本地 `docker compose up` 成功 |
| 5 | 实现 Authelia 配置生成 | 步骤 4 | 两步认证成功 |
| 6 | 实现清理脚本 | 步骤 2-5 | `./destroy.sh` 能释放资源 |
| 7 | 端到端测试 | 步骤 1-6 | 完整部署流程通过 |
| 8 | 编写文档 | 步骤 7 | README 完整 |

### 8.3 测试要点

| 测试项 | 测试方法 | 验证标准 |
|--------|----------|----------|
| 部署脚本 | 执行 `./deploy.sh` 完整流程 | 所有资源创建成功，服务可访问 |
| HTTPS 访问 | `curl -I https://<eip>.nip.io` | 返回 302 重定向到登录页 |
| 密码登录（1FA） | 浏览器输入用户名密码 | 进入 2FA 页面 |
| Passkey 验证（2FA） | 使用 Passkey 验证 | 登录成功进入 OpenCode |
| OpenCode 功能 | 在 Web UI 中使用 AI 对话 | 正常响应 |
| 清理脚本 | 执行 `./destroy.sh` | 所有资源释放，账单停止 |

---

## 9. 运维与备份

### 9.1 数据备份策略

Docker Volume 中存储了重要数据，建议定期备份：

| 数据 | 存储位置 | 备份方式 | 频率 |
|------|----------|----------|------|
| 工作区文件 | opencode_workspace | ECS 快照 / rsync | 每周 |
| OpenCode 配置 | opencode_config | ECS 快照 | 每周 |
| Authelia 用户数据 | authelia_data | ECS 快照 | 每月 |
| SSL 证书 | caddy_data | 自动续期 | 无需备份 |

**备份命令示例：**

```bash
# 在 ECS 上执行
docker run --rm \
  -v cloudcode_opencode_workspace:/data \
  -v $(pwd)/backup:/backup \
  alpine tar czf /backup/workspace-$(date +%Y%m%d).tar.gz /data

# 或使用阿里云 ECS 快照
aliyun ecs CreateSnapshot --InstanceId i-xxx --SnapshotName "cloudcode-backup-$(date +%Y%m%d)"
```

### 9.2 常见运维任务

| 任务 | 命令 |
|------|------|
| 查看日志 | `docker logs opencode` |
| 重启服务 | `docker compose restart` |
| 更新 OpenCode | `docker compose build opencode && docker compose up -d opencode` |
| 查看 SSL 证书状态 | `docker exec caddy caddy list-certs` |

### 9.3 故障恢复

| 场景 | 恢复方式 |
|------|----------|
| ECS 实例故障 | 从快照恢复或重新部署 |
| 数据丢失 | 从备份恢复 Docker Volume |
| SSL 证书过期 | Caddy 自动续期，如失败可重启容器 |

---

## 参考文献

### 核心组件

- [OpenCode 官方文档](https://opencode.ai) — AI 编程助手
- [Authelia 官方文档](https://www.authelia.com/) — 两步认证网关
- [Caddy 官方文档](https://caddyserver.com/docs/) — 自动 HTTPS 反向代理
- [Docker Compose 文档](https://docs.docker.com/compose/) — 容器编排

### 阿里云

- [阿里云 CLI 文档](https://help.aliyun.com/zh/cli/) — 命令行工具
- [ECS API 参考](https://help.aliyun.com/zh/ecs/developer-reference/api-ecs-2014-05-26-createinstance) — 云服务器 API
- [EIP API 参考](https://help.aliyun.com/zh/vpc/developer-reference/api-eip-2016-04-28-allocateeipaddress) — 弹性公网 IP API

### 安全参考

- [WebAuthn 规范](https://www.w3.org/TR/webauthn/) — Passkey 技术标准
- [OWASP 认证安全指南](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) — 认证最佳实践
- [Let's Encrypt 文档](https://letsencrypt.org/docs/) — 免费 SSL 证书

### 参考项目

- [opencode-cloud](https://github.com/pRizz/opencode-cloud) — AWS/Railway 部署参考
- [Authelia 示例配置](https://github.com/authelia/authelia/tree/master/examples/compose) — Docker Compose 示例

---

**文档版本**: 1.1  
**更新日期**: 2026-02-14

**修订记录**：
- v1.1: 修正 Authelia Passkey 为 2FA（非一级认证）；补充 80 端口；修正 Caddyfile 语法；补充 SSH 密钥管理、ECS 镜像、备份策略；移除 Docker-in-Docker 决策理由；更新 Authelia 配置格式为 4.38+ 版本
