package alicloud

import (
	"fmt"
)

type CallerIdentity struct {
	AccountID string
	UserID    string
	ARN       string
}

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
