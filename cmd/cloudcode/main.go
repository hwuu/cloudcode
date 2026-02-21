// Package main 是 CloudCode CLI 的入口。
// 提供 4 个子命令：deploy（部署）、status（状态）、destroy（销毁）、version（版本）。
// 版本信息通过 ldflags 在构建时注入。
package main

import (
	"fmt"
	"os"
	"runtime"

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

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
