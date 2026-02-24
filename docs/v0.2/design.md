# CloudCode v0.2.0 设计文档

## 目录

- [1. 背景与目标](#1-背景与目标)
  - [1.1 v0.1.x 回顾](#11-v01x-回顾)
  - [1.2 v0.2.0 目标](#12-v020-目标)
  - [1.3 非目标](#13-非目标)
- [2. 设计决策](#2-设计决策)
  - [2.1 容器重命名：为什么从 opencode 改为 devbox](#21-容器重命名为什么从-opencode-改为-devbox)
  - [2.2 Web Terminal：为什么选 ttyd 内置而非独立容器](#22-web-terminal为什么选-ttyd-内置而非独立容器)
  - [2.3 DNS 方案：为什么优先阿里云 DNS + nip.io 兜底](#23-dns-方案为什么优先阿里云-dns--nipio-兜底)
  - [2.4 按需使用：为什么选 ECS 停机而非磁盘快照](#24-按需使用为什么选-ecs-停机而非磁盘快照)
  - [2.5 跨版本快照兼容策略](#25-跨版本快照兼容策略)
  - [2.6 状态存储：为什么用 OSS 而非本地文件](#26-状态存储为什么用-oss-而非本地文件)
- [3. 组件设计](#3-组件设计)
  - [3.1 重命名 opencode → devbox](#31-重命名-opencode--devbox)
  - [3.2 浏览器 Web Terminal](#32-浏览器-web-terminal)
  - [3.3 自有域名 + 自动 DNS 更新](#33-自有域名--自动-dns-更新)
  - [3.4 按需使用 — suspend/resume + 可选快照](#34-按需使用--suspendresume--可选快照)
  - [3.5 cloudcode init — 统一配置管理](#35-cloudcode-init--统一配置管理)
  - [3.6 OSS 状态管理与分布式锁](#36-oss-状态管理与分布式锁)
- [4. 实现规划](#4-实现规划)
  - [4.1 优先级](#41-优先级)
  - [4.2 依赖关系](#42-依赖关系)
  - [4.3 实现步骤](#43-实现步骤)
- [5. 成本影响](#5-成本影响)
- [变更记录](#变更记录)

---

## 1. 背景与目标

### 1.1 v0.1.x 回顾

v0.1.x 实现了 CloudCode 的核心功能：一键部署 OpenCode 到阿里云 ECS，通过 Caddy + Authelia 提供 HTTPS 和两步认证。

v0.1.x 在实际使用中暴露了以下痛点：

| 痛点 | 说明 |
|------|------|
| **HTTPS 证书不稳定** | nip.io 共享 Let's Encrypt 速率限制，经常申请失败 |
| **destroy 丢数据** | 销毁 ECS 后会话历史、配置、安装的工具全部丢失 |
| **无浏览器终端** | 需要本地 SSH 客户端才能进入容器执行命令 |
| **容器命名不准确** | 容器名 `opencode` 不再反映其实际内容（含 nodejs、python、gh 等） |

### 1.2 v0.2.0 目标

| 目标 | 说明 |
|------|------|
| **稳定 HTTPS** | 支持自有域名 + 阿里云 DNS 自动关联 EIP，nip.io 作为兜底 |
| **按需使用** | suspend/resume 秒级恢复，destroy 可选保留快照 |
| **浏览器终端** | 内置 ttyd，浏览器直接进入 devbox 容器执行命令 |
| **容器重命名** | opencode → devbox，反映"开发环境"定位 |
| **统一配置** | `cloudcode init` 一次配置凭证，取代环境变量 |

### 1.3 非目标

- **不做多 DNS 平台适配**：仅自动支持阿里云 DNS，其他平台手动配置
- **不做增量快照**：每次 destroy 创建全量快照，不做增量备份链
- **不做部署交互回退**：当前仅 3 步交互，回退价值不大，暂缓
- **不做配置热更新**：running 实例的配置（Caddyfile、Authelia 等）不支持在线更新，需 destroy 后重新 deploy

---

## 2. 设计决策

### 2.1 容器重命名：为什么从 opencode 改为 devbox

容器内已不只运行 opencode，还包含 nodejs、python3、gh CLI，v0.2.0 还会加入 ttyd。`opencode` 这个名字容易误导，改为 `devbox` 更准确地反映"开发环境容器"的定位。

**影响范围**：Docker 镜像名、容器名、Dockerfile、docker-compose 服务名、volume 名前缀、CI workflow、代码引用、文档。

### 2.2 Web Terminal：为什么选 ttyd 内置而非独立容器

| 维度 | 独立 ttyd 容器 | 内置到 devbox |
|------|---------------|--------------|
| 安全性 | 需挂载 docker.sock（高风险） | 无需 docker.sock |
| 实现复杂度 | 低（独立镜像） | 中（修改 Dockerfile + entrypoint） |
| 维护成本 | 多一个容器管理 | 统一在 devbox 内 |
| 用户体验 | 相同 | 相同 |

**决策**：内置到 devbox 容器。

**理由**：
1. 避免挂载 docker.sock 的安全风险 — docker.sock 等同于 root 权限，一旦 ttyd 被攻破，攻击者可控制宿主机所有容器
2. 减少容器数量，简化运维
3. ttyd 二进制仅 ~3MB，对镜像体积影响可忽略

**代价**：
1. devbox 容器需要同时运行两个进程（opencode + ttyd），通过 `init: true`（tini）+ bash 脚本管理
2. ttyd 崩溃时由脚本自动重启，不影响 opencode 主进程
3. ttyd 安全完全依赖 Authelia forward_auth，若 Caddy 配置出错可能直接暴露终端。需确保 Caddyfile 模板中 forward_auth 覆盖所有路由

### 2.3 DNS 方案：为什么优先阿里云 DNS + nip.io 兜底

| 方案 | 优点 | 缺点 |
|------|------|------|
| 仅 nip.io | 零配置 | Let's Encrypt 速率限制，经常失败 |
| 仅自有域名 | 稳定 | 强制用户购买域名 |
| 阿里云 DNS 自动 + nip.io 兜底 | 有域名时全自动，无域名时也能用 | 非阿里云 DNS 需手动配置 |
| 多 DNS 平台适配 | 最灵活 | 实现复杂，每个平台一套 API |

**决策**：阿里云 DNS 自动更新 + nip.io 兜底。

**理由**：
1. CloudCode 已深度绑定阿里云（ECS/VPC/EIP），用户大概率也在阿里云管理域名
2. 阿里云 DNS SDK 与现有 SDK 体系一致，实现成本低
3. nip.io 兜底保证无域名用户也能使用（虽然证书可能不稳定）
4. 非阿里云域名用户可手动配置 DNS，deploy 时轮询等待生效

**域名拆分策略**：用户输入 `oc.example.com`，需拆分为主域名和主机记录。多级后缀（如 `.co.uk`）难以通过字符串解析处理，因此调用 `DescribeDomains` API 获取用户域名列表自动匹配。

### 2.4 按需使用：为什么选 ECS 停机而非磁盘快照

用户期望按需使用 cloudcode：不用时省钱，用时快速恢复，且"一切和原来一样"。

| 方案 | suspend 费用 | resume 速度 | 恢复完整度 | 实现复杂度 |
|------|------------|-----------|----------|----------|
| A: 删 ECS + 磁盘快照 + 保留 EIP | ~$4/月 | 5-10 分钟 | 完美 | 高 |
| B: ECS 停机（StopCharging） | ~$1.2/月 | 几秒 | 完美 | 极低 |

**决策**：选择方案 B（ECS 停机）作为 suspend/resume 的核心机制，磁盘快照作为 destroy 时的可选安全网。

**前置条件**：StopCharging 仅适用于**按量付费**实例，包年包月实例不支持。CloudCode 默认创建按量付费实例。

**理由**：
1. 阿里云按量付费 ECS 支持"停机不收费"（`StoppedMode: StopCharging`），停机后释放 CPU/内存，只收磁盘费
2. resume 只需 `StartInstance`，几秒恢复，用户体验远优于快照重建
3. 费用更低（$1.2/月 vs $4/月）
4. 实现极简 — 一个 API 调用，无需快照创建/等待/恢复流程
5. EIP 绑定停机实例不收闲置费

**操作模型**：

| 操作 | 行为 | 保留 | 删除 | 停机费用 |
|------|------|------|------|---------|
| `suspend` | 停机省钱 | ECS（停机）、EIP、VPC 等全部 | 无 | ~$1.2/月 |
| `resume` | 恢复运行 | — | — | — |
| `destroy` | 彻底销毁 | 可选保留磁盘快照 | 所有云资源 | 快照 ~$0.40/月 |
| `deploy` | 创建环境 | — | — | 有快照则从快照恢复 |

**磁盘快照的角色**：不再是 suspend/resume 的核心机制，而是 destroy 时的可选安全网。用户可以选择在 destroy 前保留快照，下次 deploy 时从快照恢复 100% 的环境。

**快照生命周期**：只保留最新一份快照。创建新快照成功后，自动删除旧快照。

### 2.5 跨版本快照兼容策略

快照恢复的是整个系统盘（含旧容器、旧镜像、旧 volume 名），但新版本 CLI 上传的配置模板可能期望新架构（如服务名变更、新增路由等）。

注意：快照功能从 v0.2 开始引入，不存在 v0.1 的快照。此策略面向 v0.2.x 及后续版本之间的兼容。

#### 2.5.1 版本定义

CloudCode 使用语义化版本号 `MAJOR.MINOR.PATCH`（如 `0.2.1`）：

- **大版本**：MINOR 变更（如 0.2.x → 0.3.x），可能包含破坏性架构变更（服务名、volume 名等）
- **小版本**：PATCH 变更（如 0.2.0 → 0.2.1），仅 bug 修复和小改进，保证配置兼容

注：当前处于 0.x 阶段，MINOR 视为大版本。进入 1.0 后，MAJOR 变更（1.x → 2.x）为大版本。

#### 2.5.2 版本兼容策略

| 场景 | 行为 |
|------|------|
| 同版本恢复（如 0.2.1 → 0.2.1） | 正常恢复 |
| 跨小版本恢复（如 0.2.1 → 0.2.3） | 正常恢复（小版本保证配置兼容） |
| 跨大版本恢复（如 0.2.x → 0.3.x） | 警告用户，提供选项 |

跨大版本时提供两个选项：

```
检测到快照版本 v0.2.3，当前 CLI 版本 v0.3.0
快照版本不兼容，请选择:
  1) 全新部署（丢弃快照数据）
  2) 迁移恢复（保留数据，但容器内 apt 安装的工具可能丢失）
选择 [1]:
```

#### 2.5.3 设计原则

- **小版本**（patch/minor）保证快照兼容：不改 volume 名、服务名等基础架构
- **大版本**（major feature）允许破坏性变更，但提供迁移脚本
- 迁移脚本随版本发布，放在 `internal/migrate/` 下
- `backup.json` 记录 `cloudcode_version`，deploy 时对比版本决定恢复策略

#### 2.5.4 backup.json 版本字段

```json
{
  "snapshot_id": "s-t4nxxxxxxxxx",
  "cloudcode_version": "0.2.0",
  "created_at": "2026-02-22T10:00:00Z",
  "region": "ap-southeast-1",
  "disk_size": 60
}
```

### 2.6 状态存储：为什么用 OSS 而非本地文件

v0.1.x 将 state.json 存储在本地 `~/.cloudcode/`，存在以下问题：

| 问题 | 说明 |
|------|------|
| **跨机器操作** | 用户可能在笔记本、台式机等多台机器上操作同一实例 |
| **操作中断** | deploy/destroy/suspend 到一半断电，状态不一致 |
| **并发冲突** | 多个终端同时操作，状态竞争 |
| **孤儿资源** | 中途失败后资源泄露，无法恢复 |

#### 2.6.1 方案对比

| 方案 | 优点 | 缺点 |
|------|------|------|
| 本地文件 | 简单 | 无法跨机器，无锁机制 |
| ECS 标签 | 无额外资源 | deploy 初期无 ECS；destroy 后丢失 |
| **OSS bucket** | 全流程可用，支持条件写入作锁，跨机器共享 | 需创建额外资源（成本极低） |
| 云数据库 | 功能完整 | 过于复杂，增加依赖 |
| DNS TXT 记录 | 复用现有域名 | 不适合存大量状态，修改慢 |

**决策**：使用 OSS bucket 存储状态。

#### 2.6.2 选择 OSS 的理由

1. **跨机器共享**：状态存储在云端，任意机器可访问
2. **分布式锁**：OSS 支持条件写入（`If-None-Match: *`），天然适合分布式锁
3. **全生命周期覆盖**：从 deploy 开始到 destroy 结束，状态始终可用
4. **成本极低**：OSS 标准存储 ~0.12 元/GB/月，state.json 几 KB，费用可忽略
5. **无需额外依赖**：用户已有阿里云账号，OSS 是同生态服务

#### 2.6.3 本地 vs OSS 职责划分

| 文件 | 存储 | 理由 |
|------|------|------|
| `credentials` | 本地 | 敏感信息（AccessKey） |
| `ssh_key` | 本地 | 私钥，不应上传云端 |
| `state.json` | OSS | 跨机器共享、分布式锁 |
| `backup.json` | OSS | 跨机器共享 |
| `history/*.json` | OSS | 审计追踪 |

---

## 3. 组件设计

### 3.1 重命名 opencode → devbox

#### 3.1.1 改动清单

| 改动项 | 之前 | 之后 |
|--------|------|------|
| Docker 镜像 | `ghcr.io/hwuu/cloudcode-opencode` | `ghcr.io/hwuu/cloudcode-devbox` |
| 容器名 | `opencode` | `devbox` |
| Dockerfile | `Dockerfile.opencode` | `Dockerfile.devbox` |
| Docker Compose 服务名 | `opencode` | `devbox` |
| Volume 名前缀 | `opencode_*` | `devbox_*` |
| GitHub Actions | 构建推送 `cloudcode-opencode` | 构建推送 `cloudcode-devbox` |

#### 3.1.2 修改文件

| 文件 | 改动 |
|------|------|
| `internal/template/templates/Dockerfile.opencode` | 重命名为 `Dockerfile.devbox` |
| `internal/template/templates/docker-compose.yml.tmpl` | 服务名、镜像名、volume 名 |
| `internal/template/templates/Caddyfile.tmpl` | `reverse_proxy devbox:4096` |
| `internal/template/render.go` | 文件名引用更新 |
| `.github/workflows/docker.yml` | 镜像名改为 `cloudcode-devbox` |
| `.github/workflows/release.yml` | 镜像名改为 `cloudcode-devbox` |
| `internal/deploy/deploy.go` | 容器名引用 |
| `internal/deploy/status.go` | 容器名引用 |
| `tests/unit/` | 相关测试更新 |

---

### 3.2 浏览器 Web Terminal

#### 3.2.1 架构

```
+-------------------+
|      Browser      |
+---------+---------+
          |
          v
+---------+---------+
|       Caddy       |
|  (reverse proxy)  |
+---+-----+-----+---+
    |     |     |
    v     v     v
  auth. /term-  /*
  dom   inal/*
    |     |     |
    v     |     |
+---+-+  |     |
|Auth |  |     |
|elia |  |     |
+-----+  |     |
         v     v
      +-+------+--+
      |  devbox   |
      | (opencode |
      |  + ttyd)  |
      +-----------+
```

沿用 v0.1 的子域名模式（`auth.{{ .Domain }}`）。Authelia 路径模式（`/auth/*`）需要额外配置 `server.path`，静态资源路径容易出错，社区主流方案均为子域名模式。

请求分发：
- `auth.{{ .Domain }}` → Authelia 容器（登录/注册页面）
- `{{ .Domain }}/terminal/*` → devbox 容器的 ttyd（需 forward_auth 认证）
- `{{ .Domain }}/*` → devbox 容器的 opencode（需 forward_auth 认证，不支持 base-path 必须运行在根路径）

#### 3.2.2 Dockerfile.devbox 改动

在 `USER opencode` 指令**之前**安装 ttyd 二进制（需要 root 权限），ENTRYPOINT 放在 Dockerfile 最后：

```dockerfile
# === 在 USER opencode 之前（需要 root 权限）===

# 安装 ttyd
ARG TARGETARCH
RUN TTYD_ARCH=$(case "$TARGETARCH" in amd64) echo "x86_64" ;; arm64) echo "aarch64" ;; esac) && \
    curl -fsSL -o /usr/local/bin/ttyd \
    "https://github.com/tsl0922/ttyd/releases/latest/download/ttyd.${TTYD_ARCH}" && \
    chmod +x /usr/local/bin/ttyd

# 复制 entrypoint 脚本
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# === 原有指令（创建用户、安装 opencode 等）===

RUN useradd -m -s /bin/bash opencode ...
USER opencode
WORKDIR /home/opencode
# ... 其他原有内容 ...

# === Dockerfile 最后 ===

# 覆盖默认 ENTRYPOINT（必须在最后）
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
```

**说明**：ttyd 安装和 entrypoint.sh 复制必须在 `USER opencode` 之前完成，因为需要 root 权限写入 `/usr/local/bin/`。ENTRYPOINT 指令放在 Dockerfile 最后是 Docker 最佳实践。

#### 3.2.3 启动脚本

docker-compose 配置 `init: true` 注入 tini 作为 PID 1，处理信号转发和僵尸进程回收。entrypoint 脚本管理两个进程：

```bash
#!/bin/bash
# entrypoint.sh

# ttyd 后台运行，崩溃自动重启
# 注：脚本以 opencode 用户执行（Dockerfile 中 USER opencode）
while true; do
    ttyd --port 7681 --base-path /terminal /bin/bash
    sleep 1
done &

# opencode 作为主进程
exec opencode web --hostname 0.0.0.0 --port 4096
```

**用户上下文说明**：
- Dockerfile 中设置了 `USER opencode`，entrypoint 脚本及其子进程均以 `opencode` 用户身份运行
- ttyd 启动的 bash 自然继承了 `opencode` 用户身份，用户在 Web Terminal 中的环境与 SSH 登录一致
- 端口 7681 和 4096 均 > 1024，opencode 用户有权限绑定

opencode 作为主进程，退出时容器重启（`restart: unless-stopped`），ttyd 随之重启。ttyd 单独崩溃时由 while 循环自动重启，不影响 opencode。

#### 3.2.4 Caddyfile.tmpl 路由

```caddyfile
{{ .Domain }} {
    # Web Terminal（需认证）
    # 注：使用 handle 而非 handle_path，保留 /terminal 前缀，
    # 因为 ttyd 配置了 --base-path /terminal，期望收到带前缀的请求
    handle /terminal/* {
        forward_auth authelia:9091 {
            uri /api/authz/forward-auth
            copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
        }
        reverse_proxy devbox:7681
    }

    # OpenCode Web UI（需认证）
    # 注：opencode web 不支持 base-path，必须运行在根路径
    handle {
        forward_auth authelia:9091 {
            uri /api/authz/forward-auth
            copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
        }
        reverse_proxy devbox:4096
    }
}

{{ .Domain }}:8443 {
    # 8443 备用端口（部分网络环境 443 被封锁时使用）
    # 路由规则与主站点相同
    handle /terminal/* {
        forward_auth authelia:9091 {
            uri /api/authz/forward-auth
            copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
        }
        reverse_proxy devbox:7681
    }

    handle {
        forward_auth authelia:9091 {
            uri /api/authz/forward-auth
            copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
        }
        reverse_proxy devbox:4096
    }
}

auth.{{ .Domain }} {
    reverse_proxy authelia:9091
}

auth.{{ .Domain }}:8443 {
    reverse_proxy authelia:9091
}
```

**路由策略说明**：

- `auth.{{ .Domain }}` → Authelia 登录/注册页面（独立子域名，沿用 v0.1 方案）
- `{{ .Domain }}/terminal/*` → ttyd Web Terminal（需 forward_auth 认证，使用 `handle` 保留路径前缀，配合 ttyd `--base-path /terminal`）
- `{{ .Domain }}/*`（根路径）→ opencode Web UI（需 forward_auth 认证，不支持 base-path）

**Caddy 路由优先级**：`handle` 指定具体路径会优先匹配，`handle`（无参数）作为兜底匹配所有未匹配的请求。

**`handle` vs `handle_path`**：`handle_path` 会 strip 路径前缀再转发（如 `/terminal/ws` → `/ws`），而 `handle` 保留原始路径。ttyd 配置了 `--base-path /terminal`，期望收到带 `/terminal` 前缀的请求，因此必须使用 `handle`。

用户访问 `https://<domain>/` 直接进入 opencode Web UI，访问 `https://<domain>/terminal/` 进入 Web Terminal。

#### 3.2.5 docker-compose.yml

```yaml
  devbox:
    image: ghcr.io/hwuu/cloudcode-devbox:latest
    container_name: devbox
    restart: unless-stopped
    init: true
    expose:
      - 4096
      - 7681
    # ... volumes, env_file, networks ...
```

`init: true` 让 Docker 自动注入 tini 作为 PID 1。

#### 3.2.6 修改文件

| 文件 | 改动 |
|------|------|
| `internal/template/templates/Dockerfile.devbox` | 在 USER opencode 之前：安装 ttyd、COPY entrypoint.sh、添加 ENTRYPOINT 指令 |
| `internal/template/templates/entrypoint.sh` | 新增文件：ttyd + opencode 双进程管理脚本 |
| `internal/template/templates/docker-compose.yml.tmpl` | devbox 暴露 7681 端口，添加 init: true |
| `internal/template/templates/Caddyfile.tmpl` | 新增 /terminal 路由，Authelia 保持子域名模式 |

#### 3.2.7 测试要点

- 浏览器访问 `https://<domain>/terminal/`，经 Authelia 认证后进入 bash
- 在 Web Terminal 中执行 `opencode -v`、`git status` 等命令
- 未认证时访问 /terminal 应跳转到登录页
- opencode Web UI 和 ttyd 同时正常运行
- 容器重启后两个服务自动恢复

---

### 3.3 自有域名 + 自动 DNS 更新

#### 3.3.1 用户场景

| 场景 | 域名输入 | DNS 托管 | 行为 |
|------|---------|---------|------|
| 无域名 | 留空 | — | 使用 `<EIP>.nip.io`，Let's Encrypt 可能失败 |
| 阿里云域名 | `oc.example.com` | 阿里云 DNS | SDK 自动创建/更新两条 A 记录（`oc` + `auth.oc`） |
| 外部域名 | `oc.example.com` | Cloudflare 等 | 提示用户手动配置两条 A 记录，轮询等待生效 |

示例：用户输入 `oc.example.com`，系统拆分为 baseDomain=`example.com`，主机记录=`oc`，最终创建两条 A 记录：
- `oc.example.com → EIP`
- `auth.oc.example.com → EIP`

#### 3.3.2 deploy 流程

```
+---------------------+
| Prompt: Domain      |
| (empty = nip.io)    |
+----------+----------+
           |
           v
+----------+----------+
| Create ECS + EIP    |
+----------+----------+
           |
           v
+----------+----------+
| isCustomDomain?     |
+----+----------+-----+
     |          |
    yes         no
     |          |
     v          v
+----+------+ +-+------------+
| Describe  | | Use          |
| Domains   | | <EIP>.nip.io |
+----+------+ +--------------+
     |
     v
+----+----------+
| Domain found  |
| in Alibaba?   |
+----+------+---+
     |      |
    yes     no
     |      |
     v      v
+----+--+ +-+------------+
| Auto  | | Print DNS    |
| update| | config, wait |
| A rec | | propagation  |
+-------+ +--------------+
```

#### 3.3.3 DNS API 接口

新增 `internal/alicloud/dns.go`：

```go
type DnsAPI interface {
    DescribeDomains(req *dnsclient.DescribeDomainsRequest) (*dnsclient.DescribeDomainsResponse, error)
    DescribeDomainRecords(req *dnsclient.DescribeDomainRecordsRequest) (*dnsclient.DescribeDomainRecordsResponse, error)
    AddDomainRecord(req *dnsclient.AddDomainRecordRequest) (*dnsclient.AddDomainRecordResponse, error)
    UpdateDomainRecord(req *dnsclient.UpdateDomainRecordRequest) (*dnsclient.UpdateDomainRecordResponse, error)
}
```

#### 3.3.4 域名拆分

```go
// FindBaseDomain matches user input against Alibaba Cloud DNS domain list.
//
// Input:  fullDomain="oc.example.com", domains=["example.com", "other.org"]
// Output: baseDomain="example.com", rr="oc"
//
// Input:  fullDomain="oc.example.co.uk", domains=["example.co.uk"]
// Output: baseDomain="example.co.uk", rr="oc"
func FindBaseDomain(fullDomain string, domains []string) (baseDomain, rr string, err error)
```

通过 `DescribeDomains` API 获取用户域名列表，逐个尝试后缀匹配，避免手动解析多级后缀。

#### 3.3.5 A 记录管理

需要创建/更新两条 A 记录（沿用 v0.1 的 Authelia 子域名模式）：

| A 记录 | 记录值 | 用途 |
|--------|--------|------|
| `oc.example.com` | EIP | 主域名（opencode + ttyd） |
| `auth.oc.example.com` | EIP | Authelia 认证子域名 |

其中 `oc` 和 `auth.oc` 为主机记录，`example.com` 为 baseDomain，由 `FindBaseDomain` 拆分得到。

```go
// EnsureDNSRecord creates or updates a single A record.
// If record exists with different IP, update it.
// If record doesn't exist, create it.
// 调用方需分别为主域名和 auth 子域名各调用一次。
func EnsureDNSRecord(cli DnsAPI, baseDomain, rr, ip string) error
```

#### 3.3.6 非阿里云域名处理

```go
d.printf("请配置 DNS A 记录:\n")
d.printf("  %s  →  %s\n", cfg.Domain, eip)
d.printf("  auth.%s  →  %s\n", cfg.Domain, eip)
d.printf("等待 DNS 生效...\n")
// Poll every 5s, timeout 5min
waitForDNS(cfg.Domain, eip, 5*time.Minute)
```

`waitForDNS` 使用 `net.LookupHost` 轮询，直到解析结果匹配 EIP 或超时。

#### 3.3.7 修改文件

| 文件 | 改动 |
|------|------|
| `go.mod` | 新增 `github.com/alibabacloud-go/alidns-20150109/v4` |
| `internal/alicloud/dns.go` | DnsAPI 接口、FindBaseDomain、EnsureDNSRecord |
| `internal/alicloud/interfaces.go` | 新增 DnsAPI 接口定义 |
| `internal/deploy/deploy.go` | deploy 流程集成 DNS 更新 |
| `internal/deploy/dns.go` | waitForDNS 轮询逻辑 |
| `internal/template/templates/authelia/configuration.yml.tmpl` | session domain 配置适配新域名 |
| `cmd/cloudcode/main.go` | 初始化 DNS client |
| `tests/unit/alicloud_test.go` | DNS mock 和测试 |

#### 3.3.8 测试要点

- 阿里云 DNS 域名：deploy 后 A 记录自动指向 EIP
- 非阿里云 DNS 域名：提示手动配置，轮询等待生效
- 重复 deploy（EIP 变化）后 DNS 记录自动更新
- 留空域名时使用 nip.io 兜底
- FindBaseDomain 正确匹配多级域名（`.com`、`.co.uk` 等）
- DNS API 失败时给出明确错误提示

---

### 3.4 按需使用 — suspend/resume + 可选快照

#### 3.4.1 suspend 流程

```
+----------------------+
| cloudcode suspend    |
+-----------+----------+
            |
            v
+-----------+----------+
| Confirm suspend?     |
| [y/N]                |
+----+----------+------+
    yes         no
     |           |
     v           v
+----+--------+ +----+--------+
| StopInstance | | Abort       |
| (StopCharging| | (exit)      |
|  mode)       | +-------------+
+----+--------+
     |
     v
+----+----------+
| Wait for       |
| Stopped        |
+----+----------+
     |
     v
+----+----------+
| Update         |
| state.json     |
| (status:       |
|  suspended)    |
+---------------+
```

实现极简：一个 `StopInstance` API 调用，设置 `StoppedMode: StopCharging`。停机后 CPU/内存释放不收费，仅收磁盘费 ~$1.2/月。

**关于 EIP**：StopCharging 模式会释放实例的公网 IP，但 CloudCode 使用的是独立 EIP（弹性公网 IP），EIP 是独立资源，不受 StopCharging 影响，停机期间保持绑定且不收闲置费。

#### 3.4.2 resume 流程

```
+----------------------+
| cloudcode resume     |
+-----------+----------+
            |
            v
+-----------+----------+
| Confirm resume?      |
| [y/N]                |
+----+----------+------+
    yes         no
     |           |
     v           v
+----+--------+ +----+--------+
| StartInstance| | Abort       |
+----+--------+ | (exit)      |
     |          +-------------+
     v
+----+----------+
| Wait for       |
| Running        |
+----+----------+
     |
     v
+----+----------+
| SSH connect    |
+----+----------+
     |
     v
+----+----------+
| Health check   |
| (docker compose|
|  ps)           |
+----+----------+
     |
     v
+----+----------+
| Update         |
| state.json     |
| (status:       |
|  running)      |
+---------------+
```

`StartInstance` 后几秒恢复，Docker 容器随 `restart: unless-stopped` 自动启动。resume 流程需重新建立 SSH 连接（suspend 时连接断开），然后通过 SSH 执行 `docker compose ps` 检查容器健康状态。

#### 3.4.3 deploy 与已有实例的交互

执行 `cloudcode deploy` 时，根据 state.json 状态决定行为：

| 情况 | 行为 |
|------|------|
| `state.json` 中 `status: running` | 提示已有运行中实例，建议 `cloudcode deploy --app` 重部署应用层 |
| `state.json` 中 `status: suspended` | 报错并提示用户使用 `cloudcode resume` |
| `state.json` 中 `status: destroyed` 且 `backup.json` 存在 | 按快照恢复流程创建新 ECS（零交互，见 3.4.5） |
| `state.json` 不存在 | 全新部署 |

running/suspended 实例已有完整资源（ECS/EIP/VPC 等），不应重新创建。`deploy` 仅用于首次部署或从快照恢复。

`deploy --app` 仅重部署应用层（重新渲染配置 + pull + up），跳过云资源创建和交互配置。适用于更新 Caddyfile、docker-compose.yml 等配置后重新部署。`--app` 模式跳过 Authelia 配置上传（密码哈希和 secrets 已在磁盘上，避免 encryption key 不匹配）。

**其他命令在非 running 状态下的行为**：

| 命令 | suspended | destroyed |
|------|-----------|-----------|
| `status` | 显示"已停机"，提示 `cloudcode resume` | 显示"已销毁"，提示 `cloudcode deploy` |
| `ssh` / `exec` / `otc` / `logs` | 报错提示 `cloudcode resume` | 报错提示 `cloudcode deploy` |

#### 3.4.4 destroy 流程（可选保留快照）

```
+----------------------+
| cloudcode destroy    |
+-----------+----------+
            |
            v
+-----------+----------+
| Keep snapshot? [Y/n] |
+----+----------+------+
    yes         no
     |           |
     v           |
+----+--------+  |
| Stop ECS    |  |
+----+--------+  |
     |           |
     v           |
+----+--------+  |
| Get disk ID |  |
| Create snap |  |
| Wait done   |  |
| Save to     |  |
| backup.json |  |
+----+--------+  |
     |           |
     +-----+-----+
           |
           v
+-----------+----------+
| Confirm destroy?     |
| [y/N]                |
+----+----------+------+
    yes         no
     |           |
     v           v
+----+--------+ +----+--------+
| Delete all | | Abort       |
| resources  | | (exit)      |
| (ECS, EIP, | +-------------+
|  VPC, SG,  |
|  VSwitch)  |
+------------+
```

交互提示：

```
是否保留磁盘快照（下次 deploy 可恢复）? [Y/n]
确认销毁所有云资源? [y/N]
```

**为什么先 Stop ECS 再创建快照**：虽然阿里云支持在线创建快照，但停机后创建能确保数据一致性（文件系统缓存已刷新），避免恢复时出现数据损坏。

**不保留快照时**：跳过停机和快照步骤，直接确认后删除。`DeleteInstance(Force=true)` 会自动停止运行中的实例再删除。

快照创建失败时通过 `PromptConfirm("继续销毁（数据将丢失）?", false)` 让用户确认。

**确认提示默认值原则**：

| 场景 | 默认值 | 原则 |
|------|--------|------|
| 保留快照 | Y | 保护数据 |
| 确认销毁 | N | 不可逆操作 |
| 快照失败继续销毁 | N | 不可逆操作 |
| 确认停机 (suspend) | Y | 用户主动发起 |
| 确认恢复 (resume) | Y | 用户主动发起 |
| 覆盖已有配置 (init) | N | 保护数据 |
| 验证失败重试 (init) | Y | 用户主动发起 |

#### 3.4.5 deploy 流程（从快照恢复）

从快照恢复为零交互流程：域名和用户名从 `backup.json` 读取，密码哈希和 Authelia secrets 在磁盘快照中保留。

```
+---------------------+
| cloudcode deploy    |
+----------+----------+
           |
           v
+----------+----------+
| Check backup.json   |
+----+----------+-----+
   exists      empty
     |           |
     v           v
+----+--------+ +----+--------+
| Read domain | | PromptConfig|
| & username  | | (interactive|
| from backup | |  input)     |
+----+--------+ +----+--------+
     |           |
     v           |
+----+--------+  |
| Snapshot →  |  |
| Image →     |  |
| CreateInst  |  |
+----+--------+  |
     |           |
     +-----+-----+
           |
           v
+----------+----------+
| Create EIP, bindto  |
| ECS                  |
+----------+----------+
           |
           v
+----------+----------+
| Update DNS records  |
| (if custom domain)  |
+----------+----------+
           |
           v
+----------+----------+
| Upload new configs  |
| (Caddyfile, Authelia|
|  with new domain/IP)|
+----------+----------+
           |
           v
+----------+----------+
| docker compose up -d|
+----------+----------+
           |
           v
+----------+----------+
| Health check        |
+---------------------+
```

从快照恢复的 ECS 包含完整旧环境。deploy 重新渲染非 Authelia 配置文件（Caddyfile、docker-compose.yml、.env）并上传。Authelia 配置（configuration.yml、users_database.yml）保留磁盘快照中的版本，避免 encryption key 不匹配导致 Authelia 启动失败。

**快照恢复时为什么跳过 Authelia 配置**：Authelia 的 `storage.encryption_key` 用于加密 `db.sqlite3`。每次 deploy 会重新生成 secrets，如果覆盖配置文件，新 key 与旧 db 不匹配，Authelia 无法启动。

**快照恢复时为什么不需要自定义镜像清理**：阿里云 ECS SDK 的 `CreateInstanceRequestSystemDisk` 不支持直接指定 `SnapshotId`。实现上先从快照创建自定义镜像，再用该镜像创建 ECS，实例创建成功后立即删除临时镜像（避免存储费用）。

**快照恢复后的容器状态**：ECS 启动后，Docker 容器随 `restart: unless-stopped` 策略自动启动。`docker compose up -d` 会检测容器是否已存在：
- 若容器已运行：无操作（幂等）
- 若容器停止：启动容器
- 若容器不存在：创建并启动

#### 3.4.6 备份元数据

`~/.cloudcode/backup.json`，格式见 [2.5.4](#254-backupjson-版本字段)。

#### 3.4.7 新增/修改 ECS API

```go
// StopECSInstance 需增加 StoppedMode 参数
func StopECSInstance(ecsCli ECSAPI, instanceID string, stopCharging bool) error

// 新增快照和镜像相关 API
type ECSAPI interface {
    // ... existing methods ...
    DescribeDisks(req *ecs.DescribeDisksRequest) (*ecs.DescribeDisksResponse, error)
    CreateSnapshot(req *ecs.CreateSnapshotRequest) (*ecs.CreateSnapshotResponse, error)
    DescribeSnapshots(req *ecs.DescribeSnapshotsRequest) (*ecs.DescribeSnapshotsResponse, error)
    DeleteSnapshot(req *ecs.DeleteSnapshotRequest) (*ecs.DeleteSnapshotResponse, error)
    // 自定义镜像（快照恢复时使用：快照 → 镜像 → ECS）
    CreateImage(req *ecs.CreateImageRequest) (*ecs.CreateImageResponse, error)
    DescribeImages(req *ecs.DescribeImagesRequest) (*ecs.DescribeImagesResponse, error)
    DeleteImage(req *ecs.DeleteImageRequest) (*ecs.DeleteImageResponse, error)
}
```

#### 3.4.8 state.json 扩展

新增 `status` 字段跟踪实例状态：

```json
{
  "resources": { ... },
  "status": "running"
}
```

| status | 含义 |
|--------|------|
| `running` | ECS 运行中 |
| `suspended` | ECS 停机（StopCharging） |
| `destroyed` | 已销毁（仅当保留快照时保留 state.json 和 backup.json） |

不保留快照时，同时删除 state.json 和 backup.json。

#### 3.4.9 修改文件

| 文件 | 改动 |
|------|------|
| `internal/alicloud/interfaces.go` | ECSAPI 新增快照相关方法 |
| `internal/alicloud/ecs.go` | StopECSInstance 增加 StopCharging；新增快照函数 |
| `internal/config/state.go` | state.json 新增 status 字段 |
| `internal/config/backup.go` | backup.json 读写 |
| `internal/deploy/suspend.go` | suspend 逻辑 |
| `internal/deploy/resume.go` | resume 逻辑 |
| `internal/deploy/destroy.go` | destroy 增加可选快照 |
| `internal/deploy/deploy.go` | deploy 支持从快照恢复 |
| `cmd/cloudcode/main.go` | 新增 suspend/resume 命令 |
| `tests/unit/` | 新增相关 mock 和测试 |

#### 3.4.10 测试要点

- `suspend` 后 ECS 状态为 Stopped，不收计算费用
- `resume` 后 ECS 恢复运行，Docker 容器自动启动
- 用户选择保留快照时，创建快照后删除所有资源，保留 backup.json
- 用户不保留快照时直接删除，同时删除 backup.json
- `deploy` 检测到快照时从快照创建 ECS
- 恢复后 devbox 容器内所有内容完整
- 恢复后新的 Caddyfile 正确渲染（新 EIP/域名）
- 首次 deploy（无快照）正常工作
- 快照创建失败时阻塞 destroy，用户确认后可继续
- 跨大版本快照恢复时提示用户选择

---

### 3.5 cloudcode init — 统一配置管理

#### 3.5.1 动机

v0.1.x 通过环境变量（`ALICLOUD_ACCESS_KEY_ID`、`ALICLOUD_ACCESS_KEY_SECRET`、`ALICLOUD_REGION`）传递阿里云凭证。用户需要在 `.bashrc`/`.zshrc` 中手动设置，体验不佳。

`cloudcode init` 采用 CLI 标准做法（类似 `aws configure`、`gcloud init`），一次配置后所有命令自动读取。

#### 3.5.2 配置文件

`~/.cloudcode/credentials`（权限 600）：

```
access_key_id=LTAI5t...
access_key_secret=...
region=ap-southeast-1
```

使用 key=value 格式（`=` 周围无空格），避免解析歧义，方便用户手动编辑。解析时只取第一个 `=` 分割，确保值中包含 `=` 字符时不会出错。

#### 3.5.3 配置读取优先级

```
环境变量 → ~/.cloudcode/credentials → 报错提示 cloudcode init
```

环境变量优先，方便 CI/CD 场景覆盖。

#### 3.5.4 目录结构

本地目录（仅存敏感信息）：

```
~/.cloudcode/
├── credentials     # AccessKeyID、AccessKeySecret、Region（权限 600）
└── ssh_key         # SSH 私钥
```

OSS bucket（状态共享）：

```
cloudcode-state-<account_id>/
├── state.json          # 当前状态
├── backup.json         # 快照元数据（可选）
├── .lock               # 分布式锁（临时）
└── history/            # 操作历史
    ├── 2026-02-24T10-00-00.json
    └── ...
```

**说明**：
- OSS bucket 名称使用 `<account_id>` 后缀确保全局唯一
- bucket 在首次 `cloudcode init` 时创建
- bucket 权限为私有，仅账号持有者可访问

#### 3.5.5 交互流程

```
$ cloudcode init
阿里云 Access Key ID: ********
阿里云 Access Key Secret: ********
默认区域 [ap-southeast-1]: 
验证凭证... ✗ InvalidAccessKeyId
重试? [Y/n]: y
阿里云 Access Key ID: ********
阿里云 Access Key Secret: ********
默认区域 [ap-southeast-1]:
验证凭证... ✓
创建 OSS bucket (cloudcode-state-123456789)... ✓
配置已保存到 ~/.cloudcode/credentials
```

**init 完成的操作**：

1. 调用 `GetCallerIdentity`（STS API）验证凭证有效性
2. 创建 OSS bucket `cloudcode-state-<account_id>`（若已存在则跳过）
3. 保存凭证到本地 `~/.cloudcode/credentials`

**错误处理**：

| 场景 | 行为 |
|------|------|
| 凭证无效 | 提示重试或退出 |
| bucket 已存在 | 跳过创建，继续使用 |
| bucket 名称冲突（其他账号） | 提示：bucket 已被占用，请使用其他区域 |
| 无 OSS 权限 | 提示：请开通 OSS 服务或添加 AliyunOSSFullAccess 权限 |

#### 3.5.6 修改文件

| 文件 | 改动 |
|------|------|
| `internal/alicloud/client.go` | `LoadConfigFromEnv` 改为 `LoadConfig`，支持 credentials 文件 + 环境变量 |
| `internal/config/credentials.go` | 新增 credentials 文件读写 |
| `cmd/cloudcode/main.go` | 新增 init 命令 |
| `internal/alicloud/errors.go` | 错误信息改为提示 `cloudcode init` |
| `tests/unit/` | 更新配置加载测试 |

#### 3.5.7 测试要点

- `cloudcode init` 交互式输入后正确保存 credentials 文件
- credentials 文件权限为 600
- 环境变量优先于 credentials 文件
- 无环境变量且无 credentials 文件时报错提示 `cloudcode init`
- 凭证验证失败时提示重试或退出
- credentials 文件格式错误时（缺少字段、格式不对）给出明确错误提示
- 重复 `init` 覆盖旧配置（确认提示）

---

## 4. 实现规划

### 4.1 优先级

| 功能 | 优先级 | 理由 |
|------|--------|------|
| cloudcode init | P0 | 基础设施，所有命令依赖配置读取 |
| 重命名 opencode → devbox | P0 | 基础重构，其他功能依赖 |
| 自有域名 + 自动 DNS | P0 | 解决 HTTPS 证书不稳定的核心痛点 |
| suspend/resume + 可选快照 | P0 | 支持按需使用模式 |
| 浏览器 Web Terminal | P1 | 新功能，依赖 devbox 重命名 |

### 4.2 依赖关系

```
+------------------+     +------------------+
| 3.1 rename       |     | 3.5 init         |
| opencode->devbox |     | (credentials)    |
+--------+---------+     +--------+---------+
         |                        |
    +----+----+              +----+----+
    |    |    |              |         |
    v    v    v              v         v
+---+-+ ++-+ ++------+  +---+---+ +--+------+
| 3.2 | |3.3| | 3.4  |  | 3.3   | | 3.4     |
| Web | |DNS| | sus/  |  | DNS   | | suspend |
| Term| |   | | res   |  |       | | resume  |
+-----+ +---+ +------+  +-------+ +--------+
```

- 3.1（rename）和 3.5（init）互相独立，可并行开发
- 3.2（Web Terminal）仅依赖 3.1（纯容器内改动，不需要阿里云凭证）
- 3.3（DNS）和 3.4（suspend/resume）同时依赖 3.1（模板引用 devbox）和 3.5（需要阿里云 API 凭证）

### 4.3 实现步骤

| 步骤 | 任务 | 依赖 |
|------|------|------|
| 1 | 重命名 opencode → devbox（3.1） | 无 |
| 2 | cloudcode init（3.5） | 无（可与步骤 1 并行） |
| 3 | 自有域名 + 自动 DNS（3.3） | 步骤 1、2 |
| 4 | suspend/resume + 可选快照（3.4） | 步骤 1、2 |
| 5 | 浏览器 Web Terminal（3.2） | 步骤 1 |

---

## 5. 成本影响

v0.2.0 对月费用的影响：

| 状态 | 月费用 | 说明 |
|------|--------|------|
| running | ~$23 | ECS + EIP |
| suspended | ~$1.2 | 仅磁盘费用（StopCharging 模式） |
| destroyed（保留快照） | ~$0.40 | 仅快照存储费用 |

对比 v0.1.x：suspend 模式让停机期间费用从 ~$23 降至 ~$1.2，节省 ~95%。

---

## 变更记录

- v1.13 (2026-02-25): 实现反馈更新 — `--force` 移除，改为 `deploy --app`（仅重部署应用层，零交互）；快照恢复零交互（域名/用户名从 backup.json 读取，Authelia 配置保留磁盘版本避免 encryption key 不匹配）；PromptConfirm 统一接口并明确默认值原则（用户主动操作默认 Y，不可逆操作默认 N）；destroy 默认保留快照；suspended/destroyed 状态下 ssh/exec/status 等命令友好提示；ECSAPI 新增镜像方法（快照→镜像→ECS 流程）；3.4.5 流程图更新

- v1.12 (2026-02-24): suspend/resume 流程图补充取消分支，与 destroy 流程图保持一致
- v1.11 (2026-02-24): CC+OC 第三轮 review — 修复 handle_path 与 ttyd --base-path 冲突（改用 handle 保留路径前缀）；架构图统一容器名为 devbox 并简化去重；credentials 解析明确只取第一个 = 分割；4.2 依赖图重画；3.4.3 标题更新覆盖 running 状态；1.3 非目标补充"不支持配置热更新"；补充 handle vs handle_path 说明
- v1.10 (2026-02-24): CC+OC 第二轮 review — 1.2 目标表补充 init；deploy 补充 status:running 检查；Caddyfile 补充 auth 子域名 8443 端口；init 验证失败改为循环重试；destroy 流程图补充取消退出分支；修正依赖关系（rename 不依赖 init）；credentials 格式去掉等号周围空格；补充格式错误测试要点
- v1.9 (2026-02-24): CC+OC 联合 review — suspend/resume 补充交互确认；destroy 流程图体现两次确认；deploy 检测 suspended 改为报错提示 resume；3.3.1 补充两条 A 记录说明；3.3.5 改用完整域名示例；EnsureDNSRecord 注释明确为单条操作；2.5 新增版本定义（大版本/小版本）；Caddyfile 补充 8443 备用端口；3.3.7 补充 Authelia 配置模板；新增 3.5 cloudcode init 统一配置管理
- v1.8 (2026-02-23): Authelia 恢复子域名模式（路径模式配置复杂易出错）；A 记录恢复为两条；3.1.2/3.2.6 修改文件表更新（docker-compose.yml.tmpl、docker.yml、render.go）；3.4 编号规范化（消除 3.4.2.1）；backup.json 去重引用；补充 StopCharging 与 EIP 行为说明；补充 destroy 不保留快照时的删除说明
- v1.7 (2026-02-23): 修正 ENTRYPOINT 位置（应在 Dockerfile 最后）；补充快照前停机的原因说明；补充快照恢复后容器状态处理；补充 Caddy 路由优先级说明
- v1.6 (2026-02-23): 补充 Dockerfile.devbox 细节 — 明确 ttyd 安装在 USER opencode 之前、补充 COPY entrypoint.sh 和 ENTRYPOINT 指令；resume 流程补充 SSH 连接步骤；3.2.6 修改文件表补充完整
- v1.5 (2026-02-23): 重大修正 — opencode 不支持 base-path，改为保留根路径，仅 /terminal 使用 base-path；补充架构图；明确 deploy 与 suspended 状态交互；补充 ttyd 用户上下文说明；明确域名拆分示例
- v1.4 (2026-02-23): 修正技术设计问题 — Caddyfile 补充 /auth 路由；A 记录从两条改为一条；destroy 流程图调整为先问快照再确认销毁；补充 StopCharging 前置条件；修复 Go API 代码格式；明确 backup.json 处理逻辑；修正测试要点描述
- v1.3 (2026-02-22): 3.4 重写为 suspend/resume + 可选快照模型（ECS 停机不收费替代磁盘快照作为核心机制）；2.4 更新决策理由
- v1.2 (2026-02-22): 简化 2.5 跨版本兼容策略（快照功能从 v0.2 引入，不存在 v0.1 快照，去掉具体迁移脚本，改为面向未来的通用策略）
- v1.1 (2026-02-22): 根据 OC review 修订 — 补充快照生命周期策略（只保留最新）、deploy 流程增加 DNS 更新步骤、补充 ttyd 安全说明、明确快照失败确认机制、修正流程图对齐、opencode 路由改为 /opencode/*
- v1.0 (2026-02-22): 初始版本，包含四个功能设计

