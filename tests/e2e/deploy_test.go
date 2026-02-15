//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hwuu/cloudcode/internal/alicloud"
	"github.com/hwuu/cloudcode/internal/config"
	"github.com/hwuu/cloudcode/internal/deploy"
	"github.com/hwuu/cloudcode/internal/remote"
)

// E2E 测试需要真实阿里云账号，通过 build tag 隔离：
//   go test ./tests/e2e/ -tags e2e -v -timeout 30m
//
// 环境变量：
//   ALICLOUD_ACCESS_KEY_ID
//   ALICLOUD_ACCESS_KEY_SECRET
//   ALICLOUD_REGION (可选，默认 ap-southeast-1)
//   E2E_DOMAIN (可选，留空使用 nip.io)
//   E2E_OPENAI_API_KEY (必须)

func skipIfNoCredentials(t *testing.T) {
	t.Helper()
	if os.Getenv("ALICLOUD_ACCESS_KEY_ID") == "" || os.Getenv("ALICLOUD_ACCESS_KEY_SECRET") == "" {
		t.Skip("跳过 E2E 测试：未设置 ALICLOUD_ACCESS_KEY_ID / ALICLOUD_ACCESS_KEY_SECRET")
	}
	if os.Getenv("E2E_OPENAI_API_KEY") == "" {
		t.Skip("跳过 E2E 测试：未设置 E2E_OPENAI_API_KEY")
	}
}

func newE2EDeployer(t *testing.T, stateDir string) (*deploy.Deployer, *alicloud.Config) {
	t.Helper()

	cfg, err := alicloud.LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("加载阿里云配置失败: %v", err)
	}

	clients, err := alicloud.NewClients(cfg)
	if err != nil {
		t.Fatalf("初始化阿里云 SDK 失败: %v", err)
	}

	output := &bytes.Buffer{}
	prompter := config.NewPrompter(strings.NewReader(""), output)

	d := &deploy.Deployer{
		ECS:      clients.ECS,
		VPC:      clients.VPC,
		STS:      clients.STS,
		Prompter: prompter,
		Output:   output,
		Region:   cfg.RegionID,
		StateDir: stateDir,
		SSHDialFunc: func(host string, port int, user string, privateKey []byte) remote.DialFunc {
			return remote.NewSSHDialFunc(host, port, user, privateKey)
		},
		SFTPFactory: remote.NewSFTPClient,
		GetPublicIP: remote.GetPublicIP,
	}

	return d, cfg
}

func newE2EDestroyer(t *testing.T, stateDir string, cfg *alicloud.Config) *deploy.Destroyer {
	t.Helper()

	clients, err := alicloud.NewClients(cfg)
	if err != nil {
		t.Fatalf("初始化阿里云 SDK 失败: %v", err)
	}

	output := &bytes.Buffer{}
	return &deploy.Destroyer{
		ECS:      clients.ECS,
		VPC:      clients.VPC,
		Prompter: config.NewPrompter(strings.NewReader(""), output),
		Output:   output,
		StateDir: stateDir,
		Region:   cfg.RegionID,
	}
}

// TestE2E_FullLifecycle 完整生命周期：deploy → status → destroy
func TestE2E_FullLifecycle(t *testing.T) {
	skipIfNoCredentials(t)

	stateDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	deployer, cfg := newE2EDeployer(t, stateDir)

	// --- 阶段 1: PreflightCheck ---
	t.Log("阶段 1: PreflightCheck")
	if err := deployer.PreflightCheck(ctx); err != nil {
		t.Fatalf("PreflightCheck 失败: %v", err)
	}

	// --- 阶段 2: CreateResources ---
	t.Log("阶段 2: CreateResources")
	state := config.NewState(cfg.RegionID, alicloud.DefaultImageID)
	state.CloudCode = config.CloudCodeConfig{
		Username: "e2e-admin",
		Domain:   os.Getenv("E2E_DOMAIN"),
	}

	if err := deployer.CreateResources(ctx, state, ""); err != nil {
		// 清理已创建的资源
		t.Logf("CreateResources 失败，尝试清理: %v", err)
		destroyer := newE2EDestroyer(t, stateDir, cfg)
		_ = destroyer.Run(context.Background(), true, false)
		t.Fatalf("CreateResources 失败: %v", err)
	}

	// 确保测试结束后清理资源
	defer func() {
		t.Log("清理: destroy")
		destroyer := newE2EDestroyer(t, stateDir, cfg)
		if err := destroyer.Run(context.Background(), true, false); err != nil {
			t.Logf("destroy 失败（需手动清理）: %v", err)
		}
	}()

	if !state.IsComplete() {
		t.Fatal("CreateResources 后 state 应该是 complete")
	}
	t.Logf("资源创建完成: ECS=%s, EIP=%s", state.Resources.ECS.ID, state.Resources.EIP.IP)

	// --- 阶段 3: DeployApp ---
	t.Log("阶段 3: DeployApp")
	domain := os.Getenv("E2E_DOMAIN")
	if domain == "" {
		domain = state.Resources.EIP.IP + ".nip.io"
	}

	deployConfig := &deploy.DeployConfig{
		Domain:       domain,
		Username:     "e2e-admin",
		Password:     "E2eTestPass123!",
		Email:        "e2e@example.com",
		OpenAIAPIKey: os.Getenv("E2E_OPENAI_API_KEY"),
	}

	if err := deployer.DeployApp(ctx, state, deployConfig); err != nil {
		t.Fatalf("DeployApp 失败: %v", err)
	}

	// --- 阶段 4: HealthCheck ---
	t.Log("阶段 4: HealthCheck")
	if err := deployer.HealthCheck(ctx, state); err != nil {
		t.Logf("HealthCheck 失败（非致命）: %v", err)
	}

	// --- 阶段 5: Status ---
	t.Log("阶段 5: Status")
	statusOutput := &bytes.Buffer{}
	statusRunner := &deploy.StatusRunner{
		Output:   statusOutput,
		StateDir: stateDir,
		SSHDialFunc: func(host string, port int, user string, privateKey []byte) remote.DialFunc {
			return remote.NewSSHDialFunc(host, port, user, privateKey)
		},
	}
	if err := statusRunner.Run(ctx); err != nil {
		t.Fatalf("Status 失败: %v", err)
	}
	t.Logf("Status 输出:\n%s", statusOutput.String())

	// --- 阶段 6: 幂等性检查 ---
	t.Log("阶段 6: 幂等性检查")
	ecsID := state.Resources.ECS.ID
	if err := deployer.CreateResources(ctx, state, ""); err != nil {
		t.Fatalf("幂等 CreateResources 失败: %v", err)
	}
	if state.Resources.ECS.ID != ecsID {
		t.Error("幂等检查失败: ECS ID 不应改变")
	}

	// --- 阶段 7: Destroy（由 defer 执行）---
	t.Log("E2E 测试通过，defer 将执行 destroy")
}

// TestE2E_DestroyDryRun 验证 dry-run 不删除资源
func TestE2E_DestroyDryRun(t *testing.T) {
	skipIfNoCredentials(t)

	stateDir := t.TempDir()
	ctx := context.Background()

	deployer, cfg := newE2EDeployer(t, stateDir)

	// 仅创建 VPC 用于测试
	state := config.NewState(cfg.RegionID, alicloud.DefaultImageID)
	vpc, err := alicloud.CreateVPC(deployer.VPC, cfg.RegionID, "cloudcode-e2e-dryrun")
	if err != nil {
		t.Fatalf("创建 VPC 失败: %v", err)
	}
	state.Resources.VPC = config.VPCResource{ID: vpc.ID, CIDR: vpc.CIDR}

	// 保存 state
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, config.StateFileName), data, 0600); err != nil {
		t.Fatalf("保存 state 失败: %v", err)
	}

	defer func() {
		// 真正清理
		destroyer := newE2EDestroyer(t, stateDir, cfg)
		_ = destroyer.Run(ctx, true, false)
	}()

	// dry-run 不应删除
	destroyer := newE2EDestroyer(t, stateDir, cfg)
	if err := destroyer.Run(ctx, true, true); err != nil {
		t.Fatalf("dry-run 失败: %v", err)
	}

	// VPC 应该还在
	_, err = alicloud.DescribeVPC(deployer.VPC, vpc.ID, cfg.RegionID)
	if err != nil {
		t.Error("dry-run 后 VPC 不应被删除")
	}
}
