# CloudCode

一键部署 [OpenCode](https://github.com/anomalyco/opencode) 到阿里云 ECS，带 HTTPS + Authelia 两步认证 + 浏览器 Web Terminal。

## 功能

- 自动创建阿里云 ECS 实例（VPC、安全组、EIP 等）
- Docker Compose 编排：Caddy（HTTPS）+ Authelia（认证）+ Devbox（OpenCode + ttyd）
- 自有域名 + 自动 DNS 更新（阿里云域名自动配置，非阿里云域名提示手动配置）
- 浏览器 Web Terminal（ttyd，通过 /terminal 访问）
- 停机省钱：suspend/resume（StopCharging 模式，停机仅收磁盘费）
- 可选磁盘快照：destroy 时保留快照，下次 deploy 零交互恢复
- 幂等部署：中断后可从断点继续

## 安装

```bash
curl -fsSL https://github.com/hwuu/cloudcode/releases/latest/download/install.sh | bash
```

安装脚本会自动配置 bash/zsh 命令补全。

或从 [Releases](https://github.com/hwuu/cloudcode/releases) 下载对应平台二进制。

## 前置条件

- 阿里云账号，开通 ECS、VPC、EIP 服务
- 获取 AccessKey：登录 [阿里云控制台](https://ram.console.aliyun.com/manage/ak) → AccessKey 管理 → 创建 AccessKey（建议使用 RAM 子账号，授予 ECS/VPC/STS/DNS 权限）

## 使用

### 初始化

```bash
cloudcode init
```

交互式配置阿里云凭证（AccessKey、Region），验证后保存到 `~/.cloudcode/credentials`。

### 部署

```bash
cloudcode deploy
```

交互式收集配置（域名、用户名、密码），然后自动创建云资源并部署应用。

### 重新部署应用层

```bash
cloudcode deploy --app
```

跳过云资源创建和交互配置，仅更新 Caddyfile、docker-compose.yml 等配置并重启容器。

### 停机 / 恢复

```bash
cloudcode suspend   # StopCharging 模式停机，仅收磁盘费 (~$1.2/月)
cloudcode resume    # 恢复运行，容器自动启动
```

### 销毁资源

```bash
cloudcode destroy           # 交互确认，默认保留磁盘快照
cloudcode destroy --force   # 跳过确认
cloudcode destroy --dry-run # 仅展示将删除的资源
```

### 查看状态

```bash
cloudcode status
```

### 运维命令

```bash
cloudcode version                      # 显示版本信息
cloudcode otc                          # 读取 Authelia 一次性验证码（首次注册 Passkey 用）
cloudcode logs                         # 查看所有容器日志（默认最后 50 行）
cloudcode logs authelia                # 查看指定容器日志
cloudcode logs -n 100                  # 显示最后 100 行
cloudcode logs -f                      # 实时跟踪日志
cloudcode ssh                          # SSH 登录到 ECS 宿主机
cloudcode ssh devbox                   # 进入 devbox 容器
cloudcode ssh authelia                 # 进入 authelia 容器
cloudcode ssh caddy                    # 进入 caddy 容器
cloudcode exec devbox opencode -v      # 在容器内执行命令
```

## 架构

```
浏览器 → EIP → ECS 实例
                ├── Caddy (HTTPS 反向代理)
                ├── Authelia (两步认证)
                └── Devbox
                    ├── OpenCode (AI 编程助手)
                    └── ttyd (Web Terminal)
```

## 月费用

| 状态 | 月费用 | 说明 |
|------|--------|------|
| running | ~$23 | ECS + EIP |
| suspended | ~$1.2 | 仅磁盘费用 |
| destroyed（保留快照） | ~$0.40 | 仅快照存储 |

## 开发

### 本地构建

```bash
# 构建（版本 tag 自动取当前分支名）
make build

# 指定版本构建
make VERSION=0.2.0 build

# 安装到系统
sudo cp bin/cloudcode /usr/local/bin/
```

### 测试

```bash
# 单元测试
make test

# 或直接
go test ./... -count=1
```

### Docker 镜像

分支构建时 Docker 镜像 tag 为分支名（`/` 替换为 `-`），如 `ghcr.io/hwuu/cloudcode-devbox:hwuu-v0.2.0-dev`。

release 时从 git tag 取版本号，如 `ghcr.io/hwuu/cloudcode-devbox:0.2.0`。

手动触发 Docker 构建（修改 Dockerfile 或 docker.yml 后自动触发）：

```bash
# 本地构建 devbox 镜像（测试用）
docker build -f internal/template/templates/Dockerfile.devbox -t cloudcode-devbox:local .
```

## License

MIT
