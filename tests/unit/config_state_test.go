package unit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hwuu/cloudcode/internal/config"
)

func TestState_SaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	now := time.Now().UTC().Format(time.RFC3339)
	state := &config.State{
		Version:   config.StateFileVersion,
		CreatedAt: now,
		Region:    "ap-southeast-1",
		OSImage:   "ubuntu_24_04_x64",
		Resources: config.Resources{
			VPC: config.VPCResource{
				ID:   "vpc-xxx",
				CIDR: "192.168.0.0/16",
			},
			VSwitch: config.VSwitchResource{
				ID:     "vsw-xxx",
				ZoneID: "ap-southeast-1a",
			},
			SecurityGroup: config.SecurityGroupResource{
				ID: "sg-xxx",
			},
			ECS: config.ECSResource{
				ID:           "i-xxx",
				InstanceType: "ecs.e-c1m2.large",
				SystemDiskSize: 60,
				PublicIP:     "47.123.45.67",
				PrivateIP:    "192.168.1.100",
			},
			EIP: config.EIPResource{
				ID: "eip-xxx",
				IP: "47.123.45.67",
			},
			SSHKeyPair: config.SSHKeyPairResource{
				Name:           "cloudcode-ssh-key",
				PrivateKeyPath: ".cloudcode/ssh_key",
			},
		},
		CloudCode: config.CloudCodeConfig{
			Username: "admin",
			Domain:   "opencode.example.com",
		},
	}

	if err := config.SaveState(state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded, err := config.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if loaded.Version != state.Version {
		t.Errorf("Version mismatch: got %s, want %s", loaded.Version, state.Version)
	}
	if loaded.Region != state.Region {
		t.Errorf("Region mismatch: got %s, want %s", loaded.Region, state.Region)
	}
	if loaded.Resources.VPC.ID != state.Resources.VPC.ID {
		t.Errorf("VPC ID mismatch: got %s, want %s", loaded.Resources.VPC.ID, state.Resources.VPC.ID)
	}
	if loaded.Resources.ECS.PublicIP != state.Resources.ECS.PublicIP {
		t.Errorf("ECS PublicIP mismatch: got %s, want %s", loaded.Resources.ECS.PublicIP, state.Resources.ECS.PublicIP)
	}
}

func TestLoadState_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	_, err := config.LoadState()
	if !errors.Is(err, config.ErrStateNotFound) {
		t.Errorf("expected ErrStateNotFound, got %v", err)
	}
}

func TestState_DirPermissions(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	state := config.NewState("ap-southeast-1", "ubuntu_24_04_x64")
	if err := config.SaveState(state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	stateDir := filepath.Join(tempDir, config.StateDirName)
	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf("failed to stat state dir: %v", err)
	}

	if info.Mode().Perm() != 0700 {
		t.Errorf("state dir permissions: got %o, want 0700", info.Mode().Perm())
	}
}

func TestResolveKeyPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "relative path",
			input:    ".cloudcode/ssh_key",
			expected: filepath.Join(home, ".cloudcode", "ssh_key"),
		},
		{
			name:     "nested path",
			input:    ".cloudcode/secrets/key.pem",
			expected: filepath.Join(home, ".cloudcode", "secrets", "key.pem"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ResolveKeyPath(tt.input)
			if err != nil {
				t.Fatalf("ResolveKeyPath failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestState_HasMethods(t *testing.T) {
	state := &config.State{
		Resources: config.Resources{
			VPC:           config.VPCResource{ID: "vpc-xxx"},
			VSwitch:       config.VSwitchResource{ID: "vsw-xxx"},
			SecurityGroup: config.SecurityGroupResource{ID: "sg-xxx"},
			ECS:           config.ECSResource{ID: "i-xxx"},
			EIP:           config.EIPResource{ID: "eip-xxx"},
			SSHKeyPair:    config.SSHKeyPairResource{Name: "key"},
		},
	}

	if !state.HasVPC() {
		t.Error("HasVPC should return true")
	}
	if !state.HasVSwitch() {
		t.Error("HasVSwitch should return true")
	}
	if !state.HasSecurityGroup() {
		t.Error("HasSecurityGroup should return true")
	}
	if !state.HasECS() {
		t.Error("HasECS should return true")
	}
	if !state.HasEIP() {
		t.Error("HasEIP should return true")
	}
	if !state.HasSSHKeyPair() {
		t.Error("HasSSHKeyPair should return true")
	}
	if !state.IsComplete() {
		t.Error("IsComplete should return true")
	}
}

func TestState_HasMethods_Empty(t *testing.T) {
	state := &config.State{}

	if state.HasVPC() {
		t.Error("HasVPC should return false for empty state")
	}
	if state.IsComplete() {
		t.Error("IsComplete should return false for empty state")
	}
}

func TestDeleteState(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	state := config.NewState("ap-southeast-1", "ubuntu_24_04_x64")
	if err := config.SaveState(state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	if err := config.DeleteState(); err != nil {
		t.Fatalf("DeleteState failed: %v", err)
	}

	_, err := config.LoadState()
	if !errors.Is(err, config.ErrStateNotFound) {
		t.Errorf("expected ErrStateNotFound after delete, got %v", err)
	}
}

func TestNewState_SetsCreatedAt(t *testing.T) {
	state := config.NewState("ap-southeast-1", "ubuntu_24_04_x64")

	if state.CreatedAt == "" {
		t.Error("NewState should set CreatedAt")
	}

	if state.Version != config.StateFileVersion {
		t.Errorf("Version: got %s, want %s", state.Version, config.StateFileVersion)
	}

	if state.Region != "ap-southeast-1" {
		t.Errorf("Region: got %s, want ap-southeast-1", state.Region)
	}
}
