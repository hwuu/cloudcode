package deploy

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hwuu/cloudcode/internal/config"
	"github.com/hwuu/cloudcode/internal/remote"
)

// StatusRunner 状态查询器
type StatusRunner struct {
	Output      io.Writer
	StateDir    string
	SSHDialFunc SSHDialFactory
}

func (s *StatusRunner) printf(format string, args ...interface{}) {
	fmt.Fprintf(s.Output, format, args...)
}

func (s *StatusRunner) loadState() (*config.State, error) {
	if s.StateDir != "" {
		return loadStateFrom(s.StateDir)
	}
	return config.LoadState()
}

// Run 执行状态查询
func (s *StatusRunner) Run(ctx context.Context) error {
	state, err := s.loadState()
	if err != nil {
		s.printf("未找到部署记录。请先运行 cloudcode deploy\n")
		return nil
	}

	s.printf("CloudCode 部署状态\n")
	s.printf("─────────────────────────────────────────\n")
	s.printf("区域: %s\n", state.Region)
	s.printf("创建时间: %s\n", state.CreatedAt)
	s.printf("\n")

	// 云资源
	s.printf("云资源:\n")
	s.printResource("VPC", state.Resources.VPC.ID)
	s.printResource("交换机", state.Resources.VSwitch.ID)
	s.printResource("安全组", state.Resources.SecurityGroup.ID)
	s.printResource("SSH 密钥对", state.Resources.SSHKeyPair.Name)
	s.printResource("ECS 实例", state.Resources.ECS.ID)
	if state.Resources.EIP.ID != "" {
		s.printf("  %-12s %s (IP: %s)\n", "EIP", state.Resources.EIP.ID, state.Resources.EIP.IP)
	} else {
		s.printf("  %-12s ❌ 未创建\n", "EIP")
	}

	// 应用信息
	if state.CloudCode.Domain != "" {
		s.printf("\n应用:\n")
		s.printf("  域名: %s\n", state.CloudCode.Domain)
		s.printf("  用户: %s\n", state.CloudCode.Username)
		s.printf("  地址: https://%s\n", state.CloudCode.Domain)
	}

	// 容器状态（通过 SSH）
	if state.Resources.EIP.IP != "" && state.Resources.SSHKeyPair.Name != "" && s.SSHDialFunc != nil {
		s.printf("\n容器状态:\n")
		if err := s.checkContainers(ctx, state); err != nil {
			s.printf("  ⚠ 无法获取容器状态: %v\n", err)
		}
	}

	s.printf("─────────────────────────────────────────\n")
	return nil
}

func (s *StatusRunner) printResource(name, id string) {
	if id != "" {
		s.printf("  %-12s %s\n", name, id)
	} else {
		s.printf("  %-12s ❌ 未创建\n", name)
	}
}

func (s *StatusRunner) checkContainers(ctx context.Context, state *config.State) error {
	privateKey, err := readSSHKeyFrom(s.StateDir, state)
	if err != nil {
		return err
	}

	dialFunc := s.SSHDialFunc(state.Resources.EIP.IP, 22, "root", privateKey)
	sshClient, err := remote.WaitForSSH(ctx, dialFunc, remote.WaitSSHOptions{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer sshClient.Close()

	output, err := sshClient.RunCommand(ctx, "cd ~/cloudcode && docker compose ps --format '{{.Name}} {{.State}}'")
	if err != nil {
		return err
	}

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line != "" {
			s.printf("  %s\n", line)
		}
	}

	return nil
}
