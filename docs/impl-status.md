# CloudCode 实现状态跟踪

## 当前进度

| 步骤 | 任务 | 状态 | 完成日期 |
|------|------|------|----------|
| 1 | Go 项目初始化 + Cobra CLI 框架 | ✅ 完成 | 2026-02-15 |
| 2 | internal/alicloud — 阿里云资源管理 | ✅ 完成 | 2026-02-15 |
| 3 | internal/config — 状态文件与交互输入 | ✅ 完成 | 2026-02-15 |
| 4 | internal/remote — SSH/SFTP 远程操作 | ✅ 完成 | 2026-02-15 |
| 5 | internal/template — 模板渲染 | ✅ 完成 | 2026-02-15 |
| 6 | deploy 命令 — 串联完整部署流程 | ✅ 完成 | 2026-02-15 |
| 7 | status 命令 | ✅ 完成 | 2026-02-15 |
| 8 | destroy 命令 | ✅ 完成 | 2026-02-15 |
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

---

## 步骤 3 详情

**状态**：✅ 完成

**新增文件**：
- `internal/config/state.go` — State 结构体 + LoadState/SaveState/DeleteState + HasXxx 判断 + ResolveKeyPath
- `internal/config/prompt.go` — Prompter（抽象 io.Reader/Writer）+ Argon2id 哈希 + Secret 生成
- `tests/unit/config_state_test.go` — 状态文件测试（8 个测试用例）
- `tests/unit/config_prompt_test.go` — Prompter 测试（11 个测试用例）

**核心依赖**：
- `golang.org/x/crypto` v0.24.0 — Argon2id 密码哈希

**测试结果**：
- `TestState_SaveAndLoad` — PASS
- `TestLoadState_NotFound` — PASS
- `TestState_DirPermissions` — PASS
- `TestResolveKeyPath` — PASS（2 子测试）
- `TestState_HasMethods` — PASS
- `TestState_HasMethods_Empty` — PASS
- `TestDeleteState` — PASS
- `TestNewState_SetsCreatedAt` — PASS
- `TestPrompter_Prompt` — PASS
- `TestPrompter_PromptWithDefault` — PASS（2 子测试）
- `TestPrompter_PromptConfirm` — PASS（5 子测试）
- `TestPrompter_PromptSelect` — PASS（4 子测试）
- `TestPrompter_PromptSelect_Invalid` — PASS
- `TestPrompter_PromptSelect_OutOfRange` — PASS
- `TestHashPassword_Format` — PASS
- `TestHashPassword_MemoryParameter` — PASS
- `TestHashPassword_UniqueSalts` — PASS
- `TestGenerateSecret_Length` — PASS
- `TestGenerateSecret_Uniqueness` — PASS

**关键设计**：
- State 结构体与 design-oc.md 5.1.4 完全一致
- `~/.cloudcode/` 目录自动创建（0700 权限）
- Prompter 抽象 stdin/stdout，支持 mock 测试
- Argon2id 参数：iterations=1, salt_length=16, parallelism=8, memory=64 MiB (65536 KiB)
- Secret 生成：crypto/rand 生成 32 字节随机数据，base64 编码输出
- 哈希格式：`$argon2id$v=19$m=65536,t=1,p=8$<salt>$<hash>`

**Review 修复（2026-02-15）**：
- Argon2id memory 参数修正为 65536 KiB（与 Authelia 配置一致）
- NewState 自动填充 CreatedAt 字段
- 添加 TODO 标注 readPassword 当前实现为空壳（密码明文回显，后续优化）

---

## 步骤 4 详情

**状态**：✅ 完成

**新增文件**：
- `internal/remote/ssh.go` — SSHClient 接口 + WaitForSSH（指数退避 1s→10s，context 超时 2 分钟）+ 常量定义
- `internal/remote/sftp.go` — SFTPClient 接口 + UploadFiles 批量上传
- `tests/unit/remote_test.go` — 9 个测试用例

**测试结果**：
- `TestRunCommand_Success` — PASS
- `TestRunCommand_Error` — PASS
- `TestWaitForSSH_Success` — PASS
- `TestWaitForSSH_Timeout` — PASS
- `TestWaitForSSH_ExponentialBackoff` — PASS
- `TestUploadFile_Success` — PASS
- `TestUploadFile_Error` — PASS
- `TestUploadFiles_Multiple` — PASS
- `TestUploadFiles_StopsOnError` — PASS

**关键设计**：
- SSHClient / SFTPClient 接口抽象，支持 mock 测试
- WaitForSSH 使用指数退避（InitialInterval → MaxInterval），context 控制超时
- UploadFiles 批量上传，任一失败立即返回错误
- 常量定义：DefaultCommandTimeout=5min, DockerInstallTimeout=10min

---

## 步骤 5 详情

**状态**：✅ 完成

**新增文件**：
- `internal/template/render.go` — go:embed 嵌入 + RenderTemplate / GetStaticFile / RenderAll
- `internal/template/templates/docker-compose.yml` — 静态文件
- `internal/template/templates/Caddyfile.tmpl` — 模板文件
- `internal/template/templates/env.tmpl` — 模板文件
- `internal/template/templates/Dockerfile.opencode` — 静态文件
- `internal/template/templates/authelia/configuration.yml.tmpl` — 模板文件
- `internal/template/templates/authelia/users_database.yml.tmpl` — 模板文件
- `tests/unit/template_render_test.go` — 12 个测试用例

**测试结果**：
- `TestStaticFiles_NonEmpty` — PASS
- `TestTemplateFiles_NonEmpty` — PASS
- `TestRenderCaddyfile` — PASS
- `TestRenderEnv` — PASS
- `TestRenderEnv_OptionalFieldsEmpty` — PASS
- `TestRenderAutheliaConfig` — PASS
- `TestRenderAutheliaUsersDB` — PASS
- `TestGetStaticFile_DockerCompose` — PASS
- `TestGetStaticFile_Dockerfile` — PASS
- `TestRenderAll` — PASS
- `TestRenderTemplate_NotFound` — PASS
- `TestGetStaticFile_NotFound` — PASS

**关键设计**：
- 模板文件放在 `internal/template/templates/`（go:embed 路径相对于源文件目录）
- TemplateData 结构体包含所有渲染字段（Domain/Username/HashedPassword/Email/SessionSecret/StorageEncryptionKey/OpenAIAPIKey/OpenAIBaseURL/AnthropicAPIKey）
- RenderAll 返回 ECS 目标路径 → 内容的映射，与 design-oc.md 5.1.6 文件映射表一致
- 静态文件（docker-compose.yml、Dockerfile.opencode）原样输出，模板文件（.tmpl）渲染后输出

**注意**：design-oc.md 5.1.1 中模板目录在项目根目录 `templates/`，实际放在 `internal/template/templates/`，因为 go:embed 路径必须相对于源文件目录

**Review 修复（2026-02-15）**：
- env.tmpl 可选字段（OpenAIBaseURL/AnthropicAPIKey）为空时不输出对应行

---

## 步骤 6 详情

**状态**：✅ 完成

**新增文件**：
- `internal/deploy/deploy.go` — Deployer 编排器，5 阶段部署流程，依赖注入支持 mock
- `internal/remote/ssh_impl.go` — 真实 SSH/SFTP 实现 + GetPublicIP（ipify）
- `tests/unit/deploy_test.go` — 6 个测试用例

**修改文件**：
- `cmd/cloudcode/main.go` — deploy 命令注入真实阿里云 SDK/SSH/SFTP + `--force` flag

**核心依赖**：
- `golang.org/x/crypto/ssh` — SSH 连接
- `github.com/pkg/sftp` — SFTP 文件上传

**编排流程**：
1. `PreflightCheck` — STS GetCallerIdentity 验证凭证
2. `PromptConfig` — 交互收集域名/用户名/密码/邮箱/AI 提供商/API Key/SSH IP 限制
3. `CreateResources` — 幂等创建 VPC→VSwitch→SG→KeyPair→ECS→Start→WaitRunning→EIP→Associate，每步 SaveState
4. `DeployApp` — WaitForSSH→InstallDocker→RenderTemplates→UploadFiles→docker compose up
5. `HealthCheck` — SSH 执行 docker compose ps 检查容器状态

**测试结果**：
- `TestPreflightCheck_Success` — PASS
- `TestPreflightCheck_STSError` — PASS
- `TestCreateResources_FullDeploy` — PASS
- `TestCreateResources_Idempotent` — PASS
- `TestDeployApp_Success` — PASS
- `TestHealthCheck_Success` — PASS

**关键设计**：
- Deployer 通过依赖注入接收所有外部依赖（ECS/VPC/STS/SSH/SFTP/GetPublicIP），完全可 mock
- `--force` 跳过云资源创建，仅重新部署应用层
- CreateResources 幂等：已存在的资源跳过，不重复创建
- SSH IP 限制：调用 ipify 获取用户公网 IP，询问是否限制 SSH 仅允许该 IP
- nip.io 域名：用户留空域名时自动使用 EIP.nip.io
- WaitInterval/WaitTimeout 可配置，测试中使用 10ms/1s 避免慢测试
- 健康检查失败不阻塞部署，仅输出警告

**Review 修复（2026-02-15）**：
- SSH IP 限制正确传递到 CreateResources → DefaultSecurityGroupRules
- 删除多余的 CreateResourcesWithSSHIP 方法
- Run 方法始终收集 PromptConfig，消除 --force 模式下 cfg 为 nil 的风险
