package alicloud

// 本文件管理 ECS 云服务器实例的生命周期和 SSH 密钥对。
// 包括：可用区选择、实例创建/启动/停止/删除、状态等待、SSH 密钥对管理。

import (
	"context"
	"fmt"
	"time"

	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
)

const (
	DefaultInstanceType       = "ecs.e-c1m2.large"                       // 默认实例规格：2vCPU 4GiB
	DefaultImageID            = "ubuntu_24_04_x64_20G_alibase_20260119.vhd" // Ubuntu 24.04 镜像（新加坡区域完整格式）
	DefaultSystemDiskSize     = 60                                        // 系统盘大小（GB）
	DefaultSystemDiskCategory = "cloud_essd"                              // 系统盘类型：ESSD 云盘
	DefaultSSHKeyName         = "cloudcode-ssh-key"                       // SSH 密钥对名称

	DefaultWaitInterval = 5 * time.Second  // 状态轮询间隔
	DefaultWaitTimeout  = 5 * time.Minute  // 状态等待超时
)

// DefaultZonePriority 新加坡区域可用区优先级（按库存充足程度排序）
var DefaultZonePriority = []string{
	"ap-southeast-1a",
	"ap-southeast-1b",
	"ap-southeast-1c",
}

// ECSResource ECS 实例资源信息（注意：与 config.ECSResource 不同，这是 SDK 层的返回值）
type ECSResource struct {
	ID           string
	InstanceType string
	PublicIP     string
	PrivateIP    string
	ZoneID       string
	TempImageID  string // 从快照恢复时创建的临时镜像 ID，调用方应清理
}

// ZoneInfo 可用区信息
type ZoneInfo struct {
	ZoneID    string
	Available bool // 是否支持创建 ECS 实例
}

// DescribeAvailableZones 查询指定区域的可用区列表及其资源可用性
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

// SelectAvailableZone 按优先级选择一个可用的可用区。
// 优先使用 preferredZones 列表中靠前的可用区，全部不可用时返回 ErrNoAvailableZone。
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

// CreateECSInstance 创建 ECS 实例（按量付费，不分配公网 IP，通过 EIP 访问）
// snapshotID 非空时先从快照创建自定义镜像，再用该镜像创建实例。
// 返回的 ECSResource.ImageID 非空时，调用方应在实例就绪后调用 DeleteImage 清理临时镜像。
func CreateECSInstance(ecsCli ECSAPI, regionID, zoneID, instanceType, imageID, sgID, vswitchID, sshKeyName, instanceName, snapshotID string) (*ECSResource, error) {
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

	if snapshotID != "" {
		// 从快照创建自定义镜像，再用该镜像创建实例
		imgID, err := CreateImageFromSnapshot(ecsCli, snapshotID, regionID, "cloudcode-restore")
		if err != nil {
			return nil, fmt.Errorf("从快照创建镜像失败: %w", err)
		}
		if err := WaitForImageReady(context.Background(), ecsCli, imgID, regionID, 0, 0); err != nil {
			return nil, fmt.Errorf("等待镜像就绪失败: %w", err)
		}
		imageID = imgID
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

	result := &ECSResource{
		ID:           *resp.Body.InstanceId,
		InstanceType: instanceType,
		ZoneID:       zoneID,
	}
	if snapshotID != "" {
		result.TempImageID = imageID // 临时镜像，调用方应清理
	}
	return result, nil
}

// StartECSInstance 启动 ECS 实例（实例必须处于 Stopped 状态）
func StartECSInstance(ecsCli ECSAPI, instanceID string) error {
	req := &ecsclient.StartInstanceRequest{
		InstanceId: &instanceID,
	}
	_, err := ecsCli.StartInstance(req)
	return err
}

// StopECSInstance 停止 ECS 实例
// stopCharging=true 时使用 StopCharging 模式，释放 CPU/内存不收费
func StopECSInstance(ecsCli ECSAPI, instanceID string, stopCharging bool) error {
	forceStop := true
	req := &ecsclient.StopInstanceRequest{
		InstanceId: &instanceID,
		ForceStop:  &forceStop,
	}
	if stopCharging {
		req.StoppedMode = teaString("StopCharging")
	}
	_, err := ecsCli.StopInstance(req)
	return err
}

// DeleteECSInstance 强制删除 ECS 实例（Force=true 会自动停止运行中的实例）
func DeleteECSInstance(ecsCli ECSAPI, instanceID string) error {
	req := &ecsclient.DeleteInstanceRequest{
		InstanceId: &instanceID,
		Force:      teaBoolean(true),
	}
	_, err := ecsCli.DeleteInstance(req)
	return err
}

// DescribeECSInstance 查询 ECS 实例详情（IP 地址、规格、可用区等）
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

// WaitForInstanceStatus 轮询等待 ECS 实例达到指定状态（如 Stopped/Running）。
// 创建实例后需等待 Stopped 才能启动，启动后需等待 Running 才能 SSH。
func WaitForInstanceStatus(ctx context.Context, ecsCli ECSAPI, instanceID, regionID, targetStatus string, interval, timeout time.Duration) error {
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
			return fmt.Errorf("等待 ECS 实例状态 %s 超时", targetStatus)
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
			if inst.Status != nil && *inst.Status == targetStatus {
				return nil
			}
		}
	}
}

// WaitForInstanceRunning 轮询等待 ECS 实例进入 Running 状态，并返回实例详情（含 IP 地址）
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

// SSHKeyPairResource SSH 密钥对资源信息
type SSHKeyPairResource struct {
	Name        string
	PrivateKey  string // 仅创建时返回，后续无法再获取
	FingerPrint string
}

// CreateSSHKeyPair 创建 SSH 密钥对。如果同名密钥对已存在，自动删除后重建（因为私钥只在创建时返回）。
func CreateSSHKeyPair(ecsCli ECSAPI, keyName, regionID string) (*SSHKeyPairResource, error) {
	req := &ecsclient.CreateKeyPairRequest{
		KeyPairName: &keyName,
		RegionId:    &regionID,
	}

	resp, err := ecsCli.CreateKeyPair(req)
	if err != nil {
		// 如果密钥对已存在，先删除再重建（需要获取私钥）
		if isErrorCode(err, "KeyPair.AlreadyExist") {
			if delErr := DeleteSSHKeyPair(ecsCli, keyName, regionID); delErr != nil {
				return nil, fmt.Errorf("failed to delete existing SSH key pair: %w", delErr)
			}
			time.Sleep(2 * time.Second) // 等待删除生效
			resp, err = ecsCli.CreateKeyPair(req)
			if err != nil {
				return nil, fmt.Errorf("failed to create SSH key pair after delete: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to create SSH key pair: %w", err)
		}
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

// DeleteSSHKeyPair 删除 SSH 密钥对
func DeleteSSHKeyPair(ecsCli ECSAPI, keyName, regionID string) error {
	req := &ecsclient.DeleteKeyPairsRequest{
		KeyPairNames: teaString(fmt.Sprintf(`["%s"]`, keyName)),
		RegionId:     &regionID,
	}
	_, err := ecsCli.DeleteKeyPairs(req)
	return err
}

// ImportSSHKeyPair 导入已有的 SSH 公钥（用于自定义密钥场景）
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

// GetSystemDiskID 获取 ECS 实例的系统盘 ID
func GetSystemDiskID(ecsCli ECSAPI, instanceID, regionID string) (string, error) {
	diskType := "system"
	req := &ecsclient.DescribeDisksRequest{
		InstanceId: &instanceID,
		RegionId:   &regionID,
		DiskType:   &diskType,
	}
	resp, err := ecsCli.DescribeDisks(req)
	if err != nil {
		return "", fmt.Errorf("查询系统盘失败: %w", err)
	}
	if resp == nil || resp.Body == nil || resp.Body.Disks == nil ||
		resp.Body.Disks.Disk == nil || len(resp.Body.Disks.Disk) == 0 {
		return "", fmt.Errorf("未找到系统盘")
	}
	disk := resp.Body.Disks.Disk[0]
	if disk.DiskId == nil {
		return "", fmt.Errorf("系统盘 ID 为空")
	}
	return *disk.DiskId, nil
}

// CreateDiskSnapshot 创建磁盘快照
func CreateDiskSnapshot(ecsCli ECSAPI, diskID, snapshotName string) (string, error) {
	req := &ecsclient.CreateSnapshotRequest{
		DiskId:       &diskID,
		SnapshotName: &snapshotName,
	}
	resp, err := ecsCli.CreateSnapshot(req)
	if err != nil {
		return "", fmt.Errorf("创建快照失败: %w", err)
	}
	if resp == nil || resp.Body == nil || resp.Body.SnapshotId == nil {
		return "", fmt.Errorf("创建快照返回无效响应")
	}
	return *resp.Body.SnapshotId, nil
}

// WaitForSnapshotReady 等待快照创建完成
func WaitForSnapshotReady(ctx context.Context, ecsCli ECSAPI, snapshotID, regionID string, interval, timeout time.Duration) error {
	if interval == 0 {
		interval = DefaultWaitInterval
	}
	if timeout == 0 {
		timeout = 10 * time.Minute // 快照可能较慢
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待快照完成超时")
		case <-ticker.C:
			req := &ecsclient.DescribeSnapshotsRequest{
				SnapshotIds: teaString(fmt.Sprintf(`["%s"]`, snapshotID)),
				RegionId:    &regionID,
			}
			resp, err := ecsCli.DescribeSnapshots(req)
			if err != nil {
				continue
			}
			if resp == nil || resp.Body == nil || resp.Body.Snapshots == nil ||
				resp.Body.Snapshots.Snapshot == nil || len(resp.Body.Snapshots.Snapshot) == 0 {
				continue
			}
			snap := resp.Body.Snapshots.Snapshot[0]
			if snap.Status != nil && *snap.Status == "accomplished" {
				return nil
			}
		}
	}
}

// DeleteSnapshot 删除快照
func DeleteSnapshot(ecsCli ECSAPI, snapshotID string) error {
	req := &ecsclient.DeleteSnapshotRequest{
		SnapshotId: &snapshotID,
	}
	_, err := ecsCli.DeleteSnapshot(req)
	if err != nil {
		return fmt.Errorf("删除快照失败: %w", err)
	}
	return nil
}

// CreateImageFromSnapshot 从快照创建自定义镜像
func CreateImageFromSnapshot(ecsCli ECSAPI, snapshotID, regionID, imageName string) (string, error) {
	req := &ecsclient.CreateImageRequest{
		SnapshotId: &snapshotID,
		RegionId:   &regionID,
		ImageName:  &imageName,
	}
	resp, err := ecsCli.CreateImage(req)
	if err != nil {
		return "", fmt.Errorf("创建镜像失败: %w", err)
	}
	if resp == nil || resp.Body == nil || resp.Body.ImageId == nil {
		return "", fmt.Errorf("创建镜像返回无效响应")
	}
	return *resp.Body.ImageId, nil
}

// WaitForImageReady 等待自定义镜像创建完成
func WaitForImageReady(ctx context.Context, ecsCli ECSAPI, imageID, regionID string, interval, timeout time.Duration) error {
	if interval == 0 {
		interval = DefaultWaitInterval
	}
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待镜像就绪超时")
		case <-ticker.C:
			req := &ecsclient.DescribeImagesRequest{
				ImageId:  &imageID,
				RegionId: &regionID,
			}
			resp, err := ecsCli.DescribeImages(req)
			if err != nil {
				continue
			}
			if resp == nil || resp.Body == nil || resp.Body.Images == nil ||
				resp.Body.Images.Image == nil || len(resp.Body.Images.Image) == 0 {
				continue
			}
			img := resp.Body.Images.Image[0]
			if img.Status != nil && *img.Status == "Available" {
				return nil
			}
		}
	}
}

// DeleteImage 删除自定义镜像
func DeleteImage(ecsCli ECSAPI, imageID, regionID string) error {
	force := true
	req := &ecsclient.DeleteImageRequest{
		ImageId:  &imageID,
		RegionId: &regionID,
		Force:    &force,
	}
	_, err := ecsCli.DeleteImage(req)
	if err != nil {
		return fmt.Errorf("删除镜像失败: %w", err)
	}
	return nil
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
