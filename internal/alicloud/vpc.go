package alicloud

// 本文件管理 VPC 网络资源：VPC、VSwitch（交换机）、安全组。
// VPC 是阿里云的虚拟专有网络，ECS 实例必须部署在 VPC 内。

import (
	"fmt"
	"time"

	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
	vpcclient "github.com/alibabacloud-go/vpc-20160428/v6/client"
)

const (
	DefaultVPCCIDR = "192.168.0.0/16" // VPC 默认网段
)

// VPCResource VPC 资源信息
type VPCResource struct {
	ID   string
	CIDR string
}

// VSwitchResource 交换机资源信息（VPC 内的子网，绑定到特定可用区）
type VSwitchResource struct {
	ID     string
	ZoneID string
	CIDR   string
}

// SecurityGroupResource 安全组资源信息（控制 ECS 实例的入站/出站规则）
type SecurityGroupResource struct {
	ID string
}

// CreateVPC 创建 VPC（默认网段 192.168.0.0/16）
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

// WaitVPCAvailable 等待 VPC 状态变为 Available。
// VPC 创建后需要等待就绪才能创建 VSwitch，否则会报 DependencyViolation。
func WaitVPCAvailable(vpcCli VPCAPI, vpcID, regionID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req := &vpcclient.DescribeVpcsRequest{
			VpcId:    &vpcID,
			RegionId: &regionID,
		}
		resp, err := vpcCli.DescribeVpcs(req)
		if err != nil {
			return err
		}
		if resp.Body != nil && resp.Body.Vpcs != nil && resp.Body.Vpcs.Vpc != nil && len(resp.Body.Vpcs.Vpc) > 0 {
			status := resp.Body.Vpcs.Vpc[0].Status
			if status != nil && *status == "Available" {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("VPC %s 未在 %v 内就绪", vpcID, timeout)
}

// DeleteVPC 删除 VPC（必须先删除其下所有 VSwitch）
func DeleteVPC(vpcCli VPCAPI, vpcID string) error {
	req := &vpcclient.DeleteVpcRequest{
		VpcId: &vpcID,
	}
	_, err := vpcCli.DeleteVpc(req)
	return err
}

// DescribeVPC 查询 VPC 详情
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

// CreateVSwitch 在指定 VPC 和可用区内创建交换机（子网）
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

// DeleteVSwitch 删除交换机
func DeleteVSwitch(vpcCli VPCAPI, vswitchID string) error {
	req := &vpcclient.DeleteVSwitchRequest{
		VSwitchId: &vswitchID,
	}
	_, err := vpcCli.DeleteVSwitch(req)
	return err
}

// CreateSecurityGroup 在指定 VPC 内创建安全组（注意：安全组 API 属于 ECS SDK）
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

// DeleteSecurityGroup 删除安全组
func DeleteSecurityGroup(ecsCli ECSAPI, sgID, regionID string) error {
	req := &ecsclient.DeleteSecurityGroupRequest{
		SecurityGroupId: &sgID,
		RegionId:        &regionID,
	}
	_, err := ecsCli.DeleteSecurityGroup(req)
	return err
}

// SecurityGroupRule 安全组入站规则
type SecurityGroupRule struct {
	Protocol    string // 协议：TCP/UDP/ICMP
	PortRange   string // 端口范围，格式 "起始端口/结束端口"，如 "22/22"
	SourceCIDR  string // 允许的源 IP 段，如 "0.0.0.0/0" 表示所有
	Description string // 规则描述
}

// AuthorizeSecurityGroupIngress 批量添加安全组入站规则
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

// DefaultSecurityGroupRules 返回 CloudCode 默认的安全组规则：SSH(22)/HTTP(80)/HTTPS(443)。
// 如果指定了 sshIP，SSH 端口仅允许该 IP 访问；否则对所有 IP 开放。
func DefaultSecurityGroupRules(sshIP string) []SecurityGroupRule {
	sshSource := sshIP
	if sshSource == "" {
		sshSource = "0.0.0.0/0"
	}

	return []SecurityGroupRule{
		{Protocol: "TCP", PortRange: "22/22", SourceCIDR: sshSource, Description: "SSH"},
		{Protocol: "TCP", PortRange: "80/80", SourceCIDR: "0.0.0.0/0", Description: "HTTP"},
		{Protocol: "TCP", PortRange: "443/443", SourceCIDR: "0.0.0.0/0", Description: "HTTPS"},
		{Protocol: "TCP", PortRange: "8443/8443", SourceCIDR: "0.0.0.0/0", Description: "HTTPS (备用端口)"},
	}
}
