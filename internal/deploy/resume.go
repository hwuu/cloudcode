package deploy

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hwuu/cloudcode/internal/alicloud"
	"github.com/hwuu/cloudcode/internal/config"
	"github.com/hwuu/cloudcode/internal/remote"
)

// Resumer 恢复操作器
type Resumer struct {
	ECS          alicloud.ECSAPI
	Prompter     *config.Prompter
	Output       io.Writer
	Region       string
	StateDir     string
	SSHDialFunc  SSHDialFactory
	WaitInterval time.Duration
	WaitTimeout  time.Duration
}

func (r *Resumer) printf(format string, args ...interface{}) {
	fmt.Fprintf(r.Output, format, args...)
}

func (r *Resumer) loadState() (*config.State, error) {
	if r.StateDir != "" {
		return loadStateFrom(r.StateDir)
	}
	return config.LoadState()
}

func (r *Resumer) saveState(state *config.State) error {
	if r.StateDir != "" {
		return saveStateTo(r.StateDir, state)
	}
	return config.SaveState(state)
}

// Run 执行 resume 流程
func (r *Resumer) Run(ctx context.Context) error {
	state, err := r.loadState()
	if err != nil {
		return fmt.Errorf("未找到部署记录，请先运行 cloudcode deploy")
	}

	if state.Status == "running" || state.Status == "" {
		r.printf("实例已在运行中。\n")
		return nil
	}

	if state.Status != "suspended" {
		if state.Status == "destroyed" {
			return fmt.Errorf("实例已销毁，请使用 cloudcode deploy 从快照恢复或重新部署")
		}
		return fmt.Errorf("实例状态为 %s，无法恢复", state.Status)
	}

	if !state.HasECS() {
		return fmt.Errorf("未找到 ECS 实例")
	}

	confirmed, err := r.Prompter.PromptConfirm("确认恢复运行?", true)
	if err != nil {
		return err
	}
	if !confirmed {
		r.printf("已取消。\n")
		return nil
	}

	// 启动实例
	r.printf("恢复中...\n")
	if err := alicloud.StartECSInstance(r.ECS, state.Resources.ECS.ID); err != nil {
		return fmt.Errorf("启动失败: %w", err)
	}

	// 等待 Running
	if _, err := alicloud.WaitForInstanceRunning(ctx, r.ECS, state.Resources.ECS.ID, r.Region, r.WaitInterval, r.WaitTimeout); err != nil {
		return fmt.Errorf("等待启动完成失败: %w", err)
	}
	r.printf("  ✓ ECS 已启动\n")

	// SSH 连接 + 健康检查
	privateKey, err := readSSHKeyFrom(r.getStateDir(), state)
	if err != nil {
		r.printf("  ⚠ 无法读取 SSH 私钥，跳过健康检查\n")
	} else {
		dialFunc := r.SSHDialFunc(state.Resources.EIP.IP, 22, "root", privateKey)
		sshClient, err := remote.WaitForSSH(ctx, dialFunc, remote.WaitSSHOptions{})
		if err != nil {
			r.printf("  ⚠ SSH 连接失败: %v\n", err)
		} else {
			defer sshClient.Close()
			r.printf("  ✓ SSH 连接成功\n")

			output, err := sshClient.RunCommand(ctx, "cd ~/cloudcode && docker compose ps --format '{{.Name}} {{.State}}'")
			if err != nil {
				r.printf("  ⚠ 健康检查失败: %v\n", err)
			} else {
				r.printf("  容器状态:\n")
				for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
					if line != "" {
						r.printf("    %s\n", line)
					}
				}
			}
		}
	}

	// 更新 state
	state.Status = "running"
	if err := r.saveState(state); err != nil {
		return err
	}

	r.printf("✅ 实例已恢复运行\n")
	r.printf("  访问地址: https://%s\n", state.CloudCode.Domain)
	return nil
}

func (r *Resumer) getStateDir() string {
	if r.StateDir != "" {
		return r.StateDir
	}
	dir, _ := config.GetStateDir()
	return dir
}
