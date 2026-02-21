package alicloud

// 本文件定义阿里云 SDK 相关的错误常量和错误码检查工具。

import (
	"errors"
	"strings"
)

var (
	ErrMissingAccessKeyID     = errors.New("ALICLOUD_ACCESS_KEY_ID environment variable is not set")
	ErrMissingAccessKeySecret = errors.New("ALICLOUD_ACCESS_KEY_SECRET environment variable is not set")
	ErrNoAvailableZone        = errors.New("no available zone with sufficient stock")
	ErrECSWaitTimeout         = errors.New("timeout waiting for ECS instance to be running")
	ErrResourceNotFound       = errors.New("resource not found")
)

// isErrorCode 检查阿里云 SDK 错误是否包含指定错误码。
// 阿里云 SDK 的错误信息中嵌入了 Code 字段，通过字符串匹配判断。
func isErrorCode(err error, code string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), code)
}
