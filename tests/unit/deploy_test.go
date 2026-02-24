package unit

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hwuu/cloudcode/internal/config"
	"github.com/hwuu/cloudcode/internal/deploy"
	"github.com/hwuu/cloudcode/internal/remote"

	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
	stsclient "github.com/alibabacloud-go/sts-20150401/v2/client"
	vpcclient "github.com/alibabacloud-go/vpc-20160428/v6/client"
)

// --- Deploy-specific mocks ---

type deployMockSTS struct {
	err bool
}

func (m *deployMockSTS) GetCallerIdentity() (*stsclient.GetCallerIdentityResponse, error) {
	if m.err {
		return nil, errors.New("STS error")
	}
	accountID := "123456789"
	userID := "test-user"
	arn := "acs:ram::123456789:user/test"
	return &stsclient.GetCallerIdentityResponse{
		Body: &stsclient.GetCallerIdentityResponseBody{
			AccountId: &accountID,
			UserId:    &userID,
			Arn:       &arn,
		},
	}, nil
}

type deployMockECS struct {
	createdInstances  []string
	startedInstances  []string
	describeStatus    string
}

func (m *deployMockECS) CreateInstance(req *ecsclient.CreateInstanceRequest) (*ecsclient.CreateInstanceResponse, error) {
	id := "i-test-001"
	m.createdInstances = append(m.createdInstances, id)
	return &ecsclient.CreateInstanceResponse{
		Body: &ecsclient.CreateInstanceResponseBody{InstanceId: &id},
	}, nil
}

func (m *deployMockECS) StartInstance(req *ecsclient.StartInstanceRequest) (*ecsclient.StartInstanceResponse, error) {
	m.startedInstances = append(m.startedInstances, *req.InstanceId)
	return &ecsclient.StartInstanceResponse{}, nil
}

func (m *deployMockECS) DescribeInstances(req *ecsclient.DescribeInstancesRequest) (*ecsclient.DescribeInstancesResponse, error) {
	status := m.describeStatus
	if status == "" {
		// 启动前返回 Stopped，启动后返回 Running
		if len(m.startedInstances) > 0 {
			status = "Running"
		} else {
			status = "Stopped"
		}
	}
	id := "i-test-001"
	instType := "ecs.e-c1m2.large"
	zone := "ap-southeast-1a"
	privateIP := "192.168.1.100"
	return &ecsclient.DescribeInstancesResponse{
		Body: &ecsclient.DescribeInstancesResponseBody{
			Instances: &ecsclient.DescribeInstancesResponseBodyInstances{
				Instance: []*ecsclient.DescribeInstancesResponseBodyInstancesInstance{
					{
						InstanceId:   &id,
						Status:       &status,
						InstanceType: &instType,
						ZoneId:       &zone,
						VpcAttributes: &ecsclient.DescribeInstancesResponseBodyInstancesInstanceVpcAttributes{
							PrivateIpAddress: &ecsclient.DescribeInstancesResponseBodyInstancesInstanceVpcAttributesPrivateIpAddress{
								IpAddress: []*string{&privateIP},
							},
						},
					},
				},
			},
		},
	}, nil
}

func (m *deployMockECS) DeleteInstance(req *ecsclient.DeleteInstanceRequest) (*ecsclient.DeleteInstanceResponse, error) {
	return &ecsclient.DeleteInstanceResponse{}, nil
}

func (m *deployMockECS) StopInstance(req *ecsclient.StopInstanceRequest) (*ecsclient.StopInstanceResponse, error) {
	return &ecsclient.StopInstanceResponse{}, nil
}

func (m *deployMockECS) CreateKeyPair(req *ecsclient.CreateKeyPairRequest) (*ecsclient.CreateKeyPairResponse, error) {
	name := *req.KeyPairName
	pk := "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----"
	fp := "aa:bb:cc:dd"
	return &ecsclient.CreateKeyPairResponse{
		Body: &ecsclient.CreateKeyPairResponseBody{
			KeyPairName:        &name,
			PrivateKeyBody:     &pk,
			KeyPairFingerPrint: &fp,
		},
	}, nil
}

func (m *deployMockECS) DeleteKeyPairs(req *ecsclient.DeleteKeyPairsRequest) (*ecsclient.DeleteKeyPairsResponse, error) {
	return &ecsclient.DeleteKeyPairsResponse{}, nil
}

func (m *deployMockECS) DescribeKeyPairs(req *ecsclient.DescribeKeyPairsRequest) (*ecsclient.DescribeKeyPairsResponse, error) {
	return &ecsclient.DescribeKeyPairsResponse{}, nil
}

func (m *deployMockECS) DescribeZones(req *ecsclient.DescribeZonesRequest) (*ecsclient.DescribeZonesResponse, error) {
	zone := "ap-southeast-1a"
	return &ecsclient.DescribeZonesResponse{
		Body: &ecsclient.DescribeZonesResponseBody{
			Zones: &ecsclient.DescribeZonesResponseBodyZones{
				Zone: []*ecsclient.DescribeZonesResponseBodyZonesZone{
					{
						ZoneId: &zone,
						AvailableResourceCreation: &ecsclient.DescribeZonesResponseBodyZonesZoneAvailableResourceCreation{
							ResourceTypes: []*string{teaString("Instance")},
						},
					},
				},
			},
		},
	}, nil
}

func (m *deployMockECS) DescribeAccountAttributes(req *ecsclient.DescribeAccountAttributesRequest) (*ecsclient.DescribeAccountAttributesResponse, error) {
	return &ecsclient.DescribeAccountAttributesResponse{}, nil
}

func (m *deployMockECS) ImportKeyPair(req *ecsclient.ImportKeyPairRequest) (*ecsclient.ImportKeyPairResponse, error) {
	name := *req.KeyPairName
	return &ecsclient.ImportKeyPairResponse{
		Body: &ecsclient.ImportKeyPairResponseBody{KeyPairName: &name},
	}, nil
}

func (m *deployMockECS) CreateSecurityGroup(req *ecsclient.CreateSecurityGroupRequest) (*ecsclient.CreateSecurityGroupResponse, error) {
	sgID := "sg-test-001"
	return &ecsclient.CreateSecurityGroupResponse{
		Body: &ecsclient.CreateSecurityGroupResponseBody{SecurityGroupId: &sgID},
	}, nil
}

func (m *deployMockECS) DeleteSecurityGroup(req *ecsclient.DeleteSecurityGroupRequest) (*ecsclient.DeleteSecurityGroupResponse, error) {
	return &ecsclient.DeleteSecurityGroupResponse{}, nil
}

func (m *deployMockECS) DescribeSecurityGroups(req *ecsclient.DescribeSecurityGroupsRequest) (*ecsclient.DescribeSecurityGroupsResponse, error) {
	return &ecsclient.DescribeSecurityGroupsResponse{}, nil
}

func (m *deployMockECS) AuthorizeSecurityGroup(req *ecsclient.AuthorizeSecurityGroupRequest) (*ecsclient.AuthorizeSecurityGroupResponse, error) {
	return &ecsclient.AuthorizeSecurityGroupResponse{}, nil
}

func (m *deployMockECS) DescribeDisks(req *ecsclient.DescribeDisksRequest) (*ecsclient.DescribeDisksResponse, error) {
	return &ecsclient.DescribeDisksResponse{}, nil
}

func (m *deployMockECS) CreateSnapshot(req *ecsclient.CreateSnapshotRequest) (*ecsclient.CreateSnapshotResponse, error) {
	return &ecsclient.CreateSnapshotResponse{}, nil
}

func (m *deployMockECS) DescribeSnapshots(req *ecsclient.DescribeSnapshotsRequest) (*ecsclient.DescribeSnapshotsResponse, error) {
	return &ecsclient.DescribeSnapshotsResponse{}, nil
}

func (m *deployMockECS) DeleteSnapshot(req *ecsclient.DeleteSnapshotRequest) (*ecsclient.DeleteSnapshotResponse, error) {
	return &ecsclient.DeleteSnapshotResponse{}, nil
}

func (m *deployMockECS) CreateImage(req *ecsclient.CreateImageRequest) (*ecsclient.CreateImageResponse, error) {
	return &ecsclient.CreateImageResponse{}, nil
}

func (m *deployMockECS) DescribeImages(req *ecsclient.DescribeImagesRequest) (*ecsclient.DescribeImagesResponse, error) {
	return &ecsclient.DescribeImagesResponse{}, nil
}

func (m *deployMockECS) DeleteImage(req *ecsclient.DeleteImageRequest) (*ecsclient.DeleteImageResponse, error) {
	return &ecsclient.DeleteImageResponse{}, nil
}

type deployMockVPC struct{}

func (m *deployMockVPC) CreateVpc(req *vpcclient.CreateVpcRequest) (*vpcclient.CreateVpcResponse, error) {
	id := "vpc-test-001"
	return &vpcclient.CreateVpcResponse{
		Body: &vpcclient.CreateVpcResponseBody{VpcId: &id},
	}, nil
}

func (m *deployMockVPC) DeleteVpc(req *vpcclient.DeleteVpcRequest) (*vpcclient.DeleteVpcResponse, error) {
	return &vpcclient.DeleteVpcResponse{}, nil
}

func (m *deployMockVPC) DescribeVpcs(req *vpcclient.DescribeVpcsRequest) (*vpcclient.DescribeVpcsResponse, error) {
	status := "Available"
	vpcID := "vpc-test-001"
	cidr := "192.168.0.0/16"
	return &vpcclient.DescribeVpcsResponse{
		Body: &vpcclient.DescribeVpcsResponseBody{
			Vpcs: &vpcclient.DescribeVpcsResponseBodyVpcs{
				Vpc: []*vpcclient.DescribeVpcsResponseBodyVpcsVpc{
					{VpcId: &vpcID, CidrBlock: &cidr, Status: &status},
				},
			},
		},
	}, nil
}

func (m *deployMockVPC) CreateVSwitch(req *vpcclient.CreateVSwitchRequest) (*vpcclient.CreateVSwitchResponse, error) {
	id := "vsw-test-001"
	return &vpcclient.CreateVSwitchResponse{
		Body: &vpcclient.CreateVSwitchResponseBody{VSwitchId: &id},
	}, nil
}

func (m *deployMockVPC) DeleteVSwitch(req *vpcclient.DeleteVSwitchRequest) (*vpcclient.DeleteVSwitchResponse, error) {
	return &vpcclient.DeleteVSwitchResponse{}, nil
}

func (m *deployMockVPC) DescribeVSwitches(req *vpcclient.DescribeVSwitchesRequest) (*vpcclient.DescribeVSwitchesResponse, error) {
	return &vpcclient.DescribeVSwitchesResponse{}, nil
}

func (m *deployMockVPC) AllocateEipAddress(req *vpcclient.AllocateEipAddressRequest) (*vpcclient.AllocateEipAddressResponse, error) {
	id := "eip-test-001"
	ip := "47.100.1.1"
	return &vpcclient.AllocateEipAddressResponse{
		Body: &vpcclient.AllocateEipAddressResponseBody{AllocationId: &id, EipAddress: &ip},
	}, nil
}

func (m *deployMockVPC) ReleaseEipAddress(req *vpcclient.ReleaseEipAddressRequest) (*vpcclient.ReleaseEipAddressResponse, error) {
	return &vpcclient.ReleaseEipAddressResponse{}, nil
}

func (m *deployMockVPC) DescribeEipAddresses(req *vpcclient.DescribeEipAddressesRequest) (*vpcclient.DescribeEipAddressesResponse, error) {
	id := "eip-test-001"
	ip := "47.100.1.1"
	status := "InUse"
	return &vpcclient.DescribeEipAddressesResponse{
		Body: &vpcclient.DescribeEipAddressesResponseBody{
			EipAddresses: &vpcclient.DescribeEipAddressesResponseBodyEipAddresses{
				EipAddress: []*vpcclient.DescribeEipAddressesResponseBodyEipAddressesEipAddress{
					{AllocationId: &id, IpAddress: &ip, Status: &status},
				},
			},
		},
	}, nil
}

func (m *deployMockVPC) AssociateEipAddress(req *vpcclient.AssociateEipAddressRequest) (*vpcclient.AssociateEipAddressResponse, error) {
	return &vpcclient.AssociateEipAddressResponse{}, nil
}

func (m *deployMockVPC) UnassociateEipAddress(req *vpcclient.UnassociateEipAddressRequest) (*vpcclient.UnassociateEipAddressResponse, error) {
	return &vpcclient.UnassociateEipAddressResponse{}, nil
}

// --- Tests ---

func newTestDeployer(stateDir string, promptInput string) *deploy.Deployer {
	output := &bytes.Buffer{}
	prompter := config.NewPrompter(strings.NewReader(promptInput), output)

	return &deploy.Deployer{
		ECS:          &deployMockECS{},
		VPC:          &deployMockVPC{},
		STS:          &deployMockSTS{},
		Prompter:     prompter,
		Output:       output,
		Region:       "ap-southeast-1",
		StateDir:     stateDir,
		WaitInterval: 10 * time.Millisecond,
		WaitTimeout:  1 * time.Second,
		SSHDialFunc: func(host string, port int, user string, privateKey []byte) remote.DialFunc {
			return func() (remote.SSHClient, error) {
				return &MockSSHClient{
					RunCommandFunc: func(ctx context.Context, cmd string) (string, error) {
						if strings.Contains(cmd, "docker compose ps") {
							return "caddy running\nauthelia running\ndevbox running\n", nil
						}
						return "", nil
					},
				}, nil
			}
		},
		SFTPFactory: func(host string, port int, user string, privateKey []byte) (remote.SFTPClient, error) {
			return &MockSFTPClient{
				UploadFileFunc: func(content []byte, remotePath string) error {
					return nil
				},
			}, nil
		},
		GetPublicIP: func() (string, error) {
			return "1.2.3.4", nil
		},
	}
}

func TestPreflightCheck_Success(t *testing.T) {
	d := newTestDeployer(t.TempDir(), "")
	ctx := context.Background()

	err := d.PreflightCheck(ctx)
	if err != nil {
		t.Fatalf("PreflightCheck failed: %v", err)
	}
}

func TestPreflightCheck_STSError(t *testing.T) {
	d := newTestDeployer(t.TempDir(), "")
	d.STS = &deployMockSTS{err: true}
	ctx := context.Background()

	err := d.PreflightCheck(ctx)
	if err == nil {
		t.Error("expected error when STS fails")
	}
}

func TestCreateResources_FullDeploy(t *testing.T) {
	stateDir := t.TempDir()
	d := newTestDeployer(stateDir, "")
	ctx := context.Background()

	state := config.NewState("ap-southeast-1", "ubuntu_24_04_x64")
	state.CloudCode = config.CloudCodeConfig{
		Username: "admin",
		Domain:   "test.example.com",
	}

	err := d.CreateResources(ctx, state, "")
	if err != nil {
		t.Fatalf("CreateResources failed: %v", err)
	}

	if !state.HasVPC() {
		t.Error("state should have VPC after CreateResources")
	}
	if !state.HasVSwitch() {
		t.Error("state should have VSwitch after CreateResources")
	}
	if !state.HasSecurityGroup() {
		t.Error("state should have SecurityGroup after CreateResources")
	}
	if !state.HasSSHKeyPair() {
		t.Error("state should have SSHKeyPair after CreateResources")
	}
	if !state.HasECS() {
		t.Error("state should have ECS after CreateResources")
	}
	if !state.HasEIP() {
		t.Error("state should have EIP after CreateResources")
	}
	if !state.IsComplete() {
		t.Error("state should be complete after full deploy")
	}
}

func TestCreateResources_Idempotent(t *testing.T) {
	stateDir := t.TempDir()
	mockECS := &deployMockECS{describeStatus: "Running"}
	d := newTestDeployer(stateDir, "")
	d.ECS = mockECS
	ctx := context.Background()

	// 预填充 state，模拟已有资源
	state := config.NewState("ap-southeast-1", "ubuntu_24_04_x64")
	state.Resources.VPC = config.VPCResource{ID: "vpc-existing", CIDR: "192.168.0.0/16"}
	state.Resources.VSwitch = config.VSwitchResource{ID: "vsw-existing", ZoneID: "ap-southeast-1a"}
	state.Resources.SecurityGroup = config.SecurityGroupResource{ID: "sg-existing"}
	state.Resources.SSHKeyPair = config.SSHKeyPairResource{Name: "existing-key", PrivateKeyPath: ".cloudcode/ssh_key"}
	state.Resources.ECS = config.ECSResource{ID: "i-existing", InstanceType: "ecs.e-c1m2.large"}
	state.Resources.EIP = config.EIPResource{ID: "eip-existing", IP: "47.1.2.3"}

	err := d.CreateResources(ctx, state, "")
	if err != nil {
		t.Fatalf("CreateResources failed: %v", err)
	}

	// 不应创建新实例
	if len(mockECS.createdInstances) > 0 {
		t.Error("should not create new instances when resources already exist")
	}
}

func writeDummySSHKey(t *testing.T, stateDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(stateDir, "ssh_key"), []byte("dummy-private-key"), 0600); err != nil {
		t.Fatalf("failed to write dummy SSH key: %v", err)
	}
}

func TestDeployApp_Success(t *testing.T) {
	stateDir := t.TempDir()
	writeDummySSHKey(t, stateDir)
	d := newTestDeployer(stateDir, "")
	ctx := context.Background()

	state := config.NewState("ap-southeast-1", "ubuntu_24_04_x64")
	state.Resources.EIP = config.EIPResource{ID: "eip-test", IP: "47.100.1.1"}
	state.Resources.SSHKeyPair = config.SSHKeyPairResource{Name: "test-key", PrivateKeyPath: ".cloudcode/ssh_key"}
	state.CloudCode = config.CloudCodeConfig{
		Username: "admin",
		Domain:   "47.100.1.1.nip.io",
	}

	deployConfig := &deploy.DeployConfig{
		Domain:               "47.100.1.1.nip.io",
		Username:             "admin",
		Password:             "test-password",
		Email:                "admin@example.com",
		OpenAIAPIKey:         "sk-test",
		OpenAIBaseURL:        "",
		AnthropicAPIKey:      "",
	}

	err := d.DeployApp(ctx, state, deployConfig)
	if err != nil {
		t.Fatalf("DeployApp failed: %v", err)
	}
}

func TestHealthCheck_Success(t *testing.T) {
	stateDir := t.TempDir()
	writeDummySSHKey(t, stateDir)
	d := newTestDeployer(stateDir, "")
	ctx := context.Background()

	state := config.NewState("ap-southeast-1", "ubuntu_24_04_x64")
	state.Resources.EIP = config.EIPResource{ID: "eip-test", IP: "47.100.1.1"}
	state.Resources.SSHKeyPair = config.SSHKeyPairResource{Name: "test-key", PrivateKeyPath: ".cloudcode/ssh_key"}

	err := d.HealthCheck(ctx, state)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}
