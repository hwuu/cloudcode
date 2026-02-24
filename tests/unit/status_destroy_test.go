package unit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
	"github.com/hwuu/cloudcode/internal/config"
	"github.com/hwuu/cloudcode/internal/deploy"
	"github.com/hwuu/cloudcode/internal/remote"
)

func writeTestState(t *testing.T, stateDir string, state *config.State) {
	t.Helper()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), data, 0600); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

func fullState() *config.State {
	state := config.NewState("ap-southeast-1", "ubuntu_24_04_x64")
	state.Resources.VPC = config.VPCResource{ID: "vpc-test", CIDR: "192.168.0.0/16"}
	state.Resources.VSwitch = config.VSwitchResource{ID: "vsw-test", ZoneID: "ap-southeast-1a"}
	state.Resources.SecurityGroup = config.SecurityGroupResource{ID: "sg-test"}
	state.Resources.SSHKeyPair = config.SSHKeyPairResource{Name: "cloudcode-ssh-key", PrivateKeyPath: ".cloudcode/ssh_key"}
	state.Resources.ECS = config.ECSResource{ID: "i-test", InstanceType: "ecs.e-c1m2.large"}
	state.Resources.EIP = config.EIPResource{ID: "eip-test", IP: "47.100.1.1"}
	state.CloudCode = config.CloudCodeConfig{Domain: "47.100.1.1.nip.io", Username: "admin"}
	return state
}

// --- Status Tests ---

func TestStatus_NoState(t *testing.T) {
	output := &bytes.Buffer{}
	s := &deploy.StatusRunner{
		Output:   output,
		StateDir: t.TempDir(),
	}

	err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output.String(), "未找到部署记录") {
		t.Errorf("expected '未找到部署记录', got: %s", output.String())
	}
}

func TestStatus_WithState(t *testing.T) {
	stateDir := t.TempDir()
	state := fullState()
	writeTestState(t, stateDir, state)

	output := &bytes.Buffer{}
	s := &deploy.StatusRunner{
		Output:   output,
		StateDir: stateDir,
	}

	err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "vpc-test") {
		t.Error("expected VPC ID in output")
	}
	if !strings.Contains(out, "47.100.1.1") {
		t.Error("expected EIP in output")
	}
	if !strings.Contains(out, "47.100.1.1.nip.io") {
		t.Error("expected domain in output")
	}
}

func TestStatus_WithContainers(t *testing.T) {
	stateDir := t.TempDir()
	state := fullState()
	writeTestState(t, stateDir, state)
	writeDummySSHKey(t, stateDir)

	output := &bytes.Buffer{}
	s := &deploy.StatusRunner{
		Output:   output,
		StateDir: stateDir,
		SSHDialFunc: func(host string, port int, user string, privateKey []byte) remote.DialFunc {
			return func() (remote.SSHClient, error) {
				return &MockSSHClient{
					RunCommandFunc: func(ctx context.Context, cmd string) (string, error) {
						return "caddy running\nauthelia running\ndevbox running\n", nil
					},
				}, nil
			}
		},
	}

	err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "caddy") || !strings.Contains(out, "running") {
		t.Error("expected container status in output")
	}
}

// --- Destroy Tests ---

func TestDestroy_NoState(t *testing.T) {
	output := &bytes.Buffer{}
	d := &deploy.Destroyer{
		Output:   output,
		StateDir: t.TempDir(),
		Region:   "ap-southeast-1",
	}

	err := d.Run(context.Background(), false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output.String(), "未找到部署记录") {
		t.Errorf("expected '未找到部署记录', got: %s", output.String())
	}
}

func TestDestroy_DryRun(t *testing.T) {
	stateDir := t.TempDir()
	state := fullState()
	writeTestState(t, stateDir, state)

	output := &bytes.Buffer{}
	d := &deploy.Destroyer{
		ECS:      &deployMockECS{},
		VPC:      &deployMockVPC{},
		Output:   output,
		StateDir: stateDir,
		Region:   "ap-southeast-1",
	}

	err := d.Run(context.Background(), false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "dry-run") {
		t.Error("expected dry-run message")
	}
	// state 文件应该还在
	if _, err := os.Stat(filepath.Join(stateDir, "state.json")); os.IsNotExist(err) {
		t.Error("state file should still exist after dry-run")
	}
}

func TestDestroy_Force(t *testing.T) {
	stateDir := t.TempDir()
	state := fullState()
	writeTestState(t, stateDir, state)

	output := &bytes.Buffer{}
	d := &deploy.Destroyer{
		ECS:      &deployMockECS{},
		VPC:      &deployMockVPC{},
		Output:   output,
		StateDir: stateDir,
		Region:   "ap-southeast-1",
	}

	err := d.Run(context.Background(), true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "所有资源已清理完毕") {
		t.Errorf("expected success message, got: %s", out)
	}
	// state 文件应该被删除
	if _, err := os.Stat(filepath.Join(stateDir, "state.json")); !os.IsNotExist(err) {
		t.Error("state file should be deleted after destroy")
	}
}

func TestDestroy_Cancelled(t *testing.T) {
	stateDir := t.TempDir()
	state := fullState()
	writeTestState(t, stateDir, state)

	output := &bytes.Buffer{}
	prompter := config.NewPrompter(strings.NewReader("n\n"), output)
	d := &deploy.Destroyer{
		ECS:      &deployMockECS{},
		VPC:      &deployMockVPC{},
		Prompter: prompter,
		Output:   output,
		StateDir: stateDir,
		Region:   "ap-southeast-1",
	}

	err := d.Run(context.Background(), false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "已取消") {
		t.Errorf("expected cancel message, got: %s", out)
	}
	// state 文件应该还在
	if _, err := os.Stat(filepath.Join(stateDir, "state.json")); os.IsNotExist(err) {
		t.Error("state file should still exist after cancel")
	}
}

func TestDestroy_KeepSnapshot(t *testing.T) {
	stateDir := t.TempDir()
	state := fullState()
	state.Resources.ECS.SystemDiskSize = 60
	state.Status = "running"
	writeTestState(t, stateDir, state)

	diskID := "d-test-001"
	snapshotID := "s-test-001"

	mockECS := &MockECSAPI{
		StopInstanceFunc: func(req *ecsclient.StopInstanceRequest) (*ecsclient.StopInstanceResponse, error) {
			return &ecsclient.StopInstanceResponse{}, nil
		},
		DescribeInstancesFunc: func(req *ecsclient.DescribeInstancesRequest) (*ecsclient.DescribeInstancesResponse, error) {
			status := "Stopped"
			id := "i-test"
			instType := "ecs.e-c1m2.large"
			zone := "ap-southeast-1a"
			return &ecsclient.DescribeInstancesResponse{
				Body: &ecsclient.DescribeInstancesResponseBody{
					Instances: &ecsclient.DescribeInstancesResponseBodyInstances{
						Instance: []*ecsclient.DescribeInstancesResponseBodyInstancesInstance{
							{InstanceId: &id, Status: &status, InstanceType: &instType, ZoneId: &zone},
						},
					},
				},
			}, nil
		},
		DescribeDisksFunc: func(req *ecsclient.DescribeDisksRequest) (*ecsclient.DescribeDisksResponse, error) {
			return &ecsclient.DescribeDisksResponse{
				Body: &ecsclient.DescribeDisksResponseBody{
					Disks: &ecsclient.DescribeDisksResponseBodyDisks{
						Disk: []*ecsclient.DescribeDisksResponseBodyDisksDisk{
							{DiskId: &diskID},
						},
					},
				},
			}, nil
		},
		CreateSnapshotFunc: func(req *ecsclient.CreateSnapshotRequest) (*ecsclient.CreateSnapshotResponse, error) {
			return &ecsclient.CreateSnapshotResponse{
				Body: &ecsclient.CreateSnapshotResponseBody{SnapshotId: &snapshotID},
			}, nil
		},
		DescribeSnapshotsFunc: func(req *ecsclient.DescribeSnapshotsRequest) (*ecsclient.DescribeSnapshotsResponse, error) {
			status := "accomplished"
			return &ecsclient.DescribeSnapshotsResponse{
				Body: &ecsclient.DescribeSnapshotsResponseBody{
					Snapshots: &ecsclient.DescribeSnapshotsResponseBodySnapshots{
						Snapshot: []*ecsclient.DescribeSnapshotsResponseBodySnapshotsSnapshot{
							{SnapshotId: &snapshotID, Status: &status},
						},
					},
				},
			}, nil
		},
		DeleteInstanceFunc: func(req *ecsclient.DeleteInstanceRequest) (*ecsclient.DeleteInstanceResponse, error) {
			return &ecsclient.DeleteInstanceResponse{}, nil
		},
		DeleteKeyPairsFunc: func(req *ecsclient.DeleteKeyPairsRequest) (*ecsclient.DeleteKeyPairsResponse, error) {
			return &ecsclient.DeleteKeyPairsResponse{}, nil
		},
		DeleteSecurityGroupFunc: func(req *ecsclient.DeleteSecurityGroupRequest) (*ecsclient.DeleteSecurityGroupResponse, error) {
			return &ecsclient.DeleteSecurityGroupResponse{}, nil
		},
	}

	// 输入: y(保留快照) + y(确认销毁)
	output := &bytes.Buffer{}
	prompter := config.NewPrompter(strings.NewReader("y\ny\n"), output)
	d := &deploy.Destroyer{
		ECS:          mockECS,
		VPC:          &deployMockVPC{},
		Prompter:     prompter,
		Output:       output,
		StateDir:     stateDir,
		Region:       "ap-southeast-1",
		Version:      "0.2.0",
		WaitInterval: time.Second,
		WaitTimeout:  5 * time.Second,
	}

	err := d.Run(context.Background(), false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证 backup.json 存在且内容正确
	backup, err := config.LoadBackupFrom(stateDir)
	if err != nil {
		t.Fatalf("load backup failed: %v", err)
	}
	if backup == nil {
		t.Fatal("expected backup.json to exist")
	}
	if backup.SnapshotID != snapshotID {
		t.Errorf("expected snapshot ID '%s', got '%s'", snapshotID, backup.SnapshotID)
	}
	if backup.CloudCodeVersion != "0.2.0" {
		t.Errorf("expected version '0.2.0', got '%s'", backup.CloudCodeVersion)
	}
	if backup.CreatedAt == "" {
		t.Error("expected created_at to be set")
	}

	// 验证 state 标记为 destroyed
	data, err := os.ReadFile(filepath.Join(stateDir, "state.json"))
	if err != nil {
		t.Fatalf("read state failed: %v", err)
	}
	var updatedState config.State
	json.Unmarshal(data, &updatedState)
	if updatedState.Status != "destroyed" {
		t.Errorf("expected status 'destroyed', got '%s'", updatedState.Status)
	}
}

func TestDestroy_SnapshotFailed_ContinueDestroy(t *testing.T) {
	stateDir := t.TempDir()
	state := fullState()
	state.Status = "running"
	writeTestState(t, stateDir, state)

	mockECS := &MockECSAPI{
		StopInstanceFunc: func(req *ecsclient.StopInstanceRequest) (*ecsclient.StopInstanceResponse, error) {
			return &ecsclient.StopInstanceResponse{}, nil
		},
		DescribeInstancesFunc: func(req *ecsclient.DescribeInstancesRequest) (*ecsclient.DescribeInstancesResponse, error) {
			status := "Stopped"
			id := "i-test"
			instType := "ecs.e-c1m2.large"
			zone := "ap-southeast-1a"
			return &ecsclient.DescribeInstancesResponse{
				Body: &ecsclient.DescribeInstancesResponseBody{
					Instances: &ecsclient.DescribeInstancesResponseBodyInstances{
						Instance: []*ecsclient.DescribeInstancesResponseBodyInstancesInstance{
							{InstanceId: &id, Status: &status, InstanceType: &instType, ZoneId: &zone},
						},
					},
				},
			}, nil
		},
		DescribeDisksFunc: func(req *ecsclient.DescribeDisksRequest) (*ecsclient.DescribeDisksResponse, error) {
			return nil, fmt.Errorf("disk API error")
		},
		DeleteInstanceFunc: func(req *ecsclient.DeleteInstanceRequest) (*ecsclient.DeleteInstanceResponse, error) {
			return &ecsclient.DeleteInstanceResponse{}, nil
		},
		DeleteKeyPairsFunc: func(req *ecsclient.DeleteKeyPairsRequest) (*ecsclient.DeleteKeyPairsResponse, error) {
			return &ecsclient.DeleteKeyPairsResponse{}, nil
		},
		DeleteSecurityGroupFunc: func(req *ecsclient.DeleteSecurityGroupRequest) (*ecsclient.DeleteSecurityGroupResponse, error) {
			return &ecsclient.DeleteSecurityGroupResponse{}, nil
		},
	}

	// 输入: y(保留快照) + y(快照失败继续销毁) + y(确认销毁)
	output := &bytes.Buffer{}
	prompter := config.NewPrompter(strings.NewReader("y\ny\ny\n"), output)
	d := &deploy.Destroyer{
		ECS:          mockECS,
		VPC:          &deployMockVPC{},
		Prompter:     prompter,
		Output:       output,
		StateDir:     stateDir,
		Region:       "ap-southeast-1",
		WaitInterval: time.Second,
		WaitTimeout:  5 * time.Second,
	}

	err := d.Run(context.Background(), false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "快照创建失败") {
		t.Error("expected snapshot failure message")
	}
	if !strings.Contains(out, "所有资源已清理完毕") {
		t.Error("expected success message after continuing destroy")
	}

	// 不应有 backup.json
	backup, _ := config.LoadBackupFrom(stateDir)
	if backup != nil {
		t.Error("expected no backup.json when snapshot failed")
	}

	// state 应被删除
	if _, err := os.Stat(filepath.Join(stateDir, "state.json")); !os.IsNotExist(err) {
		t.Error("state file should be deleted")
	}
}
