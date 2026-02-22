# v0.2.0 功能设计

## 目录

- [1. 重命名：opencode → devbox](#1-重命名opencode--devbox)
- [2. 浏览器 Web Terminal](#2-浏览器-web-terminal)
- [3. 自有域名 + 自动 DNS 更新](#3-自有域名--自动-dns-更新)
- [4. 按需使用 — ECS 磁盘快照备份/恢复](#4-按需使用--ecs-磁盘快照备份恢复)
- [5. 部署交互回退（暂缓）](#5-部署交互回退暂缓)
- [6. 实现优先级与依赖关系](#6-实现优先级与依赖关系)

---

## 1. 重命名：opencode → devbox

### 背景

容器内不只运行 opencode，还包含 nodejs、python3、gh CLI，后续还会加入 ttyd（Web Terminal）。`opencode` 这个名字不再准确，改为 `devbox` 更贴切。

### 改动范围

| 改动项 | 之前 | 之后 |
|--------|------|------|
| Docker 镜像 | `ghcr.io/hwuu/cloudcode-opencode` | `ghcr.io/hwuu/cloudcode-devbox` |
| 容器名 | `opencode` | `devbox` |
| Dockerfile | `Dockerfile.opencode` | `Dockerfile.devbox` |
| Docker Compose 服务名 | `opencode` | `devbox` |
| Volume 名前缀 | `opencode_*` | `devbox_*` |
| GitHub Actions workflow | 构建推送 `cloudcode-opencode` | 构建推送 `cloudcode-devbox` |

### 修改文件

| 文件 | 改动 |
|------|------|
| `internal/template/templates/Dockerfile.opencode` | 重命名为 `Dockerfile.devbox` |
| `internal/template/templates/docker-compose.yml` | 服务名、镜像名、volume 名 |
| `internal/template/templates/Caddyfile.tmpl` | `reverse_proxy devbox:4096` |
| `.github/workflows/docker.yml` | 镜像名改为 `cloudcode-devbox` |
| `internal/deploy/deploy.go` | 容器名引用 |
| `internal/deploy/status.go` | 容器名引用 |
| `cmd/cloudcode/main.go` | 容器名引用 |
| `tests/unit/` | 相关测试更新 |
| `docs/` | 文档中的引用 |

### 测试要点

- `go build ./...` + `go test ./tests/unit/` 通过
- CI docker workflow 构建推送 `ghcr.io/hwuu/cloudcode-devbox`
- `cloudcode deploy --force` 使用新镜像名正常启动

---

## 2. 浏览器 Web Terminal

### 背景

用户希望通过浏览器直接进入 devbox 容器执行命令（安装依赖、调试、git 操作等），无需本地 SSH 客户端。

### 方案

在 devbox 容器内安装 [ttyd](https://github.com/tsl0922/ttyd)，通过 Caddy 反向代理 + Authelia 认证暴露 Web Terminal。不使用独立容器，避免挂载 docker.sock 的安全风险。

### 架构

```
Browser --> Caddy (/terminal/*) --> Authelia (forward_auth) --> devbox:7681 (ttyd)
Browser --> Caddy (/)            --> Authelia (forward_auth) --> devbox:4096 (opencode)
```

### 实现思路

#### 1. Dockerfile.devbox 安装 ttyd

```dockerfile
# 安装 ttyd
ARG TARGETARCH
RUN TTYD_ARCH=$(case "$TARGETARCH" in amd64) echo "x86_64" ;; arm64) echo "aarch64" ;; esac) && \
    curl -fsSL -o /usr/local/bin/ttyd \
    "https://github.com/tsl0922/ttyd/releases/latest/download/ttyd.${TTYD_ARCH}" && \
    chmod +x /usr/local/bin/ttyd
```

#### 2. 容器启动脚本

devbox 容器需要同时运行 opencode 和 ttyd 两个进程。使用启动脚本替代单一 ENTRYPOINT：

```bash
#!/bin/bash
# entrypoint.sh
ttyd --port 7681 --base-path /terminal /bin/bash &
exec opencode web --hostname 0.0.0.0 --port 4096
```

#### 3. docker-compose.yml

```yaml
  devbox:
    image: ghcr.io/hwuu/cloudcode-devbox:latest
    container_name: devbox
    restart: unless-stopped
    expose:
      - 4096
      - 7681
    # ...
```

#### 4. Caddyfile.tmpl 新增路由

```caddyfile
{{ .Domain }} {
    handle_path /terminal/* {
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
```

#### 5. 修改文件

| 文件 | 改动 |
|------|------|
| `internal/template/templates/Dockerfile.devbox` | 安装 ttyd，新增 entrypoint.sh |
| `internal/template/templates/docker-compose.yml` | devbox 暴露 7681 端口 |
| `internal/template/templates/Caddyfile.tmpl` | 新增 /terminal 路由 |

### 测试要点

- 浏览器访问 `https://<domain>/terminal/`，经 Authelia 认证后进入 bash
- 在 Web Terminal 中执行 `opencode -v`、`git status` 等命令
- 未认证时访问 /terminal 应跳转到登录页
- opencode Web UI 和 ttyd 同时正常运行
- 容器重启后两个服务自动恢复

---

## 3. 自有域名 + 自动 DNS 更新

### 背景

当前 HTTPS 证书方案存在两个痛点：

1. **nip.io + Let's Encrypt**：nip.io 是公共域名，所有用户共享 Let's Encrypt 速率限制（每周 100 张证书），经常申请失败
2. **自有域名手动配置**：每次 deploy 都会分配新 EIP，用户需要手动去 DNS 控制台更新 A 记录

自签名证书（`tls internal`）不是好的替代方案：
- 浏览器每次弹安全警告
- 手机浏览器体验更差，部分不允许跳过
- Passkey/WebAuthn 需要 secure context，自签名证书可能导致注册失败

### 设计决策

#### 3.1 DNS 托管平台

| 维度 | 仅支持阿里云 DNS | 支持多平台 |
|------|-----------------|-----------|
| 实现复杂度 | 低（一套 SDK） | 高（每个平台一套 API） |
| 用户体验 | 需要域名托管在阿里云 | 灵活 |
| 自动化程度 | 全自动（SDK 更新 A 记录） | 非阿里云需手动配置 |

**决策**：优先支持阿里云 DNS（SDK 自动更新），非阿里云域名由用户手动配置 DNS，nip.io 作为兜底。

#### 3.2 域名拆分策略

用户输入 `oc.example.com`，需要拆分为主域名（`example.com`）和主机记录（`oc`）。

多级后缀（如 `example.co.uk`）难以通过字符串解析处理。

**决策**：如果域名托管在阿里云 DNS，调用 `DescribeDomains` API 获取用户的域名列表，自动匹配拆分。

#### 3.3 DNS 传播与证书申请

| 场景 | 策略 |
|------|------|
| 阿里云 DNS | 传播几乎即时，Caddy 直接申请 Let's Encrypt 证书 |
| 非阿里云 DNS | deploy 后轮询 DNS 解析结果，确认生效后再启动 Caddy |

### 方案

```
deploy 交互:
  1. Prompt: Domain (e.g. oc.example.com, leave empty for nip.io)
  2. If custom domain:
     2a. Check if domain is on Alibaba Cloud DNS (DescribeDomains)
     2b. If yes: auto-update A records after EIP allocation
     2c. If no: prompt user to manually configure DNS, wait for propagation

deploy 流程:
  1. Create ECS + EIP
  2. If Alibaba Cloud DNS:
     +-- DescribeDomainRecords (check existing A record)
     +-- If exists: UpdateDomainRecord (update IP)
     +-- If not:    AddDomainRecord (create A record)
     +-- Same for auth.<domain> A record
  3. If external DNS:
     +-- Print DNS config instructions
     +-- Poll DNS resolution until A record matches EIP (timeout 5min)
  4. Upload configs + docker compose up
  5. Caddy auto-obtains Let's Encrypt cert
```

### 实现思路

#### 1. 新增阿里云 DNS SDK 依赖

```
github.com/alibabacloud-go/alidns-20150109/v4
```

#### 2. DNS API 接口（internal/alicloud/dns.go）

```go
type DnsAPI interface {
    DescribeDomains(req *dnsclient.DescribeDomainsRequest) (*dnsclient.DescribeDomainsResponse, error)
    DescribeDomainRecords(req *dnsclient.DescribeDomainRecordsRequest) (*dnsclient.DescribeDomainRecordsResponse, error)
    AddDomainRecord(req *dnsclient.AddDomainRecordRequest) (*dnsclient.AddDomainRecordResponse, error)
    UpdateDomainRecord(req *dnsclient.UpdateDomainRecordRequest) (*dnsclient.UpdateDomainRecordResponse, error)
}

// FindBaseDomain 从用户域名列表中匹配主域名
// 输入: "oc.example.com", 域名列表: ["example.com", "other.org"]
// 输出: baseDomain="example.com", rr="oc"
func FindBaseDomain(fullDomain string, domains []string) (baseDomain, rr string, err error)

// EnsureDNSRecord 确保 A 记录指向指定 IP（不存在则创建，已存在则更新）
func EnsureDNSRecord(cli DnsAPI, baseDomain, rr, ip string) error
```

#### 3. deploy 流程集成

```go
// EIP 分配后
if isCustomDomain(cfg.Domain) {
    // 尝试阿里云 DNS 自动更新
    domains, err := d.DNS.DescribeDomains(...)
    baseDomain, rr, err := alicloud.FindBaseDomain(cfg.Domain, domains)
    if err == nil {
        // 阿里云 DNS，自动更新
        alicloud.EnsureDNSRecord(d.DNS, baseDomain, rr, eip)
        alicloud.EnsureDNSRecord(d.DNS, baseDomain, "auth."+rr, eip)
    } else {
        // 非阿里云 DNS，提示手动配置并等待
        d.printf("请配置 DNS A 记录:\n")
        d.printf("  %s → %s\n", cfg.Domain, eip)
        d.printf("  auth.%s → %s\n", cfg.Domain, eip)
        d.printf("等待 DNS 生效...\n")
        waitForDNS(cfg.Domain, eip, 5*time.Minute)
    }
}
```

#### 4. 修改文件

| 文件 | 改动 |
|------|------|
| `go.mod` | 新增 alidns SDK 依赖 |
| `internal/alicloud/dns.go` | 新增 DnsAPI 接口、FindBaseDomain、EnsureDNSRecord |
| `internal/alicloud/interfaces.go` | 新增 DnsAPI 接口 |
| `internal/deploy/deploy.go` | Deployer 新增 DNS 字段，deploy 流程集成 DNS 更新 |
| `cmd/cloudcode/main.go` | 初始化 DNS client |
| `tests/unit/alicloud_test.go` | 新增 DNS mock 和测试 |

### 测试要点

- 阿里云 DNS 域名：deploy 后 A 记录自动指向 EIP
- 非阿里云 DNS 域名：提示手动配置，轮询等待生效
- 重复 deploy（EIP 变化）后 DNS 记录自动更新
- 留空域名时使用 nip.io 兜底
- FindBaseDomain 正确匹配多级域名
- DNS API 失败时给出明确错误提示
- auth 子域名 A 记录同步创建/更新

---

## 4. 按需使用 — ECS 磁盘快照备份/恢复

### 背景

用户希望 cloudcode 支持"按需使用"模式：不用时 `destroy` 释放所有云资源（零成本），用时再 `deploy`。目标是 destroy 再 deploy 后，进入 devbox 一切和原来一样 — 包括配置、会话历史、容器内安装的工具、目录结构等。

### 设计决策

#### 4.1 备份方案选型

| 维度 | A: OSS Volumes 备份 | B: Docker commit + OSS | C: ECS 磁盘快照 |
|------|---------------------|----------------------|-----------------|
| 恢复完整度 | 中 — 仅 volumes | 高 — 容器文件系统 + volumes | 完美 — 整个系统盘 |
| 速度 | 快（几百 MB） | 中（~1GB 镜像） | 中（快照创建 5-10 分钟） |
| 稳定性 | 高 | 中（镜像层叠加膨胀） | 高（阿里云原生） |
| 实现复杂度 | 中 | 中 | 低 |
| 月费用 | ~$0 | ~$0 | ~$0.40（20GB 实际数据） |

**决策**：选择方案 C（ECS 磁盘快照）。

**理由**：
1. 恢复完整度最高，满足"一切和原来一样"的需求
2. 实现最简单，几个阿里云 API 调用
3. 费用极低（$0.40/月）

### 方案

```
destroy:
  1. Confirm with user
  2. Stop ECS instance
  3. Create snapshot of system disk
     +-- Record snapshot ID in ~/.cloudcode/backup.json
  4. Wait for snapshot completion
  5. Delete ECS instance, EIP, VPC...
  6. Update state.json

deploy (with existing snapshot):
  1. Read backup.json, find snapshot ID
  2. Create ECS with system disk from snapshot
     +-- CreateInstance with SystemDisk.SnapshotId
  3. Create EIP, bindto ECS
  4. Upload new configs (Caddyfile with new domain/IP)
  5. docker compose up -d (containers auto-start from snapshot)
  6. Health check
```

### 实现思路

#### 1. 备份元数据文件（~/.cloudcode/backup.json）

```json
{
  "snapshot_id": "s-t4nxxxxxxxxx",
  "created_at": "2026-02-22T10:00:00Z",
  "region": "ap-southeast-1",
  "disk_size": 60,
  "description": "cloudcode auto backup before destroy"
}
```

#### 2. destroy 流程

```go
func (d *Destroyer) Run(ctx context.Context) error {
    // ... 确认删除 ...

    // 停止 ECS
    d.printf("停止 ECS 实例...\n")
    d.ECS.StopInstance(...)

    // 获取系统盘 ID
    diskID := getSystemDiskID(d.ECS, state.Resources.ECS.ID)

    // 创建快照
    d.printf("创建磁盘快照（保留数据）...\n")
    snapshotID, err := d.ECS.CreateSnapshot(diskID, "cloudcode auto backup")
    if err != nil {
        d.printf("⚠️  快照创建失败: %v\n", err)
        proceed, _ := d.Prompter.PromptConfirm("继续销毁（数据将丢失）?")
        if !proceed {
            return nil
        }
    } else {
        // 等待快照完成
        waitForSnapshot(d.ECS, snapshotID)
        // 保存快照信息
        saveBackupInfo(snapshotID, ...)
        d.printf("  ✓ 快照已创建: %s\n", snapshotID)
    }

    // ... 删除云资源 ...
}
```

#### 3. deploy 流程

```go
func (d *Deployer) CreateResources(ctx context.Context, state *config.State) error {
    // ... 创建 VPC/VSwitch/SG ...

    // 检查是否有快照
    backup := loadBackupInfo()
    if backup != nil {
        d.printf("检测到历史快照，从快照恢复...\n")
        // 创建 ECS 时指定 SnapshotId
        createReq.SystemDisk.SnapshotId = backup.SnapshotID
    }

    // ... 创建 ECS/EIP ...
}
```

#### 4. 新增 ECS API

```go
// 需要新增的 ECSAPI 方法
type ECSAPI interface {
    // ... 现有方法 ...
    DescribeDisks(req) (*DescribeDisksResponse, error)
    CreateSnapshot(req) (*CreateSnapshotResponse, error)
    DescribeSnapshots(req) (*DescribeSnapshotsResponse, error)
    DeleteSnapshot(req) (*DeleteSnapshotResponse, error)
}
```

#### 5. 快照恢复后的配置更新

从快照恢复的 ECS 包含旧的配置文件（旧域名、旧 IP）。deploy 需要：
1. 重新渲染 Caddyfile（新 EIP/域名）
2. 重新上传配置文件
3. `docker compose up -d`（容器从快照恢复，配置文件通过 bind mount 更新）

#### 6. 独立命令

新增 `cloudcode backup` 和 `cloudcode restore` 命令：

```bash
cloudcode backup          # 手动创建快照（不 destroy）
cloudcode restore         # 列出可用快照，选择恢复
cloudcode backup --list   # 列出所有快照
cloudcode backup --delete # 删除指定快照
```

#### 7. 修改文件

| 文件 | 改动 |
|------|------|
| `internal/alicloud/interfaces.go` | ECSAPI 新增 DescribeDisks/CreateSnapshot/DescribeSnapshots |
| `internal/alicloud/ecs.go` | 新增快照相关函数 |
| `internal/config/backup.go` | backup.json 读写 |
| `internal/deploy/destroy.go` | destroy 前创建快照 |
| `internal/deploy/deploy.go` | deploy 时检测快照并从快照创建 ECS |
| `cmd/cloudcode/main.go` | 新增 backup/restore 命令 |
| `tests/unit/` | 新增快照相关 mock 和测试 |

### 成本估算

| 项目 | 费用 |
|------|------|
| 快照存储（20GB 实际数据） | ~$0.40/月 |
| 快照 API 调用 | 可忽略 |
| 总计 | **~$0.40/月** |

### 测试要点

- destroy 时自动创建磁盘快照
- deploy 检测到快照时从快照创建 ECS
- 恢复后 devbox 容器内所有内容完整（配置、会话、安装的工具）
- 恢复后新的 Caddyfile 正确渲染（新 EIP/域名）
- 首次 deploy（无快照）正常工作
- 快照创建失败时阻塞 destroy，用户确认后可继续
- `cloudcode backup` / `cloudcode restore` 独立命令正常工作
- 快照跨可用区恢复正常

---

## 5. 部署交互回退（暂缓）

当前 PromptConfig 已简化为 3 步（域名、用户名、密码），回退价值不大。待其他功能实现后，如果交互步骤增多再考虑。

---

## 6. 实现优先级与依赖关系

### 优先级

| 功能 | 优先级 | 理由 |
|------|--------|------|
| 重命名 opencode → devbox | P0 | 基础重构，其他功能依赖 |
| 自有域名 + 自动 DNS | P0 | 解决 HTTPS 证书不稳定的核心痛点 |
| ECS 磁盘快照备份/恢复 | P0 | 支持按需使用模式，destroy 不丢数据 |
| 浏览器 Web Terminal | P1 | 新功能，依赖 devbox 重命名 |
| 部署交互回退 | P2 | 暂缓，当前步骤少 |

### 依赖关系

```
rename (devbox)
  |
  +---> Web Terminal (depends on Dockerfile.devbox)
  |
  +---> DNS + Snapshot (independent of each other)
```

### 实现步骤

| 步骤 | 任务 | 依赖 |
|------|------|------|
| 1 | 重命名 opencode → devbox | 无 |
| 2 | 自有域名 + 自动 DNS | 步骤 1 |
| 3 | ECS 磁盘快照备份/恢复 | 步骤 1 |
| 4 | 浏览器 Web Terminal | 步骤 1 |

---

**文档版本**: 1.0
**更新日期**: 2026-02-22
