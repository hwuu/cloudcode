package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	StateFileVersion = "1.0"
	StateDirName     = ".cloudcode"
	StateFileName    = "state.json"
)

var (
	ErrStateNotFound      = errors.New("state file not found")
	ErrStateCorrupted     = errors.New("state file corrupted")
	ErrStateAlreadyExists = errors.New("state already exists")
)

type VPCResource struct {
	ID   string `json:"id"`
	CIDR string `json:"cidr"`
}

type VSwitchResource struct {
	ID     string `json:"id"`
	ZoneID string `json:"zone"`
	CIDR   string `json:"cidr,omitempty"`
}

type SecurityGroupResource struct {
	ID string `json:"id"`
}

type ECSResource struct {
	ID           string `json:"id"`
	InstanceType string `json:"instance_type"`
	SystemDiskSize int   `json:"system_disk_size"`
	PublicIP     string `json:"public_ip"`
	PrivateIP    string `json:"private_ip"`
}

type EIPResource struct {
	ID string `json:"id"`
	IP string `json:"ip"`
}

type SSHKeyPairResource struct {
	Name           string `json:"name"`
	PrivateKeyPath string `json:"private_key_path"`
}

type Resources struct {
	VPC            VPCResource            `json:"vpc"`
	VSwitch        VSwitchResource        `json:"vswitch"`
	SecurityGroup  SecurityGroupResource  `json:"security_group"`
	ECS            ECSResource            `json:"ecs"`
	EIP            EIPResource            `json:"eip"`
	SSHKeyPair     SSHKeyPairResource     `json:"ssh_key_pair"`
}

type CloudCodeConfig struct {
	Username string `json:"username"`
	Domain   string `json:"domain"`
}

type State struct {
	Version   string          `json:"version"`
	CreatedAt string          `json:"created_at"`
	Region    string          `json:"region"`
	OSImage   string          `json:"os_image"`
	Resources Resources       `json:"resources"`
	CloudCode CloudCodeConfig `json:"cloudcode"`
}

func GetStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(home, StateDirName), nil
}

func GetStatePath() (string, error) {
	stateDir, err := GetStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(stateDir, StateFileName), nil
}

func EnsureStateDir() error {
	stateDir, err := GetStateDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(stateDir, 0700)
}

func LoadState() (*State, error) {
	statePath, err := GetStatePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrStateNotFound
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStateCorrupted, err)
	}

	return &state, nil
}

func SaveState(state *State) error {
	if err := EnsureStateDir(); err != nil {
		return err
	}

	statePath, err := GetStatePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(statePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

func DeleteState() error {
	statePath, err := GetStatePath()
	if err != nil {
		return err
	}

	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete state file: %w", err)
	}

	return nil
}

func (s *State) HasVPC() bool {
	return s.Resources.VPC.ID != ""
}

func (s *State) HasVSwitch() bool {
	return s.Resources.VSwitch.ID != ""
}

func (s *State) HasSecurityGroup() bool {
	return s.Resources.SecurityGroup.ID != ""
}

func (s *State) HasECS() bool {
	return s.Resources.ECS.ID != ""
}

func (s *State) HasEIP() bool {
	return s.Resources.EIP.ID != ""
}

func (s *State) HasSSHKeyPair() bool {
	return s.Resources.SSHKeyPair.Name != ""
}

func (s *State) IsComplete() bool {
	return s.HasVPC() && s.HasVSwitch() && s.HasSecurityGroup() &&
		s.HasECS() && s.HasEIP() && s.HasSSHKeyPair()
}

func ResolveKeyPath(relativePath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(home, relativePath), nil
}

func NewState(region, osImage string) *State {
	return &State{
		Version:   StateFileVersion,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Region:    region,
		OSImage:   osImage,
		Resources: Resources{},
	}
}
