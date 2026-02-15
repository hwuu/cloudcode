package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hwuu/cloudcode/internal/alicloud"
	"github.com/hwuu/cloudcode/internal/config"
	"github.com/hwuu/cloudcode/internal/remote"
	tmpl "github.com/hwuu/cloudcode/internal/template"
)

// DeployConfig 保存交互收集的部署配置
type DeployConfig struct {
	Domain          string
	Username        string
	Password        string
	Email           string
	OpenAIAPIKey    string
	OpenAIBaseURL   string
	AnthropicAPIKey string
	SSHIP           string // SSH IP 限制，空表示不限制
}

// SSHDialFactory 创建 SSH DialFunc 的工厂函数
type SSHDialFactory func(host string, port int, user string, privateKey []byte) remote.DialFunc

// SFTPFactory 创建 SFTP 客户端的工厂函数
type SFTPClientFactory func(host string, port int, user string, privateKey []byte) (remote.SFTPClient, error)

// GetPublicIPFunc 获取用户公网 IP 的函数
type GetPublicIPFunc func() (string, error)

// Deployer 部署编排器，通过依赖注入支持测试
type Deployer struct {
	ECS              alicloud.ECSAPI
	VPC              alicloud.VPCAPI
	STS              alicloud.STSAPI
	Prompter         *config.Prompter
	Output           io.Writer
	Region           string
	StateDir         string // 覆盖默认 state 目录（测试用）
	SSHDialFunc      SSHDialFactory
	SFTPFactory      SFTPClientFactory
	GetPublicIP      GetPublicIPFunc
	WaitInterval     time.Duration // ECS 等待轮询间隔（测试用，默认 5s）
	WaitTimeout      time.Duration // ECS 等待超时（测试用，默认 5min）
}

func (d *Deployer) printf(format string, args ...interface{}) {
	fmt.Fprintf(d.Output, format, args...)
}

// PreflightCheck 前置检查：验证阿里云凭证
func (d *Deployer) PreflightCheck(ctx context.Context) error {
	d.printf("[1/6] 检查环境...\n")

	identity, err := alicloud.GetCallerIdentity(d.STS)
	if err != nil {
		return fmt.Errorf("阿里云凭证验证失败: %w", err)
	}

	d.printf("  ✓ 阿里云账号: %s (UID: %s)\n", identity.AccountID, identity.UserID)
	return nil
}

// PromptConfig 交互式收集部署配置
func (d *Deployer) PromptConfig(ctx context.Context) (*DeployConfig, error) {
	d.printf("\n[2/6] 配置访问信息:\n")

	cfg := &DeployConfig{}

	// 域名
	domain, err := d.Prompter.Prompt("请输入域名 (推荐使用自有域名，留空使用 nip.io): ")
	if err != nil {
		return nil, err
	}
	cfg.Domain = domain // 留空，后续 EIP 分配后填充 nip.io

	// 用户名
	username, err := d.Prompter.PromptWithDefault("请输入管理员用户名", "admin")
	if err != nil {
		return nil, err
	}
	cfg.Username = username

	// 密码
	password, err := d.Prompter.PromptPassword("请输入管理员密码: ")
	if err != nil {
		return nil, err
	}
	confirmPassword, err := d.Prompter.PromptPassword("请确认管理员密码: ")
	if err != nil {
		return nil, err
	}
	if password != confirmPassword {
		return nil, fmt.Errorf("两次输入的密码不一致")
	}
	cfg.Password = password

	// 邮箱
	email, err := d.Prompter.Prompt("请输入管理员邮箱: ")
	if err != nil {
		return nil, err
	}
	cfg.Email = email

	// AI 模型提供商
	d.printf("\n[3/6] 配置 OpenCode:\n")
	providerIdx, err := d.Prompter.PromptSelect("请选择 AI 模型提供商:", []string{"OpenAI", "Anthropic", "自定义"})
	if err != nil {
		return nil, err
	}

	switch providerIdx {
	case 0: // OpenAI
		apiKey, err := d.Prompter.PromptPassword("请输入 OpenAI API Key: ")
		if err != nil {
			return nil, err
		}
		cfg.OpenAIAPIKey = apiKey
		baseURL, err := d.Prompter.PromptWithDefault("请输入 Base URL", "https://api.openai.com/v1")
		if err != nil {
			return nil, err
		}
		if baseURL != "https://api.openai.com/v1" {
			cfg.OpenAIBaseURL = baseURL
		}
	case 1: // Anthropic
		apiKey, err := d.Prompter.PromptPassword("请输入 Anthropic API Key: ")
		if err != nil {
			return nil, err
		}
		cfg.AnthropicAPIKey = apiKey
	case 2: // 自定义
		apiKey, err := d.Prompter.PromptPassword("请输入 API Key: ")
		if err != nil {
			return nil, err
		}
		cfg.OpenAIAPIKey = apiKey
		baseURL, err := d.Prompter.Prompt("请输入 Base URL: ")
		if err != nil {
			return nil, err
		}
		cfg.OpenAIBaseURL = baseURL
	}

	// SSH IP 限制
	publicIP, err := d.GetPublicIP()
	if err == nil && publicIP != "" {
		d.printf("\n检测到您的公网 IP: %s\n", publicIP)
		restrict, err := d.Prompter.PromptConfirm("是否限制 SSH 仅允许该 IP 访问?")
		if err != nil {
			return nil, err
		}
		if restrict {
			cfg.SSHIP = publicIP + "/32"
		}
	}

	return cfg, nil
}

// CreateResources 创建云资源（幂等：跳过已存在的资源）
func (d *Deployer) CreateResources(ctx context.Context, state *config.State, sshIP string) error {
	d.printf("\n[4/6] 创建云资源:\n")

	// VPC
	if !state.HasVPC() {
		vpc, err := alicloud.CreateVPC(d.VPC, d.Region, "cloudcode-vpc")
		if err != nil {
			return err
		}
		state.Resources.VPC = config.VPCResource{ID: vpc.ID, CIDR: vpc.CIDR}
		if err := d.saveState(state); err != nil {
			return err
		}
		d.printf("  ✓ 创建 VPC (%s)\n", vpc.ID)
	} else {
		d.printf("  ✓ VPC 已存在 (%s)\n", state.Resources.VPC.ID)
	}

	// 可用区选择
	zoneID := state.Resources.VSwitch.ZoneID
	if zoneID == "" {
		var err error
		zoneID, err = alicloud.SelectAvailableZone(d.ECS, d.Region, alicloud.DefaultInstanceType, alicloud.DefaultZonePriority)
		if err != nil {
			return err
		}
	}

	// VSwitch
	if !state.HasVSwitch() {
		vswitch, err := alicloud.CreateVSwitch(d.VPC, state.Resources.VPC.ID, zoneID, "192.168.1.0/24", "cloudcode-vswitch")
		if err != nil {
			return err
		}
		state.Resources.VSwitch = config.VSwitchResource{ID: vswitch.ID, ZoneID: vswitch.ZoneID, CIDR: vswitch.CIDR}
		if err := d.saveState(state); err != nil {
			return err
		}
		d.printf("  ✓ 创建交换机 (%s)\n", vswitch.ID)
	} else {
		d.printf("  ✓ 交换机已存在 (%s)\n", state.Resources.VSwitch.ID)
	}

	// 安全组
	if !state.HasSecurityGroup() {
		sg, err := alicloud.CreateSecurityGroup(d.ECS, state.Resources.VPC.ID, d.Region, "cloudcode-sg")
		if err != nil {
			return err
		}
		rules := alicloud.DefaultSecurityGroupRules(sshIP)
		if err := alicloud.AuthorizeSecurityGroupIngress(d.ECS, sg.ID, d.Region, rules); err != nil {
			return err
		}
		state.Resources.SecurityGroup = config.SecurityGroupResource{ID: sg.ID}
		if err := d.saveState(state); err != nil {
			return err
		}
		if sshIP != "" {
			d.printf("  ✓ 创建安全组 (%s) - 开放 80/443, SSH 限制 %s\n", sg.ID, sshIP)
		} else {
			d.printf("  ✓ 创建安全组 (%s) - 开放 22/80/443\n", sg.ID)
		}
	} else {
		d.printf("  ✓ 安全组已存在 (%s)\n", state.Resources.SecurityGroup.ID)
	}

	// SSH 密钥对
	if !state.HasSSHKeyPair() {
		keyPair, err := alicloud.CreateSSHKeyPair(d.ECS, alicloud.DefaultSSHKeyName)
		if err != nil {
			return err
		}
		// 保存私钥到本地
		keyPath := filepath.Join(d.getStateDir(), "ssh_key")
		if err := os.WriteFile(keyPath, []byte(keyPair.PrivateKey), 0600); err != nil {
			return fmt.Errorf("保存 SSH 私钥失败: %w", err)
		}
		state.Resources.SSHKeyPair = config.SSHKeyPairResource{
			Name:           keyPair.Name,
			PrivateKeyPath: ".cloudcode/ssh_key",
		}
		if err := d.saveState(state); err != nil {
			return err
		}
		d.printf("  ✓ 创建 SSH 密钥对 (%s)\n", keyPair.Name)
	} else {
		d.printf("  ✓ SSH 密钥对已存在 (%s)\n", state.Resources.SSHKeyPair.Name)
	}

	// ECS 实例
	if !state.HasECS() {
		ecs, err := alicloud.CreateECSInstance(
			d.ECS, d.Region, zoneID,
			alicloud.DefaultInstanceType, alicloud.DefaultImageID,
			state.Resources.SecurityGroup.ID, state.Resources.VSwitch.ID,
			state.Resources.SSHKeyPair.Name, "cloudcode-ecs",
		)
		if err != nil {
			return err
		}
		state.Resources.ECS = config.ECSResource{
			ID:             ecs.ID,
			InstanceType:   ecs.InstanceType,
			SystemDiskSize: alicloud.DefaultSystemDiskSize,
		}
		if err := d.saveState(state); err != nil {
			return err
		}
		d.printf("  ✓ 创建 ECS 实例 (%s)\n", ecs.ID)

		// 启动实例
		if err := alicloud.StartECSInstance(d.ECS, ecs.ID); err != nil {
			return fmt.Errorf("启动 ECS 实例失败: %w", err)
		}

		// 等待 Running
		ecsInfo, err := alicloud.WaitForInstanceRunning(ctx, d.ECS, ecs.ID, d.Region, d.WaitInterval, d.WaitTimeout)
		if err != nil {
			return err
		}
		state.Resources.ECS.PrivateIP = ecsInfo.PrivateIP
		if err := d.saveState(state); err != nil {
			return err
		}
		d.printf("  ✓ ECS 实例已运行\n")
	} else {
		d.printf("  ✓ ECS 实例已存在 (%s)\n", state.Resources.ECS.ID)
	}

	// EIP
	if !state.HasEIP() {
		eip, err := alicloud.AllocateEIP(d.VPC, d.Region, "cloudcode-eip")
		if err != nil {
			return err
		}
		// 绑定 EIP 到 ECS
		if err := alicloud.AssociateEIPToInstance(d.VPC, eip.ID, state.Resources.ECS.ID, d.Region); err != nil {
			return fmt.Errorf("绑定 EIP 失败: %w", err)
		}
		state.Resources.EIP = config.EIPResource{ID: eip.ID, IP: eip.IP}
		state.Resources.ECS.PublicIP = eip.IP
		if err := d.saveState(state); err != nil {
			return err
		}
		d.printf("  ✓ 分配 EIP (%s) - IP: %s\n", eip.ID, eip.IP)
	} else {
		d.printf("  ✓ EIP 已存在 (%s) - IP: %s\n", state.Resources.EIP.ID, state.Resources.EIP.IP)
	}

	return nil
}

// DeployApp 部署应用（SSH → Docker → 模板 → 上传 → compose up）
func (d *Deployer) DeployApp(ctx context.Context, state *config.State, cfg *DeployConfig) error {
	d.printf("\n[5/6] 部署应用:\n")

	// 读取 SSH 私钥
	privateKey, err := d.readSSHKey(state)
	if err != nil {
		return err
	}

	eipIP := state.Resources.EIP.IP

	// 等待 SSH 就绪
	dialFunc := d.SSHDialFunc(eipIP, 22, "root", privateKey)
	sshClient, err := remote.WaitForSSH(ctx, dialFunc, remote.WaitSSHOptions{})
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer sshClient.Close()
	d.printf("  ✓ SSH 连接成功\n")

	// 安装 Docker
	dockerCmd := "which docker > /dev/null 2>&1 || (curl -fsSL https://get.docker.com | sh -s -- --mirror Aliyun && systemctl enable docker && systemctl start docker)"
	cmdCtx, cancel := context.WithTimeout(ctx, remote.DockerInstallTimeout)
	defer cancel()
	if _, err := sshClient.RunCommand(cmdCtx, dockerCmd); err != nil {
		return fmt.Errorf("安装 Docker 失败: %w", err)
	}
	d.printf("  ✓ Docker 已就绪\n")

	// 创建远程目录
	mkdirCmd := "mkdir -p ~/cloudcode/authelia"
	if _, err := sshClient.RunCommand(ctx, mkdirCmd); err != nil {
		return fmt.Errorf("创建远程目录失败: %w", err)
	}

	// 确定域名
	domain := cfg.Domain
	if domain == "" {
		domain = eipIP + ".nip.io"
	}

	// 哈希密码
	hashedPassword, err := config.HashPassword(cfg.Password)
	if err != nil {
		return fmt.Errorf("密码哈希失败: %w", err)
	}

	// 生成 secrets
	sessionSecret, err := config.GenerateSecret()
	if err != nil {
		return err
	}
	storageKey, err := config.GenerateSecret()
	if err != nil {
		return err
	}

	// 渲染模板
	templateData := &tmpl.TemplateData{
		Domain:               domain,
		Username:             cfg.Username,
		HashedPassword:       hashedPassword,
		Email:                cfg.Email,
		SessionSecret:        sessionSecret,
		StorageEncryptionKey: storageKey,
		OpenAIAPIKey:         cfg.OpenAIAPIKey,
		OpenAIBaseURL:        cfg.OpenAIBaseURL,
		AnthropicAPIKey:      cfg.AnthropicAPIKey,
	}

	files, err := tmpl.RenderAll(templateData)
	if err != nil {
		return fmt.Errorf("模板渲染失败: %w", err)
	}
	d.printf("  ✓ 配置文件已渲染\n")

	// 上传文件（将 ~/cloudcode 替换为绝对路径）
	uploadFiles := make(map[string][]byte)
	for path, content := range files {
		remotePath := strings.Replace(path, "~/cloudcode", "/root/cloudcode", 1)
		uploadFiles[remotePath] = content
	}

	sftpClient, err := d.SFTPFactory(eipIP, 22, "root", privateKey)
	if err != nil {
		return fmt.Errorf("SFTP 连接失败: %w", err)
	}
	defer sftpClient.Close()

	if err := remote.UploadFiles(sftpClient, uploadFiles); err != nil {
		return fmt.Errorf("上传文件失败: %w", err)
	}
	d.printf("  ✓ 配置文件已上传\n")

	// docker compose up
	composeCmd := "cd ~/cloudcode && docker compose up -d --build"
	composeCtx, composeCancel := context.WithTimeout(ctx, remote.DockerInstallTimeout)
	defer composeCancel()
	if _, err := sshClient.RunCommand(composeCtx, composeCmd); err != nil {
		return fmt.Errorf("启动 Docker Compose 失败: %w", err)
	}
	d.printf("  ✓ Docker Compose 已启动\n")

	// 更新 state 中的域名
	state.CloudCode.Domain = domain

	return nil
}

// HealthCheck 健康检查：通过 SSH 检查容器状态
func (d *Deployer) HealthCheck(ctx context.Context, state *config.State) error {
	d.printf("\n[6/6] 验证服务:\n")

	privateKey, err := d.readSSHKey(state)
	if err != nil {
		return err
	}

	dialFunc := d.SSHDialFunc(state.Resources.EIP.IP, 22, "root", privateKey)
	sshClient, err := remote.WaitForSSH(ctx, dialFunc, remote.WaitSSHOptions{
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer sshClient.Close()

	output, err := sshClient.RunCommand(ctx, "cd ~/cloudcode && docker compose ps --format '{{.Name}} {{.State}}'")
	if err != nil {
		return fmt.Errorf("检查容器状态失败: %w", err)
	}

	d.printf("  容器状态:\n")
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line != "" {
			d.printf("    %s\n", line)
		}
	}

	return nil
}

// Run 执行完整部署流程
func (d *Deployer) Run(ctx context.Context, force bool) error {
	// 阶段 1: 前置检查
	if err := d.PreflightCheck(ctx); err != nil {
		return err
	}

	// 加载或创建 state
	state, err := d.loadState()
	if err != nil {
		state = config.NewState(d.Region, alicloud.DefaultImageID)
	}

	if state.IsComplete() && !force {
		d.printf("\n已检测到完整部署，使用 --force 重新部署应用层\n")
		return nil
	}

	// 阶段 2: 交互配置（始终收集，--force 也需要）
	cfg, err := d.PromptConfig(ctx)
	if err != nil {
		return err
	}

	// 阶段 3: 创建云资源（--force 跳过）
	if !state.IsComplete() {
		if err := d.CreateResources(ctx, state, cfg.SSHIP); err != nil {
			return err
		}
	}

	// 填充 nip.io 域名
	if cfg.Domain == "" {
		cfg.Domain = state.Resources.EIP.IP + ".nip.io"
	}

	// 更新 state
	state.CloudCode.Username = cfg.Username
	state.CloudCode.Domain = cfg.Domain

	// 阶段 4: 部署应用
	if err := d.DeployApp(ctx, state, cfg); err != nil {
		return err
	}

	// 保存最终 state
	if err := d.saveState(state); err != nil {
		return err
	}

	// 阶段 5: 健康检查
	if err := d.HealthCheck(ctx, state); err != nil {
		// 健康检查失败不阻塞，仅警告
		d.printf("  ⚠ 健康检查失败: %v\n", err)
	}

	// 输出成功信息
	d.printSuccess(state, cfg)

	return nil
}

func (d *Deployer) printSuccess(state *config.State, cfg *DeployConfig) {
	d.printf("\n─────────────────────────────────────────────────────────────\n")
	d.printf("✅ 部署完成！\n\n")
	d.printf("访问地址: https://%s\n", state.CloudCode.Domain)
	d.printf("用户名: %s\n", state.CloudCode.Username)
	d.printf("密码: <你设置的密码>\n\n")
	d.printf("认证流程: 用户名 + 密码 → Passkey 验证\n")
	d.printf("首次登录后建议注册 Passkey 作为第二因素！\n\n")
	d.printf("提示:\n")
	d.printf("  - 查看状态: cloudcode status\n")
	d.printf("  - 清理资源: cloudcode destroy\n")
	d.printf("  - SSH 访问: ssh -i ~/.cloudcode/ssh_key root@%s\n", state.Resources.EIP.IP)
	d.printf("─────────────────────────────────────────────────────────────\n")
}

// --- 内部辅助方法 ---

func (d *Deployer) getStateDir() string {
	if d.StateDir != "" {
		return d.StateDir
	}
	dir, _ := config.GetStateDir()
	return dir
}

func (d *Deployer) loadState() (*config.State, error) {
	if d.StateDir != "" {
		return loadStateFrom(d.StateDir)
	}
	return config.LoadState()
}

func (d *Deployer) saveState(state *config.State) error {
	if d.StateDir != "" {
		return saveStateTo(d.StateDir, state)
	}
	return config.SaveState(state)
}

func (d *Deployer) readSSHKey(state *config.State) ([]byte, error) {
	keyPath := filepath.Join(d.getStateDir(), "ssh_key")
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("读取 SSH 私钥失败: %w", err)
	}
	return data, nil
}

func loadStateFrom(dir string) (*config.State, error) {
	path := filepath.Join(dir, config.StateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, config.ErrStateNotFound
		}
		return nil, err
	}
	var state config.State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func saveStateTo(dir string, state *config.State) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, config.StateFileName)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
