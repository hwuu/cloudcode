package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hwuu/cloudcode/internal/alicloud"
	"github.com/hwuu/cloudcode/internal/config"
)

func TestLoadConfig_EnvPriority(t *testing.T) {
	// 设置环境变量
	t.Setenv("ALICLOUD_ACCESS_KEY_ID", "env-key-id")
	t.Setenv("ALICLOUD_ACCESS_KEY_SECRET", "env-key-secret")
	t.Setenv("ALICLOUD_REGION", "cn-hangzhou")

	cfg, err := alicloud.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.AccessKeyID != "env-key-id" {
		t.Errorf("expected env-key-id, got %s", cfg.AccessKeyID)
	}
	if cfg.AccessKeySecret != "env-key-secret" {
		t.Errorf("expected env-key-secret, got %s", cfg.AccessKeySecret)
	}
	if cfg.RegionID != "cn-hangzhou" {
		t.Errorf("expected cn-hangzhou, got %s", cfg.RegionID)
	}
}

func TestLoadConfig_EnvDefaultRegion(t *testing.T) {
	t.Setenv("ALICLOUD_ACCESS_KEY_ID", "env-key-id")
	t.Setenv("ALICLOUD_ACCESS_KEY_SECRET", "env-key-secret")
	t.Setenv("ALICLOUD_REGION", "")

	cfg, err := alicloud.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.RegionID != alicloud.DefaultRegion {
		t.Errorf("expected default region %s, got %s", alicloud.DefaultRegion, cfg.RegionID)
	}
}

func TestLoadConfig_CredentialsFile(t *testing.T) {
	// 清除环境变量
	t.Setenv("ALICLOUD_ACCESS_KEY_ID", "")
	t.Setenv("ALICLOUD_ACCESS_KEY_SECRET", "")

	// 创建临时 credentials 文件
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")
	cred := &config.Credentials{
		AccessKeyID:     "file-key-id",
		AccessKeySecret: "file-key-secret",
		Region:          "ap-southeast-1",
	}
	if err := config.SaveCredentialsTo(credPath, cred); err != nil {
		t.Fatalf("SaveCredentialsTo failed: %v", err)
	}

	// LoadConfig 会尝试 ~/.cloudcode/credentials，这里直接测试 LoadCredentialsFrom
	loaded, err := config.LoadCredentialsFrom(credPath)
	if err != nil {
		t.Fatalf("LoadCredentialsFrom failed: %v", err)
	}
	if loaded.AccessKeyID != "file-key-id" {
		t.Errorf("expected file-key-id, got %s", loaded.AccessKeyID)
	}
}

func TestLoadConfig_EnvOverridesFile(t *testing.T) {
	// 即使 credentials 文件存在，环境变量应优先
	t.Setenv("ALICLOUD_ACCESS_KEY_ID", "env-key-id")
	t.Setenv("ALICLOUD_ACCESS_KEY_SECRET", "env-key-secret")
	t.Setenv("ALICLOUD_REGION", "")

	// 创建 credentials 文件（不应被使用）
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")
	os.WriteFile(credPath, []byte("access_key_id=file-key-id\naccess_key_secret=file-secret\nregion=cn-beijing\n"), 0600)

	cfg, err := alicloud.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.AccessKeyID != "env-key-id" {
		t.Errorf("env should override file, expected env-key-id, got %s", cfg.AccessKeyID)
	}
}

func TestLoadConfig_PartialEnv_MissingSecret(t *testing.T) {
	t.Setenv("ALICLOUD_ACCESS_KEY_ID", "env-key-id")
	t.Setenv("ALICLOUD_ACCESS_KEY_SECRET", "")

	// 确保 credentials 文件不存在
	home, _ := os.UserHomeDir()
	credPath := filepath.Join(home, ".cloudcode", "credentials")
	origContent, hadFile := tryReadFile(credPath)
	os.Remove(credPath)
	if hadFile {
		defer os.WriteFile(credPath, origContent, 0600)
	}

	_, err := alicloud.LoadConfig()
	if err == nil {
		t.Error("expected error when only partial env is set and no credentials file")
	}
}

func tryReadFile(path string) ([]byte, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return content, true
}
