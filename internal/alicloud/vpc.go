package alicloud

import (
	"fmt"

	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
	vpcclient "github.com/alibabacloud-go/vpc-20160428/v6/client"
)

const (
	DefaultVPCCIDR = "192.168.0.0/16"
)

type VPCResource struct {
	ID   string
	CIDR string
}

type VSwitchResource struct {
	ID     string
	ZoneID string
	CIDR   string
}

type SecurityGroupResource struct {
	ID string
}

func CreateVPC(vpcCli VPCAPI, regionID, vpcName string) (*VPCResource, error) {
	cidr := DefaultVPCCIDR
	req := &vpcclient.CreateVpcRequest{
		RegionId:  &regionID,
		CidrBlock: &cidr,
	}
	if vpcName != "" {
		req.VpcName = &vpcName
	}

	resp, err := vpcCli.CreateVpc(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create VPC: %w", err)
	}

	if resp == nil || resp.Body == nil || resp.Body.VpcId == nil {
		return nil, fmt.Errorf("invalid response from CreateVpc")
	}

	return &VPCResource{
		ID:   *resp.Body.VpcId,
		CIDR: cidr,
	}, nil
}

func DeleteVPC(vpcCli VPCAPI, vpcID string) error {
	req := &vpcclient.DeleteVpcRequest{
		VpcId: &vpcID,
	}
	_, err := vpcCli.DeleteVpc(req)
	return err
}

func DescribeVPC(vpcCli VPCAPI, vpcID, regionID string) (*VPCResource, error) {
	req := &vpcclient.DescribeVpcsRequest{
		VpcId:    &vpcID,
		RegionId: &regionID,
	}
	resp, err := vpcCli.DescribeVpcs(req)
	if err != nil {
		return nil, err
	}

	if resp == nil || resp.Body == nil || resp.Body.Vpcs == nil ||
		resp.Body.Vpcs.Vpc == nil || len(resp.Body.Vpcs.Vpc) == 0 {
		return nil, ErrResourceNotFound
	}

	vpc := resp.Body.Vpcs.Vpc[0]
	return &VPCResource{
		ID:   *vpc.VpcId,
		CIDR: *vpc.CidrBlock,
	}, nil
}

func CreateVSwitch(vpcCli VPCAPI, vpcID, zoneID, cidr, vswitchName string) (*VSwitchResource, error) {
	req := &vpcclient.CreateVSwitchRequest{
		VpcId:     &vpcID,
		ZoneId:    &zoneID,
		CidrBlock: &cidr,
	}
	if vswitchName != "" {
		req.VSwitchName = &vswitchName
	}

	resp, err := vpcCli.CreateVSwitch(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create VSwitch: %w", err)
	}

	if resp == nil || resp.Body == nil || resp.Body.VSwitchId == nil {
		return nil, fmt.Errorf("invalid response from CreateVSwitch")
	}

	return &VSwitchResource{
		ID:     *resp.Body.VSwitchId,
		ZoneID: zoneID,
		CIDR:   cidr,
	}, nil
}

func DeleteVSwitch(vpcCli VPCAPI, vswitchID string) error {
	req := &vpcclient.DeleteVSwitchRequest{
		VSwitchId: &vswitchID,
	}
	_, err := vpcCli.DeleteVSwitch(req)
	return err
}

func CreateSecurityGroup(ecsCli ECSAPI, vpcID, regionID, sgName string) (*SecurityGroupResource, error) {
	req := &ecsclient.CreateSecurityGroupRequest{
		VpcId:    &vpcID,
		RegionId: &regionID,
	}
	if sgName != "" {
		req.SecurityGroupName = &sgName
	}

	resp, err := ecsCli.CreateSecurityGroup(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create security group: %w", err)
	}

	if resp == nil || resp.Body == nil || resp.Body.SecurityGroupId == nil {
		return nil, fmt.Errorf("invalid response from CreateSecurityGroup")
	}

	return &SecurityGroupResource{
		ID: *resp.Body.SecurityGroupId,
	}, nil
}

func DeleteSecurityGroup(ecsCli ECSAPI, sgID, regionID string) error {
	req := &ecsclient.DeleteSecurityGroupRequest{
		SecurityGroupId: &sgID,
		RegionId:        &regionID,
	}
	_, err := ecsCli.DeleteSecurityGroup(req)
	return err
}

type SecurityGroupRule struct {
	Protocol    string
	PortRange   string
	SourceCIDR  string
	Description string
}

func AuthorizeSecurityGroupIngress(ecsCli ECSAPI, sgID, regionID string, rules []SecurityGroupRule) error {
	for _, rule := range rules {
		req := &ecsclient.AuthorizeSecurityGroupRequest{
			SecurityGroupId: &sgID,
			RegionId:        &regionID,
			IpProtocol:      &rule.Protocol,
			PortRange:       &rule.PortRange,
			SourceCidrIp:    &rule.SourceCIDR,
		}
		if rule.Description != "" {
			req.Description = &rule.Description
		}

		if _, err := ecsCli.AuthorizeSecurityGroup(req); err != nil {
			return fmt.Errorf("failed to authorize security group rule: %w", err)
		}
	}
	return nil
}

func DefaultSecurityGroupRules(sshIP string) []SecurityGroupRule {
	sshSource := sshIP
	if sshSource == "" {
		sshSource = "0.0.0.0/0"
	}

	return []SecurityGroupRule{
		{Protocol: "TCP", PortRange: "22/22", SourceCIDR: sshSource, Description: "SSH"},
		{Protocol: "TCP", PortRange: "80/80", SourceCIDR: "0.0.0.0/0", Description: "HTTP"},
		{Protocol: "TCP", PortRange: "443/443", SourceCIDR: "0.0.0.0/0", Description: "HTTPS"},
	}
}
