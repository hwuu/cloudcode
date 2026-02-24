package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	CredentialsFileName = "credentials"
)

// Credentials 阿里云凭证，从 ~/.cloudcode/credentials 文件加载
type Credentials struct {
	AccessKeyID     string
	AccessKeySecret string
	Region          string
}

// LoadCredentials 从 ~/.cloudcode/credentials 文件加载凭证。
// 文件格式为 key=value（只取第一个 = 分割）。
func LoadCredentials() (*Credentials, error) {
	stateDir, err := GetStateDir()
	if err != nil {
		return nil, err
	}
	return LoadCredentialsFrom(filepath.Join(stateDir, CredentialsFileName))
}

// LoadCredentialsFrom 从指定路径加载凭证文件
func LoadCredentialsFrom(path string) (*Credentials, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("凭证文件不存在，请先运行 cloudcode init")
		}
		return nil, fmt.Errorf("读取凭证文件失败: %w", err)
	}
	defer f.Close()

	kv := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		kv[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取凭证文件失败: %w", err)
	}

	cred := &Credentials{
		AccessKeyID:     kv["access_key_id"],
		AccessKeySecret: kv["access_key_secret"],
		Region:          kv["region"],
	}

	if cred.AccessKeyID == "" {
		return nil, fmt.Errorf("凭证文件缺少 access_key_id，请运行 cloudcode init 重新配置")
	}
	if cred.AccessKeySecret == "" {
		return nil, fmt.Errorf("凭证文件缺少 access_key_secret，请运行 cloudcode init 重新配置")
	}

	return cred, nil
}

// SaveCredentials 将凭证保存到 ~/.cloudcode/credentials，权限 600
func SaveCredentials(cred *Credentials) error {
	stateDir, err := GetStateDir()
	if err != nil {
		return err
	}
	return SaveCredentialsTo(filepath.Join(stateDir, CredentialsFileName), cred)
}

// SaveCredentialsTo 将凭证保存到指定路径，权限 600
func SaveCredentialsTo(path string, cred *Credentials) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	content := fmt.Sprintf("access_key_id=%s\naccess_key_secret=%s\nregion=%s\n",
		cred.AccessKeyID, cred.AccessKeySecret, cred.Region)

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("保存凭证文件失败: %w", err)
	}
	return nil
}
