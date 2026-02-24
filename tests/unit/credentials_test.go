package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hwuu/cloudcode/internal/config"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	cred := &config.Credentials{
		AccessKeyID:     "LTAI5tTestKey",
		AccessKeySecret: "TestSecret123",
		Region:          "ap-southeast-1",
	}

	if err := config.SaveCredentialsTo(path, cred); err != nil {
		t.Fatalf("SaveCredentialsTo failed: %v", err)
	}

	// 验证文件权限为 600
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected permission 0600, got %04o", perm)
	}

	// 加载并验证
	loaded, err := config.LoadCredentialsFrom(path)
	if err != nil {
		t.Fatalf("LoadCredentialsFrom failed: %v", err)
	}
	if loaded.AccessKeyID != cred.AccessKeyID {
		t.Errorf("AccessKeyID: expected %s, got %s", cred.AccessKeyID, loaded.AccessKeyID)
	}
	if loaded.AccessKeySecret != cred.AccessKeySecret {
		t.Errorf("AccessKeySecret: expected %s, got %s", cred.AccessKeySecret, loaded.AccessKeySecret)
	}
	if loaded.Region != cred.Region {
		t.Errorf("Region: expected %s, got %s", cred.Region, loaded.Region)
	}
}

func TestLoadCredentials_NotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent")
	_, err := config.LoadCredentialsFrom(path)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadCredentials_MissingAccessKeyID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	os.WriteFile(path, []byte("access_key_secret=secret\nregion=ap-southeast-1\n"), 0600)

	_, err := config.LoadCredentialsFrom(path)
	if err == nil {
		t.Error("expected error for missing access_key_id")
	}
}

func TestLoadCredentials_MissingAccessKeySecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	os.WriteFile(path, []byte("access_key_id=LTAI5t\nregion=ap-southeast-1\n"), 0600)

	_, err := config.LoadCredentialsFrom(path)
	if err == nil {
		t.Error("expected error for missing access_key_secret")
	}
}

func TestLoadCredentials_ValueContainsEquals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	// secret 中包含 = 字符
	os.WriteFile(path, []byte("access_key_id=LTAI5t\naccess_key_secret=abc=def=ghi\nregion=ap-southeast-1\n"), 0600)

	cred, err := config.LoadCredentialsFrom(path)
	if err != nil {
		t.Fatalf("LoadCredentialsFrom failed: %v", err)
	}
	if cred.AccessKeySecret != "abc=def=ghi" {
		t.Errorf("expected secret 'abc=def=ghi', got '%s'", cred.AccessKeySecret)
	}
}

func TestLoadCredentials_DefaultRegion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	os.WriteFile(path, []byte("access_key_id=LTAI5t\naccess_key_secret=secret\n"), 0600)

	cred, err := config.LoadCredentialsFrom(path)
	if err != nil {
		t.Fatalf("LoadCredentialsFrom failed: %v", err)
	}
	if cred.Region != "" {
		t.Errorf("expected empty region, got '%s'", cred.Region)
	}
}

func TestLoadCredentials_CommentsAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	content := "# 阿里云凭证\naccess_key_id=LTAI5t\n\n# secret\naccess_key_secret=secret\nregion=ap-southeast-1\n"
	os.WriteFile(path, []byte(content), 0600)

	cred, err := config.LoadCredentialsFrom(path)
	if err != nil {
		t.Fatalf("LoadCredentialsFrom failed: %v", err)
	}
	if cred.AccessKeyID != "LTAI5t" {
		t.Errorf("expected 'LTAI5t', got '%s'", cred.AccessKeyID)
	}
}
