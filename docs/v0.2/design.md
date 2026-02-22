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
- [3. 组件设计](#3-组件设计)
  - [3.1 重命名 opencode → devbox](#31-重命名-opencode--devbox)
  - [3.2 浏览器 Web Terminal](#32-浏览器-web-terminal)
  - [3.3 自有域名 + 自动 DNS 更新](#33-自有域名--自动-dns-更新)
  - [3.4 按需使用 — suspend/resume + 可选快照](#34-按需使用--suspendresume--可选快照)
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
| **按需使用** | destroy 前自动创建磁盘快照，deploy 时从快照恢复，一切如初 |
| **浏览器终端** | 内置 ttyd，浏览器直接进入 devbox 容器执行命令 |
| **容器重命名** | opencode → devbox，反映"开发环境"定位 |

### 1.3 非目标

- **不做多 DNS 平台适配**：仅自动支持阿里云 DNS，其他平台手动配置
- **不做增量快照**：每次 destroy 创建全量快照，不做增量备份链
- **不做部署交互回退**：当前仅 3 步交互，回退价值不大，暂缓

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

#### 2.5.1 版本兼容策略

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

#### 2.5.2 设计原则

- **小版本**（patch/minor）保证快照兼容：不改 volume 名、服务名等基础架构
- **大版本**（major feature）允许破坏性变更，但提供迁移脚本
- 迁移脚本随版本发布，放在 `internal/migrate/` 下
- `backup.json` 记录 `cloudcode_version`，deploy 时对比版本决定恢复策略

#### 2.5.3 backup.json 版本字段

```json
{
  "snapshot_id": "s-t4nxxxxxxxxx",
  "cloudcode_version": "0.2.0",
  "created_at": "2026-02-22T10:00:00Z",
  "region": "ap-southeast-1",
  "disk_size": 60
}
```

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
| `internal/template/templates/docker-compose.yml` | 服务名、镜像名、volume 名 |
| `internal/template/templates/Caddyfile.tmpl` | `reverse_proxy devbox:4096` |
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
+----+----------+---+
     |          |
     v          v
/terminal/* /opencode/*
     |          |
     v          v
+----+----+  +--+------+
|  ttyd   |  | opencode|
|  :7681  |  |  :4096  |
+----+----+  +--+------+
     |          |
     +----+-----+
          |
    +-----+------+
    |   devbox    |
    |  container  |
    +-------------+
```

所有请求经 Caddy → Authelia forward_auth 认证后，按路径分发到 devbox 容器内的两个服务。

#### 3.2.2 Dockerfile.devbox 改动

安装 ttyd 二进制（~3MB）：

```dockerfile
ARG TARGETARCH
RUN TTYD_ARCH=$(case "$TARGETARCH" in amd64) echo "x86_64" ;; arm64) echo "aarch64" ;; esac) && \
    curl -fsSL -o /usr/local/bin/ttyd \
    "https://github.com/tsl0922/ttyd/releases/latest/download/ttyd.${TTYD_ARCH}" && \
    chmod +x /usr/local/bin/ttyd
```

#### 3.2.3 启动脚本

docker-compose 配置 `init: true` 注入 tini 作为 PID 1，处理信号转发和僵尸进程回收。entrypoint 脚本管理两个进程：

```bash
#!/bin/bash
# entrypoint.sh

# ttyd 后台运行，崩溃自动重启
while true; do
    ttyd --port 7681 --base-path /terminal /bin/bash
    sleep 1
done &

# opencode 作为主进程
exec opencode web --hostname 0.0.0.0 --port 4096
```

opencode 作为主进程，退出时容器重启（`restart: unless-stopped`），ttyd 随之重启。ttyd 单独崩溃时由 while 循环自动重启，不影响 opencode。

#### 3.2.4 Caddyfile.tmpl 路由

```caddyfile
{{ .Domain }} {
    handle_path /terminal/* {
        forward_auth authelia:9091 {
            uri /api/authz/forward-auth
            copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
        }
        reverse_proxy devbox:7681
    }

    handle_path /opencode/* {
        forward_auth authelia:9091 {
            uri /api/authz/forward-auth
            copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
        }
        reverse_proxy devbox:4096
    }
}
```

`handle_path` 自动剥离路径前缀后转发。ttyd 的 `--base-path /terminal` 确保 WebSocket 路径匹配。

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
| `internal/template/templates/Dockerfile.devbox` | 安装 ttyd，新增 entrypoint.sh |
| `internal/template/templates/docker-compose.yml` | devbox 暴露 7681 端口 |
| `internal/template/templates/Caddyfile.tmpl` | 新增 /terminal 路由，改用 handle 分流 |

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
| 阿里云域名 | `oc.example.com` | 阿里云 DNS | SDK 自动创建/更新 A 记录 |
| 外部域名 | `oc.example.com` | Cloudflare 等 | 提示用户手动配置，轮询等待生效 |

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

需要创建/更新两条 A 记录：

| 主机记录 | 记录值 | 用途 |
|---------|--------|------|
| `oc` | EIP | 主域名 |
| `auth.oc` | EIP | Authelia 认证子域名 |

```go
// EnsureDNSRecord creates or updates an A record.
// If record exists with different IP, update it.
// If record doesn't exist, create it.
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
| StopInstance         |
| (StopCharging mode)  |
+-----------+----------+
            |
            v
+-----------+----------+
| Wait for Stopped     |
+-----------+----------+
            |
            v
+-----------+----------+
| Update state.json    |
| (status: suspended)  |
+----------------------+
```

实现极简：一个 `StopInstance` API 调用，设置 `StoppedMode: StopCharging`。停机后 CPU/内存释放不收费，仅收磁盘费 ~$1.2/月。EIP 绑定停机实例不收闲置费。

#### 3.4.2 resume 流程

```
+----------------------+
| cloudcode resume     |
+-----------+----------+
            |
            v
+-----------+----------+
| StartInstance        |
+-----------+----------+
            |
            v
+-----------+----------+
| Wait for Running     |
+-----------+----------+
            |
            v
+-----------+----------+
| Health check         |
| (docker compose ps)  |
+-----------+----------+
            |
            v
+-----------+----------+
| Update state.json    |
| (status: running)    |
+----------------------+
```

`StartInstance` 后几秒恢复，Docker 容器随 `restart: unless-stopped` 自动启动。

#### 3.4.3 destroy 流程（可选保留快照）

```
+----------------------+
| cloudcode destroy    |
+-----------+----------+
            |
            v
+-----------+----------+
| Confirm with user    |
+-----------+----------+
            |
            v
+-----------+----------+
| Keep snapshot?       |
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
| Delete all resources |
| (ECS, EIP, VPC,     |
|  SG, VSwitch)        |
+----------------------+
```

交互提示：

```
确认销毁所有云资源? [y/N]
是否保留磁盘快照（下次 deploy 可恢复）? [Y/n]
```

快照创建失败时通过 `PromptConfirm("继续销毁（数据将丢失）?")` 让用户确认。

#### 3.4.4 deploy 流程（从快照恢复）

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
| CreateInst  | | CreateInst  |
| with        | | with        |
| SnapshotId  | | fresh disk  |
+----+--------+ +----+--------+
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

从快照恢复的 ECS 包含完整旧环境。deploy 重新渲染配置文件并上传，通过 bind mount 覆盖旧配置。

#### 3.4.5 备份元数据

`~/.cloudcode/backup.json`：

```json
{
  "snapshot_id": "s-t4nxxxxxxxxx",
  "cloudcode_version": "0.2.0",
  "created_at": "2026-02-22T10:00:00Z",
  "region": "ap-southeast-1",
  "disk_size": 60
}
```

#### 3.4.6 新增/修改 ECS API

```go
// StopECSInstance 需增加 StoppedMode 参数
func StopECSInstance(ecsCli ECSAPI, instanceID string, stopCharging bool) error

// 新增快照相关 API
type ECSAPI interface {
    // ... existing methods ...
    DescribeDisks(req) (*DescribeDisksResponse, error)
    CreateSnapshot(req) (*CreateSnapshotResponse, error)
    DescribeSnapshots(req) (*DescribeSnapshotsResponse, error)
    DeleteSnapshot(req) (*DeleteSnapshotResponse, error)
}
```

#### 3.4.7 state.json 扩展

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
| `destroyed` | 已销毁（仅当保留快照时保留 state） |

#### 3.4.8 修改文件

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

#### 3.4.9 测试要点

- `suspend` 后 ECS 状态为 Stopped，不收计算费用
- `resume` 后 ECS 恢复运行，Docker 容器自动启动
- `destroy --keep-snapshot` 创建快照后删除所有资源
- `destroy` 不保留快照时直接删除
- `deploy` 检测到快照时从快照创建 ECS
- 恢复后 devbox 容器内所有内容完整
- 恢复后新的 Caddyfile 正确渲染（新 EIP/域名）
- 首次 deploy（无快照）正常工作
- 快照创建失败时阻塞 destroy，用户确认后可继续
- 跨大版本快照恢复时提示用户选择

---

## 4. 实现规划

### 4.1 优先级

| 功能 | 优先级 | 理由 |
|------|--------|------|
| 重命名 opencode → devbox | P0 | 基础重构，其他功能依赖 |
| 自有域名 + 自动 DNS | P0 | 解决 HTTPS 证书不稳定的核心痛点 |
| suspend/resume + 可选快照 | P0 | 支持按需使用模式 |
| 浏览器 Web Terminal | P1 | 新功能，依赖 devbox 重命名 |

### 4.2 依赖关系

```
+------------------+
| 3.1 rename       |
| opencode->devbox |
+--------+---------+
         |
    +----+----+--------+
    |         |        |
    v         v        v
+---+---+ +--+---+ +--+------+
| 3.2   | | 3.3  | | 3.4     |
| Web   | | DNS  | | suspend |
| Term  | |      | | resume  |
+-------+ +------+ +--------+
```

3.2/3.3/3.4 互相独立，但都依赖 3.1 完成。

### 4.3 实现步骤

| 步骤 | 任务 | 依赖 |
|------|------|------|
| 1 | 重命名 opencode → devbox（3.1） | 无 |
| 2 | 自有域名 + 自动 DNS（3.3） | 步骤 1 |
| 3 | ECS 磁盘快照备份/恢复（3.4） | 步骤 1 |
| 4 | 浏览器 Web Terminal（3.2） | 步骤 1 |

---

## 5. 成本影响

v0.2.0 对月费用的影响：

| 项目 | v0.1.x | v0.2.0 | 变化 |
|------|--------|--------|------|
| ECS 实例 | ~$20 | ~$20 | 不变 |
| EIP | ~$3 | ~$3 | 不变 |
| 磁盘快照（停机期间） | — | ~$0.40 | 新增 |
| 阿里云 DNS | — | 免费 | — |
| **总计（运行中）** | **~$23** | **~$23** | **不变** |
| **总计（停机期间）** | **$0** | **~$0.40** | **+$0.40** |

---

## 变更记录

- v1.3 (2026-02-22): 3.4 重写为 suspend/resume + 可选快照模型（ECS 停机不收费替代磁盘快照作为核心机制）；2.4 更新决策理由
- v1.2 (2026-02-22): 简化 2.5 跨版本兼容策略（快照功能从 v0.2 引入，不存在 v0.1 快照，去掉具体迁移脚本，改为面向未来的通用策略）
- v1.1 (2026-02-22): 根据 OC review 修订 — 补充快照生命周期策略（只保留最新）、deploy 流程增加 DNS 更新步骤、补充 ttyd 安全说明、明确快照失败确认机制、修正流程图对齐、opencode 路由改为 /opencode/*
- v1.0 (2026-02-22): 初始版本，包含四个功能设计

