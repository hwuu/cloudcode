package alicloud

import (
	"errors"
)

var (
	ErrMissingAccessKeyID     = errors.New("ALICLOUD_ACCESS_KEY_ID environment variable is not set")
	ErrMissingAccessKeySecret = errors.New("ALICLOUD_ACCESS_KEY_SECRET environment variable is not set")
	ErrNoAvailableZone        = errors.New("no available zone with sufficient stock")
	ErrECSWaitTimeout         = errors.New("timeout waiting for ECS instance to be running")
	ErrResourceNotFound       = errors.New("resource not found")
)
