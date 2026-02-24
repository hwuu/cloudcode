package alicloud

// 本文件定义阿里云 SDK 客户端的接口抽象，用于依赖注入和 mock 测试。
// 每个接口对应一个阿里云产品 SDK，仅暴露 CloudCode 实际使用的方法。

import (
	dnsclient "github.com/alibabacloud-go/alidns-20150109/v4/client"
	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
	stsclient "github.com/alibabacloud-go/sts-20150401/v2/client"
	vpcclient "github.com/alibabacloud-go/vpc-20160428/v6/client"
)

// STSAPI 安全令牌服务接口，用于验证阿里云凭证
type STSAPI interface {
	GetCallerIdentity() (*stsclient.GetCallerIdentityResponse, error)
}

// VPCAPI 专有网络接口，管理 VPC/VSwitch/EIP 资源
type VPCAPI interface {
	// VPC 管理
	CreateVpc(req *vpcclient.CreateVpcRequest) (*vpcclient.CreateVpcResponse, error)
	DeleteVpc(req *vpcclient.DeleteVpcRequest) (*vpcclient.DeleteVpcResponse, error)
	DescribeVpcs(req *vpcclient.DescribeVpcsRequest) (*vpcclient.DescribeVpcsResponse, error)

	// VSwitch（交换机）管理
	CreateVSwitch(req *vpcclient.CreateVSwitchRequest) (*vpcclient.CreateVSwitchResponse, error)
	DeleteVSwitch(req *vpcclient.DeleteVSwitchRequest) (*vpcclient.DeleteVSwitchResponse, error)
	DescribeVSwitches(req *vpcclient.DescribeVSwitchesRequest) (*vpcclient.DescribeVSwitchesResponse, error)

	// EIP（弹性公网 IP）管理
	AllocateEipAddress(req *vpcclient.AllocateEipAddressRequest) (*vpcclient.AllocateEipAddressResponse, error)
	ReleaseEipAddress(req *vpcclient.ReleaseEipAddressRequest) (*vpcclient.ReleaseEipAddressResponse, error)
	DescribeEipAddresses(req *vpcclient.DescribeEipAddressesRequest) (*vpcclient.DescribeEipAddressesResponse, error)
	AssociateEipAddress(req *vpcclient.AssociateEipAddressRequest) (*vpcclient.AssociateEipAddressResponse, error)
	UnassociateEipAddress(req *vpcclient.UnassociateEipAddressRequest) (*vpcclient.UnassociateEipAddressResponse, error)
}

// ECSAPI 云服务器接口，管理 ECS 实例/安全组/SSH 密钥对/可用区/快照
type ECSAPI interface {
	// ECS 实例生命周期
	CreateInstance(req *ecsclient.CreateInstanceRequest) (*ecsclient.CreateInstanceResponse, error)
	DeleteInstance(req *ecsclient.DeleteInstanceRequest) (*ecsclient.DeleteInstanceResponse, error)
	DescribeInstances(req *ecsclient.DescribeInstancesRequest) (*ecsclient.DescribeInstancesResponse, error)
	StartInstance(req *ecsclient.StartInstanceRequest) (*ecsclient.StartInstanceResponse, error)
	StopInstance(req *ecsclient.StopInstanceRequest) (*ecsclient.StopInstanceResponse, error)

	// SSH 密钥对管理
	CreateKeyPair(req *ecsclient.CreateKeyPairRequest) (*ecsclient.CreateKeyPairResponse, error)
	DeleteKeyPairs(req *ecsclient.DeleteKeyPairsRequest) (*ecsclient.DeleteKeyPairsResponse, error)
	DescribeKeyPairs(req *ecsclient.DescribeKeyPairsRequest) (*ecsclient.DescribeKeyPairsResponse, error)
	ImportKeyPair(req *ecsclient.ImportKeyPairRequest) (*ecsclient.ImportKeyPairResponse, error)

	// 可用区与账号属性查询
	DescribeZones(req *ecsclient.DescribeZonesRequest) (*ecsclient.DescribeZonesResponse, error)
	DescribeAccountAttributes(req *ecsclient.DescribeAccountAttributesRequest) (*ecsclient.DescribeAccountAttributesResponse, error)

	// 安全组管理
	CreateSecurityGroup(req *ecsclient.CreateSecurityGroupRequest) (*ecsclient.CreateSecurityGroupResponse, error)
	DeleteSecurityGroup(req *ecsclient.DeleteSecurityGroupRequest) (*ecsclient.DeleteSecurityGroupResponse, error)
	DescribeSecurityGroups(req *ecsclient.DescribeSecurityGroupsRequest) (*ecsclient.DescribeSecurityGroupsResponse, error)
	AuthorizeSecurityGroup(req *ecsclient.AuthorizeSecurityGroupRequest) (*ecsclient.AuthorizeSecurityGroupResponse, error)

	// 磁盘与快照管理
	DescribeDisks(req *ecsclient.DescribeDisksRequest) (*ecsclient.DescribeDisksResponse, error)
	CreateSnapshot(req *ecsclient.CreateSnapshotRequest) (*ecsclient.CreateSnapshotResponse, error)
	DescribeSnapshots(req *ecsclient.DescribeSnapshotsRequest) (*ecsclient.DescribeSnapshotsResponse, error)
	DeleteSnapshot(req *ecsclient.DeleteSnapshotRequest) (*ecsclient.DeleteSnapshotResponse, error)

	// 自定义镜像管理（快照恢复时使用）
	CreateImage(req *ecsclient.CreateImageRequest) (*ecsclient.CreateImageResponse, error)
	DescribeImages(req *ecsclient.DescribeImagesRequest) (*ecsclient.DescribeImagesResponse, error)
	DeleteImage(req *ecsclient.DeleteImageRequest) (*ecsclient.DeleteImageResponse, error)
}

// DnsAPI 云解析 DNS 接口，管理域名和解析记录
type DnsAPI interface {
	DescribeDomains(req *dnsclient.DescribeDomainsRequest) (*dnsclient.DescribeDomainsResponse, error)
	DescribeDomainRecords(req *dnsclient.DescribeDomainRecordsRequest) (*dnsclient.DescribeDomainRecordsResponse, error)
	AddDomainRecord(req *dnsclient.AddDomainRecordRequest) (*dnsclient.AddDomainRecordResponse, error)
	UpdateDomainRecord(req *dnsclient.UpdateDomainRecordRequest) (*dnsclient.UpdateDomainRecordResponse, error)
}
