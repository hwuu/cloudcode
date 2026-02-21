package unit

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
						return "caddy running\nauthelia running\nopencode running\n", nil
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
