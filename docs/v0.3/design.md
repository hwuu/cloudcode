# CloudCode v0.3.0 设计文档

## 目录

- [1. 背景与目标](#1-背景与目标)
  - [1.1 v0.2.x 回顾](#11-v02x-回顾)
  - [1.2 v0.3.0 目标](#12-v030-目标)
  - [1.3 非目标](#13-非目标)
- [2. 设计决策](#2-设计决策)
  - [2.1 状态存储：为什么用 OSS 而非本地文件](#21-状态存储为什么用-oss-而非本地文件)
- [3. 组件设计](#3-组件设计)
  - [3.1 OSS 状态管理与分布式锁](#31-oss-状态管理与分布式锁)
  - [3.2 init 命令扩展](#32-init-命令扩展)
- [4. 实现规划](#4-实现规划)
- [5. 成本影响](#5-成本影响)
- [变更记录](#变更记录)

---

## 1. 背景与目标

### 1.1 v0.2.x 回顾

v0.2.x 实现了以下功能：

- 容器重命名 opencode → devbox
- 浏览器 Web Terminal（ttyd）
- 自有域名 + 自动 DNS 更新
- suspend/resume + 可选快照
- cloudcode init 统一配置管理

v0.2.x 将 state.json 存储在本地 `~/.cloudcode/`，在实际使用中存在以下问题：

| 问题 | 说明 |
|------|------|
| **跨机器操作** | 用户可能在笔记本、台式机等多台机器上操作同一实例 |
| **操作中断** | deploy/destroy/suspend 到一半断电，状态不一致 |
| **并发冲突** | 多个终端同时操作，状态竞争 |
| **孤儿资源** | 中途失败后资源泄露，无法恢复 |

### 1.2 v0.3.0 目标

| 目标 | 说明 |
|------|------|
| **OSS 状态存储** | 将 state.json、backup.json 迁移到 OSS，支持跨机器共享 |
| **分布式锁** | 支持多终端/多机器并发操作控制 |
| **中断恢复** | 支持操作中断后恢复或接管 |
| **操作历史** | 记录所有操作历史，便于审计和问题排查 |

### 1.3 非目标

- **不做多区域支持**：状态存储在单一区域的 OSS bucket
- **不做状态版本控制**：只保留最新状态和历史记录，不做完整版本链

---

## 2. 设计决策

### 2.1 状态存储：为什么用 OSS 而非本地文件

#### 2.1.1 方案对比

| 方案 | 优点 | 缺点 |
|------|------|------|
| 本地文件 | 简单 | 无法跨机器，无锁机制 |
| ECS 标签 | 无额外资源 | deploy 初期无 ECS；destroy 后丢失 |
| **OSS bucket** | 全流程可用，支持条件写入作锁，跨机器共享 | 需创建额外资源（成本极低） |
| 云数据库 | 功能完整 | 过于复杂，增加依赖 |
| DNS TXT 记录 | 复用现有域名 | 不适合存大量状态，修改慢 |

**决策**：使用 OSS bucket 存储状态。

#### 2.1.2 选择 OSS 的理由

1. **跨机器共享**：状态存储在云端，任意机器可访问
2. **分布式锁**：OSS 支持条件写入（`If-None-Match: *`），天然适合分布式锁
3. **全生命周期覆盖**：从 deploy 开始到 destroy 结束，状态始终可用
4. **成本极低**：OSS 标准存储 ~0.12 元/GB/月，state.json 几 KB，费用可忽略
5. **无需额外依赖**：用户已有阿里云账号，OSS 是同生态服务

#### 2.1.3 本地 vs OSS 职责划分

| 文件 | 存储 | 理由 |
|------|------|------|
| `credentials` | 本地 | 敏感信息（AccessKey） |
| `ssh_key` | 本地 | 私钥，不应上传云端 |
| `state.json` | OSS | 跨机器共享、分布式锁 |
| `backup.json` | OSS | 跨机器共享 |
| `history/*.json` | OSS | 审计追踪 |

---

## 3. 组件设计

### 3.1 OSS 状态管理与分布式锁

#### 3.1.1 状态定义

| status | 含义 | 可转换到 |
|--------|------|----------|
| `deploying` | 部署中 | `running`（成功），失败时保持 `deploying`（从断点恢复） |
| `running` | 运行中 | `suspending`, `destroying` |
| `suspending` | 停机中 | `suspended`（成功），失败时回退到 `running` |
| `suspended` | 已停机 | `resuming`, `destroying` |
| `resuming` | 恢复中 | `running`（成功），失败时回退到 `suspended` |
| `destroying` | 销毁中 | `destroyed`（成功），失败时回退到 `previous_status` |
| `destroyed` | 已销毁（有快照） | `deploying`（从快照恢复）；无快照时直接删除 state.json |

**`previous_status` 字段**：进入 `destroying` 状态时，将当前 status（`running` 或 `suspended`）保存到 `previous_status`。destroy 失败时根据此字段回退状态。其他过渡状态（deploying/suspending/resuming）失败时回退目标固定，不需要此字段。

**State Transition Diagram**:

图例：`[状态]` 表示状态，"动作" 表示触发动作，success/failed 表示转换结果

```
                                 [no state]
                                      |
                                 start deploy
                                      v
                              +---------------+
                              | [deploying]   | <---------------------+
                              +---------------+                       |
                                   |        |                         |
                            success|        |failed(保留已创建资源,    |
                                   |        | 状态仍为 deploying,     |
                                   |        | 下次 deploy 从断点恢复) |
                                   v        |                         |
                          +---------------+ |                         |
                          |  [running]    | |                         |
                          +---------------+ |                         |
                               |       |    |                         |
                        suspend|  destroy   |                         |
                               v       |    |                         |
                      +---------------+ |   |                         |
                      | [suspending]  | |   |                         |
                      +---------------+ |   |                         |
                           |        |   |   |                         |
                    success|        |failed  |                        |
                           v        v   |   |                         |
                  +---------------+ +---+---+---+                     |
                  | [suspended]   | |  [running] |                    |
                  +---------------+ +------------+                    |
                       |       |                                      |
                 resume|  destroy                                     |
                       v       |                                      |
                  +---------------+                                   |
                  | [resuming]    |                                   |
                  +---------------+                                   |
                       |       |                                      |
                success|       |failed                                |
                       v       v                                      |
              +---------------+ +---------------+                     |
              |  [running]    | | [suspended]   |                     |
              +---------------+ +---------------+                     |
                                                                      |
  [running] or [suspended]                                            |
         |                                                            |
    destroy                                                           |
         v                                                            |
  +---------------+                                                   |
  | [destroying]  |                                                   |
  +---------------+                                                   |
       |        |                                                     |
  success|      |failed(回退到之前状态:                                |
       |        | running 或 suspended)                               |
       v        |                                                     |
  +---------------+                                                   |
  | [destroyed]   |                                                   |
  +---------------+                                                   |
         |                                                            |
   deploy from snapshot                                               |
         +------------------------------------------------------------+
```

**deploying 失败处理**：deploy 失败时状态保持 `deploying`，已创建的资源 ID 保留在 state 中。下次执行 `cloudcode deploy` 时检测到 `deploying` 状态，提示用户继续或放弃（放弃则清理已创建资源）。

#### 3.1.2 OSS 文件结构

```
cloudcode-state-<account_id>/
├── state.json          # 当前状态
├── backup.json         # 快照元数据（可选）
├── .lock               # 分布式锁（临时）
└── history/            # 操作历史
    ├── 2026-02-24T10-00-00.json
    └── ...
```

**state.json** — 当前状态：

```json
{
  "version": "0.3.0",
  "status": "running",
  "previous_status": "",
  "region": "ap-southeast-1",
  "resources": {
    "vpc": {"id": "vpc-xxx", "cidr": "192.168.0.0/16"},
    "vswitch": {"id": "vsw-xxx", "zone_id": "ap-southeast-1a"},
    "security_group": {"id": "sg-xxx"},
    "ssh_key_pair": {"name": "cloudcode-ssh-key"},
    "ecs": {"id": "i-xxx", "instance_type": "ecs.e-c1m2.large"},
    "eip": {"id": "eip-xxx", "ip": "47.100.1.1"}
  },
  "cloudcode": {
    "domain": "oc.example.com",
    "username": "admin"
  },
  "updated_at": "2026-02-24T10:00:00Z"
}
```

**backup.json** — 快照元数据：

```json
{
  "snapshot_id": "s-t4nxxxxxxxxx",
  "cloudcode_version": "0.3.0",
  "created_at": "2026-02-22T10:00:00Z",
  "region": "ap-southeast-1",
  "disk_size": 60
}
```

**.lock** — 分布式锁：

```json
{
  "operation": "suspend",
  "started_at": "2026-02-24T10:00:00Z",
  "client_id": "laptop-hwuu"
}
```

**history/<timestamp>.json** — 操作历史：

```json
{
  "timestamp": "2026-02-24T10:00:00Z",
  "operation": "suspend",
  "from_status": "running",
  "to_status": "suspended",
  "client_id": "laptop-hwuu",
  "success": true,
  "error": null,
  "duration_ms": 5000
}
```

**历史记录清理机制**：
- 使用 OSS 生命周期规则自动清理，不在应用层处理
- init 创建 bucket 时配置生命周期规则：`history/` 前缀下的对象 30 天后自动删除
- 避免多客户端并发清理的竞态问题

#### 3.1.3 分布式锁实现

OSS 支持条件写入（`If-None-Match: *`）和条件删除（`If-Match: <ETag>`），用于实现分布式锁。

**锁文件路径**：`.lock`（bucket 根目录）

```go
type Lock struct {
    Operation string    `json:"operation"`
    StartedAt time.Time `json:"started_at"`
    ClientID  string    `json:"client_id"`  // hostname + PID，确保唯一
}

// AcquireLock 获取锁（原子操作：不存在才写入）
// 调用方应先检查锁是否过期：过期锁通过 ForceAcquireLock 接管，未过期锁提示用户选择强制接管
func AcquireLock(ossClient OSSClient, lock Lock) error {
    body, _ := json.Marshal(lock)
    _, err := ossClient.PutObject(".lock", body,
        oss.IfNoneMatch("*"))  // 原子：不存在才写入
    if isConditionFailed(err) {
        return ErrLockConflict
    }
    return err
}

// ReleaseLock 释放锁（原子操作：用 ETag 条件删除，避免误删他人锁）
func ReleaseLock(ossClient OSSClient, clientID string) error {
    lock, etag, err := GetLockWithETag(ossClient)
    if err != nil {
        return err
    }
    if lock == nil {
        return nil  // 锁已不存在
    }
    if lock.ClientID != clientID {
        return ErrNotOwner
    }
    // 条件删除：ETag 匹配才删除，防止 Get→Delete 之间锁被其他客户端修改
    return ossClient.DeleteObject(".lock", oss.IfMatch(etag))
}

// ForceAcquireLock 强制接管（先删后写，用 IfNoneMatch 保证写入原子性）
func ForceAcquireLock(ossClient OSSClient, lock Lock) error {
    if err := ossClient.DeleteObject(".lock"); err != nil && !isNotFound(err) {
        return fmt.Errorf("删除旧锁失败: %w", err)
    }
    body, _ := json.Marshal(lock)
    _, err := ossClient.PutObject(".lock", body,
        oss.IfNoneMatch("*"))  // 原子写入，若其他客户端抢先则失败
    if isConditionFailed(err) {
        return ErrLockConflict  // 其他客户端在 Delete→Put 之间抢先获取了锁
    }
    return err
}

// GetLockWithETag 读取锁及其 ETag（用于条件删除）
func GetLockWithETag(ossClient OSSClient) (*Lock, string, error) {
    body, etag, err := ossClient.GetObjectWithETag(".lock")
    if isNotFound(err) {
        return nil, "", nil
    }
    if err != nil {
        return nil, "", err
    }
    var lock Lock
    if err := json.Unmarshal(body, &lock); err != nil {
        return nil, "", fmt.Errorf("锁文件损坏: %w", err)
    }
    return &lock, etag, nil
}
```

**锁续期机制**：

长操作（如 deploy）可能超过锁超时时间。持锁期间启动后台 goroutine 定期续期：

```go
// RenewLock 续期：更新 started_at，保持锁有效
// 调用方在获取锁后启动 goroutine，每 5 分钟调用一次
func RenewLock(ossClient OSSClient, clientID string) error {
    lock, etag, err := GetLockWithETag(ossClient)
    if err != nil || lock == nil || lock.ClientID != clientID {
        return ErrNotOwner
    }
    lock.StartedAt = time.Now()
    body, _ := json.Marshal(lock)
    // 条件写入：ETag 匹配才更新，防止覆盖他人锁
    _, err = ossClient.PutObject(".lock", body, oss.IfMatch(etag))
    if isConditionFailed(err) {
        return ErrNotOwner  // 锁已被其他客户端接管
    }
    return err
}
```

**锁超时机制**：
- 锁的 `started_at` 超过 15 分钟视为过期（续期间隔 5 分钟，3 次未续期即过期）
- 过期锁可被其他客户端自动接管（通过 `ForceAcquireLock`）
- 续期失败时：通过 context 取消通知主操作 goroutine 中止，保留当前 state（含已创建资源），不释放锁（让其自然过期），下次执行同一命令时从断点恢复

#### 3.1.4 操作流程（带锁）

**deploy 流程**：

```
1. 从 OSS 读 state.json
2. 检查状态：
   - 不存在 → 全新部署
   - destroyed → 从快照恢复
   - deploying → 提示"上次部署未完成，是否继续？"
   - 其他过渡状态 → 提示"操作进行中"
3. 获取锁（失败则提示并退出）
4. 写 OSS state (status: deploying)

5. 创建 VPC → 写 OSS state（VPC 阶段完成）
6. 创建 VSwitch → 写 OSS state（VSwitch 阶段完成）

7. 创建安全组 → 写 OSS state（安全组阶段完成）

8. 创建 ECS → 写 OSS state（ECS 阶段完成）

9. 绑定 EIP → 写 OSS state（EIP 阶段完成）

10. 部署应用（SSH 上传配置、docker compose up）→ 写 OSS state（应用阶段完成）

11. 写 OSS state (status: running)
12. 写 OSS history
13. 释放锁
```

**分阶段保存策略**：
- 每个资源创建完成后立即写入 OSS state（VPC、VSwitch、SG、ECS、EIP、应用）
- **先创建资源，再写 OSS**：若 OSS 写入失败但资源已创建，下次恢复时通过幂等性检查跳过已存在的资源
- 若 OSS 写入失败，操作中止并提示用户重试（已创建的资源不回滚，下次 deploy 从断点恢复）
- 阿里云 API 本身具备幂等性：重复创建同名 VPC/安全组等会返回已存在的资源 ID，不会重复创建

**suspend 流程**：

```
1. 从 OSS 读 state → 检查 status: running
2. 获取锁
3. 写 OSS state (status: suspending)
4. StopInstance + 等待
5. 写 OSS state (status: suspended)
6. 写 OSS history
7. 释放锁
```

**resume 流程**：

```
1. 从 OSS 读 state → 检查 status: suspended
2. 获取锁
3. 写 OSS state (status: resuming)
4. StartInstance + 等待 + SSH 健康检查
5. 写 OSS state (status: running)
6. 写 OSS history
7. 释放锁
```

**destroy 流程**：

```
1. 从 OSS 读 state
2. 获取锁
3. 写 OSS state (status: destroying, previous_status: 当前状态)
4. 保留快照？
   - 是：创建快照 → 写 OSS backup.json
   - 否：删除 OSS backup.json（若存在）
5. 删除资源
   - 若失败：写 OSS state (status: previous_status)，释放锁，返回错误
6. 保留快照？
   - 是：写 OSS state (status: destroyed)
   - 否：删除 OSS state.json
7. 写 OSS history
8. 释放锁
```

#### 3.1.5 中断恢复

**场景：deploy 到一半断电**

```
重启后执行 cloudcode deploy：
1. 从 OSS 读 state → status: deploying
2. 检查锁：
   - 有锁且未过期（started_at ≤ 15 分钟前）→ 提示"操作进行中，是否强制接管？"
   - 有锁且已过期（started_at > 15 分钟前）→ 自动接管继续
3. 根据 state 中已有资源，跳过已创建的步骤
4. 继续未完成的部署
```

**场景：destroy 中断（正在创建快照）**

```
重启后执行 cloudcode status：
1. 从 OSS 读 state → status: destroying
2. 检查 backup.json 是否存在
   - 存在：快照已创建，可重新执行 destroy
   - 不存在：快照未创建，ECS 可能仍存在
3. 提示用户选择：继续销毁 / 放弃
```

**场景：suspend 到一半断电**

```
重启后执行 cloudcode status：
1. 从 OSS 读 state → status: suspending
2. 检查 ECS 实际状态：
   - Stopped → 更新 state (status: suspended)
   - Running → 恢复 state (status: running)，提示用户重新 suspend
```

**场景：resume 到一半断电**

```
重启后执行 cloudcode status：
1. 从 OSS 读 state → status: resuming
2. 检查 ECS 实际状态：
   - Running → 更新 state (status: running)
   - Stopped → 恢复 state (status: suspended)，提示用户重新 resume
```

#### 3.1.6 并发控制

**多终端并发场景**：

```
终端 A: cloudcode suspend
  → OSS 锁被 A 持有
  → state.json (status: suspending)

终端 B: cloudcode resume（同时）
  → 读到 status: suspending
  → 尝试获取锁失败
  → 提示："suspend 操作进行中（开始于 2 分钟前），是否强制接管？[y/N]"
```

**强制接管**：

- 用户确认后，删除旧锁，写入新锁
- 旧操作的资源状态可能不一致，需检查并恢复

#### 3.1.7 幂等性保证

| 操作 | 幂等性处理 |
|------|-----------|
| `suspend` | 若 status 已是 suspended，跳过；若 suspending，等待完成或强制接管 |
| `resume` | 若 status 已是 running，跳过；若 resuming，等待完成或强制接管 |
| `destroy` | 基于 state 中资源 ID，删除存在的资源；已删除的跳过 |
| `deploy` | 基于 state 中已有资源 + 实际云上状态，跳过已存在且正常的资源 |

**deploy 幂等性增强**：
- 不能仅依赖 state 中记录的资源 ID，还要验证云上资源是否真实存在
- 验证 ECS 是否存在、状态是否正常
- 验证 EIP 是否已绑定
- 如果云上资源已被用户手动删除，deploy 应尝试重新创建（而非跳过）

```
deploy 跳过逻辑：
if state 中有 vpc_id:
    if DescribeVPC(vpc_id) 存在:
        skip 创建 VPC
    else:
        创建新 VPC（记录新 ID）
```

#### 3.1.8 Edge Case 处理

| 场景 | 处理 |
|------|------|
| suspend 到一半断电 | 重启后检测 status: suspending，查询 ECS 实际状态修正 |
| resume 到一半断电 | 重启后检测 status: resuming，查询 ECS 实际状态修正 |
| deploy 到一半断电 | 重启后检测 status: deploying，从断点恢复（幂等性保证） |
| destroy 不成功 | 保留 state，记录失败原因，回退到之前状态，支持重试 |
| 多进程并发 | 分布式锁（IfNoneMatch 原子写入）+ 状态检查 + 强制接管选项 |
| 锁过期 | 超过 15 分钟未续期视为过期，允许自动接管 |
| 续期失败 | 通过 context 取消主操作，保留 state，锁自然过期，下次从断点恢复 |
| OSS 写入失败 | 操作中止，已创建资源保留，下次从断点恢复 |
| OSS 不可用 | 报错退出（v0.3 不支持降级到本地模式） |
| 云上资源被手动删除 | deploy 幂等性检查发现资源不存在，重新创建 |

#### 3.1.9 修改文件

| 文件 | 改动 |
|------|------|
| `internal/alicloud/oss.go` | OSS client、锁操作、状态读写 |
| `internal/alicloud/interfaces.go` | 新增 OSSAPI 接口 |
| `internal/config/state.go` | 新增过渡状态、OSS 交互 |
| `internal/config/backup.go` | 改为 OSS 存储 |
| `internal/config/history.go` | 新增：操作历史记录 |
| `internal/deploy/*.go` | 所有操作增加锁和状态检查 |
| `cmd/cloudcode/main.go` | 新增 --force 接管选项 |
| `tests/unit/` | 相关 mock 和测试 |

#### 3.1.10 测试要点

- 分布式锁获取/释放/冲突检测
- 状态转换的原子性
- 中断后的恢复流程
- 并发操作的正确处理
- 幂等性验证（重复执行同一操作）
- OSS 不可用时的降级处理
- 锁过期自动接管

---

### 3.2 init 命令扩展

#### 3.2.1 交互流程

```
$ cloudcode init
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

#### 3.2.2 错误处理

| 场景 | 行为 |
|------|------|
| 凭证无效 | 提示重试或退出 |
| bucket 已存在 | 跳过创建，继续使用 |
| bucket 名称冲突（其他账号） | 提示：bucket 已被占用，请使用其他区域 |
| 无 OSS 权限 | 提示：请开通 OSS 服务或添加 AliyunOSSFullAccess 权限 |

#### 3.2.3 本地目录结构

```
~/.cloudcode/
├── credentials     # AccessKeyID、AccessKeySecret、Region（权限 600）
└── ssh_key         # SSH 私钥
```

---

## 4. 实现规划

### 4.1 优先级

| 功能 | 优先级 | 理由 |
|------|--------|------|
| OSS client + 分布式锁 | P0 | 基础设施，所有操作依赖 |
| 状态迁移（本地 → OSS） | P0 | 兼容 v0.2 用户 |
| 操作流程改造 | P0 | 核心功能 |
| 操作历史 | P1 | 增强功能 |

### 4.2 实现步骤

| 步骤 | 任务 | 依赖 |
|------|------|------|
| 1 | OSS client、锁操作、状态读写 | 无 |
| 2 | init 命令增加 OSS bucket 创建 | 步骤 1 |
| 3 | 迁移逻辑：检测本地 state.json 自动迁移到 OSS | 步骤 1 |
| 4 | 改造 deploy/suspend/resume/destroy 流程 | 步骤 1 |
| 5 | 操作历史记录 | 步骤 1 |

#### 4.2.1 v0.2 升级迁移

v0.2 用户升级到 v0.3 时，需要将本地 state.json 迁移到 OSS：

```
首次执行 cloudcode（任意命令）：
1. 检查 OSS bucket 是否存在（不存在则提示 cloudcode init）
2. 读取本地 ~/.cloudcode/state.json
3. 若本地 state 存在且 OSS state 不存在：
   - 提示用户："检测到 v0.2 部署记录，是否迁移到云端？[Y/n]"
   - 用户确认后：
     a. 获取锁（防止多台机器同时迁移）
     b. 再次检查 OSS state 是否存在（双重检查，防止获取锁期间其他机器已迁移）
     c. 若 OSS state 仍不存在：写入 OSS state.json + backup.json（若有）
     d. 备份本地 state.json 到 state.json.bak
     e. 释放锁
   - 用户拒绝：报错退出（v0.3 不支持纯本地模式）
4. 若本地 state 和 OSS state 都存在：
   - 以 OSS state 为准，提示用户本地记录已过时
5. 若只有 OSS state（无本地）：
   - 正常使用
```

**迁移注意事项**：
- 迁移使用分布式锁 + 双重检查，防止多台机器同时迁移导致覆盖
- v0.3 不支持降级到纯本地模式，拒绝迁移则无法继续操作
- 建议用户先在所有机器上升级到 v0.3，再执行第一次操作

---

## 5. 成本影响

新增 OSS bucket 存储：

| 资源 | 月费用 |
|------|--------|
| OSS 存储（< 1MB） | ~$0.01 |
| OSS 请求费用 | 可忽略 |

对整体成本影响可忽略不计。

---

## 变更记录

- v1.4 (2026-02-25): 第三轮 Review 修复 — 锁过期检测时间统一为 15 分钟；补充 previous_status 设置时机说明；destroyed 状态补充无快照时删除 state.json；suspending/resuming 失败路径写法与 destroying 统一；destroy 流程补充失败回退分支；AcquireLock 补充过期锁处理策略说明
- v1.3 (2026-02-25): 第二轮 Review 修复 — 状态表 deploying 失败描述统一为保持 deploying；destroying 失败回退增加 previous_status 字段；GetLockWithETag 处理 JSON 解析错误；RenewLock 条件写入失败返回 ErrNotOwner；ForceAcquireLock 检查 delete 错误；续期失败明确通过 context 取消 + 保留 state + 锁自然过期
- v1.2 (2026-02-25): Review 修复 — ForceAcquireLock/ReleaseLock 改用条件写入（IfNoneMatch/IfMatch）解决竞态；新增锁续期机制（5 分钟间隔，15 分钟过期）；去掉 partial state，deploying 失败保持 deploying 状态从断点恢复；状态图补充 destroying 失败回退路径；deploy 分阶段保存明确"先创建资源再写 OSS"策略；迁移流程加分布式锁+双重检查；补充 resume 中断恢复场景；history 清理改用 OSS 生命周期规则；去掉 --local 降级模式；修复路径不一致（统一用 .lock）
- v1.1 (2026-02-24): 补充状态转换图失败分支；锁实现增加 client_id 验证和强制接管；分阶段保存替代增量保存；补充历史记录清理机制（30 天/100 条）；补充 v0.2 升级迁移逻辑；补充 deploy 幂等性增强（验证云上资源是否真实存在）
- v1.0 (2026-02-24): 初始版本，包含 OSS 状态管理、分布式锁、中断恢复、操作历史
