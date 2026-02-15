package alicloud

import (
	"context"
	"fmt"
	"time"

	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
)

const (
	DefaultInstanceType       = "ecs.e-c1m2.large"
	DefaultImageID            = "ubuntu_24_04_x64"
	DefaultSystemDiskSize     = 60
	DefaultSystemDiskCategory = "cloud_essd"
	DefaultSSHKeyName         = "cloudcode-ssh-key"

	DefaultWaitInterval = 5 * time.Second
	DefaultWaitTimeout  = 5 * time.Minute
)

var DefaultZonePriority = []string{
	"ap-southeast-1a",
	"ap-southeast-1b",
	"ap-southeast-1c",
}

type ECSResource struct {
	ID           string
	InstanceType string
	PublicIP     string
	PrivateIP    string
	ZoneID       string
}

type ZoneInfo struct {
	ZoneID    string
	Available bool
}

func DescribeAvailableZones(ecsCli ECSAPI, regionID, instanceType string) ([]ZoneInfo, error) {
	req := &ecsclient.DescribeZonesRequest{
		RegionId:          &regionID,
		InstanceChargeType: teaString("PostPaid"),
	}

	resp, err := ecsCli.DescribeZones(req)
	if err != nil {
		return nil, fmt.Errorf("failed to describe zones: %w", err)
	}

	if resp == nil || resp.Body == nil || resp.Body.Zones == nil || resp.Body.Zones.Zone == nil {
		return nil, fmt.Errorf("invalid response from DescribeZones")
	}

	var zones []ZoneInfo
	for _, z := range resp.Body.Zones.Zone {
		available := false
		if z.AvailableResourceCreation != nil && z.AvailableResourceCreation.ResourceTypes != nil {
			for _, rt := range z.AvailableResourceCreation.ResourceTypes {
				if rt != nil && *rt == "Instance" {
					available = true
					break
				}
			}
		}
		zones = append(zones, ZoneInfo{
			ZoneID:    *z.ZoneId,
			Available: available,
		})
	}

	return zones, nil
}

func SelectAvailableZone(ecsCli ECSAPI, regionID, instanceType string, preferredZones []string) (string, error) {
	zones, err := DescribeAvailableZones(ecsCli, regionID, instanceType)
	if err != nil {
		return "", err
	}

	zoneMap := make(map[string]bool)
	for _, z := range zones {
		zoneMap[z.ZoneID] = z.Available
	}

	for _, zoneID := range preferredZones {
		if available, ok := zoneMap[zoneID]; ok && available {
			return zoneID, nil
		}
	}

	return "", ErrNoAvailableZone
}

func CreateECSInstance(ecsCli ECSAPI, regionID, zoneID, instanceType, imageID, sgID, vswitchID, sshKeyName, instanceName string) (*ECSResource, error) {
	if instanceType == "" {
		instanceType = DefaultInstanceType
	}
	if imageID == "" {
		imageID = DefaultImageID
	}

	diskCategory := DefaultSystemDiskCategory

	req := &ecsclient.CreateInstanceRequest{
		RegionId:                &regionID,
		ZoneId:                  &zoneID,
		InstanceType:            &instanceType,
		ImageId:                 &imageID,
		SecurityGroupId:         &sgID,
		VSwitchId:               &vswitchID,
		InstanceName:            &instanceName,
		InternetMaxBandwidthOut: teaInt32(0),
		SystemDisk: &ecsclient.CreateInstanceRequestSystemDisk{
			Size:     teaInt32(DefaultSystemDiskSize),
			Category: &diskCategory,
		},
	}

	if sshKeyName != "" {
		req.KeyPairName = &sshKeyName
	}

	resp, err := ecsCli.CreateInstance(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create ECS instance: %w", err)
	}

	if resp == nil || resp.Body == nil || resp.Body.InstanceId == nil {
		return nil, fmt.Errorf("invalid response from CreateInstance")
	}

	return &ECSResource{
		ID:           *resp.Body.InstanceId,
		InstanceType: instanceType,
		ZoneID:       zoneID,
	}, nil
}

func StartECSInstance(ecsCli ECSAPI, instanceID string) error {
	req := &ecsclient.StartInstanceRequest{
		InstanceId: &instanceID,
	}
	_, err := ecsCli.StartInstance(req)
	return err
}

func StopECSInstance(ecsCli ECSAPI, instanceID string) error {
	forceStop := true
	req := &ecsclient.StopInstanceRequest{
		InstanceId: &instanceID,
		ForceStop:  &forceStop,
	}
	_, err := ecsCli.StopInstance(req)
	return err
}

func DeleteECSInstance(ecsCli ECSAPI, instanceID string) error {
	req := &ecsclient.DeleteInstanceRequest{
		InstanceId: &instanceID,
		Force:      teaBoolean(true),
	}
	_, err := ecsCli.DeleteInstance(req)
	return err
}

func DescribeECSInstance(ecsCli ECSAPI, instanceID, regionID string) (*ECSResource, error) {
	req := &ecsclient.DescribeInstancesRequest{
		InstanceIds: teaString(fmt.Sprintf(`["%s"]`, instanceID)),
		RegionId:    &regionID,
	}

	resp, err := ecsCli.DescribeInstances(req)
	if err != nil {
		return nil, err
	}

	if resp == nil || resp.Body == nil || resp.Body.Instances == nil ||
		resp.Body.Instances.Instance == nil || len(resp.Body.Instances.Instance) == 0 {
		return nil, ErrResourceNotFound
	}

	inst := resp.Body.Instances.Instance[0]
	var publicIP, privateIP string
	if inst.PublicIpAddress != nil && inst.PublicIpAddress.IpAddress != nil && len(inst.PublicIpAddress.IpAddress) > 0 {
		publicIP = *inst.PublicIpAddress.IpAddress[0]
	}
	if inst.InnerIpAddress != nil && inst.InnerIpAddress.IpAddress != nil && len(inst.InnerIpAddress.IpAddress) > 0 {
		privateIP = *inst.InnerIpAddress.IpAddress[0]
	} else if inst.VpcAttributes != nil && inst.VpcAttributes.PrivateIpAddress != nil && len(inst.VpcAttributes.PrivateIpAddress.IpAddress) > 0 {
		privateIP = *inst.VpcAttributes.PrivateIpAddress.IpAddress[0]
	}

	return &ECSResource{
		ID:           *inst.InstanceId,
		InstanceType: *inst.InstanceType,
		PublicIP:     publicIP,
		PrivateIP:    privateIP,
		ZoneID:       *inst.ZoneId,
	}, nil
}

func WaitForInstanceRunning(ctx context.Context, ecsCli ECSAPI, instanceID, regionID string, interval, timeout time.Duration) (*ECSResource, error) {
	if interval == 0 {
		interval = DefaultWaitInterval
	}
	if timeout == 0 {
		timeout = DefaultWaitTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ErrECSWaitTimeout
		case <-ticker.C:
			req := &ecsclient.DescribeInstancesRequest{
				InstanceIds: teaString(fmt.Sprintf(`["%s"]`, instanceID)),
				RegionId:    &regionID,
			}

			resp, err := ecsCli.DescribeInstances(req)
			if err != nil {
				continue
			}

			if resp == nil || resp.Body == nil || resp.Body.Instances == nil ||
				resp.Body.Instances.Instance == nil || len(resp.Body.Instances.Instance) == 0 {
				continue
			}

			inst := resp.Body.Instances.Instance[0]
			if inst.Status == nil || *inst.Status != "Running" {
				continue
			}

			var publicIP, privateIP string
			if inst.PublicIpAddress != nil && inst.PublicIpAddress.IpAddress != nil && len(inst.PublicIpAddress.IpAddress) > 0 {
				publicIP = *inst.PublicIpAddress.IpAddress[0]
			}
			if inst.VpcAttributes != nil && inst.VpcAttributes.PrivateIpAddress != nil && len(inst.VpcAttributes.PrivateIpAddress.IpAddress) > 0 {
				privateIP = *inst.VpcAttributes.PrivateIpAddress.IpAddress[0]
			}

			return &ECSResource{
				ID:           *inst.InstanceId,
				InstanceType: *inst.InstanceType,
				PublicIP:     publicIP,
				PrivateIP:    privateIP,
				ZoneID:       *inst.ZoneId,
			}, nil
		}
	}
}

type SSHKeyPairResource struct {
	Name        string
	PrivateKey  string
	FingerPrint string
}

func CreateSSHKeyPair(ecsCli ECSAPI, keyName string) (*SSHKeyPairResource, error) {
	req := &ecsclient.CreateKeyPairRequest{
		KeyPairName: &keyName,
	}

	resp, err := ecsCli.CreateKeyPair(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH key pair: %w", err)
	}

	if resp == nil || resp.Body == nil || resp.Body.KeyPairName == nil {
		return nil, fmt.Errorf("invalid response from CreateKeyPair")
	}

	result := &SSHKeyPairResource{
		Name: *resp.Body.KeyPairName,
	}
	if resp.Body.PrivateKeyBody != nil {
		result.PrivateKey = *resp.Body.PrivateKeyBody
	}
	if resp.Body.KeyPairFingerPrint != nil {
		result.FingerPrint = *resp.Body.KeyPairFingerPrint
	}

	return result, nil
}

func DeleteSSHKeyPair(ecsCli ECSAPI, keyName string) error {
	req := &ecsclient.DeleteKeyPairsRequest{
		KeyPairNames: teaString(fmt.Sprintf(`["%s"]`, keyName)),
	}
	_, err := ecsCli.DeleteKeyPairs(req)
	return err
}

func ImportSSHKeyPair(ecsCli ECSAPI, keyName, publicKey string) (*SSHKeyPairResource, error) {
	req := &ecsclient.ImportKeyPairRequest{
		KeyPairName:   &keyName,
		PublicKeyBody: &publicKey,
	}

	resp, err := ecsCli.ImportKeyPair(req)
	if err != nil {
		return nil, fmt.Errorf("failed to import SSH key pair: %w", err)
	}

	if resp == nil || resp.Body == nil || resp.Body.KeyPairName == nil {
		return nil, fmt.Errorf("invalid response from ImportKeyPair")
	}

	result := &SSHKeyPairResource{
		Name: *resp.Body.KeyPairName,
	}
	if resp.Body.KeyPairFingerPrint != nil {
		result.FingerPrint = *resp.Body.KeyPairFingerPrint
	}

	return result, nil
}

func teaString(s string) *string {
	return &s
}

func teaInt32(i int32) *int32 {
	return &i
}

func teaBoolean(b bool) *bool {
	return &b
}
