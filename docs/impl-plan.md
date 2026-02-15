# CloudCode Go CLI 实现计划

基于 design-oc.md v2.7 设计文档，从零构建 `cloudcode` Go CLI 工具。

## 步骤总览

| 步骤 | 任务 | 依赖 | 验证方式 |
|------|------|------|----------|
| 1 | Go 项目初始化 + Cobra CLI 框架 | 无 | `go build` + `cloudcode --help` + `go test` |
| 2 | internal/alicloud — 阿里云资源管理 | 步骤 1 | mock 测试通过 |
| 3 | internal/config — 状态文件与交互输入 | 步骤 1 | state.json 读写测试通过 |
| 4 | internal/remote — SSH/SFTP 远程操作 | 步骤 2 | mock 测试通过 |
| 5 | internal/template — 模板渲染 | 步骤 1 | 模板渲染测试通过 |
| 6 | deploy 命令 — 串联完整部署流程 | 步骤 2-5 | `cloudcode deploy` 端到端 |
| 7 | status 命令 | 步骤 3-4 | `cloudcode status` 正确输出 |
| 8 | destroy 命令 | 步骤 2-3 | `cloudcode destroy` 资源释放 |
| 9 | goreleaser + GitHub Actions | 步骤 6-8 | `goreleaser check` 通过 |
| 10 | 端到端测试 + 文档更新 | 步骤 9 | 全流程通过 |

---

## 步骤 1：Go 项目初始化 + Cobra CLI 框架

**目标**：可编译运行的 CLI 骨架，`cloudcode --help` 显示所有子命令。

**新增文件**：
- `go.mod` — `go mod init github.com/hwuu/cloudcode`
- `cmd/cloudcode/main.go` — 入口 + rootCmd + deploy/status/destroy/version 子命令（空壳）
- `Makefile` — build/test/clean 目标
- `.gitignore` — 追加 Go 相关忽略项
- `tests/unit/main_test.go` — CLI 骨架测试

**TDD 测试**（集成测试，构建二进制后执行验证）：
- `TestRootCommandHelp` — 构建二进制，执行 `--help`，验证输出包含 "cloudcode" 及所有子命令
- `TestVersionOutput` — 构建二进制，执行 `version`，验证输出包含 version/commit/built/go
- `TestSubcommandsRegistered` — 构建二进制，执行 `--help`，验证 deploy/status/destroy/version 子命令存在

**决策记录**：version 使用独立子命令（非 cobra 内置 `--version`），因为需要多行输出。

---

## 步骤 2：internal/alicloud — 阿里云资源管理

**目标**：封装所有阿里云 API 调用，通过接口抽象支持 mock 测试。

**新增文件**：
- `internal/alicloud/client.go` — SDK 客户端初始化（ECS/VPC/STS）
- `internal/alicloud/sts.go` — STS GetCallerIdentity 前置检查
- `internal/alicloud/vpc.go` — VPC / VSwitch / SecurityGroup CRUD
- `internal/alicloud/ecs.go` — ECS 实例 + SSH 密钥对 + 可用区选择 + 等待 Running
- `internal/alicloud/eip.go` — EIP 分配/绑定/解绑/释放
- `tests/unit/alicloud_*_test.go` — 各模块单元测试

**关键设计**：
- 为每个 SDK client 定义 interface，单元测试注入 mock
- 安全组规则：22（用户 IP 或 0.0.0.0/0）、80、443
- 可用区降级：ap-southeast-1a → 1b → 1c
- ECS 状态等待：5 秒轮询，5 分钟超时（参数可配置），等待实例变为 Running

**TDD 测试**：
- 缺少 AK/SK 返回错误
- STS GetCallerIdentity 调用成功/失败
- mock SDK 验证 VPC/ECS/EIP 创建参数和返回值
- 可用区降级逻辑
- ECS 状态等待超时处理

---

## 步骤 3：internal/config — 状态文件与交互输入

**目标**：state.json 读写 + 幂等性判断 + 交互式配置收集。

**新增文件**：
- `internal/config/state.go` — State 结构体 + LoadState/SaveState + HasXxx 判断 + ResolveKeyPath
- `internal/config/prompt.go` — Prompter（抽象 io.Reader/Writer）+ Argon2id 哈希 + secret 生成
- `tests/unit/config_*_test.go`

**关键设计**：
- State 结构体与 design-oc.md 5.1.4 完全一致
- `~/.cloudcode/` 目录自动创建（0700 权限）
- Prompter 抽象 stdin/stdout，支持 mock 测试
- Argon2id 参数（与 design-oc.md 5.5 一致）：iterations=1, salt_length=16, parallelism=8, memory=64KB
- Secret 生成：crypto/rand 生成 32 字节随机数据，base64 编码输出

**TDD 测试**：
- SaveState → LoadState 往返一致
- 文件不存在返回 nil
- ResolveKeyPath 正确拼接
- Argon2id 哈希格式（`$argon2id$v=19$m=64,t=1,p=8$...`）
- GenerateSecret 长度正确（32 字节 → base64 输出）
- GenerateSecret 随机性验证（多次生成不重复）

---

## 步骤 4：internal/remote — SSH/SFTP 远程操作

**目标**：SSH 连接（指数退避重试）+ 命令执行 + SFTP 文件上传。

**新增文件**：
- `internal/remote/ssh.go` — SSH 连接 + RunCommand + WaitForSSH（指数退避 1s→10s，总超时 2 分钟）
- `internal/remote/sftp.go` — SFTP 文件上传（自动创建远程目录）
- `tests/unit/remote_*_test.go`

**关键设计**：
- SSHExecutor / SFTPUploader 接口抽象，支持 mock
- WaitForSSH 前提：ECS 已 Running（步骤 2 的等待逻辑已完成），仅等待 SSH 服务就绪
- Docker 安装命令超时 10 分钟，普通命令 5 分钟
- 文件上传按 design-oc.md 5.1.6 映射表

**TDD 测试**：
- mock SSH 连接和命令执行
- WaitForSSH 重试逻辑（指数退避）
- WaitForSSH 超时处理
- SFTP 上传路径和目录创建

---

## 步骤 5：internal/template — 模板渲染

**目标**：go:embed 嵌入模板文件 + text/template 渲染。

**新增文件**：
- `internal/template/render.go` — go:embed + RenderTemplate / GetStaticFile
- `templates/` 目录下 6 个文件：
  - `docker-compose.yml`（静态）
  - `Caddyfile.tmpl`（模板）
  - `env.tmpl`（模板）
  - `Dockerfile.opencode`（静态）
  - `authelia/configuration.yml.tmpl`（模板）
  - `authelia/users_database.yml.tmpl`（模板）
- `tests/unit/template_render_test.go`

**关键设计**：
- TemplateData 结构体包含所有渲染字段（见下方）
- 静态文件原样输出，模板文件渲染后输出

**TemplateData 字段**：
```go
type TemplateData struct {
    Domain               string  // 域名
    Username             string  // 管理员用户名
    HashedPassword       string  // Argon2id 哈希后的密码
    Email                string  // 管理员邮箱
    SessionSecret        string  // Authelia session 密钥
    StorageEncryptionKey string  // Authelia storage 加密密钥
    OpenAIAPIKey         string  // OpenAI API Key
    OpenAIBaseURL        string  // OpenAI Base URL（可选）
    AnthropicAPIKey      string  // Anthropic API Key（可选）
}
```

**TDD 测试**：
- 每个模板的渲染结果验证（关键字段替换）
- 嵌入文件非空检查
- 静态文件与模板文件区分正确

---

## 步骤 6：deploy 命令 — 串联完整部署流程

**目标**：`cloudcode deploy` 端到端部署成功。

**新增/修改文件**：
- `internal/deploy/deploy.go` — 部署编排
- `cmd/cloudcode/main.go` — 注册 deploy 命令实现 + `--force` flag
- `tests/unit/deploy_test.go`

**编排流程**：
1. PreflightCheck（环境变量/SDK/余额/配额）
2. PromptConfig（域名/用户名/密码/邮箱/API Key/SSH IP 限制）
3. LoadState → 逐个检查并创建缺失资源 → 每步 SaveState
4. WaitForSSH → InstallDocker → RenderTemplates → UploadFiles → StartCompose
5. HealthCheck → PrintSuccess

**PromptConfig 详细项**：
- 域名：自有域名或留空使用 nip.io
- 管理员用户名：默认 admin
- 管理员密码：二次确认
- 管理员邮箱
- AI 模型提供商：OpenAI / Anthropic / 自定义
- API Key / Base URL
- SSH IP 限制：自动检测用户公网 IP（调用 https://api.ipify.org），询问是否限制 SSH 仅允许该 IP

**TDD 测试**：mock 所有依赖，验证调用顺序、幂等性、--force 行为

---

## 步骤 7：status 命令

**新增文件**：
- `internal/deploy/status.go` — 读 state + SSH `docker ps` 解析
- `tests/unit/status_test.go`

---

## 步骤 8：destroy 命令

**新增文件**：
- `internal/deploy/destroy.go` — 确认 → 按序删除 → 清理本地文件
- `tests/unit/destroy_test.go`

**删除顺序**：解绑 EIP → 释放 EIP → 停止 ECS → 删除 ECS → 删除 SSH 密钥对 → 删除安全组 → 删除 VSwitch → 删除 VPC → 删除 state.json

**flags**：`--force`（跳过确认）、`--dry-run`（仅展示）

---

## 步骤 9：goreleaser + GitHub Actions

**新增文件**：
- `.goreleaser.yml` — 按 design-oc.md 5.1.9
- `.github/workflows/release.yml` — tag 触发发布
- `install.sh` — 按 design-oc.md 5.1.8

**验证**：`goreleaser check` + `goreleaser build --snapshot --clean`

---

## 步骤 10：端到端测试 + 文档更新

**新增文件**：
- `tests/e2e/deploy_test.go` — deploy → status → destroy 全流程（需真实阿里云账号）
- `README.md` — 项目说明

**E2E 测试场景**（参考 design-oc.md 8.3）：

| 场景 | 验证方式 |
|------|----------|
| 完整部署 | `cloudcode deploy` → 所有资源创建成功，HTTPS 可访问 |
| 幂等性 | 部署后再次 `cloudcode deploy` → 检测已部署，提示使用 --force |
| 中断恢复 | 部署中途 Ctrl+C → 再次 deploy → 从断点继续 |
| 配置更新 | `cloudcode deploy --force` → 仅更新应用层，不重建云资源 |
| 状态查看 | `cloudcode status` → 正确显示资源和容器状态 |
| 资源销毁 | `cloudcode destroy` → 资源全部释放 |
| dry-run | `cloudcode destroy --dry-run` → 仅显示将删除的资源 |
| HTTPS 访问 | `curl -I https://<domain>` → 返回 302 重定向 |
| 两步认证 | 浏览器登录 → 密码 → Passkey → 进入 OpenCode |

**验证**：`go test ./...` 全部通过

---

## 验证方式

每个步骤完成后：
1. `go test ./...` 全部通过
2. `go build` 编译成功
3. 手动验证对应功能
4. 汇报并等待 Review

端到端验证（步骤 6 完成后）：
- `cloudcode deploy` 创建所有资源，HTTPS 可访问
- `cloudcode status` 显示 3 个容器 running
- `cloudcode destroy` 释放所有资源
