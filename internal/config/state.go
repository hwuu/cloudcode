// Package config 管理 CloudCode 的持久化状态和用户交互。
// 状态文件（state.json）记录所有已创建的云资源 ID，支持幂等部署和中断恢复。
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
	StateDirName     = ".cloudcode"  // 状态目录，位于用户 home 下
	StateFileName    = "state.json"  // 状态文件名
)

var (
	ErrStateNotFound      = errors.New("state file not found")
	ErrStateCorrupted     = errors.New("state file corrupted")
	ErrStateAlreadyExists = errors.New("state already exists")
)

// 以下结构体对应 state.json 中的资源记录，每个字段存储阿里云资源 ID。
// deploy 每创建一个资源就立即写入 state，确保中断后可恢复。

// VPCResource VPC 资源
type VPCResource struct {
	ID   string `json:"id"`
	CIDR string `json:"cidr"`
}

// VSwitchResource 交换机资源
type VSwitchResource struct {
	ID     string `json:"id"`
	ZoneID string `json:"zone"`
	CIDR   string `json:"cidr,omitempty"`
}

// SecurityGroupResource 安全组资源
type SecurityGroupResource struct {
	ID string `json:"id"`
}

// ECSResource ECS 实例资源
type ECSResource struct {
	ID             string `json:"id"`
	InstanceType   string `json:"instance_type"`
	SystemDiskSize int    `json:"system_disk_size"`
	PublicIP       string `json:"public_ip"`
	PrivateIP      string `json:"private_ip"`
}

// EIPResource 弹性公网 IP 资源
type EIPResource struct {
	ID string `json:"id"`
	IP string `json:"ip"`
}

// SSHKeyPairResource SSH 密钥对资源（私钥存储在本地文件，不写入 state）
type SSHKeyPairResource struct {
	Name           string `json:"name"`
	PrivateKeyPath string `json:"private_key_path"`
}

// Resources 所有云资源的集合
type Resources struct {
	VPC           VPCResource           `json:"vpc"`
	VSwitch       VSwitchResource       `json:"vswitch"`
	SecurityGroup SecurityGroupResource `json:"security_group"`
	ECS           ECSResource           `json:"ecs"`
	EIP           EIPResource           `json:"eip"`
	SSHKeyPair    SSHKeyPairResource    `json:"ssh_key_pair"`
}

// CloudCodeConfig 应用层配置（域名、用户名等）
type CloudCodeConfig struct {
	Username string `json:"username"`
	Domain   string `json:"domain"`
}

// State 部署状态，序列化为 ~/.cloudcode/state.json
type State struct {
	Version   string          `json:"version"`
	CreatedAt string          `json:"created_at"`
	Region    string          `json:"region"`
	OSImage   string          `json:"os_image"`
	Status    string          `json:"status,omitempty"` // running / suspended / destroyed
	Resources Resources       `json:"resources"`
	CloudCode CloudCodeConfig `json:"cloudcode"`
}

// GetStateDir 返回状态目录路径（~/.cloudcode/）
func GetStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(home, StateDirName), nil
}

// GetStatePath 返回状态文件完整路径（~/.cloudcode/state.json）
func GetStatePath() (string, error) {
	stateDir, err := GetStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(stateDir, StateFileName), nil
}

// EnsureStateDir 确保状态目录存在（权限 0700，仅当前用户可访问）
func EnsureStateDir() error {
	stateDir, err := GetStateDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(stateDir, 0700)
}

// LoadState 从默认路径加载状态文件
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

// SaveState 将状态写入默认路径（自动创建目录，权限 0600）
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

// DeleteState 删除状态文件
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

// HasVPC ~ HasSSHKeyPair 判断对应资源是否已创建（用于幂等部署）
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

// IsComplete 判断所有云资源是否已创建完毕
func (s *State) IsComplete() bool {
	return s.HasVPC() && s.HasVSwitch() && s.HasSecurityGroup() &&
		s.HasECS() && s.HasEIP() && s.HasSSHKeyPair()
}

// ResolveKeyPath 将相对路径解析为基于用户 home 目录的绝对路径
func ResolveKeyPath(relativePath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(home, relativePath), nil
}

// NewState 创建新的空状态（自动填充版本号和创建时间）
func NewState(region, osImage string) *State {
	return &State{
		Version:   StateFileVersion,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Region:    region,
		OSImage:   osImage,
		Resources: Resources{},
	}
}
