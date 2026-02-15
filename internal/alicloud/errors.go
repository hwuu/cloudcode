package alicloud

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

// isErrorCode 检查阿里云 SDK 错误是否包含指定错误码
func isErrorCode(err error, code string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), code)
}
