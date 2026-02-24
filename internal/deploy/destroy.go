package deploy

// destroy.go 按序销毁所有云资源，支持 --force（跳过确认）和 --dry-run（仅预览）。
// 可选保留磁盘快照，下次 deploy 可从快照恢复。
// 删除顺序：解绑EIP → 释放EIP → 删除ECS → 删除SSH密钥对 → 删除安全组 → 删除VSwitch → 删除VPC。
// 每步删除成功后立即更新 state，支持中断后重新执行（跳过已删除的资源）。
// 单个资源删除失败不阻塞后续删除，最后汇总输出失败资源。

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hwuu/cloudcode/internal/alicloud"
	"github.com/hwuu/cloudcode/internal/config"
)

// Destroyer 资源销毁器
type Destroyer struct {
	ECS          alicloud.ECSAPI
	VPC          alicloud.VPCAPI
	Prompter     *config.Prompter
	Output       io.Writer
	Region       string
	StateDir     string
	Version      string        // CloudCode 版本号（写入 backup.json）
	WaitInterval time.Duration // 快照等待轮询间隔（测试用）
	WaitTimeout  time.Duration // 快照等待超时（测试用）
}

func (d *Destroyer) printf(format string, args ...interface{}) {
	fmt.Fprintf(d.Output, format, args...)
}

func (d *Destroyer) loadState() (*config.State, error) {
	if d.StateDir != "" {
		return loadStateFrom(d.StateDir)
	}
	return config.LoadState()
}

func (d *Destroyer) saveState(state *config.State) error {
	if d.StateDir != "" {
		return saveStateTo(d.StateDir, state)
	}
	return config.SaveState(state)
}

func (d *Destroyer) getStateDir() string {
	if d.StateDir != "" {
		return d.StateDir
	}
	dir, _ := config.GetStateDir()
	return dir
}

// Run 执行资源销毁
func (d *Destroyer) Run(ctx context.Context, force, dryRun bool) error {
	state, err := d.loadState()
	if err != nil {
		d.printf("未找到部署记录，无需清理。\n")
		return nil
	}

	// 展示将要删除的资源
	d.printf("将要删除以下资源:\n")
	d.printIfSet("EIP", state.Resources.EIP.ID)
	d.printIfSet("ECS 实例", state.Resources.ECS.ID)
	d.printIfSet("SSH 密钥对", state.Resources.SSHKeyPair.Name)
	d.printIfSet("安全组", state.Resources.SecurityGroup.ID)
	d.printIfSet("交换机", state.Resources.VSwitch.ID)
	d.printIfSet("VPC", state.Resources.VPC.ID)

	if dryRun {
		d.printf("\n(dry-run 模式，不会实际删除)\n")
		return nil
	}

	// 可选保留快照（默认保留）
	keepSnapshot := false
	if !force && state.Resources.ECS.ID != "" {
		keepSnapshot, err = d.Prompter.PromptConfirm("是否保留磁盘快照（下次 deploy 可恢复）?", true)
		if err != nil {
			return err
		}
	}

	// 创建快照
	if keepSnapshot {
		if err := d.createSnapshot(ctx, state); err != nil {
			d.printf("  ⚠ 快照创建失败: %v\n", err)
			continueDestroy, promptErr := d.Prompter.PromptConfirm("继续销毁（数据将丢失）?", false)
			if promptErr != nil {
				return promptErr
			}
			if !continueDestroy {
				d.printf("已取消。\n")
				return nil
			}
			keepSnapshot = false
		}
	}

	// 确认销毁
	if !force {
		confirmed, err := d.Prompter.PromptConfirm("确认删除所有资源? 此操作不可恢复!", false)
		if err != nil {
			return err
		}
		if !confirmed {
			d.printf("已取消。\n")
			return nil
		}
	}

	d.printf("\n开始删除资源...\n")

	var failedResources []string

	// 1. 解绑 EIP
	if state.Resources.EIP.ID != "" && state.Resources.ECS.ID != "" {
		d.printf("  解绑 EIP...")
		if err := alicloud.UnassociateEIPFromInstance(d.VPC, state.Resources.EIP.ID, state.Resources.ECS.ID, d.Region); err != nil {
			d.printf(" ⚠ %v\n", err)
			failedResources = append(failedResources, fmt.Sprintf("解绑 EIP: %v", err))
		} else {
			d.printf(" ✓\n")
			time.Sleep(5 * time.Second)
		}
	}

	// 2. 释放 EIP
	if state.Resources.EIP.ID != "" {
		d.printf("  释放 EIP (%s)...", state.Resources.EIP.ID)
		if err := alicloud.ReleaseEIP(d.VPC, state.Resources.EIP.ID); err != nil {
			d.printf(" ⚠ %v\n", err)
			failedResources = append(failedResources, fmt.Sprintf("释放 EIP %s: %v", state.Resources.EIP.ID, err))
		} else {
			state.Resources.EIP = config.EIPResource{}
			_ = d.saveState(state)
			d.printf(" ✓\n")
		}
	}

	// 3. 删除 ECS（force delete 会自动停止）
	if state.Resources.ECS.ID != "" {
		d.printf("  删除 ECS (%s)...", state.Resources.ECS.ID)
		if err := alicloud.DeleteECSInstance(d.ECS, state.Resources.ECS.ID); err != nil {
			d.printf(" ⚠ %v\n", err)
			failedResources = append(failedResources, fmt.Sprintf("删除 ECS %s: %v", state.Resources.ECS.ID, err))
		} else {
			state.Resources.ECS = config.ECSResource{}
			_ = d.saveState(state)
			d.printf(" ✓\n")
			time.Sleep(10 * time.Second)
		}
	}

	// 4. 删除 SSH 密钥对
	if state.Resources.SSHKeyPair.Name != "" {
		d.printf("  删除 SSH 密钥对 (%s)...", state.Resources.SSHKeyPair.Name)
		if err := alicloud.DeleteSSHKeyPair(d.ECS, state.Resources.SSHKeyPair.Name, d.Region); err != nil {
			d.printf(" ⚠ %v\n", err)
			failedResources = append(failedResources, fmt.Sprintf("删除密钥对 %s: %v", state.Resources.SSHKeyPair.Name, err))
		} else {
			state.Resources.SSHKeyPair = config.SSHKeyPairResource{}
			_ = d.saveState(state)
			d.printf(" ✓\n")
		}
	}

	// 5. 删除安全组
	if state.Resources.SecurityGroup.ID != "" {
		d.printf("  删除安全组 (%s)...", state.Resources.SecurityGroup.ID)
		if err := alicloud.DeleteSecurityGroup(d.ECS, state.Resources.SecurityGroup.ID, d.Region); err != nil {
			d.printf(" ⚠ %v\n", err)
			failedResources = append(failedResources, fmt.Sprintf("删除安全组 %s: %v", state.Resources.SecurityGroup.ID, err))
		} else {
			state.Resources.SecurityGroup = config.SecurityGroupResource{}
			_ = d.saveState(state)
			d.printf(" ✓\n")
		}
	}

	// 6. 删除 VSwitch
	if state.Resources.VSwitch.ID != "" {
		d.printf("  删除交换机 (%s)...", state.Resources.VSwitch.ID)
		if err := alicloud.DeleteVSwitch(d.VPC, state.Resources.VSwitch.ID); err != nil {
			d.printf(" ⚠ %v\n", err)
			failedResources = append(failedResources, fmt.Sprintf("删除交换机 %s: %v", state.Resources.VSwitch.ID, err))
		} else {
			state.Resources.VSwitch = config.VSwitchResource{}
			_ = d.saveState(state)
			d.printf(" ✓\n")
			time.Sleep(5 * time.Second)
		}
	}

	// 7. 删除 VPC
	if state.Resources.VPC.ID != "" {
		d.printf("  删除 VPC (%s)...", state.Resources.VPC.ID)
		if err := alicloud.DeleteVPC(d.VPC, state.Resources.VPC.ID); err != nil {
			d.printf(" ⚠ %v\n", err)
			failedResources = append(failedResources, fmt.Sprintf("删除 VPC %s: %v", state.Resources.VPC.ID, err))
		} else {
			state.Resources.VPC = config.VPCResource{}
			_ = d.saveState(state)
			d.printf(" ✓\n")
		}
	}

	// 8. 删除本地 SSH 私钥
	keyPath := filepath.Join(d.getStateDir(), "ssh_key")
	_ = os.Remove(keyPath)

	// 9. 处理 state 和 backup
	if keepSnapshot {
		// 保留快照：state 标记为 destroyed
		state.Status = "destroyed"
		_ = d.saveState(state)
	} else {
		// 不保留快照：删除 state 和 backup
		_ = d.deleteState()
		_ = d.deleteBackup()
	}

	if len(failedResources) > 0 {
		d.printf("\n⚠ 以下资源删除失败，请手动清理:\n")
		for _, msg := range failedResources {
			d.printf("  - %s\n", msg)
		}
		return fmt.Errorf("%d 个资源删除失败", len(failedResources))
	}

	d.printf("\n✅ 所有资源已清理完毕。\n")
	if keepSnapshot {
		d.printf("  快照已保留，下次 cloudcode deploy 可从快照恢复。\n")
	}
	return nil
}

// createSnapshot 停机 → 获取系统盘 → 创建快照 → 等待完成 → 保存 backup.json
func (d *Destroyer) createSnapshot(ctx context.Context, state *config.State) error {
	instanceID := state.Resources.ECS.ID

	// 停机（确保数据一致性）
	d.printf("  停机中（确保数据一致性）...\n")
	if err := alicloud.StopECSInstance(d.ECS, instanceID, false); err != nil {
		return fmt.Errorf("停机失败: %w", err)
	}
	if err := alicloud.WaitForInstanceStatus(ctx, d.ECS, instanceID, d.Region, "Stopped", d.WaitInterval, d.WaitTimeout); err != nil {
		return fmt.Errorf("等待停机失败: %w", err)
	}
	d.printf("  ✓ 已停机\n")

	// 获取系统盘 ID
	diskID, err := alicloud.GetSystemDiskID(d.ECS, instanceID, d.Region)
	if err != nil {
		return err
	}

	// 创建快照
	d.printf("  创建快照...\n")
	snapshotName := fmt.Sprintf("cloudcode-%s", time.Now().UTC().Format("20060102-150405"))
	snapshotID, err := alicloud.CreateDiskSnapshot(d.ECS, diskID, snapshotName)
	if err != nil {
		return err
	}

	// 等待快照完成
	if err := alicloud.WaitForSnapshotReady(ctx, d.ECS, snapshotID, d.Region, d.WaitInterval, d.WaitTimeout); err != nil {
		return err
	}
	d.printf("  ✓ 快照已创建 (%s)\n", snapshotID)

	// 删除旧快照（只保留最新一份）
	dir := d.getStateDir()
	oldBackup, _ := config.LoadBackupFrom(dir)
	if oldBackup != nil && oldBackup.SnapshotID != "" && oldBackup.SnapshotID != snapshotID {
		_ = alicloud.DeleteSnapshot(d.ECS, oldBackup.SnapshotID)
	}

	// 保存 backup.json
	backup := &config.Backup{
		CloudCodeVersion: d.Version,
		SnapshotID:       snapshotID,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		Region:           d.Region,
		DiskSize:         state.Resources.ECS.SystemDiskSize,
		Domain:           state.CloudCode.Domain,
		Username:         state.CloudCode.Username,
	}
	if d.StateDir != "" {
		return config.SaveBackupTo(d.StateDir, backup)
	}
	return config.SaveBackup(backup)
}

func (d *Destroyer) printIfSet(name, id string) {
	if id != "" {
		d.printf("  - %s: %s\n", name, id)
	}
}

func (d *Destroyer) deleteState() error {
	dir := d.getStateDir()
	path := filepath.Join(dir, config.StateFileName)
	return os.Remove(path)
}

func (d *Destroyer) deleteBackup() error {
	if d.StateDir != "" {
		return config.DeleteBackupFrom(d.StateDir)
	}
	return config.DeleteBackup()
}
