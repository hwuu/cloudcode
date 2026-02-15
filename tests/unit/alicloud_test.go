package unit

import (
	"context"
	"errors"
	"testing"
	"time"

	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
	stsclient "github.com/alibabacloud-go/sts-20150401/v2/client"
	vpcclient "github.com/alibabacloud-go/vpc-20160428/v6/client"
	"github.com/hwuu/cloudcode/internal/alicloud"
)

func TestLoadConfigFromEnv_MissingAccessKeyID(t *testing.T) {
	cfg, err := alicloud.LoadConfigFromEnv()
	if err == nil {
		t.Error("expected error when ALICLOUD_ACCESS_KEY_ID is not set")
	}
	if cfg != nil {
		t.Error("expected nil config when error occurs")
	}
	if !errors.Is(err, alicloud.ErrMissingAccessKeyID) {
		t.Errorf("expected ErrMissingAccessKeyID, got %v", err)
	}
}

func TestGetCallerIdentity_Success(t *testing.T) {
	accountID := "123456789"
	userID := "test-user"
	arn := "acs:ram::123456789:user/test"

	mockSTS := &MockSTSAPI{
		GetCallerIdentityFunc: func() (*stsclient.GetCallerIdentityResponse, error) {
			return &stsclient.GetCallerIdentityResponse{
				Body: &stsclient.GetCallerIdentityResponseBody{
					AccountId: &accountID,
					UserId:    &userID,
					Arn:       &arn,
				},
			}, nil
		},
	}

	identity, err := alicloud.GetCallerIdentity(mockSTS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if identity.AccountID != accountID {
		t.Errorf("expected AccountID %s, got %s", accountID, identity.AccountID)
	}
	if identity.UserID != userID {
		t.Errorf("expected UserID %s, got %s", userID, identity.UserID)
	}
	if identity.ARN != arn {
		t.Errorf("expected ARN %s, got %s", arn, identity.ARN)
	}
}

func TestGetCallerIdentity_Error(t *testing.T) {
	mockSTS := &MockSTSAPI{
		GetCallerIdentityFunc: func() (*stsclient.GetCallerIdentityResponse, error) {
			return nil, errors.New("STS error")
		},
	}

	_, err := alicloud.GetCallerIdentity(mockSTS)
	if err == nil {
		t.Error("expected error from GetCallerIdentity")
	}
}

func TestCreateVPC_Success(t *testing.T) {
	vpcID := "vpc-xxx"
	regionID := "ap-southeast-1"

	mockVPC := &MockVPCAPI{
		CreateVpcFunc: func(req *vpcclient.CreateVpcRequest) (*vpcclient.CreateVpcResponse, error) {
			if *req.RegionId != regionID {
				t.Errorf("expected RegionId %s, got %s", regionID, *req.RegionId)
			}
			return &vpcclient.CreateVpcResponse{
				Body: &vpcclient.CreateVpcResponseBody{
					VpcId: &vpcID,
				},
			}, nil
		},
	}

	vpc, err := alicloud.CreateVPC(mockVPC, regionID, "test-vpc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vpc.ID != vpcID {
		t.Errorf("expected VPC ID %s, got %s", vpcID, vpc.ID)
	}
	if vpc.CIDR != alicloud.DefaultVPCCIDR {
		t.Errorf("expected CIDR %s, got %s", alicloud.DefaultVPCCIDR, vpc.CIDR)
	}
}

func TestCreateVSwitch_Success(t *testing.T) {
	vpcID := "vpc-xxx"
	zoneID := "ap-southeast-1a"
	vswitchID := "vsw-xxx"
	cidr := "192.168.1.0/24"

	mockVPC := &MockVPCAPI{
		CreateVSwitchFunc: func(req *vpcclient.CreateVSwitchRequest) (*vpcclient.CreateVSwitchResponse, error) {
			if *req.VpcId != vpcID {
				t.Errorf("expected VpcId %s, got %s", vpcID, *req.VpcId)
			}
			if *req.ZoneId != zoneID {
				t.Errorf("expected ZoneId %s, got %s", zoneID, *req.ZoneId)
			}
			return &vpcclient.CreateVSwitchResponse{
				Body: &vpcclient.CreateVSwitchResponseBody{
					VSwitchId: &vswitchID,
				},
			}, nil
		},
	}

	vswitch, err := alicloud.CreateVSwitch(mockVPC, vpcID, zoneID, cidr, "test-vswitch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vswitch.ID != vswitchID {
		t.Errorf("expected VSwitch ID %s, got %s", vswitchID, vswitch.ID)
	}
}

func TestCreateSecurityGroup_Success(t *testing.T) {
	sgID := "sg-xxx"
	vpcID := "vpc-xxx"
	regionID := "ap-southeast-1"

	mockECS := &MockECSAPI{
		CreateSecurityGroupFunc: func(req *ecsclient.CreateSecurityGroupRequest) (*ecsclient.CreateSecurityGroupResponse, error) {
			if *req.VpcId != vpcID {
				t.Errorf("expected VpcId %s, got %s", vpcID, *req.VpcId)
			}
			return &ecsclient.CreateSecurityGroupResponse{
				Body: &ecsclient.CreateSecurityGroupResponseBody{
					SecurityGroupId: &sgID,
				},
			}, nil
		},
	}

	sg, err := alicloud.CreateSecurityGroup(mockECS, vpcID, regionID, "test-sg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sg.ID != sgID {
		t.Errorf("expected SecurityGroup ID %s, got %s", sgID, sg.ID)
	}
}

func TestDefaultSecurityGroupRules(t *testing.T) {
	tests := []struct {
		name     string
		sshIP    string
		expected []alicloud.SecurityGroupRule
	}{
		{
			name:  "with SSH IP restriction",
			sshIP: "1.2.3.4/32",
			expected: []alicloud.SecurityGroupRule{
				{Protocol: "TCP", PortRange: "22/22", SourceCIDR: "1.2.3.4/32"},
				{Protocol: "TCP", PortRange: "80/80", SourceCIDR: "0.0.0.0/0"},
				{Protocol: "TCP", PortRange: "443/443", SourceCIDR: "0.0.0.0/0"},
			},
		},
		{
			name:  "without SSH IP restriction",
			sshIP: "",
			expected: []alicloud.SecurityGroupRule{
				{Protocol: "TCP", PortRange: "22/22", SourceCIDR: "0.0.0.0/0"},
				{Protocol: "TCP", PortRange: "80/80", SourceCIDR: "0.0.0.0/0"},
				{Protocol: "TCP", PortRange: "443/443", SourceCIDR: "0.0.0.0/0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := alicloud.DefaultSecurityGroupRules(tt.sshIP)
			if len(rules) != len(tt.expected) {
				t.Errorf("expected %d rules, got %d", len(tt.expected), len(rules))
			}
			for i, rule := range rules {
				if rule.SourceCIDR != tt.expected[i].SourceCIDR {
					t.Errorf("rule %d: expected SourceCIDR %s, got %s", i, tt.expected[i].SourceCIDR, rule.SourceCIDR)
				}
			}
		})
	}
}

func TestSelectAvailableZone_FirstAvailable(t *testing.T) {
	regionID := "ap-southeast-1"
	zone1a := "ap-southeast-1a"

	mockECS := &MockECSAPI{
		DescribeZonesFunc: func(req *ecsclient.DescribeZonesRequest) (*ecsclient.DescribeZonesResponse, error) {
			return &ecsclient.DescribeZonesResponse{
				Body: &ecsclient.DescribeZonesResponseBody{
					Zones: &ecsclient.DescribeZonesResponseBodyZones{
						Zone: []*ecsclient.DescribeZonesResponseBodyZonesZone{
							{
								ZoneId: &zone1a,
								AvailableResourceCreation: &ecsclient.DescribeZonesResponseBodyZonesZoneAvailableResourceCreation{
									ResourceTypes: []*string{teaString("Instance")},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	zone, err := alicloud.SelectAvailableZone(mockECS, regionID, "", alicloud.DefaultZonePriority)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if zone != zone1a {
		t.Errorf("expected zone %s, got %s", zone1a, zone)
	}
}

func TestSelectAvailableZone_Fallback(t *testing.T) {
	regionID := "ap-southeast-1"
	zone1a := "ap-southeast-1a"
	zone1b := "ap-southeast-1b"
	zone1c := "ap-southeast-1c"

	mockECS := &MockECSAPI{
		DescribeZonesFunc: func(req *ecsclient.DescribeZonesRequest) (*ecsclient.DescribeZonesResponse, error) {
			return &ecsclient.DescribeZonesResponse{
				Body: &ecsclient.DescribeZonesResponseBody{
					Zones: &ecsclient.DescribeZonesResponseBodyZones{
						Zone: []*ecsclient.DescribeZonesResponseBodyZonesZone{
							{
								ZoneId: &zone1a,
								AvailableResourceCreation: &ecsclient.DescribeZonesResponseBodyZonesZoneAvailableResourceCreation{
									ResourceTypes: []*string{},
								},
							},
							{
								ZoneId: &zone1b,
								AvailableResourceCreation: &ecsclient.DescribeZonesResponseBodyZonesZoneAvailableResourceCreation{
									ResourceTypes: []*string{teaString("Instance")},
								},
							},
							{
								ZoneId: &zone1c,
								AvailableResourceCreation: &ecsclient.DescribeZonesResponseBodyZonesZoneAvailableResourceCreation{
									ResourceTypes: []*string{teaString("Instance")},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	zone, err := alicloud.SelectAvailableZone(mockECS, regionID, "", alicloud.DefaultZonePriority)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if zone != zone1b {
		t.Errorf("expected zone %s (fallback), got %s", zone1b, zone)
	}
}

func TestSelectAvailableZone_NoAvailableZone(t *testing.T) {
	regionID := "ap-southeast-1"
	zone1a := "ap-southeast-1a"

	mockECS := &MockECSAPI{
		DescribeZonesFunc: func(req *ecsclient.DescribeZonesRequest) (*ecsclient.DescribeZonesResponse, error) {
			return &ecsclient.DescribeZonesResponse{
				Body: &ecsclient.DescribeZonesResponseBody{
					Zones: &ecsclient.DescribeZonesResponseBodyZones{
						Zone: []*ecsclient.DescribeZonesResponseBodyZonesZone{
							{
								ZoneId: &zone1a,
								AvailableResourceCreation: &ecsclient.DescribeZonesResponseBodyZonesZoneAvailableResourceCreation{
									ResourceTypes: []*string{},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	_, err := alicloud.SelectAvailableZone(mockECS, regionID, "", alicloud.DefaultZonePriority)
	if err == nil {
		t.Error("expected error when no zone is available")
	}
	if !errors.Is(err, alicloud.ErrNoAvailableZone) {
		t.Errorf("expected ErrNoAvailableZone, got %v", err)
	}
}

func TestCreateECSInstance_Success(t *testing.T) {
	instanceID := "i-xxx"
	regionID := "ap-southeast-1"
	zoneID := "ap-southeast-1a"
	sgID := "sg-xxx"
	vswitchID := "vsw-xxx"
	sshKeyName := "test-key"

	mockECS := &MockECSAPI{
		CreateInstanceFunc: func(req *ecsclient.CreateInstanceRequest) (*ecsclient.CreateInstanceResponse, error) {
			if *req.RegionId != regionID {
				t.Errorf("expected RegionId %s, got %s", regionID, *req.RegionId)
			}
			if *req.ZoneId != zoneID {
				t.Errorf("expected ZoneId %s, got %s", zoneID, *req.ZoneId)
			}
			if *req.InstanceType != alicloud.DefaultInstanceType {
				t.Errorf("expected InstanceType %s, got %s", alicloud.DefaultInstanceType, *req.InstanceType)
			}
			if *req.KeyPairName != sshKeyName {
				t.Errorf("expected KeyPairName %s, got %s", sshKeyName, *req.KeyPairName)
			}
			return &ecsclient.CreateInstanceResponse{
				Body: &ecsclient.CreateInstanceResponseBody{
					InstanceId: &instanceID,
				},
			}, nil
		},
	}

	ecs, err := alicloud.CreateECSInstance(mockECS, regionID, zoneID, "", "", sgID, vswitchID, sshKeyName, "test-instance")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ecs.ID != instanceID {
		t.Errorf("expected Instance ID %s, got %s", instanceID, ecs.ID)
	}
}

func TestWaitForInstanceRunning_Timeout(t *testing.T) {
	instanceID := "i-xxx"
	regionID := "ap-southeast-1"
	status := "Pending"

	mockECS := &MockECSAPI{
		DescribeInstancesFunc: func(req *ecsclient.DescribeInstancesRequest) (*ecsclient.DescribeInstancesResponse, error) {
			return &ecsclient.DescribeInstancesResponse{
				Body: &ecsclient.DescribeInstancesResponseBody{
					Instances: &ecsclient.DescribeInstancesResponseBodyInstances{
						Instance: []*ecsclient.DescribeInstancesResponseBodyInstancesInstance{
							{
								InstanceId: &instanceID,
								Status:     &status,
							},
						},
					},
				},
			}, nil
		},
	}

	ctx := context.Background()
	_, err := alicloud.WaitForInstanceRunning(ctx, mockECS, instanceID, regionID, 100*time.Millisecond, 300*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
	if !errors.Is(err, alicloud.ErrECSWaitTimeout) {
		t.Errorf("expected ErrECSWaitTimeout, got %v", err)
	}
}

func TestAllocateEIP_Success(t *testing.T) {
	allocationID := "eip-xxx"
	ip := "47.123.45.67"
	regionID := "ap-southeast-1"

	mockVPC := &MockVPCAPI{
		AllocateEipAddressFunc: func(req *vpcclient.AllocateEipAddressRequest) (*vpcclient.AllocateEipAddressResponse, error) {
			if *req.RegionId != regionID {
				t.Errorf("expected RegionId %s, got %s", regionID, *req.RegionId)
			}
			return &vpcclient.AllocateEipAddressResponse{
				Body: &vpcclient.AllocateEipAddressResponseBody{
					AllocationId: &allocationID,
					EipAddress:   &ip,
				},
			}, nil
		},
	}

	eip, err := alicloud.AllocateEIP(mockVPC, regionID, "test-eip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if eip.ID != allocationID {
		t.Errorf("expected Allocation ID %s, got %s", allocationID, eip.ID)
	}
	if eip.IP != ip {
		t.Errorf("expected IP %s, got %s", ip, eip.IP)
	}
}

func TestCreateSSHKeyPair_Success(t *testing.T) {
	keyName := "test-key"
	privateKey := "-----BEGIN RSA PRIVATE KEY-----\n..."
	fingerPrint := "89:f0:ba:62:ac:b8:aa:e1:61:5e:fd:81:69:86:6d:6b"

	mockECS := &MockECSAPI{
		CreateKeyPairFunc: func(req *ecsclient.CreateKeyPairRequest) (*ecsclient.CreateKeyPairResponse, error) {
			if *req.KeyPairName != keyName {
				t.Errorf("expected KeyPairName %s, got %s", keyName, *req.KeyPairName)
			}
			return &ecsclient.CreateKeyPairResponse{
				Body: &ecsclient.CreateKeyPairResponseBody{
					KeyPairName:        &keyName,
					PrivateKeyBody:     &privateKey,
					KeyPairFingerPrint: &fingerPrint,
				},
			}, nil
		},
	}

	keyPair, err := alicloud.CreateSSHKeyPair(mockECS, keyName, "ap-southeast-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if keyPair.Name != keyName {
		t.Errorf("expected KeyPairName %s, got %s", keyName, keyPair.Name)
	}
	if keyPair.PrivateKey != privateKey {
		t.Errorf("expected PrivateKey %s, got %s", privateKey, keyPair.PrivateKey)
	}
}

func teaString(s string) *string {
	return &s
}
