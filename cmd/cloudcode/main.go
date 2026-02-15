package main

import (
	"fmt"
	"os"
	"runtime"

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
	return &cobra.Command{
		Use:   "deploy",
		Short: "部署 OpenCode 到阿里云 ECS",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("deploy: 尚未实现")
			return nil
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看部署状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("status: 尚未实现")
			return nil
		},
	}
}

func newDestroyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "destroy",
		Short: "销毁所有云资源",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("destroy: 尚未实现")
			return nil
		},
	}
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
