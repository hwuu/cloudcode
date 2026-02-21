package alicloud

import (
	"fmt"
)

// CallerIdentity 阿里云账号身份信息，由 STS GetCallerIdentity 返回
type CallerIdentity struct {
	AccountID string // 阿里云主账号 ID
	UserID    string // RAM 用户 ID
	ARN       string // 资源名称（ARN）
}

// GetCallerIdentity 调用 STS 验证当前凭证，返回账号身份信息。
// 用于部署前的前置检查，确认 AccessKey 有效。
func GetCallerIdentity(stsCli STSAPI) (*CallerIdentity, error) {
	resp, err := stsCli.GetCallerIdentity()
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %w", err)
	}

	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("empty response from GetCallerIdentity")
	}

	return &CallerIdentity{
		AccountID: *resp.Body.AccountId,
		UserID:    *resp.Body.UserId,
		ARN:       *resp.Body.Arn,
	}, nil
}
