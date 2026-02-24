// Package main 是 CloudCode CLI 的入口。
// 提供子命令：deploy（部署）、status（状态）、destroy（销毁）、
// otc（读取验证码）、logs（容器日志）、ssh（登录 ECS）、exec（容器内执行命令）、version（版本）。
// 版本信息通过 ldflags 在构建时注入。
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/hwuu/cloudcode/internal/alicloud"
	"github.com/hwuu/cloudcode/internal/config"
	"github.com/hwuu/cloudcode/internal/deploy"
	"github.com/hwuu/cloudcode/internal/remote"
	"github.com/spf13/cobra"
)

// 构建时通过 ldflags 注入
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "cloudcode",
		Short: "一键部署 OpenCode 到阿里云 ECS",
		Long:  "CloudCode — 一键部署 OpenCode 到阿里云 ECS，带 HTTPS + Authelia 两步认证。",
	}

	rootCmd.AddCommand(newDeployCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newDestroyCmd())
	rootCmd.AddCommand(newOTCCmd())
	rootCmd.AddCommand(newLogsCmd())
	rootCmd.AddCommand(newSSHCmd())
	rootCmd.AddCommand(newExecCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

func newDeployCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "部署 OpenCode 到阿里云 ECS",
		RunE: func(cmd *cobra.Command, args []string) error {
			// 加载阿里云配置
			cfg, err := alicloud.LoadConfigFromEnv()
			if err != nil {
				return fmt.Errorf("阿里云配置错误: %w", err)
			}

			// 创建 SDK 客户端
			clients, err := alicloud.NewClients(cfg)
			if err != nil {
				return fmt.Errorf("初始化阿里云 SDK 失败: %w", err)
			}

			// 创建 Deployer
			prompter := config.NewPrompter(os.Stdin, os.Stdout)
			d := &deploy.Deployer{
				ECS:      clients.ECS,
				VPC:      clients.VPC,
				STS:      clients.STS,
				Prompter: prompter,
				Output:   os.Stdout,
				Region:   cfg.RegionID,
				SSHDialFunc: func(host string, port int, user string, privateKey []byte) remote.DialFunc {
					return remote.NewSSHDialFunc(host, port, user, privateKey)
				},
				SFTPFactory: remote.NewSFTPClient,
				GetPublicIP: remote.GetPublicIP,
				Version:     version,
			}

			return d.Run(cmd.Context(), force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "强制重新部署应用层（跳过云资源创建）")

	return cmd
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看部署状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			s := &deploy.StatusRunner{
				Output: os.Stdout,
				SSHDialFunc: func(host string, port int, user string, privateKey []byte) remote.DialFunc {
					return remote.NewSSHDialFunc(host, port, user, privateKey)
				},
			}
			return s.Run(cmd.Context())
		},
	}
}

func newDestroyCmd() *cobra.Command {
	var force, dryRun bool

	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "销毁所有云资源",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := alicloud.LoadConfigFromEnv()
			if err != nil {
				return fmt.Errorf("阿里云配置错误: %w", err)
			}

			clients, err := alicloud.NewClients(cfg)
			if err != nil {
				return fmt.Errorf("初始化阿里云 SDK 失败: %w", err)
			}

			prompter := config.NewPrompter(os.Stdin, os.Stdout)
			d := &deploy.Destroyer{
				ECS:      clients.ECS,
				VPC:      clients.VPC,
				Prompter: prompter,
				Output:   os.Stdout,
				Region:   cfg.RegionID,
			}

			return d.Run(cmd.Context(), force, dryRun)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "跳过确认直接删除")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "仅展示将要删除的资源，不实际删除")

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "显示版本信息",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("cloudcode %s\n", version)
			fmt.Printf("  commit: %s\n", commit)
			fmt.Printf("  built:  %s\n", date)
			fmt.Printf("  go:     %s\n", runtime.Version())
		},
	}
}

// sshRunCommand 从 state 读取连接信息，SSH 到 ECS 执行命令并返回输出
func sshRunCommand(ctx context.Context, cmd string) (string, error) {
	state, privateKey, err := loadStateAndKey("")
	if err != nil {
		return "", err
	}
	dialFunc := remote.NewSSHDialFunc(state.Resources.EIP.IP, 22, "root", privateKey)
	client, err := remote.WaitForSSH(ctx, dialFunc, remote.WaitSSHOptions{Timeout: 10 * remote.DefaultInitialInterval})
	if err != nil {
		return "", fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()
	return client.RunCommand(ctx, cmd)
}

// loadStateAndKey 加载 state 和 SSH 私钥
func loadStateAndKey(stateDir string) (*config.State, []byte, error) {
	state, err := config.LoadState()
	if err != nil {
		return nil, nil, fmt.Errorf("未找到部署记录，请先运行 cloudcode deploy")
	}
	if state.Resources.EIP.IP == "" {
		return nil, nil, fmt.Errorf("EIP 未分配，请先完成部署")
	}
	dir := stateDir
	if dir == "" {
		dir, err = config.GetStateDir()
		if err != nil {
			return nil, nil, err
		}
	}
	keyPath := dir + "/ssh_key"
	privateKey, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("读取 SSH 私钥失败: %w", err)
	}
	return state, privateKey, nil
}

// newOTCCmd 读取 Authelia One-Time Code（首次注册 Passkey 时使用）
func newOTCCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "otc",
		Short: "读取 Authelia 一次性验证码（用于首次注册 Passkey）",
		RunE: func(cmd *cobra.Command, args []string) error {
			output, err := sshRunCommand(cmd.Context(), "docker exec authelia cat /config/notification.txt 2>/dev/null")
			if err != nil {
				return fmt.Errorf("读取验证码失败: %w", err)
			}
			output = strings.TrimSpace(output)
			if output == "" {
				fmt.Println("暂无验证码记录。请先在浏览器中触发 Authelia 验证操作。")
				return nil
			}
			fmt.Println(output)
			return nil
		},
	}
}

// newLogsCmd 查看容器日志
func newLogsCmd() *cobra.Command {
	var tail int
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs [container]",
		Short: "查看容器日志",
		Long:  "查看 Docker Compose 容器日志。可选指定容器名（authelia/caddy/devbox）。",
		ValidArgs: []string{"authelia", "caddy", "devbox"},
		RunE: func(cmd *cobra.Command, args []string) error {
			composeCmd := "cd ~/cloudcode && docker compose logs"
			if tail > 0 {
				composeCmd += fmt.Sprintf(" --tail=%d", tail)
			}
			if follow {
				// follow 模式需要交互式 SSH，用 exec 替代
				state, _, err := loadStateAndKey("")
				if err != nil {
					return err
				}
				dir, _ := config.GetStateDir()
				keyPath := dir + "/ssh_key"
				sshArgs := []string{
					"-i", keyPath,
					"-o", "StrictHostKeyChecking=no",
					"-o", "UserKnownHostsFile=/dev/null",
					"-o", "LogLevel=ERROR",
					"root@" + state.Resources.EIP.IP,
					composeCmd + " -f",
				}
				if len(args) > 0 {
					sshArgs[len(sshArgs)-1] += " " + args[0]
				}
				return syscall.Exec(sshBinary(), append([]string{"ssh"}, sshArgs...), os.Environ())
			}
			if len(args) > 0 {
				composeCmd += " " + args[0]
			}
			output, err := sshRunCommand(cmd.Context(), composeCmd)
			if err != nil {
				return fmt.Errorf("获取日志失败: %w", err)
			}
			fmt.Print(output)
			return nil
		},
	}

	cmd.Flags().IntVarP(&tail, "tail", "n", 50, "显示最后 N 行日志")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "实时跟踪日志输出")

	return cmd
}

// newSSHCmd 快捷 SSH 登录 ECS 或进入容器
func newSSHCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh [target]",
		Short: "SSH 登录到 ECS 实例或容器",
		Long: `SSH 登录到 ECS 实例或指定容器。

target 可选值：
  host       登录 ECS 宿主机（默认）
  devbox     进入 devbox 容器
  authelia   进入 authelia 容器
  caddy      进入 caddy 容器`,
		ValidArgs: []string{"host", "devbox", "authelia", "caddy"},
		RunE: func(cmd *cobra.Command, args []string) error {
			state, _, err := loadStateAndKey("")
			if err != nil {
				return err
			}
			dir, _ := config.GetStateDir()
			keyPath := dir + "/ssh_key"

			target := "host"
			if len(args) > 0 {
				target = args[0]
			}

			sshArgs := []string{
				"ssh",
				"-i", keyPath,
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
				"-t",
				"root@" + state.Resources.EIP.IP,
			}

			if target != "host" {
				// 进入容器的交互式 shell
				sshArgs = append(sshArgs, fmt.Sprintf("cd ~/cloudcode && docker compose exec %s sh -c 'if command -v bash >/dev/null; then bash; else sh; fi'", target))
			}

			return syscall.Exec(sshBinary(), sshArgs, os.Environ())
		},
	}
	return cmd
}

// newExecCmd 在容器内执行命令
func newExecCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "exec <container> <command> [args...]",
		Short: "在容器内执行命令",
		Long:  "在指定容器内执行命令。例如: cloudcode exec devbox opencode --version",
		Args:  cobra.MinimumNArgs(2),
		ValidArgs: []string{"authelia", "caddy", "devbox"},
		RunE: func(cmd *cobra.Command, args []string) error {
			container := args[0]
			containerCmd := strings.Join(args[1:], " ")
			remoteCmd := fmt.Sprintf("cd ~/cloudcode && docker compose exec %s %s", container, containerCmd)
			output, err := sshRunCommand(cmd.Context(), remoteCmd)
			if err != nil {
				return fmt.Errorf("执行失败: %w", err)
			}
			fmt.Print(output)
			return nil
		},
	}
}

// sshBinary 查找 ssh 可执行文件路径
func sshBinary() string {
	path, err := exec.LookPath("ssh")
	if err != nil {
		return "/usr/bin/ssh"
	}
	return path
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
