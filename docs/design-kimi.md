# OpenCode 阿里云一键部署方案设计文档

## 1. 项目概述

### 1.1 项目目标

开发一个一键式部署工具，将 [OpenCode](https://opencode.ai) (开源 AI 编程助手) 部署到阿里云海外地域，提供安全、隔离、可公网访问的 Web 端编程环境。

### 1.2 核心功能

| 功能 | 描述 |
|------|------|
| 自动资源创建 | 按需创建 ECS、VPC、安全组、EIP |
| 资源复用 | 检测并复用已有 VM/EIP |
| Docker 沙箱隔离 | OpenCode 运行在 Docker 容器中 |
| 安全访问 | HTTP Basic Auth + 防火墙规则 |
| GitHub 集成 | 内置 gh CLI，支持仓库操作 |
| 一键清理 | 完整释放所有阿里云资源 |

---

## 2. 技术架构

### 2.1 整体架构图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              用户本地环境                                │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    deploy-opencode.sh (主脚本)                   │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │   │
│  │  │  交互式输入  │  │ 阿里云CLI  │  │    Docker/Docker Compose │  │   │
│  │  │  配置管理   │  │  API调用   │  │       容器编排            │  │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ HTTPS/SSH
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                           阿里云海外地域                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                        VPC + 交换机                              │   │
│  │  ┌─────────────────────────────────────────────────────────┐   │   │
│  │  │                    ECS 实例 (2C4G)                         │   │   │
│  │  │  ┌─────────────────────────────────────────────────────┐ │   │   │
│  │  │  │              Docker Engine                           │ │   │   │
│  │  │  │  ┌─────────────────────────────────────────────┐   │ │   │   │
│  │  │  │  │         OpenCode Container                   │   │ │   │   │
│  │  │  │  │  ┌─────────────────────────────────────┐   │   │ │   │   │
│  │  │  │  │  │      OpenCode Web Server             │   │   │ │   │   │
│  │  │  │  │  │      Port: 4096                      │   │   │ │   │   │
│  │  │  │  │  │      Auth: Basic Auth                │   │   │ │   │   │
│  │  │  │  │  └─────────────────────────────────────┘   │   │ │   │   │
│  │  │  │  │  ┌─────────────────────────────────────┐   │   │ │   │   │
│  │  │  │  │  │    Dev Environment (Node/Python/Go)  │   │   │ │   │   │
│  │  │  │  │  │    GitHub CLI (gh)                   │   │   │ │   │   │
│  │  │  │  │  └─────────────────────────────────────┘   │   │ │   │   │
│  │  │  │  └─────────────────────────────────────────────┘   │ │   │   │
│  │  │  └─────────────────────────────────────────────────────┘ │   │   │
│  │  └─────────────────────────────────────────────────────────┘   │   │
│  │                              │                                   │   │
│  │                              │ 绑定                              │   │
│  │                              ▼                                   │   │
│  │                      ┌───────────────┐                          │   │
│  │                      │  弹性公网IP    │                          │   │
│  │                      │  (EIP)        │                          │   │
│  │                      └───────────────┘                          │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

### 2.2 组件说明

| 组件 | 用途 | 技术选型 |
|------|------|----------|
| 部署脚本 | 用户交互、资源编排 | Bash + 阿里云 CLI |
| 阿里云 CLI | 云资源管理 | aliyun-cli |
| Docker | 容器运行时 | Docker CE 27.x |
| Docker Compose | 容器编排 | Compose v2 |
| OpenCode | AI 编程助手 | anomalyco/opencode |
| GitHub CLI | GitHub 集成 | gh |
| Nginx (可选) | 反向代理 + HTTPS | nginx:alpine |

---

## 3. 部署流程设计

### 3.1 主流程图

```
┌─────────────┐
│   开始部署   │
└──────┬──────┘
       │
       ▼
┌─────────────────┐
│ 1. 检查依赖环境  │
│   - aliyun-cli  │
│   - jq          │
│   - ssh         │
└──────┬──────────┘
       │
       ▼
┌─────────────────┐     否    ┌─────────────────┐
│ 2. 配置阿里云   │──────────▶│ 引导用户配置    │
│    认证信息     │           │ AccessKey/Secret│
└──────┬──────────┘           └─────────────────┘
       │ 是
       ▼
┌─────────────────┐
│ 3. 交互式输入   │
│   - 大模型配置  │
│   - 实例规格选择│
│   - 地域选择    │
│   - 访问密码    │
└──────┬──────────┘
       │
       ▼
┌─────────────────┐
│ 4. 检测已有资源 │──────┬────────┬────────┐
│   (VM/EIP)      │      │        │        │
└──────┬──────────┘      │        │        │
       │                 │        │        │
   有现有资源            无       无       无
       │                 │        │        │
       ▼                 ▼        ▼        ▼
┌─────────────┐   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│ 询问复用或  │   │ 创建 VPC    │  │ 创建安全组  │  │ 创建 ECS    │
│ 新建        │   │ 创建交换机  │  │ 配置规则    │  │ 实例        │
└──────┬──────┘   └─────────────┘  └─────────────┘  └──────┬──────┘
       │                                                   │
       └───────────────────────────────────────────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ 5. 创建/绑定 EIP │
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ 6. 安装 Docker  │
                  │    和 Docker    │
                  │    Compose      │
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ 7. 部署 OpenCode│
                  │    容器         │
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ 8. 配置 GitHub  │
                  │    CLI          │
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ 9. 输出访问信息 │
                  │    完成部署     │
                  └─────────────────┘
```

### 3.2 清理流程

```
┌─────────────┐
│ 开始清理    │
└──────┬──────┘
       │
       ▼
┌─────────────────┐
│ 读取部署状态文件 │
│ ~/.opencode-alicloud/state.json
└──────┬──────────┘
       │
       ▼
┌─────────────────┐
│ 确认清理操作    │
│ (二次确认)      │
└──────┬──────────┘
       │
       ▼
┌─────────────────┐
│ 按顺序释放资源: │
│ 1. 解绑/释放EIP │
│ 2. 删除 ECS     │
│ 3. 删除安全组   │
│ 4. 删除交换机   │
│ 5. 删除 VPC     │
└──────┬──────────┘
       │
       ▼
┌─────────────────┐
│ 清理本地状态文件│
└─────────────────┘
```

---

## 4. 详细设计

### 4.1 用户输入配置

#### 4.1.1 阿里云认证

```bash
# 方式1: 环境变量
export ALICLOUD_ACCESS_KEY="your-access-key"
export ALICLOUD_SECRET_KEY="your-secret-key"

# 方式2: 交互式输入
请输入阿里云 AccessKey ID: 
请输入阿里云 AccessKey Secret: 
```

#### 4.1.2 大模型配置

```bash
# OpenCode 支持的模型提供商配置
请选择模型提供商:
  1) OpenAI (GPT-4/GPT-3.5)
  2) Anthropic (Claude)
  3) Google (Gemini)
  4) 自定义 (OpenAI-compatible API)

请输入 Base URL [https://api.openai.com/v1]: 
请输入 API Key: 
请选择模型 [gpt-4o]: 
```

#### 4.1.3 实例规格选择

| 规格 | CPU | 内存 | 按量价格(参考) | 适用场景 |
|------|-----|------|----------------|----------|
| ecs.e-c1m1.large | 2 | 2GB | ~0.08元/小时 | 轻量级试用 |
| **ecs.e-c1m2.large** | **2** | **4GB** | **~0.15元/小时** | **默认推荐** |
| ecs.g7.large | 2 | 8GB | ~0.52元/小时 | 标准开发 |
| ecs.g7.xlarge | 4 | 16GB | ~1.04元/小时 | 大型项目 |

#### 4.1.4 地域选择

| 地域 ID | 位置 | 备注 |
|---------|------|------|
| ap-southeast-1 | 新加坡 | 推荐，延迟低 |
| us-west-1 | 美国(硅谷) | 北美访问 |
| us-east-1 | 美国(弗吉尼亚) | 美东访问 |
| eu-central-1 | 德国(法兰克福) | 欧洲访问 |
| ap-northeast-1 | 日本(东京) | 东亚访问 |
| cn-hongkong | 中国香港 | 国内访问友好 |

### 4.2 安全组规则设计

```yaml
SecurityGroupRules:
  # SSH 访问 (建议限制 IP)
  - Protocol: tcp
    PortRange: 22/22
    SourceCidrIp: 0.0.0.0/0  # 实际部署建议限制用户 IP
    Description: SSH Access
  
  # OpenCode Web 界面
  - Protocol: tcp
    PortRange: 4096/4096
    SourceCidrIp: 0.0.0.0/0
    Description: OpenCode Web UI
  
  # HTTPS (可选，配合 Nginx)
  - Protocol: tcp
    PortRange: 443/443
    SourceCidrIp: 0.0.0.0/0
    Description: HTTPS
```

### 4.3 Docker 配置

#### 4.3.1 Dockerfile

```dockerfile
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV OPENCODE_INSTALL_DIR=/usr/local/bin

# 安装系统依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    git \
    openssh-client \
    sudo \
    nodejs \
    npm \
    python3 \
    python3-pip \
    golang-go \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# 创建非 root 用户
RUN useradd -m -s /bin/bash opencode \
    && echo "opencode ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/opencode \
    && chmod 0440 /etc/sudoers.d/opencode

# 安装 GitHub CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | \
    dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | \
    tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y gh \
    && rm -rf /var/lib/apt/lists/*

USER opencode
WORKDIR /home/opencode

# 安装 OpenCode
RUN curl -fsSL https://opencode.ai/install | bash

# 配置 SSH
RUN mkdir -p /home/opencode/.ssh \
    && touch /home/opencode/.ssh/known_hosts \
    && ssh-keyscan -T 5 github.com 2>/dev/null >> /home/opencode/.ssh/known_hosts || true

# 创建工作目录
RUN mkdir -p /home/opencode/workspace

VOLUME ["/home/opencode/workspace"]
VOLUME ["/home/opencode/.config/opencode"]

EXPOSE 4096

ENTRYPOINT ["opencode", "web", "--hostname", "0.0.0.0", "--port", "4096"]
```

#### 4.3.2 Docker Compose 配置

```yaml
version: '3.8'

services:
  opencode:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: opencode-server
    restart: unless-stopped
    ports:
      - "4096:4096"
    environment:
      - OPENCODE_SERVER_USERNAME=${OPENCODE_USERNAME:-opencode}
      - OPENCODE_SERVER_PASSWORD=${OPENCODE_PASSWORD}
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - OPENAI_BASE_URL=${OPENAI_BASE_URL}
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
      - ANTHROPIC_BASE_URL=${ANTHROPIC_BASE_URL}
    volumes:
      - opencode-workspace:/home/opencode/workspace
      - opencode-config:/home/opencode/.config/opencode
      - /var/run/docker.sock:/var/run/docker.sock  # 允许容器内调用 Docker
    networks:
      - opencode-network

  # 可选: Nginx 反向代理 + HTTPS
  nginx:
    image: nginx:alpine
    container_name: opencode-nginx
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./ssl:/etc/nginx/ssl:ro
    depends_on:
      - opencode
    networks:
      - opencode-network
    profiles:
      - https

volumes:
  opencode-workspace:
    driver: local
  opencode-config:
    driver: local

networks:
  opencode-network:
    driver: bridge
```

### 4.4 脚本模块设计

#### 4.4.1 模块结构

```
deploy-opencode/
├── deploy.sh                 # 主部署脚本
├── destroy.sh                # 清理脚本
├── lib/
│   ├── alicloud.sh           # 阿里云 API 封装
│   ├── docker.sh             # Docker 部署封装
│   ├── utils.sh              # 工具函数
│   └── config.sh             # 配置管理
├── templates/
│   ├── Dockerfile
│   ├── docker-compose.yml
│   └── nginx.conf
└── state/
    └── .gitkeep
```

#### 4.4.2 核心函数设计

```bash
# alicloud.sh
alicloud_auth()           # 阿里云认证
check_existing_resources() # 检测已有资源
create_vpc()              # 创建 VPC
create_vswitch()          # 创建交换机
create_security_group()   # 创建安全组
create_ecs()              # 创建 ECS 实例
allocate_eip()            # 分配 EIP
bind_eip()                # 绑定 EIP
destroy_all_resources()   # 释放所有资源

# docker.sh
install_docker()          # 安装 Docker
build_opencode_image()    # 构建 OpenCode 镜像
deploy_compose()          # 部署 Compose 栈
configure_github_cli()    # 配置 GitHub CLI

# utils.sh
log_info()                # 信息日志
log_error()               # 错误日志
confirm()                 # 交互确认
save_state()              # 保存状态
load_state()              # 读取状态
```

---

## 5. 安全设计

### 5.1 访问安全

| 层级 | 措施 | 说明 |
|------|------|------|
| 网络层 | 安全组 | 仅开放必要端口 (22, 4096) |
| 应用层 | Basic Auth | OPENCODE_SERVER_PASSWORD 保护 |
| 传输层 | SSH Tunnel (推荐) | 本地端口转发访问 |
| 可选 | HTTPS | Nginx + 自签名/Let's Encrypt 证书 |

### 5.2 数据安全

```yaml
安全措施:
  - 容器隔离: OpenCode 运行在独立容器，不污染宿主机
  - 持久化卷: 工作区和配置使用 Docker Volume
  - 密钥管理: API Key 通过环境变量注入，不写入镜像
  - SSH 密钥: 用户自行管理，支持 GitHub CLI 认证流程
```

### 5.3 建议的安全增强

```bash
# 1. 限制 SSH 访问 IP
# 修改安全组规则，只允许特定 IP 访问 22 端口

# 2. 使用 SSH Tunnel 访问 (推荐)
ssh -L 4096:localhost:4096 root@<eip>
# 然后在本地访问 http://localhost:4096

# 3. 配置 HTTPS (使用自签名证书)
# 脚本支持自动生成证书

# 4. 定期更换访问密码
```

---

## 6. 状态管理

### 6.1 状态文件格式

```json
{
  "version": "1.0",
  "created_at": "2026-02-14T10:30:00Z",
  "region": "ap-southeast-1",
  "resources": {
    "vpc": {
      "id": "vpc-xxx",
      "cidr": "192.168.0.0/16"
    },
    "vswitch": {
      "id": "vsw-xxx",
      "zone": "ap-southeast-1a"
    },
    "security_group": {
      "id": "sg-xxx"
    },
    "ecs": {
      "id": "i-xxx",
      "instance_type": "ecs.e-c1m2.large",
      "public_ip": "47.x.x.x",
      "private_ip": "192.168.1.100"
    },
    "eip": {
      "id": "eip-xxx",
      "ip": "47.x.x.x"
    }
  },
  "opencode": {
    "username": "opencode",
    "port": 4096
  }
}
```

### 6.2 状态文件位置

```
~/.opencode-alicloud/
├── state.json          # 部署状态
├── config.json         # 用户配置
└── logs/
    └── deploy-2026-02-14.log
```

---

## 7. 使用指南

### 7.1 快速开始

```bash
# 1. 下载部署脚本
curl -fsSL https://raw.githubusercontent.com/your-repo/opencode-alicloud-deploy/main/deploy.sh -o deploy-opencode.sh
chmod +x deploy-opencode.sh

# 2. 运行部署
./deploy-opencode.sh

# 3. 按提示输入配置
# - 阿里云认证信息
# - 大模型 API 配置
# - 实例规格选择
# - 访问密码设置

# 4. 部署完成后，访问输出地址
# http://<eip>:4096
```

### 7.2 命令行参数

```bash
./deploy-opencode.sh [选项]

选项:
  -h, --help              显示帮助信息
  -y, --yes               自动确认，跳过交互
  -d, --destroy           清理所有资源
  -c, --config FILE       使用配置文件
  -r, --region REGION     指定地域
  -t, --type TYPE         指定实例规格
  --reuse-existing        复用已有资源

配置文件示例 (config.json):
{
  "alicloud": {
    "access_key": "xxx",
    "secret_key": "xxx",
    "region": "ap-southeast-1"
  },
  "instance": {
    "type": "ecs.e-c1m2.large",
    "password": "your-instance-password"
  },
  "opencode": {
    "provider": "openai",
    "base_url": "https://api.openai.com/v1",
    "api_key": "sk-xxx",
    "model": "gpt-4o",
    "server_username": "opencode",
    "server_password": "your-web-password"
  }
}
```

### 7.3 常用操作

```bash
# 通过 SSH 进入 ECS
ssh root@<eip>

# 查看 OpenCode 容器状态
docker ps
docker logs opencode-server

# 重启 OpenCode
docker compose restart opencode

# 进入 OpenCode 容器
docker exec -it opencode-server bash

# 配置 GitHub 认证 (容器内)
gh auth login

# 本地使用 SSH Tunnel
ssh -L 4096:localhost:4096 root@<eip>
```

---

## 8. 成本估算

### 8.1 按量付费 (推荐用于测试)

| 资源 | 规格 | 单价 | 月费用估算 |
|------|------|------|-----------|
| ECS | ecs.e-c1m2.large (2C4G) | ~0.15元/小时 | ~108元 |
| EIP | 1Mbps 带宽 | ~0.02元/小时 | ~14元 |
| 流量 | 按实际使用 | 0.8元/GB | 视使用情况 |
| **总计** | | | **~122元/月** |

### 8.2 节省建议

```
1. 使用抢占式实例: 价格可低至按量付费的 10%
2. 不需要时停止 ECS: 仅保留 EIP 费用
3. 使用共享带宽: 多实例共享带宽降低成本
```

---

## 9. 风险与注意事项

### 9.1 已知限制

| 限制 | 说明 | 解决方案 |
|------|------|----------|
| OpenCode Web Auth | Basic Auth 与 CORS 有兼容问题 | 使用 SSH Tunnel 访问 |
| 首次冷启动 | 容器启动需要拉取镜像 | 预留 2-3 分钟启动时间 |
| GitHub CLI 认证 | 需要交互式浏览器认证 | 使用 SSH 进入容器后执行 |

### 9.2 注意事项

```
1. 阿里云 AccessKey 安全:
   - 建议使用 RAM 子账号，仅授予 ECS/VPC/EIP 权限
   - 不要泄露 AccessKey

2. 实例密码安全:
   - 使用强密码
   - 定期更换密码

3. 网络安全:
   - 建议限制 SSH 访问 IP
   - 生产环境使用 HTTPS

4. 数据备份:
   - 重要代码及时推送到 GitHub
   - 定期备份 Docker Volume
```

---

## 10. 后续优化方向

### 10.1 功能增强

- [ ] 支持 HTTPS 自动配置 (Let's Encrypt)
- [ ] 支持自定义域名 + DNS 自动解析
- [ ] 支持多实例部署 (负载均衡)
- [ ] 支持定时启停 (节省成本)
- [ ] 支持阿里云函数计算部署 (更轻量)

### 10.2 体验优化

- [ ] Web UI 配置向导
- [ ] 部署状态监控
- [ ] 一键升级 OpenCode
- [ ] 多语言支持

---

## 11. 附录

### 11.1 阿里云 RAM 权限策略

```json
{
  "Version": "1",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ecs:*",
        "vpc:*",
        "eip:*"
      ],
      "Resource": "*"
    }
  ]
}
```

### 11.2 参考链接

- [OpenCode 官方文档](https://opencode.ai/docs)
- [阿里云 CLI 文档](https://help.aliyun.com/zh/cli/)
- [Docker 官方文档](https://docs.docker.com/)
- [GitHub CLI 文档](https://cli.github.com/manual/)

---

**文档版本**: 1.0  
**更新日期**: 2026-02-14  
**作者**: AI Assistant
