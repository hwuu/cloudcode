package alicloud

import (
	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
	stsclient "github.com/alibabacloud-go/sts-20150401/v2/client"
	vpcclient "github.com/alibabacloud-go/vpc-20160428/v6/client"
)

type STSAPI interface {
	GetCallerIdentity() (*stsclient.GetCallerIdentityResponse, error)
}

type VPCAPI interface {
	CreateVpc(req *vpcclient.CreateVpcRequest) (*vpcclient.CreateVpcResponse, error)
	DeleteVpc(req *vpcclient.DeleteVpcRequest) (*vpcclient.DeleteVpcResponse, error)
	DescribeVpcs(req *vpcclient.DescribeVpcsRequest) (*vpcclient.DescribeVpcsResponse, error)

	CreateVSwitch(req *vpcclient.CreateVSwitchRequest) (*vpcclient.CreateVSwitchResponse, error)
	DeleteVSwitch(req *vpcclient.DeleteVSwitchRequest) (*vpcclient.DeleteVSwitchResponse, error)
	DescribeVSwitches(req *vpcclient.DescribeVSwitchesRequest) (*vpcclient.DescribeVSwitchesResponse, error)

	AllocateEipAddress(req *vpcclient.AllocateEipAddressRequest) (*vpcclient.AllocateEipAddressResponse, error)
	ReleaseEipAddress(req *vpcclient.ReleaseEipAddressRequest) (*vpcclient.ReleaseEipAddressResponse, error)
	DescribeEipAddresses(req *vpcclient.DescribeEipAddressesRequest) (*vpcclient.DescribeEipAddressesResponse, error)
	AssociateEipAddress(req *vpcclient.AssociateEipAddressRequest) (*vpcclient.AssociateEipAddressResponse, error)
	UnassociateEipAddress(req *vpcclient.UnassociateEipAddressRequest) (*vpcclient.UnassociateEipAddressResponse, error)
}

type ECSAPI interface {
	CreateInstance(req *ecsclient.CreateInstanceRequest) (*ecsclient.CreateInstanceResponse, error)
	DeleteInstance(req *ecsclient.DeleteInstanceRequest) (*ecsclient.DeleteInstanceResponse, error)
	DescribeInstances(req *ecsclient.DescribeInstancesRequest) (*ecsclient.DescribeInstancesResponse, error)
	StartInstance(req *ecsclient.StartInstanceRequest) (*ecsclient.StartInstanceResponse, error)
	StopInstance(req *ecsclient.StopInstanceRequest) (*ecsclient.StopInstanceResponse, error)

	CreateKeyPair(req *ecsclient.CreateKeyPairRequest) (*ecsclient.CreateKeyPairResponse, error)
	DeleteKeyPairs(req *ecsclient.DeleteKeyPairsRequest) (*ecsclient.DeleteKeyPairsResponse, error)
	DescribeKeyPairs(req *ecsclient.DescribeKeyPairsRequest) (*ecsclient.DescribeKeyPairsResponse, error)

	DescribeZones(req *ecsclient.DescribeZonesRequest) (*ecsclient.DescribeZonesResponse, error)
	DescribeAccountAttributes(req *ecsclient.DescribeAccountAttributesRequest) (*ecsclient.DescribeAccountAttributesResponse, error)

	ImportKeyPair(req *ecsclient.ImportKeyPairRequest) (*ecsclient.ImportKeyPairResponse, error)

	CreateSecurityGroup(req *ecsclient.CreateSecurityGroupRequest) (*ecsclient.CreateSecurityGroupResponse, error)
	DeleteSecurityGroup(req *ecsclient.DeleteSecurityGroupRequest) (*ecsclient.DeleteSecurityGroupResponse, error)
	DescribeSecurityGroups(req *ecsclient.DescribeSecurityGroupsRequest) (*ecsclient.DescribeSecurityGroupsResponse, error)
	AuthorizeSecurityGroup(req *ecsclient.AuthorizeSecurityGroupRequest) (*ecsclient.AuthorizeSecurityGroupResponse, error)
}
