# CloudCode 故障排除指南

## 部署阶段

### Docker Compose 启动超时

**症状：**
```
Error: 启动 Docker Compose 失败: context deadline exceeded
```

**原因：**
首次部署时需要从 ghcr.io 拉取 OpenCode 镜像（约 300MB），网络较慢时可能超时。

**解决方案：**
1. SSH 到 ECS 手动运行，查看详细进度：
   ```bash
   ssh -i ~/.cloudcode/ssh_key root@<EIP>
   cd ~/cloudcode && docker compose pull && docker compose up -d
   ```

2. 构建完成后检查容器状态：
   ```bash
   docker compose ps
   ```

3. 如果容器正常运行，说明部署成功，CLI 只是提前退出。

---

## HTTPS / 证书问题

### nip.io 证书签发失败

**症状：**
浏览器访问 `https://<EIP>.nip.io` 报错 `ERR_SSL_PROTOCOL_ERROR`

**Caddy 日志：**
```json
{"level":"error","logger":"tls.obtain","msg":"will retry","error":"... too many certificates (100000) already issued for \"nip.io\" in the last 168h0m0s ..."}
```

**原因：**
nip.io 是公共服务，所有用户共享 Let's Encrypt 的速率限制（每个注册域名每周 100 张证书）。当 nip.io 达到限制时，无法为新部署签发证书。

**解决方案一：使用自有域名（推荐）**

如果你有域名：
1. 配置 DNS A 记录指向 EIP（需要配置两条：`example.com` 和 `auth.example.com`）
2. 重新部署：
   ```bash
   cloudcode deploy --force
   # 输入你的自有域名
   ```

**解决方案二：使用自签名证书**

修改 Caddyfile 使用内部证书：

```bash
# SSH 到 ECS
ssh -i ~/.cloudcode/ssh_key root@<EIP>
cd ~/cloudcode

# 编辑 Caddyfile，在每个域名块中添加 "tls internal"
# 示例：
#   auth.<EIP>.nip.io {
#       tls internal
#       ...
#   }
#   <EIP>.nip.io {
#       tls internal
#       ...
#   }

# 重启 Caddy
docker compose restart caddy
```

浏览器会显示安全警告，点击"继续访问"即可。

**解决方案三：等待速率限制重置**

根据错误日志中的 `retry after` 时间等待，通常为 7 天。

---

## 认证问题

### Authelia 架构说明

CloudCode 使用子域名架构部署 Authelia：
- `auth.<domain>` — Authelia 登录页面
- `<domain>` — OpenCode 服务（受 forward_auth 保护）

访问流程：
1. 访问 `https://<domain>`
2. 未登录时重定向到 `https://auth.<domain>`
3. 登录成功后返回 `https://<domain>`

### Authelia 登录失败

**检查步骤：**
```bash
# 查看 Authelia 日志
docker logs authelia

# 检查用户配置
cat ~/cloudcode/authelia/users_database.yml
```

### 忘记密码

**解决方案：**
重新部署应用层会重新生成密码哈希：
```bash
cloudcode deploy --force
```

---

## SSH 连接问题

### SSH 连接被拒绝

**症状：**
```
SSH 连接失败: connection refused
```

**可能原因：**
1. ECS 实例尚未完全启动
2. 安全组未开放 22 端口
3. SSH IP 限制但你的 IP 已变化

**解决方案：**
1. 等待 2-3 分钟后重试
2. 检查阿里云控制台安全组规则
3. 如果 IP 变化，通过阿里云控制台修改安全组规则

### SSH 密钥权限错误

**症状：**
```
Warning: Identity file ... not accessible: No such file or directory
```

**解决方案：**
```bash
# 检查私钥文件是否存在
ls -la ~/.cloudcode/ssh_key

# 确保权限正确
chmod 600 ~/.cloudcode/ssh_key
```

---

## 容器问题

### 容器启动失败

**检查步骤：**
```bash
# 查看所有容器状态
docker compose ps

# 查看容器日志
docker logs caddy
docker logs authelia
docker logs opencode
```

### OpenCode 容器反复重启

**可能原因：**
- 环境变量配置错误
- API Key 无效

**解决方案：**
```bash
# 检查 .env 文件
cat ~/cloudcode/.env

# 查看详细日志
docker logs opencode
```

---

## 资源清理

### 删除残留资源

如果 `cloudcode destroy` 失败，手动清理：

```bash
# 通过阿里云控制台或 CLI 逐个删除：
# 1. 解绑并释放 EIP
# 2. 删除 ECS 实例
# 3. 删除安全组
# 4. 删除 VSwitch
# 5. 删除 VPC
```

### 删除本地状态

```bash
rm -rf ~/.cloudcode/
```
