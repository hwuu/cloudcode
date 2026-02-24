package deploy

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/hwuu/cloudcode/internal/alicloud"
	"github.com/hwuu/cloudcode/internal/config"
)

// Suspender 停机操作器
type Suspender struct {
	ECS          alicloud.ECSAPI
	Prompter     *config.Prompter
	Output       io.Writer
	Region       string
	StateDir     string
	WaitInterval time.Duration
	WaitTimeout  time.Duration
}

func (s *Suspender) printf(format string, args ...interface{}) {
	fmt.Fprintf(s.Output, format, args...)
}

func (s *Suspender) loadState() (*config.State, error) {
	if s.StateDir != "" {
		return loadStateFrom(s.StateDir)
	}
	return config.LoadState()
}

func (s *Suspender) saveState(state *config.State) error {
	if s.StateDir != "" {
		return saveStateTo(s.StateDir, state)
	}
	return config.SaveState(state)
}

// Run 执行 suspend 流程
func (s *Suspender) Run(ctx context.Context) error {
	state, err := s.loadState()
	if err != nil {
		return fmt.Errorf("未找到部署记录，请先运行 cloudcode deploy")
	}

	if state.Status == "suspended" {
		s.printf("实例已处于停机状态。\n")
		return nil
	}

	if state.Status == "destroyed" {
		return fmt.Errorf("实例已销毁，请使用 cloudcode deploy 从快照恢复或重新部署")
	}

	if !state.HasECS() {
		return fmt.Errorf("未找到 ECS 实例")
	}

	confirmed, err := s.Prompter.PromptConfirm("确认停机? 停机后仅收磁盘费 (~$1.2/月)")
	if err != nil {
		return err
	}
	if !confirmed {
		s.printf("已取消。\n")
		return nil
	}

	// StopCharging 模式停机
	s.printf("停机中...\n")
	if err := alicloud.StopECSInstance(s.ECS, state.Resources.ECS.ID, true); err != nil {
		return fmt.Errorf("停机失败: %w", err)
	}

	// 等待 Stopped
	if err := alicloud.WaitForInstanceStatus(ctx, s.ECS, state.Resources.ECS.ID, s.Region, "Stopped", s.WaitInterval, s.WaitTimeout); err != nil {
		return fmt.Errorf("等待停机完成失败: %w", err)
	}

	// 更新 state
	state.Status = "suspended"
	if err := s.saveState(state); err != nil {
		return err
	}

	s.printf("✅ 实例已停机（StopCharging 模式）\n")
	s.printf("  恢复运行: cloudcode resume\n")
	return nil
}
