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

const (
	DefaultInitialInterval = 1 * time.Second
	DefaultMaxInterval     = 10 * time.Second
	DefaultSSHTimeout      = 2 * time.Minute
	DefaultCommandTimeout  = 5 * time.Minute
	DockerInstallTimeout   = 10 * time.Minute
)

// SSHClient 抽象 SSH 连接，支持 mock 测试
type SSHClient interface {
	RunCommand(ctx context.Context, cmd string) (string, error)
	Close() error
}

// DialFunc 用于建立 SSH 连接的函数类型
type DialFunc func() (SSHClient, error)

// WaitSSHOptions 配置 WaitForSSH 的重试参数
type WaitSSHOptions struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Timeout         time.Duration
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
