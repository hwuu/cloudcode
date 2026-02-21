// Package remote 封装 SSH/SFTP 远程操作，用于连接 ECS 实例执行命令和上传文件。
// 通过接口抽象（SSHClient/SFTPClient）支持 mock 测试。
package remote

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrSSHWaitTimeout = errors.New("timeout waiting for SSH connection")
)

// 超时常量
const (
	DefaultInitialInterval = 1 * time.Second   // SSH 重试初始间隔
	DefaultMaxInterval     = 10 * time.Second   // SSH 重试最大间隔（指数退避上限）
	DefaultSSHTimeout      = 2 * time.Minute    // SSH 连接总超时
	DefaultCommandTimeout  = 5 * time.Minute    // 单条命令执行超时
	DockerInstallTimeout   = 10 * time.Minute   // Docker 安装超时（含下载）
)

// SSHClient 抽象 SSH 连接，支持 mock 测试
type SSHClient interface {
	RunCommand(ctx context.Context, cmd string) (string, error)
	Close() error
}

// DialFunc 用于建立 SSH 连接的函数类型（工厂模式，每次调用创建新连接）
type DialFunc func() (SSHClient, error)

// WaitSSHOptions 配置 WaitForSSH 的重试参数
type WaitSSHOptions struct {
	InitialInterval time.Duration // 首次重试间隔
	MaxInterval     time.Duration // 最大重试间隔
	Timeout         time.Duration // 总超时时间
}

func (o *WaitSSHOptions) withDefaults() {
	if o.InitialInterval == 0 {
		o.InitialInterval = DefaultInitialInterval
	}
	if o.MaxInterval == 0 {
		o.MaxInterval = DefaultMaxInterval
	}
	if o.Timeout == 0 {
		o.Timeout = DefaultSSHTimeout
	}
}

// WaitForSSH 使用指数退避重试连接 SSH，直到成功或超时
func WaitForSSH(ctx context.Context, dial DialFunc, opts WaitSSHOptions) (SSHClient, error) {
	opts.withDefaults()

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	interval := opts.InitialInterval

	for {
		client, err := dial()
		if err == nil {
			return client, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: last error: %v", ErrSSHWaitTimeout, err)
		case <-time.After(interval):
			// 指数退避，不超过 MaxInterval
			interval = interval * 2
			if interval > opts.MaxInterval {
				interval = opts.MaxInterval
			}
		}
	}
}
