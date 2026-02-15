package deploy

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hwuu/cloudcode/internal/alicloud"
	"github.com/hwuu/cloudcode/internal/config"
)

// Destroyer 资源销毁器
type Destroyer struct {
	ECS      alicloud.ECSAPI
	VPC      alicloud.VPCAPI
	Prompter *config.Prompter
	Output   io.Writer
	Region   string
	StateDir string
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

	// 确认
	if !force {
		confirmed, err := d.Prompter.PromptConfirm("确认删除所有资源? 此操作不可恢复!")
		if err != nil {
			return err
		}
		if !confirmed {
			d.printf("已取消。\n")
			return nil
		}
	}

	d.printf("\n开始删除资源...\n")

	// 1. 解绑 EIP
	if state.Resources.EIP.ID != "" && state.Resources.ECS.ID != "" {
		d.printf("  解绑 EIP...")
		if err := alicloud.UnassociateEIPFromInstance(d.VPC, state.Resources.EIP.ID, state.Resources.ECS.ID, d.Region); err != nil {
			d.printf(" ⚠ %v\n", err)
		} else {
			d.printf(" ✓\n")
		}
	}

	// 2. 释放 EIP
	if state.Resources.EIP.ID != "" {
		d.printf("  释放 EIP (%s)...", state.Resources.EIP.ID)
		if err := alicloud.ReleaseEIP(d.VPC, state.Resources.EIP.ID); err != nil {
			d.printf(" ⚠ %v\n", err)
		} else {
			state.Resources.EIP = config.EIPResource{}
			_ = d.saveState(state)
			d.printf(" ✓\n")
		}
	}

	// 3. 停止 ECS
	if state.Resources.ECS.ID != "" {
		d.printf("  停止 ECS (%s)...", state.Resources.ECS.ID)
		if err := alicloud.StopECSInstance(d.ECS, state.Resources.ECS.ID); err != nil {
			d.printf(" ⚠ %v\n", err)
		} else {
			d.printf(" ✓\n")
		}
	}

	// 4. 删除 ECS
	if state.Resources.ECS.ID != "" {
		d.printf("  删除 ECS (%s)...", state.Resources.ECS.ID)
		if err := alicloud.DeleteECSInstance(d.ECS, state.Resources.ECS.ID); err != nil {
			d.printf(" ⚠ %v\n", err)
		} else {
			state.Resources.ECS = config.ECSResource{}
			_ = d.saveState(state)
			d.printf(" ✓\n")
		}
	}

	// 5. 删除 SSH 密钥对
	if state.Resources.SSHKeyPair.Name != "" {
		d.printf("  删除 SSH 密钥对 (%s)...", state.Resources.SSHKeyPair.Name)
		if err := alicloud.DeleteSSHKeyPair(d.ECS, state.Resources.SSHKeyPair.Name); err != nil {
			d.printf(" ⚠ %v\n", err)
		} else {
			state.Resources.SSHKeyPair = config.SSHKeyPairResource{}
			_ = d.saveState(state)
			d.printf(" ✓\n")
		}
	}

	// 6. 删除安全组
	if state.Resources.SecurityGroup.ID != "" {
		d.printf("  删除安全组 (%s)...", state.Resources.SecurityGroup.ID)
		if err := alicloud.DeleteSecurityGroup(d.ECS, state.Resources.SecurityGroup.ID, d.Region); err != nil {
			d.printf(" ⚠ %v\n", err)
		} else {
			state.Resources.SecurityGroup = config.SecurityGroupResource{}
			_ = d.saveState(state)
			d.printf(" ✓\n")
		}
	}

	// 7. 删除 VSwitch
	if state.Resources.VSwitch.ID != "" {
		d.printf("  删除交换机 (%s)...", state.Resources.VSwitch.ID)
		if err := alicloud.DeleteVSwitch(d.VPC, state.Resources.VSwitch.ID); err != nil {
			d.printf(" ⚠ %v\n", err)
		} else {
			state.Resources.VSwitch = config.VSwitchResource{}
			_ = d.saveState(state)
			d.printf(" ✓\n")
		}
	}

	// 8. 删除 VPC
	if state.Resources.VPC.ID != "" {
		d.printf("  删除 VPC (%s)...", state.Resources.VPC.ID)
		if err := alicloud.DeleteVPC(d.VPC, state.Resources.VPC.ID); err != nil {
			d.printf(" ⚠ %v\n", err)
		} else {
			state.Resources.VPC = config.VPCResource{}
			_ = d.saveState(state)
			d.printf(" ✓\n")
		}
	}

	// 9. 删除本地 SSH 私钥
	keyPath := filepath.Join(d.getStateDir(), "ssh_key")
	_ = os.Remove(keyPath)

	// 10. 删除 state 文件
	if err := d.deleteState(); err != nil {
		d.printf("  ⚠ 删除 state 文件失败: %v\n", err)
	}

	d.printf("\n✅ 所有资源已清理完毕。\n")
	return nil
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
