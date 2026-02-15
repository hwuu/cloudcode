package alicloud

import (
	"context"
	"fmt"
	"time"

	vpcclient "github.com/alibabacloud-go/vpc-20160428/v6/client"
)

type EIPResource struct {
	ID     string
	IP     string
	Status string
}

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

func ReleaseEIP(vpcCli VPCAPI, allocationID string) error {
	req := &vpcclient.ReleaseEipAddressRequest{
		AllocationId: &allocationID,
	}
	_, err := vpcCli.ReleaseEipAddress(req)
	return err
}

func AssociateEIPToInstance(vpcCli VPCAPI, allocationID, instanceID, regionID string) error {
	req := &vpcclient.AssociateEipAddressRequest{
		AllocationId: &allocationID,
		InstanceId:   &instanceID,
		RegionId:     &regionID,
	}
	_, err := vpcCli.AssociateEipAddress(req)
	return err
}

func UnassociateEIPFromInstance(vpcCli VPCAPI, allocationID, instanceID, regionID string) error {
	req := &vpcclient.UnassociateEipAddressRequest{
		AllocationId: &allocationID,
		InstanceId:   &instanceID,
		RegionId:     &regionID,
	}
	_, err := vpcCli.UnassociateEipAddress(req)
	return err
}

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
