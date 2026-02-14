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
  - [3.5 实现语言：为什么选 Go 而非 Bash](#35-实现语言为什么选-go-而非-bash)
  - [3.6 分发方式：CLI 工具而非脚本集合](#36-分发方式cli-工具而非脚本集合)
- [4. 架构设计](#4-架构设计)
  - [4.1 核心分层](#41-核心分层)
  - [4.2 网络架构](#42-网络架构)
  - [4.3 安全架构](#43-安全架构)
- [5. 组件设计](#5-组件设计)
  - [5.1 CloudCode CLI（Go 命令行工具）](#51-cloudcode-cligo-命令行工具)
  - [5.2 Docker Compose（容器编排）](#52-docker-compose容器编排)
  - [5.3 Caddy（反向代理 + HTTPS）](#53-caddy反向代理--https)
  - [5.4 环境变量](#54-环境变量)
  - [5.5 Authelia（认证网关）](#55-authelia认证网关)
  - [5.6 OpenCode（AI 编程助手）](#56-opencodeai-编程助手)
- [6. 用户体验流程](#6-用户体验流程)
  - [6.1 安装](#61-安装)
  - [6.2 部署流程](#62-部署流程)
  - [6.3 查看状态](#63-查看状态)
  - [6.4 首次登录（注册 Passkey）](#64-首次登录注册-passkey)
  - [6.5 日常使用](#65-日常使用)
  - [6.6 域名 DNS 配置（自有域名）](#66-域名-dns-配置自有域名)
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
                        +------------------+
                        |   EIP (公网 IP)   |
                        |   47.x.x.x       |
                        +------------------+
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
|  |  |  • opencode_workspace (工作区持久化)                         |  |  |
|  |  |  • opencode_config (OpenCode 配置)                          |  |  |
|  |  |  • caddy_data (SSL 证书)                                    |  |  |
|  |  |  • caddy_config (Caddy 配置缓存)                            |  |  |
|  |  |  配置文件 (bind mount):                                      |  |  |
|  |  |  • ./authelia (Authelia 配置及运行时数据)                    |  |  |
|  |  +------------------------------------------------------------+  |  |
|  +----------------------------------------------------------------+  |
+----------------------------------------------------------------------+
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

### 3.5 实现语言：为什么选 Go 而非 Bash

| 维度 | Go | Bash |
|------|-----|------|
| 阿里云 SDK | 官方 SDK，类型安全 | 封装 aliyun CLI，解析文本输出 |
| SSH 操作 | golang.org/x/crypto/ssh，原生库 | 调用 ssh/scp 命令 |
| 模板渲染 | text/template 内置 | sed/envsubst 替换 |
| 错误处理 | 强类型，编译期检查 | set -e，脆弱 |
| JSON 操作 | encoding/json 内置 | 依赖 jq |
| 单元测试 | 内置 testing | bats（第三方） |
| 分发方式 | 单一二进制，交叉编译 | 需要 makeself 打包或 git clone |
| 跨平台 | Linux/macOS/Windows | 仅 Linux/macOS |

**决策**：选择 Go。

**理由**：
1. 阿里云官方 Go SDK，API 调用类型安全、错误处理完善
2. 编译为单一二进制，用户 `curl | bash` 安装即可使用
3. 内置模板引擎和 `embed`，模板文件编译进二进制
4. CLI 框架 cobra 是业界标准（kubectl、docker、gh 都在用）

**代价**：
1. 开发门槛比 Bash 高
2. 需要构建和发布流程（GitHub Actions + goreleaser）

### 3.6 分发方式：CLI 工具而非脚本集合

用户使用方式：

```bash
# 安装
curl -fsSL https://github.com/hwuu/cloudcode/releases/latest/download/install.sh | bash

# 使用
cloudcode deploy      # 部署
cloudcode status      # 查看状态
cloudcode destroy     # 销毁资源
```

**理由**：
1. 比 `git clone` + `./deploy.sh` 更友好
2. 单一二进制，无运行时依赖
3. 状态文件持久化在 `~/.cloudcode/`，不依赖工作目录

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
|  |  |  Named Volumes (运行时数据):                              |  |  |
|  |  |  • opencode_workspace: 工作区文件                         |  |  |
|  |  |  • opencode_config: OpenCode 配置                         |  |  |
|  |  |  • caddy_data: SSL 证书                                    |  |  |
|  |  |  • caddy_config: Caddy 配置缓存                            |  |  |
|  |  +----------------------------------------------------------+  |  |
|  |  +----------------------------------------------------------+  |  |
|  |  |  Bind Mounts (配置文件，部署时预填充):                     |  |  |
|  |  |  • ./authelia: Authelia 配置 + db.sqlite3                |  |  |
|  |  |  • ./Caddyfile: Caddy 配置                                |  |  |
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
- SSH 端口限制实现：部署时自动检测用户公网 IP（通过 https://api.ipify.org 或类似服务获取），或交互式询问用户是否限制 IP。如用户选择限制，安全组规则 Source 改为具体 IP；如选择不限制，Source 为 0.0.0.0/0 并显示安全警告。
- 阿里云 Ubuntu 镜像默认使用 `root` 用户登录（与 AWS 不同）。

**SSH IP 限制后的访问恢复：**

如果用户 IP 变化（如更换网络环境）导致无法 SSH 连接，可通过以下方式恢复：

1. **阿里云控制台**：进入 ECS 实例详情 → 安全组 → 修改入站规则，更新 SSH 端口的 Source IP
2. **ECS 远程连接（VNC）**：控制台 → 实例详情 → 远程连接 → 通过 VNC 登录后手动修改安全组规则
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

### 5.1 CloudCode CLI（Go 命令行工具）

#### 5.1.1 Go 包结构

```
cloudcode/
├── cmd/
│   └── cloudcode/
│       └── main.go                # 入口
├── internal/
│   ├── alicloud/                  # 阿里云资源管理 (官方 SDK)
│   │   ├── client.go              # SDK 客户端初始化
│   │   ├── ecs.go                 # ECS 实例管理
│   │   ├── vpc.go                 # VPC/VSwitch/安全组
│   │   └── eip.go                 # EIP 管理
│   ├── deploy/                    # 部署编排
│   │   ├── deploy.go              # deploy 命令逻辑
│   │   ├── destroy.go             # destroy 命令逻辑
│   │   └── status.go              # status 命令逻辑
│   ├── remote/                    # SSH 远程操作
│   │   ├── ssh.go                 # SSH 连接管理
│   │   └── sftp.go                # 文件传输
│   ├── config/                    # 配置与状态管理
│   │   ├── state.go               # ~/.cloudcode/state.json 读写
│   │   └── prompt.go              # 交互式输入
│   └── template/                  # 模板渲染
│       └── render.go              # go:embed + text/template
├── templates/                     # 嵌入的模板文件 (go:embed)
│   ├── docker-compose.yml
│   ├── Caddyfile.tmpl
│   ├── Dockerfile.opencode
│   ├── env.tmpl
│   └── authelia/
│       ├── configuration.yml.tmpl
│       └── users_database.yml.tmpl
├── install.sh                     # 安装脚本 (检测 OS/ARCH，下载二进制)
├── go.mod
├── go.sum
├── Makefile                       # 构建命令
├── .goreleaser.yml                # 发布配置
└── docs/
    └── design-oc.md
```

#### 5.1.2 核心依赖

| 功能 | 库 |
|------|-----|
| CLI 框架 | `github.com/spf13/cobra` |
| 阿里云 ECS SDK | `github.com/alibabacloud-go/ecs-20140526/v4` |
| 阿里云 VPC SDK | `github.com/alibabacloud-go/vpc-20160428/v6` |
| SSH | `golang.org/x/crypto/ssh` |
| SFTP | `github.com/pkg/sftp` |
| 模板嵌入 | `embed`（标准库） |
| 密码哈希 | `golang.org/x/crypto/argon2` |
| 阿里云 STS SDK | `github.com/alibabacloud-go/sts-20150401` |

#### 5.1.3 CLI 命令设计

```
cloudcode deploy      # 交互式部署（创建云资源 + 部署应用）
cloudcode status      # 查看部署状态（SSH 检查容器健康）
cloudcode destroy     # 销毁所有云资源（需确认，支持 --force 跳过）
cloudcode version     # 显示版本号、commit、构建时间
```

version 输出示例：

```
$ cloudcode version
cloudcode v1.0.0
  commit: e7f4a81
  built:  2026-02-14T10:30:00Z
  go:     go1.23.0
```

版本信息通过 ldflags 在构建时注入（见 5.1.9 goreleaser 配置）。

#### 5.1.4 状态文件格式

状态文件路径：`~/.cloudcode/state.json`

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
      "system_disk_size": 60,
      "public_ip": "47.x.x.x",
      "private_ip": "192.168.1.100"
    },
    "eip": { "id": "eip-xxx", "ip": "47.x.x.x" },
    "ssh_key_pair": { "name": "cloudcode-ssh-key", "private_key_path": ".cloudcode/ssh_key" }
  },
  "cloudcode": {
    "username": "admin",
    "domain": "opencode.example.com"
  }
}
```

**路径解析示例：**

状态文件中的 `private_key_path` 使用相对路径，运行时通过以下代码解析为绝对路径：

```go
func ResolveKeyPath(relativePath string) (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(home, relativePath), nil
}

// 示例：".cloudcode/ssh_key" → "/home/user/.cloudcode/ssh_key"
```

#### 5.1.5 SSH 密钥管理

`cloudcode deploy` 会自动管理 SSH 密钥：

1. **自动创建**：在 `~/.cloudcode/` 目录创建密钥对（如果不存在）
2. **上传公钥**：创建 ECS 时将公钥注入到实例
3. **记录路径**：私钥路径保存在 `state.json` 中
4. **权限设置**：私钥权限设为 600

```
~/.cloudcode/ssh_key          # 私钥 (chmod 600)
~/.cloudcode/ssh_key.pub      # 公钥
```

#### 5.1.6 幂等性与失败回滚

**幂等性策略：**

deploy 命令会检测 `state.json` 中记录的资源是否已存在：

| 场景 | 行为 |
|------|------|
| `state.json` 不存在 | 首次部署，从零创建所有资源 |
| `state.json` 存在，资源完整 | 检测到已部署，提示用户使用 `--force` 重新部署应用层 |
| `state.json` 存在，部分资源缺失 | 中断恢复模式，只创建缺失的资源 |

`--force` 重新部署行为：不销毁云资源（VPC/ECS/EIP），仅重新渲染模板、通过 SFTP 上传配置文件、重启 Docker Compose 栈。适用于修改配置后需要更新的场景。

**部署文件映射：**

| 源文件（go:embed） | 是否渲染 | ECS 目标路径 |
|---|---|---|
| `docker-compose.yml` | 否 | `~/cloudcode/docker-compose.yml` |
| `Caddyfile.tmpl` | 是 | `~/cloudcode/Caddyfile` |
| `env.tmpl` | 是 | `~/cloudcode/.env` |
| `Dockerfile.opencode` | 否 | `~/cloudcode/Dockerfile.opencode` |
| `authelia/configuration.yml.tmpl` | 是 | `~/cloudcode/authelia/configuration.yml` |
| `authelia/users_database.yml.tmpl` | 是 | `~/cloudcode/authelia/users_database.yml` |

**Docker 安装方式：**

SSH 连接 ECS 后，使用阿里云镜像源加速安装 Docker：

```bash
# 安装 Docker
curl -fsSL https://get.docker.com | sh -s -- --mirror Aliyun

# 启动并设置开机自启
systemctl enable docker
systemctl start docker

# 验证安装
docker --version
```

**失败回滚策略：**

采用"检测-创建-记录"模式，每创建一个资源就更新 `state.json`：

```
CreateVPC()           → 记录 vpc.id
CreateVSwitch()       → 记录 vswitch.id
CreateSecurityGroup() → 记录 sg.id
CreateECS()           → 记录 ecs.id
...
```

如果中途失败：

1. 查看错误信息，确认失败原因
2. 修复问题后重新执行 `cloudcode deploy`，会从断点继续
3. 如需完全重置，先执行 `cloudcode destroy` 清理所有资源

**destroy 确认流程：**

destroy 命令默认需要用户确认，防止误操作：

```
$ cloudcode destroy

⚠️  即将删除以下资源:
  • VPC: vpc-xxx (192.168.0.0/16)
  • VSwitch: vsw-xxx
  • SecurityGroup: sg-xxx
  • ECS: i-xxx (47.123.45.67)
  • EIP: eip-xxx (47.123.45.67)
  • SSH Key Pair: cloudcode-ssh-key

此操作不可逆！确认删除? [y/N]: y

正在删除...
  ✓ 解绑 EIP (eip-xxx) 从 ECS (i-xxx)
  ✓ 释放 EIP (eip-xxx)
  ✓ 停止 ECS (i-xxx)
  ✓ 删除 ECS (i-xxx)
  ✓ 删除 SSH 密钥对
  ✓ 删除安全组
  ✓ 删除 VSwitch
  ✓ 删除 VPC
✅ 资源已全部释放
状态文件已删除: ~/.cloudcode/state.json
```

支持 `--force` 跳过确认，`--dry-run` 仅显示将要删除的资源。

#### 5.1.7 前置检查

deploy 命令在创建资源前会执行以下检查：

| 检查项 | 检查方式 | 失败处理 |
|--------|----------|----------|
| 环境变量 | 检查 ALICLOUD_ACCESS_KEY_ID/SECRET 是否设置 | 提示设置方法 |
| SDK 连接 | 调用 STS GetCallerIdentity | 提示检查网络或密钥权限 |
| 账户余额 | 调用 QueryAccountBalance | 余额 < ¥50 时警告 |
| 地域可用 | 调用 DescribeRegions | 提示选择其他地域 |
| 配额检查 | 检查 ECS/VPC 配额 | 提示申请提额 |

**配额检查具体项：**

| 配额类型 | 检查项 | 最低要求 | API |
|----------|--------|----------|-----|
| ECS | 实例数量 | ≥ 1 | DescribeAccountAttributes |
| VPC | VPC 数量 | ≥ 1 | DescribeAccountAttributes |
| VPC | 交换机数量 | ≥ 1 | DescribeAccountAttributes |
| VPC | 安全组数量 | ≥ 1 | DescribeAccountAttributes |
| VPC | EIP 数量 | ≥ 1 | DescribeAccountAttributes |

**可用区选择策略：**

默认使用 `ap-southeast-1a`，如该可用区无所选规格库存，自动尝试其他可用区（`ap-southeast-1b`、`ap-southeast-1c`）。实现上调用 `DescribeZones` 获取可用区列表及其资源可用状态，按顺序尝试。

#### 5.1.8 安装脚本 (install.sh)

```bash
#!/bin/bash
# 检测 OS/ARCH，下载对应二进制到 /usr/local/bin
set -e
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in x86_64) ARCH="amd64" ;; aarch64|arm64) ARCH="arm64" ;; esac
RELEASE_URL="https://github.com/hwuu/cloudcode/releases/latest/download/cloudcode-${OS}-${ARCH}"
echo "Downloading cloudcode..."
curl -fsSL "$RELEASE_URL" | sudo tee /usr/local/bin/cloudcode > /dev/null
sudo chmod +x /usr/local/bin/cloudcode
echo "✅ cloudcode installed to /usr/local/bin/cloudcode"
echo "Run 'cloudcode --help' to get started"
```

#### 5.1.9 构建与发布

使用 goreleaser + GitHub Actions 自动构建：

```yaml
# .goreleaser.yml
before:
  hooks:
    - go mod tidy

builds:
  - main: ./cmd/cloudcode
    binary: cloudcode
    goos: [linux, darwin]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}

archives:
  - format: binary
    name_template: "{{ .Binary }}-{{ .Os }}-{{ .Arch }}"

release:
  extra_files:
    - glob: ./install.sh
      name_template: install.sh

checksum:
  name_template: checksums.txt
```

发布产物：

| 文件 | 说明 |
|------|------|
| `cloudcode-linux-amd64` | Linux x86_64 二进制 |
| `cloudcode-linux-arm64` | Linux ARM64 二进制 |
| `cloudcode-darwin-amd64` | macOS Intel 二进制 |
| `cloudcode-darwin-arm64` | macOS Apple Silicon 二进制 |
| `install.sh` | 安装脚本 |

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
    env_file:
      - .env
    expose:
      - 4096
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
# Caddyfile.tmpl
{{ .Domain }} {
    # 重定向 /auth 到 /auth/ (handle_path /auth/* 不匹配无尾部斜杠的情况)
    redir /auth /auth/ 301

    # Authelia 登录页面路由 (handle_path 自动剥离 /auth 前缀)
    handle_path /auth/* {
        reverse_proxy authelia:9091
    }

    # 主应用路由（需认证）
    handle {
        # Authelia forward auth (Authelia 4.38+ 端点)
        forward_auth authelia:9091 {
            uri /api/authz/forward-auth
            copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
        }

        # 认证通过后转发到 OpenCode (Caddy 自动设置 X-Forwarded-* 头部)
        reverse_proxy opencode:4096
    }

    log {
        output stdout
        format console
    }
}
```

**模板渲染说明**：

`templates/Caddyfile.tmpl` 在部署时通过 Go 的 `text/template` 渲染为 `Caddyfile` 文件，然后通过 SFTP 上传到 ECS 的 `~/cloudcode/Caddyfile` 目录。Docker Compose 通过 bind mount 挂载该文件。

### 5.4 环境变量

```bash
# env.tmpl
OPENAI_API_KEY={{ .OpenAIAPIKey }}
OPENAI_BASE_URL={{ .OpenAIBaseURL }}
ANTHROPIC_API_KEY={{ .AnthropicAPIKey }}
```

**安全提醒**：此文件包含 API Key，部署时渲染为 `.env` 文件通过 SFTP 传输到 ECS 的 `~/cloudcode/.env`，不要提交到版本控制。Docker Compose 通过 `env_file: .env` 加载。

### 5.5 Authelia（认证网关）

```yaml
# authelia/configuration.yml.tmpl (Authelia 4.38+ 格式)
server:
  address: 'tcp://0.0.0.0:9091/'

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
  secret: {{ .SessionSecret }}
  expiration: 12h
  inactivity: 30m
  cookies:
    - domain: {{ .Domain }}
      authelia_url: https://{{ .Domain }}/auth
      name: authelia_session
      same_site: lax

storage:
  encryption_key: {{ .StorageEncryptionKey }}
  local:
    path: /config/db.sqlite3

webauthn:
  display_name: CloudCode
  attestation_conveyance_preference: indirect
  user_verification: preferred
  timeout: 60s

totp:
  issuer: CloudCode
  period: 30
  skew: 1

access_control:
  default_policy: deny
  rules:
    # 豁免 Authelia 自身路径，避免死循环
    - domain: {{ .Domain }}
      resources:
        - '^/auth([/?].*)?$'
      policy: bypass
    # 主应用需要两步认证
    - domain: {{ .Domain }}
      policy: two_factor

notifier:
  filesystem:
    filename: /config/notification.txt
```

```yaml
# authelia/users_database.yml.tmpl
users:
  {{ .Username }}:
    displayname: "{{ .Username }}"
    password: "{{ .HashedPassword }}"
    email: "{{ .Email }}"
    groups:
      - admins
```

### 5.6 OpenCode（AI 编程助手）

```dockerfile
# Dockerfile.opencode
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# 基础依赖
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

# 可选: Go 开发环境（如需在容器内编译 Go 项目，取消下方注释）
# RUN apt-get update && apt-get install -y --no-install-recommends golang-go && rm -rf /var/lib/apt/lists/*

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
RUN ARCH=$(uname -m) && \
    case "$ARCH" in x86_64) ARCH="x64" ;; aarch64|arm64) ARCH="arm64" ;; esac && \
    if [ "$OPENCODE_VERSION" = "latest" ]; then \
        curl -fsSL https://opencode.ai/install | bash; \
    else \
        curl -fsSL -o /tmp/opencode "https://github.com/opencode-ai/opencode/releases/download/v${OPENCODE_VERSION}/opencode-linux-${ARCH}" \
        && chmod +x /tmp/opencode \
        && sudo mv /tmp/opencode /usr/local/bin/; \
    fi

RUN mkdir -p /home/opencode/.ssh \
    && touch /home/opencode/.ssh/known_hosts \
    && ssh-keyscan -T 5 github.com 2>/dev/null >> /home/opencode/.ssh/known_hosts || true

RUN mkdir -p /home/opencode/workspace /home/opencode/.config/opencode \
    && chown -R opencode:opencode /home/opencode/workspace /home/opencode/.config/opencode

VOLUME ["/home/opencode/workspace"]
VOLUME ["/home/opencode/.config/opencode"]

EXPOSE 4096

ENTRYPOINT ["opencode", "web", "--hostname", "0.0.0.0", "--port", "4096"]
```

---

## 6. 用户体验流程

### 6.1 安装

```bash
curl -fsSL https://github.com/hwuu/cloudcode/releases/latest/download/install.sh | bash
```

### 6.2 部署流程

```
+----------+     check env     +-----------+     create VPC     +-----------+
|  Start   | ----------------> |  Prompt   | ----------------> |  Create   |
| cloudcode|                   |  Config   |                   |  VPC/VSwitch
|  deploy  |                   +-----------+                   +-----------+
+----------+                                                        |
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
$ cloudcode deploy

🚀 CloudCode 阿里云一键部署工具

[1/6] 检查环境变量...
✓ ALICLOUD_ACCESS_KEY_ID 已设置
✓ ALICLOUD_ACCESS_KEY_SECRET 已设置

[2/6] 配置访问信息:
请输入域名 (推荐使用自有域名，留空使用 nip.io):
✓ 将使用 nip.io: <eip>.nip.io
  ⚠️  注意: nip.io 是公共服务，可能因 Let's Encrypt 速率限制导致证书签发失败
请输入管理员用户名 [admin]:
请输入管理员密码: ****
请确认管理员密码: ****
请输入管理员邮箱: admin@example.com

[3/6] 配置 OpenCode:
请选择 AI 模型提供商:
  1) OpenAI
  2) Anthropic
  3) 自定义
选择 [1]: 1
请输入 OpenAI API Key: sk-xxx
请输入 Base URL [回车使用默认]:

[4/6] 创建云资源:
✓ 创建 SSH 密钥对 (~/.cloudcode/ssh_key)
✓ 创建 VPC (vpc-xxx)
✓ 创建交换机 (vsw-xxx)
✓ 创建安全组 (sg-xxx) - 开放 22/80/443
✓ 创建 ECS 实例 (i-xxx) - Ubuntu 24.04, 规格: ecs.e-c1m2.large
✓ 分配 EIP (eip-xxx) - IP: 47.123.45.67
✓ 绑定 EIP 到 ECS

[5/6] 部署应用:
✓ SSH 连接 ECS 成功
✓ 安装 Docker 和 Docker Compose
✓ 上传配置文件
✓ 部署 Docker Compose 栈
✓ 等待服务启动...

[6/6] 验证服务:
✓ HTTPS 可访问
✓ Authelia 登录页正常

─────────────────────────────────────────────────────────────
✅ 部署完成！

📱 访问地址: https://47.123.45.67.nip.io
👤 用户名: admin
🔑 密码: <你设置的密码>

⚠️ 认证流程: 用户名 + 密码 → Passkey 验证
⚠️ 首次登录后建议注册 Passkey 作为第二因素！

💡 提示:
   - 查看状态: cloudcode status
   - 清理资源: cloudcode destroy
   - SSH 访问: ssh -i ~/.cloudcode/ssh_key root@47.123.45.67
─────────────────────────────────────────────────────────────
```

### 6.3 查看状态

```bash
$ cloudcode status

CloudCode 部署状态
─────────────────────────────────────────────────────────────
云资源:
  ECS:  i-xxx (Running) - 47.123.45.67
  VPC:  vpc-xxx
  EIP:  eip-xxx - 47.123.45.67

容器状态 (via SSH):
  caddy:     running (Up 3 days)
  authelia:  running (Up 3 days)
  opencode:  running (Up 3 days)

访问地址: https://47.123.45.67.nip.io
─────────────────────────────────────────────────────────────
```

### 6.4 首次登录（注册 Passkey）

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

### 6.5 日常使用

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

### 6.6 域名 DNS 配置（自有域名）

如果使用自有域名，需要在 DNS 服务商配置解析。

**方式一：A 记录（推荐）**

| 记录类型 | 主机记录 | 记录值 |
|----------|----------|--------|
| A | opencode | 47.123.45.67 |

最终访问地址：`https://opencode.example.com`

**方式二：CNAME**

| 记录类型 | 主机记录 | 记录值 |
|----------|----------|--------|
| CNAME | opencode | 47.123.45.67.nip.io |

**阿里云 DNS 配置命令**：

```bash
aliyun alidns AddDomainRecord \
  --DomainName example.com \
  --RR opencode \
  --Type A \
  --Value 47.123.45.67
```

`cloudcode deploy` 完成后会输出 DNS 配置提示：

```
⚠️  如使用自有域名，请配置 DNS 解析：
  记录类型: A
  主机记录: opencode
  记录值:   47.123.45.67

  配置完成后访问: https://opencode.example.com
```

---

## 7. 成本估算

### 按量付费（月度）

| 资源项 | 规格 | 单价 | 月费用 (USD) | 备注 |
|--------|------|------|-------------|------|
| ECS | ecs.e-c1m2.large (2C4G) | ~$0.02/h | ~$15 | 新加坡地域，Ubuntu 24.04 |
| 系统盘 | ESSD 60GB | ~$0.05/GB | ~$3 | 云盘费用 |
| EIP | 按流量计费 | ~$0.003/h + $0.08/GB | ~$4 | 固定费 ~$2 + 流量费 ~$2 (预估 2.5GB) |
| **总计** | | | **~$22/月** | **约 ¥160/月** |

### 成本优化建议

| 优化方式 | 节省幅度 | 实现方式 |
|----------|----------|----------|
| 抢占式实例 | 70-90% | 设置自动竞价，但可能被回收 |
| 停止不用时 | 按实际使用 | 手动停止 ECS，仅保留 EIP 费用 |
| 改按带宽计费 | 高频访问时更省 | 适合日均流量 > 3GB 的场景 |

### 与其他方案对比

| 方案 | 月费用 | HTTPS | 两步认证 | 运维复杂度 |
|------|--------|-------|---------|-----------|
| **cloudcode (本方案)** | ~$22 | ✅ 自动 | ✅ | 中 |
| SAE 托管 | ~$110 | ✅ 自动 | 需额外配置 | 低 |
| 原设计（无认证） | ~$18 | ❌ 无 | ❌ | 中 |

---

## 8. 实现规划

### 8.1 目录结构

详见 [5.1.1 Go 包结构](#511-go-包结构)。除源码外，仓库还包含：

```
cloudcode/
├── .github/
│   └── workflows/
│       └── release.yml            # GitHub Actions 发布流程
├── .gitignore
└── ...                            # 其余结构见 5.1.1
```

### 8.2 实现步骤

| 步骤 | 任务 | 依赖 | 验证方式 |
|------|------|------|----------|
| 1 | Go 项目初始化 + cobra CLI 框架 | 无 | `go build && cloudcode --help` |
| 2 | internal/alicloud: 阿里云资源创建 | 步骤 1 | 能创建 VPC/ECS/EIP |
| 3 | internal/config: 状态文件读写 | 步骤 1 | state.json 正确持久化 |
| 4 | internal/remote: SSH/SFTP 远程操作 | 步骤 2 | 能 SSH 到 ECS 执行命令 |
| 5 | internal/template: 模板渲染 | 步骤 1 | 模板正确渲染为配置文件 |
| 6 | deploy 命令: 串联完整部署流程 | 步骤 2-5 | `cloudcode deploy` 端到端成功 |
| 7 | status 命令: SSH 检查容器状态 | 步骤 3-4 | `cloudcode status` 正确输出 |
| 8 | destroy 命令: 释放所有资源 | 步骤 2-3 | `cloudcode destroy` 资源全部释放 |
| 9 | goreleaser + GitHub Actions | 步骤 6-8 | tag 推送后自动发布二进制 |
| 10 | install.sh + 端到端测试 | 步骤 9 | `curl \| bash` 安装后完整流程通过 |

### 8.3 测试要点

| 测试项 | 测试方法 | 验证标准 |
|--------|----------|----------|
| 部署 | `cloudcode deploy` 完整流程 | 所有资源创建成功，服务可访问 |
| HTTPS 访问 | `curl -I https://<eip>.nip.io` | 返回 302 重定向到登录页 |
| 密码登录（1FA） | 浏览器输入用户名密码 | 进入 2FA 页面 |
| Passkey 验证（2FA） | 使用 Passkey 验证 | 登录成功进入 OpenCode |
| OpenCode 功能 | 在 Web UI 中使用 AI 对话 | 正常响应 |
| 状态查看 | `cloudcode status` | 正确显示资源和容器状态 |
| 销毁 | `cloudcode destroy` | 所有资源释放，账单停止 |
| 重复部署 | 已部署后再次执行 `cloudcode deploy` | 幂等处理，不重复创建资源 |
| 部署中断恢复 | 中途 Ctrl+C 后再次执行 | 能检测已有资源并继续 |
| Session 过期 | 等待 30 分钟不操作 | 自动跳转到登录页 |
| 密码重置 | 查看 notification.txt 获取重置链接 | 能成功重置密码 |
| 跨平台构建 | goreleaser 构建 linux/darwin amd64/arm64 | 四个二进制均可运行 |
| 配置更新重部署 | 修改配置后 `cloudcode deploy --force` | 仅更新应用层，不重建云资源 |
| destroy --dry-run | `cloudcode destroy --dry-run` | 仅显示将删除的资源，不实际执行 |

---

## 9. 运维与备份

### 9.1 数据备份策略

Docker Volume 和宿主机目录中存储了重要数据，建议定期备份：

| 数据 | 存储位置 | 类型 | 备份方式 | 频率 |
|------|----------|------|----------|------|
| 工作区文件 | opencode_workspace | Named Volume | ECS 快照 / rsync | 每周 |
| OpenCode 配置 | opencode_config | Named Volume | ECS 快照 | 每周 |
| Authelia 配置及用户数据 | ./authelia/ | Bind Mount | ECS 快照 / tar | 每月 |
| SSL 证书 | caddy_data | Named Volume | 自动续期 | 无需备份 |

**备份命令示例：**

```bash
# 在 ECS 上执行
docker run --rm \
  -v cloudcode_opencode_workspace:/data \
  -v $(pwd)/backup:/backup \
  alpine tar czf /backup/workspace-$(date +%Y%m%d).tar.gz /data

# 或使用阿里云 ECS 快照 (需要先查询磁盘 ID)
# aliyun ecs DescribeDisks --InstanceId i-xxx --query 'Disks.Disk[*].DiskId' --output text
aliyun ecs CreateSnapshot --DiskId d-xxx --SnapshotName "cloudcode-backup-$(date +%Y%m%d)"
```

### 9.2 常见运维任务

| 任务 | 命令 |
|------|------|
| 查看日志 | `ssh -i ~/.cloudcode/ssh_key root@<EIP> docker logs opencode` |
| 重启服务 | `ssh -i ~/.cloudcode/ssh_key root@<EIP> "cd ~/cloudcode && docker compose restart"` |
| 更新 OpenCode | `ssh -i ~/.cloudcode/ssh_key root@<EIP> "cd ~/cloudcode && docker compose build opencode && docker compose up -d opencode"` |

**密码重置流程：**

Authelia 使用文件系统通知器，密码重置链接会写入 `/config/notification.txt`：

**触发方式：** 在 Authelia 登录页点击 "Forgot Password?" 链接，输入用户名后，重置链接会写入该文件。

```bash
# SSH 到 ECS 后查看重置链接
ssh -i ~/.cloudcode/ssh_key root@<EIP> cat ~/cloudcode/authelia/notification.txt

# 输出示例:
# Date: 2026-02-14 12:00:00
# Recipient: admin
# Link: https://your-domain/auth/reset-password/identity/verify?token=xxx

# 访问链接后可重置密码
```

**注意**：重置密码不会影响已注册的 Passkey，WebAuthn 凭证与密码独立存储。

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

### Go 生态

- [Cobra CLI 框架](https://github.com/spf13/cobra) — Go CLI 标准框架
- [阿里云 Go SDK](https://github.com/alibabacloud-go) — 阿里云官方 Go SDK
- [golang.org/x/crypto/ssh](https://pkg.go.dev/golang.org/x/crypto/ssh) — Go SSH 库
- [goreleaser](https://goreleaser.com/) — Go 项目发布工具

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

**文档版本**: 2.7
**更新日期**: 2026-02-14

**修订记录**：
- v2.7: 六次审查修正 — 总体设计图补充 caddy_config Volume（与 4.1/5.2 一致）
- v2.6: 五次审查修正 — 补充 ECS 默认登录用户说明（阿里云 Ubuntu 默认 root）；补充密码重置触发条件（Forgot Password 链接）；补充 Docker 安装命令（阿里云镜像源）；补充 SSH IP 限制后访问恢复方案；补充状态文件路径解析代码示例；补充测试要点（--force 重部署、--dry-run）
- v2.5: 四次审查修正 — 核心依赖补充 STS SDK；4.1 存储层补充 caddy_config Volume；Authelia webauthn timeout 改为 duration 格式（60s）；移除废弃的 session.name 顶层字段；Authelia 配置文件名 config.yml → configuration.yml（4.38+ 默认路径）；新增部署文件映射表（源文件→渲染→ECS 目标路径）
- v2.4: 三次审查修正 — 总体设计图补充 opencode_config Volume；destroy 流程补充删除 SSH 密钥对；前置检查余额阈值 ¥10 → ¥50；安全组 SSH 规则补充实现细节（自动检测公网 IP 或交互询问）；Caddyfile 补充模板渲染说明；Dockerfile 补充 Volume 目录权限设置；配额检查补充具体检查项表格；新增可用区选择策略（自动尝试其他可用区）
- v2.3: 二次审查修正 — 总体设计图 EIP 移至浏览器与 ECS 之间（修正流量方向）；总体设计图 Volume 命名统一为下划线；交叉引用 5.1.8→5.1.9；EIP 已按流量计费，成本优化建议改为"改按带宽计费"；Docker Compose opencode 服务改用 env_file 加载 .env；Dockerfile opencode 安装自适应 CPU 架构
- v2.2: 全文审查修正 — 修正章节编号乱序（5.1.7→5.1.9）；统一 Volume 命名为下划线风格；修正 EIP 计费模型（按流量计费，去除矛盾的带宽+流量并存）；修正密码重置不影响 Passkey 的说明；destroy 流程补充 EIP 解绑步骤；docker-compose.yml 不使用 .tmpl 后缀；8.1 目录结构去重引用 5.1.1；明确 --force 仅重新部署应用层
- v2.1: 采纳评审意见 — state.json SSH 路径改为相对路径（运行时解析）；install.sh 加 sudo；goreleaser.yml 补全 ldflags/archives/checksum/extra_files；version 命令输出 build info；destroy 命令增加确认流程和 --dry-run；新增 5.1.7 前置检查（余额、配额、SDK 连接）；新增 6.6 域名 DNS 配置说明
- v2.0: 重大重构 — 从 Bash 脚本方案改为 Go CLI 工具；新增 3.5 实现语言决策、3.6 分发方式决策；重写 5.1 为 Go 包结构和 CLI 设计；新增安装脚本和构建发布流程；模板语法从 Jinja2 改为 Go template；状态文件迁移到 ~/.cloudcode/state.json；SSH 密钥迁移到 ~/.cloudcode/ssh_key；新增 status 命令（SSH 检查容器状态）；更新用户体验流程和实现规划
- v1.8: 补充 env.j2 模板内容（新增 5.4 环境变量章节）；修正成本估算（增加系统盘费用 $3，总计 ~$22/月）
- v1.7: 补充 env.j2 模板和 render_env_file() 函数；补充 ECS 系统盘大小配置 (60GB)；新增 5.1.5 幂等性与失败回滚章节；简化 Caddy reverse_proxy 配置（移除冗余 header_up）；补充密码重置流程说明；补充边界测试场景
- v1.6: Authelia 改用 bind mount (./authelia:/config)，便于部署时预填充配置文件；明确区分 Named Volumes（运行时数据）和 Bind Mounts（配置文件）；修正总体设计图存储描述
- v1.5: 修正 Docker Compose command 与 ENTRYPOINT 冲突；Authelia 添加 storage.encryption_key；Caddyfile 添加 /auth 重定向；统一 authelia 存储描述为 named volume (authelia_config)
- v1.4: Caddyfile /auth/* 路由改用 handle_path 自动剥离前缀；修正 ECS 快照 API 参数为 --DiskId
- v1.3: 修正 Authelia 4.38+ 配置：server.address 格式、session.cookies.authelia_url、access_control bypass 自身路径；Caddy 日志改为 stdout；Dockerfile 注释说明 golang-go 可选安装
- v1.2: 修正 Caddyfile 语法（合并为单一 site block，使用 handle 区分路由）；更新 Authelia forward auth 端点为 4.38+ 格式 `/api/authz/forward-auth`
- v1.1: 修正 Authelia Passkey 为 2FA（非一级认证）；补充 80 端口；补充 SSH 密钥管理、ECS 镜像、备份策略；移除 Docker-in-Docker 决策理由；更新 Authelia 配置格式为 4.38+ 版本
