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
| `deploying` | 部署中 | `running`, `destroyed`（失败） |
| `running` | 运行中 | `suspending`, `destroying` |
| `suspending` | 停机中 | `suspended`, `running`（失败） |
| `suspended` | 已停机 | `resuming`, `destroying` |
| `resuming` | 恢复中 | `running`, `suspended`（失败） |
| `destroying` | 销毁中 | `destroyed`, `running`/`suspended`（失败） |
| `destroyed` | 已销毁（有快照） | `deploying`（从快照恢复） |

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
                            success|        |failed                   |
                                   v        v                         |
                          +---------------+ +---------------+         |
                          |  [running]    | |[partial state]|         |
                          +---------------+ +---------------+         |
                                    |                                 |
                             suspend|                                 |
                                    |                                 |
                                    v                                 |
                          +---------------+                           |
                          | [suspending]  |                           |
                          +---------------+                           |
                               |        |                             |
                        success|        |failed                       |
                               v        v                             |
                      +---------------+ +---------------+             |
                      | [suspended]   | |  [running]    |             |
                      +---------------+ +---------------+             |
                                |                                     |
                          resume|                                     |
                                |                                     |
                                v                                     |
                      +---------------+                               |
                      | [resuming]    |                               |
                      +---------------+                               |
                           |       |                                  |
                    success|       |failed                            |
                           v       v                                  |
                  +---------------+ +---------------+                 |
                  |  [running]    | | [suspended]   |                 |
                  +---------------+ +---------------+                 |
                                                                      |
  [running] or [suspended]                                            |
         |                                                            |
    destroy (keep snapshot)                                           |
         v                                                            |
  +---------------+                                                   |
  | [destroying]  |                                                   |
  +---------------+                                                   |
         |                                                            |
    success                                                           |
         v                                                            |
  +---------------+                                                   |
  | [destroyed]   |                                                   |
  +---------------+                                                   |
         |                                                            |
   deploy from snapshot                                               |
         +------------------------------------------------------------+
```

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
- 每次写入新历史时，检查 `history/` 目录
- 超过 30 天的记录自动删除
- 最多保留 100 条记录（按时间排序，旧记录先删）

#### 3.1.3 分布式锁实现

OSS 支持条件写入，用于实现分布式锁：

```go
type Lock struct {
    Operation string    `json:"operation"`
    StartedAt  time.Time `json:"started_at"`
    ClientID   string    `json:"client_id"`  // 客户端唯一标识（hostname 或随机 UUID）
}

// 获取锁（不存在才写入）
func AcquireLock(ossClient OSSClient, lock Lock) error {
    body, _ := json.Marshal(lock)
    _, err := ossClient.PutObject("state/.lock", body,
        oss.PutObjectOptions{IfNoneMatch: "*"})  // 关键：不存在才写入
    if err != nil && err.Code == "PreconditionFailed" {
        return ErrLockConflict
    }
    return err
}

// 释放锁（只能释放自己持有的锁）
func ReleaseLock(ossClient OSSClient, clientID string) error {
    lock, err := GetLock(ossClient)
    if err != nil {
        return err
    }
    if lock != nil && lock.ClientID != clientID {
        return ErrNotOwner  // 不是自己的锁，不能释放
    }
    return ossClient.DeleteObject("state/.lock")
}

// 强制接管（删除任何锁）
func ForceAcquireLock(ossClient OSSClient, lock Lock) error {
    // 先删除可能存在的旧锁
    ossClient.DeleteObject("state/.lock")
    // 写入新锁
    body, _ := json.Marshal(lock)
    _, err := ossClient.PutObject("state/.lock", body, nil)
    return err
}

// 检查锁状态
func GetLock(ossClient OSSClient) (*Lock, error) {
    body, err := ossClient.GetObject("state/.lock")
    if err != nil && err.Code == "NoSuchKey" {
        return nil, nil  // 无锁
    }
    var lock Lock
    json.Unmarshal(body, &lock)
    return &lock, nil
}
```

**锁超时机制**：
- 锁的 `started_at` 超过 30 分钟视为过期
- 过期锁可被其他客户端自动接管（先删除旧锁，再写入新锁）

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
- 每个资源类型创建完成后立即保存一次（VPC、VSwitch、SG、ECS、EIP、应用）
- 避免频繁写入，但仍能在每个阶段失败后从断点恢复
- 与其"每步增量保存"，不如"分阶段保存"，减少 OSS 请求次数

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
3. 写 OSS state (status: destroying)
4. 保留快照？
   - 是：创建快照 → 写 OSS backup.json
   - 否：删除 OSS backup.json（若存在）
5. 删除资源
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
   - 有锁且 started_at > 10 分钟前 → 提示"上次部署未完成，是否接管？"
   - 无锁或锁过期 → 自动继续
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
| suspend 到一半断电 | 重启后检测 status: suspending，提示恢复或强制接管 |
| resume 到一半断电 | 重启后检测 status: resuming，检查 ECS 实际状态 |
| destroy 不成功 | 保留 state，记录失败原因，支持重试 |
| 多进程并发 | 分布式锁 + 状态检查 + 强制接管选项 |
| 锁过期 | 超过 30 分钟的锁视为过期，允许自动接管 |
| OSS 不可用 | 提示用户等待或使用 --local 降级为本地模式（只读操作） |

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
1. 读取本地 ~/.cloudcode/state.json
2. 若本地 state 存在且 OSS state 不存在：
   - 提示用户："检测到 v0.2 部署记录，是否迁移到云端？[Y/n]"
   - 用户确认后：
     a. 获取锁
     b. 写入 OSS state.json
     c. 备份本地 state.json 到 state.json.bak
     d. 释放锁
   - 用户拒绝：继续使用本地 state（降级模式）
3. 若本地 state 和 OSS state 都存在：
   - 以 OSS state 为准（提示用户本地记录将被忽略）
4. 若只有 OSS state 不存在本地：
   - 正常初始化
```

**迁移前提条件**：
- 迁移期间不能有其他机器同时操作
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

- v1.1 (2026-02-24): 补充状态转换图失败分支；锁实现增加 client_id 验证和强制接管；分阶段保存替代增量保存；补充历史记录清理机制（30 天/100 条）；补充 v0.2 升级迁移逻辑；补充 deploy 幂等性增强（验证云上资源是否真实存在）
- v1.0 (2026-02-24): 初始版本，包含 OSS 状态管理、分布式锁、中断恢复、操作历史
