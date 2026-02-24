package unit

import (
	dnsclient "github.com/alibabacloud-go/alidns-20150109/v4/client"
	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
	stsclient "github.com/alibabacloud-go/sts-20150401/v2/client"
	vpcclient "github.com/alibabacloud-go/vpc-20160428/v6/client"
)

type MockSTSAPI struct {
	GetCallerIdentityFunc func() (*stsclient.GetCallerIdentityResponse, error)
}

func (m *MockSTSAPI) GetCallerIdentity() (*stsclient.GetCallerIdentityResponse, error) {
	return m.GetCallerIdentityFunc()
}

type MockVPCAPI struct {
	CreateVpcFunc         func(req *vpcclient.CreateVpcRequest) (*vpcclient.CreateVpcResponse, error)
	DeleteVpcFunc         func(req *vpcclient.DeleteVpcRequest) (*vpcclient.DeleteVpcResponse, error)
	DescribeVpcsFunc      func(req *vpcclient.DescribeVpcsRequest) (*vpcclient.DescribeVpcsResponse, error)
	CreateVSwitchFunc     func(req *vpcclient.CreateVSwitchRequest) (*vpcclient.CreateVSwitchResponse, error)
	DeleteVSwitchFunc     func(req *vpcclient.DeleteVSwitchRequest) (*vpcclient.DeleteVSwitchResponse, error)
	DescribeVSwitchesFunc func(req *vpcclient.DescribeVSwitchesRequest) (*vpcclient.DescribeVSwitchesResponse, error)
	AllocateEipAddressFunc  func(req *vpcclient.AllocateEipAddressRequest) (*vpcclient.AllocateEipAddressResponse, error)
	ReleaseEipAddressFunc   func(req *vpcclient.ReleaseEipAddressRequest) (*vpcclient.ReleaseEipAddressResponse, error)
	DescribeEipAddressesFunc func(req *vpcclient.DescribeEipAddressesRequest) (*vpcclient.DescribeEipAddressesResponse, error)
	AssociateEipAddressFunc  func(req *vpcclient.AssociateEipAddressRequest) (*vpcclient.AssociateEipAddressResponse, error)
	UnassociateEipAddressFunc func(req *vpcclient.UnassociateEipAddressRequest) (*vpcclient.UnassociateEipAddressResponse, error)
}

func (m *MockVPCAPI) CreateVpc(req *vpcclient.CreateVpcRequest) (*vpcclient.CreateVpcResponse, error) {
	return m.CreateVpcFunc(req)
}

func (m *MockVPCAPI) DeleteVpc(req *vpcclient.DeleteVpcRequest) (*vpcclient.DeleteVpcResponse, error) {
	if m.DeleteVpcFunc == nil {
		return &vpcclient.DeleteVpcResponse{}, nil
	}
	return m.DeleteVpcFunc(req)
}

func (m *MockVPCAPI) DescribeVpcs(req *vpcclient.DescribeVpcsRequest) (*vpcclient.DescribeVpcsResponse, error) {
	return m.DescribeVpcsFunc(req)
}

func (m *MockVPCAPI) CreateVSwitch(req *vpcclient.CreateVSwitchRequest) (*vpcclient.CreateVSwitchResponse, error) {
	return m.CreateVSwitchFunc(req)
}

func (m *MockVPCAPI) DeleteVSwitch(req *vpcclient.DeleteVSwitchRequest) (*vpcclient.DeleteVSwitchResponse, error) {
	if m.DeleteVSwitchFunc == nil {
		return &vpcclient.DeleteVSwitchResponse{}, nil
	}
	return m.DeleteVSwitchFunc(req)
}

func (m *MockVPCAPI) DescribeVSwitches(req *vpcclient.DescribeVSwitchesRequest) (*vpcclient.DescribeVSwitchesResponse, error) {
	return m.DescribeVSwitchesFunc(req)
}

func (m *MockVPCAPI) AllocateEipAddress(req *vpcclient.AllocateEipAddressRequest) (*vpcclient.AllocateEipAddressResponse, error) {
	return m.AllocateEipAddressFunc(req)
}

func (m *MockVPCAPI) ReleaseEipAddress(req *vpcclient.ReleaseEipAddressRequest) (*vpcclient.ReleaseEipAddressResponse, error) {
	if m.ReleaseEipAddressFunc == nil {
		return &vpcclient.ReleaseEipAddressResponse{}, nil
	}
	return m.ReleaseEipAddressFunc(req)
}

func (m *MockVPCAPI) DescribeEipAddresses(req *vpcclient.DescribeEipAddressesRequest) (*vpcclient.DescribeEipAddressesResponse, error) {
	return m.DescribeEipAddressesFunc(req)
}

func (m *MockVPCAPI) AssociateEipAddress(req *vpcclient.AssociateEipAddressRequest) (*vpcclient.AssociateEipAddressResponse, error) {
	if m.AssociateEipAddressFunc == nil {
		return &vpcclient.AssociateEipAddressResponse{}, nil
	}
	return m.AssociateEipAddressFunc(req)
}

func (m *MockVPCAPI) UnassociateEipAddress(req *vpcclient.UnassociateEipAddressRequest) (*vpcclient.UnassociateEipAddressResponse, error) {
	if m.UnassociateEipAddressFunc == nil {
		return &vpcclient.UnassociateEipAddressResponse{}, nil
	}
	return m.UnassociateEipAddressFunc(req)
}

type MockECSAPI struct {
	CreateInstanceFunc          func(req *ecsclient.CreateInstanceRequest) (*ecsclient.CreateInstanceResponse, error)
	DeleteInstanceFunc          func(req *ecsclient.DeleteInstanceRequest) (*ecsclient.DeleteInstanceResponse, error)
	DescribeInstancesFunc       func(req *ecsclient.DescribeInstancesRequest) (*ecsclient.DescribeInstancesResponse, error)
	StartInstanceFunc           func(req *ecsclient.StartInstanceRequest) (*ecsclient.StartInstanceResponse, error)
	StopInstanceFunc            func(req *ecsclient.StopInstanceRequest) (*ecsclient.StopInstanceResponse, error)
	CreateKeyPairFunc           func(req *ecsclient.CreateKeyPairRequest) (*ecsclient.CreateKeyPairResponse, error)
	DeleteKeyPairsFunc          func(req *ecsclient.DeleteKeyPairsRequest) (*ecsclient.DeleteKeyPairsResponse, error)
	DescribeKeyPairsFunc        func(req *ecsclient.DescribeKeyPairsRequest) (*ecsclient.DescribeKeyPairsResponse, error)
	DescribeZonesFunc           func(req *ecsclient.DescribeZonesRequest) (*ecsclient.DescribeZonesResponse, error)
	DescribeAccountAttributesFunc func(req *ecsclient.DescribeAccountAttributesRequest) (*ecsclient.DescribeAccountAttributesResponse, error)
	ImportKeyPairFunc           func(req *ecsclient.ImportKeyPairRequest) (*ecsclient.ImportKeyPairResponse, error)
	CreateSecurityGroupFunc     func(req *ecsclient.CreateSecurityGroupRequest) (*ecsclient.CreateSecurityGroupResponse, error)
	DeleteSecurityGroupFunc     func(req *ecsclient.DeleteSecurityGroupRequest) (*ecsclient.DeleteSecurityGroupResponse, error)
	DescribeSecurityGroupsFunc  func(req *ecsclient.DescribeSecurityGroupsRequest) (*ecsclient.DescribeSecurityGroupsResponse, error)
	AuthorizeSecurityGroupFunc  func(req *ecsclient.AuthorizeSecurityGroupRequest) (*ecsclient.AuthorizeSecurityGroupResponse, error)
	DescribeDisksFunc           func(req *ecsclient.DescribeDisksRequest) (*ecsclient.DescribeDisksResponse, error)
	CreateSnapshotFunc          func(req *ecsclient.CreateSnapshotRequest) (*ecsclient.CreateSnapshotResponse, error)
	DescribeSnapshotsFunc       func(req *ecsclient.DescribeSnapshotsRequest) (*ecsclient.DescribeSnapshotsResponse, error)
	DeleteSnapshotFunc          func(req *ecsclient.DeleteSnapshotRequest) (*ecsclient.DeleteSnapshotResponse, error)
	CreateImageFunc             func(req *ecsclient.CreateImageRequest) (*ecsclient.CreateImageResponse, error)
	DescribeImagesFunc          func(req *ecsclient.DescribeImagesRequest) (*ecsclient.DescribeImagesResponse, error)
	DeleteImageFunc             func(req *ecsclient.DeleteImageRequest) (*ecsclient.DeleteImageResponse, error)
}

func (m *MockECSAPI) CreateInstance(req *ecsclient.CreateInstanceRequest) (*ecsclient.CreateInstanceResponse, error) {
	return m.CreateInstanceFunc(req)
}

func (m *MockECSAPI) DeleteInstance(req *ecsclient.DeleteInstanceRequest) (*ecsclient.DeleteInstanceResponse, error) {
	if m.DeleteInstanceFunc == nil {
		return &ecsclient.DeleteInstanceResponse{}, nil
	}
	return m.DeleteInstanceFunc(req)
}

func (m *MockECSAPI) DescribeInstances(req *ecsclient.DescribeInstancesRequest) (*ecsclient.DescribeInstancesResponse, error) {
	return m.DescribeInstancesFunc(req)
}

func (m *MockECSAPI) StartInstance(req *ecsclient.StartInstanceRequest) (*ecsclient.StartInstanceResponse, error) {
	if m.StartInstanceFunc == nil {
		return &ecsclient.StartInstanceResponse{}, nil
	}
	return m.StartInstanceFunc(req)
}

func (m *MockECSAPI) StopInstance(req *ecsclient.StopInstanceRequest) (*ecsclient.StopInstanceResponse, error) {
	if m.StopInstanceFunc == nil {
		return &ecsclient.StopInstanceResponse{}, nil
	}
	return m.StopInstanceFunc(req)
}

func (m *MockECSAPI) CreateKeyPair(req *ecsclient.CreateKeyPairRequest) (*ecsclient.CreateKeyPairResponse, error) {
	return m.CreateKeyPairFunc(req)
}

func (m *MockECSAPI) DeleteKeyPairs(req *ecsclient.DeleteKeyPairsRequest) (*ecsclient.DeleteKeyPairsResponse, error) {
	if m.DeleteKeyPairsFunc == nil {
		return &ecsclient.DeleteKeyPairsResponse{}, nil
	}
	return m.DeleteKeyPairsFunc(req)
}

func (m *MockECSAPI) DescribeKeyPairs(req *ecsclient.DescribeKeyPairsRequest) (*ecsclient.DescribeKeyPairsResponse, error) {
	return m.DescribeKeyPairsFunc(req)
}

func (m *MockECSAPI) DescribeZones(req *ecsclient.DescribeZonesRequest) (*ecsclient.DescribeZonesResponse, error) {
	return m.DescribeZonesFunc(req)
}

func (m *MockECSAPI) DescribeAccountAttributes(req *ecsclient.DescribeAccountAttributesRequest) (*ecsclient.DescribeAccountAttributesResponse, error) {
	return m.DescribeAccountAttributesFunc(req)
}

func (m *MockECSAPI) ImportKeyPair(req *ecsclient.ImportKeyPairRequest) (*ecsclient.ImportKeyPairResponse, error) {
	return m.ImportKeyPairFunc(req)
}

func (m *MockECSAPI) CreateSecurityGroup(req *ecsclient.CreateSecurityGroupRequest) (*ecsclient.CreateSecurityGroupResponse, error) {
	return m.CreateSecurityGroupFunc(req)
}

func (m *MockECSAPI) DeleteSecurityGroup(req *ecsclient.DeleteSecurityGroupRequest) (*ecsclient.DeleteSecurityGroupResponse, error) {
	if m.DeleteSecurityGroupFunc == nil {
		return &ecsclient.DeleteSecurityGroupResponse{}, nil
	}
	return m.DeleteSecurityGroupFunc(req)
}

func (m *MockECSAPI) DescribeSecurityGroups(req *ecsclient.DescribeSecurityGroupsRequest) (*ecsclient.DescribeSecurityGroupsResponse, error) {
	return m.DescribeSecurityGroupsFunc(req)
}

func (m *MockECSAPI) AuthorizeSecurityGroup(req *ecsclient.AuthorizeSecurityGroupRequest) (*ecsclient.AuthorizeSecurityGroupResponse, error) {
	if m.AuthorizeSecurityGroupFunc == nil {
		return &ecsclient.AuthorizeSecurityGroupResponse{}, nil
	}
	return m.AuthorizeSecurityGroupFunc(req)
}

func (m *MockECSAPI) DescribeDisks(req *ecsclient.DescribeDisksRequest) (*ecsclient.DescribeDisksResponse, error) {
	if m.DescribeDisksFunc == nil {
		return &ecsclient.DescribeDisksResponse{}, nil
	}
	return m.DescribeDisksFunc(req)
}

func (m *MockECSAPI) CreateSnapshot(req *ecsclient.CreateSnapshotRequest) (*ecsclient.CreateSnapshotResponse, error) {
	if m.CreateSnapshotFunc == nil {
		return &ecsclient.CreateSnapshotResponse{}, nil
	}
	return m.CreateSnapshotFunc(req)
}

func (m *MockECSAPI) DescribeSnapshots(req *ecsclient.DescribeSnapshotsRequest) (*ecsclient.DescribeSnapshotsResponse, error) {
	if m.DescribeSnapshotsFunc == nil {
		return &ecsclient.DescribeSnapshotsResponse{}, nil
	}
	return m.DescribeSnapshotsFunc(req)
}

func (m *MockECSAPI) DeleteSnapshot(req *ecsclient.DeleteSnapshotRequest) (*ecsclient.DeleteSnapshotResponse, error) {
	if m.DeleteSnapshotFunc == nil {
		return &ecsclient.DeleteSnapshotResponse{}, nil
	}
	return m.DeleteSnapshotFunc(req)
}

func (m *MockECSAPI) CreateImage(req *ecsclient.CreateImageRequest) (*ecsclient.CreateImageResponse, error) {
	if m.CreateImageFunc == nil {
		return &ecsclient.CreateImageResponse{}, nil
	}
	return m.CreateImageFunc(req)
}

func (m *MockECSAPI) DescribeImages(req *ecsclient.DescribeImagesRequest) (*ecsclient.DescribeImagesResponse, error) {
	if m.DescribeImagesFunc == nil {
		return &ecsclient.DescribeImagesResponse{}, nil
	}
	return m.DescribeImagesFunc(req)
}

func (m *MockECSAPI) DeleteImage(req *ecsclient.DeleteImageRequest) (*ecsclient.DeleteImageResponse, error) {
	if m.DeleteImageFunc == nil {
		return &ecsclient.DeleteImageResponse{}, nil
	}
	return m.DeleteImageFunc(req)
}

type MockDnsAPI struct {
	DescribeDomainsFunc       func(req *dnsclient.DescribeDomainsRequest) (*dnsclient.DescribeDomainsResponse, error)
	DescribeDomainRecordsFunc func(req *dnsclient.DescribeDomainRecordsRequest) (*dnsclient.DescribeDomainRecordsResponse, error)
	AddDomainRecordFunc       func(req *dnsclient.AddDomainRecordRequest) (*dnsclient.AddDomainRecordResponse, error)
	UpdateDomainRecordFunc    func(req *dnsclient.UpdateDomainRecordRequest) (*dnsclient.UpdateDomainRecordResponse, error)
}

func (m *MockDnsAPI) DescribeDomains(req *dnsclient.DescribeDomainsRequest) (*dnsclient.DescribeDomainsResponse, error) {
	return m.DescribeDomainsFunc(req)
}

func (m *MockDnsAPI) DescribeDomainRecords(req *dnsclient.DescribeDomainRecordsRequest) (*dnsclient.DescribeDomainRecordsResponse, error) {
	return m.DescribeDomainRecordsFunc(req)
}

func (m *MockDnsAPI) AddDomainRecord(req *dnsclient.AddDomainRecordRequest) (*dnsclient.AddDomainRecordResponse, error) {
	return m.AddDomainRecordFunc(req)
}

func (m *MockDnsAPI) UpdateDomainRecord(req *dnsclient.UpdateDomainRecordRequest) (*dnsclient.UpdateDomainRecordResponse, error) {
	return m.UpdateDomainRecordFunc(req)
}
