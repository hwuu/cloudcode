package deploy

// status.go 查询并展示当前部署状态：云资源信息 + 容器运行状态。
// 通过 SSH 连接 ECS 执行 docker compose ps 获取容器状态。

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

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
	s.printf("%s %s\n", padRight("区域:", 14), state.Region)
	s.printf("%s %s\n", padRight("创建时间:", 14), state.CreatedAt)
	s.printf("\n")

	// 云资源
	s.printf("云资源:\n")
	s.printResource("VPC", state.Resources.VPC.ID)
	s.printResource("交换机", state.Resources.VSwitch.ID)
	s.printResource("安全组", state.Resources.SecurityGroup.ID)
	s.printResource("SSH 密钥对", state.Resources.SSHKeyPair.Name)
	s.printResource("ECS 实例", state.Resources.ECS.ID)
	if state.Resources.EIP.ID != "" {
		s.printf("  %s %s (IP: %s)\n", padRight("EIP", 12), state.Resources.EIP.ID, state.Resources.EIP.IP)
	} else {
		s.printf("  %s ❌ 未创建\n", padRight("EIP", 12))
	}

	// 应用信息
	if state.CloudCode.Domain != "" {
		s.printf("\n应用:\n")
		s.printf("  %s %s\n", padRight("域名:", 12), state.CloudCode.Domain)
		s.printf("  %s %s\n", padRight("用户:", 12), state.CloudCode.Username)
		s.printf("  %s https://%s\n", padRight("地址:", 12), state.CloudCode.Domain)
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
	padded := padRight(name, 12)
	if id != "" {
		s.printf("  %s %s\n", padded, id)
	} else {
		s.printf("  %s ❌ 未创建\n", padded)
	}
}

// displayWidth 计算字符串在终端中的显示宽度（中文字符占 2 列）
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hangul, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hiragana, r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// padRight 按显示宽度右填充空格到指定列数
func padRight(s string, width int) string {
	dw := displayWidth(s)
	if dw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-dw)
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
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				s.printf("  %s %s\n", padRight(parts[0], 12), strings.Join(parts[1:], " "))
			} else {
				s.printf("  %s\n", line)
			}
		}
	}

	return nil
}
