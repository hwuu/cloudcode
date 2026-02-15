# CloudCode

一键部署 [OpenCode](https://github.com/opencode-ai/opencode) 到阿里云 ECS，带 HTTPS + Authelia 两步认证。

## 功能

- 自动创建阿里云 ECS 实例（VPC、安全组、EIP 等）
- Docker Compose 编排：Caddy（HTTPS）+ Authelia（认证）+ OpenCode
- 幂等部署：中断后可从断点继续
- SSH IP 限制：可选仅允许指定 IP 访问 SSH
- 一键销毁所有云资源

## 安装

```bash
curl -fsSL https://github.com/hwuu/cloudcode/releases/latest/download/install.sh | bash
```

或从 [Releases](https://github.com/hwuu/cloudcode/releases) 下载对应平台二进制。

## 前置条件

- 阿里云账号，开通 ECS、VPC、EIP 服务
- 获取 AccessKey：登录 [阿里云控制台](https://ram.console.aliyun.com/manage/ak) → AccessKey 管理 → 创建 AccessKey（建议使用 RAM 子账号，授予 ECS/VPC/STS 权限）
- 设置环境变量：

```bash
export ALICLOUD_ACCESS_KEY_ID="your-access-key-id"
export ALICLOUD_ACCESS_KEY_SECRET="your-access-key-secret"
export ALICLOUD_REGION="ap-southeast-1"  # 可选，默认新加坡
```

### 可选区域

| Region ID | 位置 |
|-----------|------|
| `ap-southeast-1` | 新加坡（默认） |
| `ap-southeast-5` | 雅加达 |
| `ap-northeast-1` | 东京 |
| `ap-northeast-2` | 首尔 |
| `us-west-1` | 硅谷 |
| `us-east-1` | 弗吉尼亚 |
| `eu-central-1` | 法兰克福 |
| `eu-west-1` | 伦敦 |
| `cn-hongkong` | 中国香港 |
| `cn-hangzhou` | 杭州 |
| `cn-shanghai` | 上海 |
| `cn-beijing` | 北京 |
| `cn-shenzhen` | 深圳 |

完整列表见 [阿里云地域和可用区](https://help.aliyun.com/document_detail/40654.html)。

## 使用

### 部署

```bash
cloudcode deploy
```

交互式收集配置（域名、用户名、密码、AI API Key 等），然后自动创建云资源并部署应用。

### 重新部署应用层

```bash
cloudcode deploy --force
```

跳过云资源创建，仅更新应用配置和容器。

### 查看状态

```bash
cloudcode status
```

### 销毁资源

```bash
cloudcode destroy           # 交互确认后删除
cloudcode destroy --force   # 跳过确认
cloudcode destroy --dry-run # 仅展示将删除的资源
```

## 架构

```
浏览器 → EIP → ECS 实例
                ├── Caddy (HTTPS 反向代理)
                ├── Authelia (两步认证)
                └── OpenCode (AI 编程助手)
```

## 开发

```bash
# 运行测试
go test ./... -count=1

# E2E 测试（需要真实阿里云账号）
go test ./tests/e2e/ -tags e2e -v -timeout 30m

# 构建
go build -o cloudcode ./cmd/cloudcode
```

## License

MIT
