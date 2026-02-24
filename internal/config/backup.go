package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const BackupFileName = "backup.json"

// Backup 快照备份元数据
type Backup struct {
	CloudCodeVersion string `json:"cloudcode_version"`
	SnapshotID       string `json:"snapshot_id"`
	CreatedAt        string `json:"created_at"`
	Region           string `json:"region"`
	DiskSize         int    `json:"disk_size"`
	Domain           string `json:"domain"`
	Username         string `json:"username"`
}

// LoadBackup 从默认路径加载备份文件
func LoadBackup() (*Backup, error) {
	stateDir, err := GetStateDir()
	if err != nil {
		return nil, err
	}
	return LoadBackupFrom(stateDir)
}

// LoadBackupFrom 从指定目录加载备份文件
func LoadBackupFrom(dir string) (*Backup, error) {
	path := filepath.Join(dir, BackupFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 无备份文件不是错误
		}
		return nil, fmt.Errorf("读取备份文件失败: %w", err)
	}
	var backup Backup
	if err := json.Unmarshal(data, &backup); err != nil {
		return nil, fmt.Errorf("解析备份文件失败: %w", err)
	}
	return &backup, nil
}

// SaveBackup 保存备份文件到默认路径
func SaveBackup(backup *Backup) error {
	stateDir, err := GetStateDir()
	if err != nil {
		return err
	}
	return SaveBackupTo(stateDir, backup)
}

// SaveBackupTo 保存备份文件到指定目录
func SaveBackupTo(dir string, backup *Backup) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, BackupFileName)
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// DeleteBackup 删除备份文件
func DeleteBackup() error {
	stateDir, err := GetStateDir()
	if err != nil {
		return err
	}
	return DeleteBackupFrom(stateDir)
}

// DeleteBackupFrom 删除指定目录的备份文件
func DeleteBackupFrom(dir string) error {
	path := filepath.Join(dir, BackupFileName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除备份文件失败: %w", err)
	}
	return nil
}
