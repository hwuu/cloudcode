# CloudCode 实现状态跟踪

## 当前进度

| 步骤 | 任务 | 状态 | 完成日期 |
|------|------|------|----------|
| 1 | Go 项目初始化 + Cobra CLI 框架 | ✅ 完成 | 2026-02-15 |
| 2 | internal/alicloud — 阿里云资源管理 | ✅ 完成 | 2026-02-15 |
| 3 | internal/config — 状态文件与交互输入 | ⏳ 待开始 | |
| 4 | internal/remote — SSH/SFTP 远程操作 | ⏳ 待开始 | |
| 5 | internal/template — 模板渲染 | ⏳ 待开始 | |
| 6 | deploy 命令 — 串联完整部署流程 | ⏳ 待开始 | |
| 7 | status 命令 | ⏳ 待开始 | |
| 8 | destroy 命令 | ⏳ 待开始 | |
| 9 | goreleaser + GitHub Actions | ⏳ 待开始 | |
| 10 | 端到端测试 + 文档更新 | ⏳ 待开始 | |

---

## 步骤 1 详情

**状态**：✅ 完成

**新增文件**：
- `go.mod` / `go.sum` — Go module，依赖 cobra v1.10.2
- `cmd/cloudcode/main.go` — CLI 入口，4 个子命令（deploy/status/destroy/version 空壳）
- `Makefile` — build/test/clean，ldflags 注入版本信息
- `tests/unit/main_test.go` — 3 个测试用例
- `.gitignore` — 追加 Go 忽略项

**测试结果**：
- `TestRootCommandHelp` — PASS
- `TestVersionOutput` — PASS
- `TestSubcommandsRegistered` — PASS

**决策记录**：
- version 使用独立子命令（非 cobra 内置 `--version`），支持多行输出

**环境问题**：
- Go 代理超时，改用 `GOPROXY=https://goproxy.cn,direct`（已记录到 CLAUDE.md）
- Go 安装在 `~/go-sdk/go/`（无 sudo 权限）

---

## 步骤 2 详情

**状态**：✅ 完成

**新增文件**：
- `internal/alicloud/client.go` — SDK 客户端初始化（ECS/VPC/STS），Config 加载
- `internal/alicloud/interfaces.go` — STSAPI/VPCAPI/ECSAPI 接口定义
- `internal/alicloud/errors.go` — 错误定义
- `internal/alicloud/sts.go` — GetCallerIdentity 前置检查
- `internal/alicloud/vpc.go` — VPC/VSwitch/SecurityGroup CRUD，安全组规则
- `internal/alicloud/ecs.go` — ECS 实例管理 + SSH 密钥对 + 可用区选择 + 等待 Running
- `internal/alicloud/eip.go` — EIP 分配/绑定/解绑/释放
- `tests/unit/alicloud_mocks_test.go` — Mock 实现
- `tests/unit/alicloud_test.go` — 单元测试（14 个测试用例）

**核心依赖**：
- `github.com/alibabacloud-go/ecs-20140526/v4` v4.26.10
- `github.com/alibabacloud-go/vpc-20160428/v6` v6.16.0
- `github.com/alibabacloud-go/sts-20150401/v2` v2.1.0
- `github.com/alibabacloud-go/darabonba-openapi/v2` v2.1.15

**测试结果**：
- `TestLoadConfigFromEnv_MissingAccessKeyID` — PASS
- `TestGetCallerIdentity_Success` — PASS
- `TestGetCallerIdentity_Error` — PASS
- `TestCreateVPC_Success` — PASS
- `TestCreateVSwitch_Success` — PASS
- `TestCreateSecurityGroup_Success` — PASS
- `TestDefaultSecurityGroupRules` — PASS
- `TestSelectAvailableZone_FirstAvailable` — PASS
- `TestSelectAvailableZone_Fallback` — PASS
- `TestSelectAvailableZone_NoAvailableZone` — PASS
- `TestCreateECSInstance_Success` — PASS
- `TestWaitForInstanceRunning_Timeout` — PASS
- `TestAllocateEIP_Success` — PASS
- `TestCreateSSHKeyPair_Success` — PASS

**关键设计**：
- 安全组 API 使用 ECS SDK（非 VPC SDK）
- SystemDisk.Size 类型为 `*int32`
- STS v2 SDK 的 GetCallerIdentity 不需要请求参数
- 使用接口抽象 SDK client，支持 mock 测试

**Review 修复（2026-02-15）**：
- WaitForEIPBound 改用 context + ticker 模式，避免紧密循环
- client.go 统一使用 client.Config 初始化所有 SDK 客户端
- 删除测试中无用的 trueVal 变量
