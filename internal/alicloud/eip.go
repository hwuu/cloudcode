package alicloud

// 本文件管理弹性公网 IP（EIP）的分配、绑定、解绑、释放和状态查询。
// EIP 绑定到 ECS 实例后，用户通过该 IP 访问部署的服务。

import (
	"context"
	"fmt"
	"time"

	vpcclient "github.com/alibabacloud-go/vpc-20160428/v6/client"
)

// EIPResource EIP 资源信息
type EIPResource struct {
	ID     string // EIP 分配 ID（AllocationId）
	IP     string // 弹性公网 IP 地址
	Status string // 状态：Available（未绑定）/ InUse（已绑定）
}

// AllocateEIP 分配一个按流量计费的 EIP（带宽 5Mbps）
func AllocateEIP(vpcCli VPCAPI, regionID, eipName string) (*EIPResource, error) {
	req := &vpcclient.AllocateEipAddressRequest{
		RegionId:           &regionID,
		Bandwidth:          teaString("5"),
		InternetChargeType: teaString("PayByTraffic"),
	}
	if eipName != "" {
		req.Name = &eipName
	}

	resp, err := vpcCli.AllocateEipAddress(req)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate EIP: %w", err)
	}

	if resp == nil || resp.Body == nil || resp.Body.AllocationId == nil {
		return nil, fmt.Errorf("invalid response from AllocateEipAddress")
	}

	return &EIPResource{
		ID: *resp.Body.AllocationId,
		IP: *resp.Body.EipAddress,
	}, nil
}

// ReleaseEIP 释放指定 EIP（必须先解绑）
func ReleaseEIP(vpcCli VPCAPI, allocationID string) error {
	req := &vpcclient.ReleaseEipAddressRequest{
		AllocationId: &allocationID,
	}
	_, err := vpcCli.ReleaseEipAddress(req)
	return err
}

// AssociateEIPToInstance 将 EIP 绑定到 ECS 实例
func AssociateEIPToInstance(vpcCli VPCAPI, allocationID, instanceID, regionID string) error {
	req := &vpcclient.AssociateEipAddressRequest{
		AllocationId: &allocationID,
		InstanceId:   &instanceID,
		RegionId:     &regionID,
	}
	_, err := vpcCli.AssociateEipAddress(req)
	return err
}

// UnassociateEIPFromInstance 将 EIP 从 ECS 实例解绑
func UnassociateEIPFromInstance(vpcCli VPCAPI, allocationID, instanceID, regionID string) error {
	req := &vpcclient.UnassociateEipAddressRequest{
		AllocationId: &allocationID,
		InstanceId:   &instanceID,
		RegionId:     &regionID,
	}
	_, err := vpcCli.UnassociateEipAddress(req)
	return err
}

// DescribeEIP 查询 EIP 详情
func DescribeEIP(vpcCli VPCAPI, allocationID, regionID string) (*EIPResource, error) {
	req := &vpcclient.DescribeEipAddressesRequest{
		AllocationId: &allocationID,
		RegionId:     &regionID,
	}

	resp, err := vpcCli.DescribeEipAddresses(req)
	if err != nil {
		return nil, err
	}

	if resp == nil || resp.Body == nil || resp.Body.EipAddresses == nil ||
		resp.Body.EipAddresses.EipAddress == nil || len(resp.Body.EipAddresses.EipAddress) == 0 {
		return nil, ErrResourceNotFound
	}

	eip := resp.Body.EipAddresses.EipAddress[0]
	status := ""
	if eip.Status != nil {
		status = *eip.Status
	}

	return &EIPResource{
		ID:     *eip.AllocationId,
		IP:     *eip.IpAddress,
		Status: status,
	}, nil
}

const (
	DefaultEIPWaitInterval = 2 * time.Second
	DefaultEIPWaitTimeout  = 2 * time.Minute
)

// WaitForEIPBound 轮询等待 EIP 状态变为 InUse（已绑定到实例）。
// 使用 context + ticker 模式，避免紧密循环。
func WaitForEIPBound(ctx context.Context, vpcCli VPCAPI, allocationID, regionID string, interval, timeout time.Duration) error {
	if interval == 0 {
		interval = DefaultEIPWaitInterval
	}
	if timeout == 0 {
		timeout = DefaultEIPWaitTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	associated := "InUse"

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for EIP to be bound")
		case <-ticker.C:
			req := &vpcclient.DescribeEipAddressesRequest{
				AllocationId: &allocationID,
				RegionId:     &regionID,
			}

			resp, err := vpcCli.DescribeEipAddresses(req)
			if err != nil {
				continue
			}

			if resp != nil && resp.Body != nil && resp.Body.EipAddresses != nil &&
				resp.Body.EipAddresses.EipAddress != nil && len(resp.Body.EipAddresses.EipAddress) > 0 {
				eip := resp.Body.EipAddresses.EipAddress[0]
				if eip.Status != nil && *eip.Status == associated {
					return nil
				}
			}
		}
	}
}
