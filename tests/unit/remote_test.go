package unit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hwuu/cloudcode/internal/remote"
)

// --- Mock implementations ---

type MockSSHClient struct {
	RunCommandFunc func(ctx context.Context, cmd string) (string, error)
	CloseFunc      func() error
}

func (m *MockSSHClient) RunCommand(ctx context.Context, cmd string) (string, error) {
	return m.RunCommandFunc(ctx, cmd)
}

func (m *MockSSHClient) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

type MockSFTPClient struct {
	UploadFileFunc func(localContent []byte, remotePath string) error
	CloseFunc      func() error
}

func (m *MockSFTPClient) UploadFile(localContent []byte, remotePath string) error {
	return m.UploadFileFunc(localContent, remotePath)
}

func (m *MockSFTPClient) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// --- SSH Tests ---

func TestRunCommand_Success(t *testing.T) {
	mock := &MockSSHClient{
		RunCommandFunc: func(ctx context.Context, cmd string) (string, error) {
			if cmd != "echo hello" {
				t.Errorf("unexpected command: %s", cmd)
			}
			return "hello\n", nil
		},
	}

	ctx := context.Background()
	output, err := mock.RunCommand(ctx, "echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "hello\n" {
		t.Errorf("expected %q, got %q", "hello\n", output)
	}
}

func TestRunCommand_Error(t *testing.T) {
	mock := &MockSSHClient{
		RunCommandFunc: func(ctx context.Context, cmd string) (string, error) {
			return "", errors.New("command failed")
		},
	}

	ctx := context.Background()
	_, err := mock.RunCommand(ctx, "bad-command")
	if err == nil {
		t.Error("expected error")
	}
}

func TestWaitForSSH_Success(t *testing.T) {
	attempts := 0
	dialFunc := func() (remote.SSHClient, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("connection refused")
		}
		return &MockSSHClient{
			RunCommandFunc: func(ctx context.Context, cmd string) (string, error) {
				return "", nil
			},
		}, nil
	}

	ctx := context.Background()
	client, err := remote.WaitForSSH(ctx, dialFunc, remote.WaitSSHOptions{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		Timeout:         2 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestWaitForSSH_Timeout(t *testing.T) {
	dialFunc := func() (remote.SSHClient, error) {
		return nil, errors.New("connection refused")
	}

	ctx := context.Background()
	_, err := remote.WaitForSSH(ctx, dialFunc, remote.WaitSSHOptions{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     20 * time.Millisecond,
		Timeout:         100 * time.Millisecond,
	})
	if err == nil {
		t.Error("expected timeout error")
	}
	if !errors.Is(err, remote.ErrSSHWaitTimeout) {
		t.Errorf("expected ErrSSHWaitTimeout, got %v", err)
	}
}

func TestWaitForSSH_ExponentialBackoff(t *testing.T) {
	var timestamps []time.Time
	dialFunc := func() (remote.SSHClient, error) {
		timestamps = append(timestamps, time.Now())
		if len(timestamps) < 4 {
			return nil, errors.New("connection refused")
		}
		return &MockSSHClient{
			RunCommandFunc: func(ctx context.Context, cmd string) (string, error) {
				return "", nil
			},
		}, nil
	}

	ctx := context.Background()
	_, err := remote.WaitForSSH(ctx, dialFunc, remote.WaitSSHOptions{
		InitialInterval: 50 * time.Millisecond,
		MaxInterval:     200 * time.Millisecond,
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证间隔递增（指数退避）
	for i := 1; i < len(timestamps)-1; i++ {
		gap1 := timestamps[i].Sub(timestamps[i-1])
		gap2 := timestamps[i+1].Sub(timestamps[i])
		// 后一个间隔应该 >= 前一个间隔（允许调度误差）
		if gap2 < gap1-20*time.Millisecond {
			t.Errorf("backoff not increasing: gap[%d]=%v, gap[%d]=%v", i-1, gap1, i, gap2)
		}
	}
}

// --- SFTP Tests ---

func TestUploadFile_Success(t *testing.T) {
	var uploadedPath string
	var uploadedContent []byte

	mock := &MockSFTPClient{
		UploadFileFunc: func(content []byte, remotePath string) error {
			uploadedContent = content
			uploadedPath = remotePath
			return nil
		},
	}

	content := []byte("test file content")
	err := mock.UploadFile(content, "/root/cloudcode/test.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uploadedPath != "/root/cloudcode/test.txt" {
		t.Errorf("expected path %q, got %q", "/root/cloudcode/test.txt", uploadedPath)
	}
	if string(uploadedContent) != "test file content" {
		t.Errorf("content mismatch")
	}
}

func TestUploadFile_Error(t *testing.T) {
	mock := &MockSFTPClient{
		UploadFileFunc: func(content []byte, remotePath string) error {
			return errors.New("upload failed")
		},
	}

	err := mock.UploadFile([]byte("data"), "/root/test.txt")
	if err == nil {
		t.Error("expected error")
	}
}

func TestUploadFiles_Multiple(t *testing.T) {
	uploaded := make(map[string]string)

	mock := &MockSFTPClient{
		UploadFileFunc: func(content []byte, remotePath string) error {
			uploaded[remotePath] = string(content)
			return nil
		},
	}

	files := map[string][]byte{
		"/root/cloudcode/docker-compose.yml": []byte("compose content"),
		"/root/cloudcode/Caddyfile":          []byte("caddy content"),
		"/root/cloudcode/.env":               []byte("env content"),
	}

	err := remote.UploadFiles(mock, files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(uploaded) != 3 {
		t.Errorf("expected 3 files uploaded, got %d", len(uploaded))
	}
	if uploaded["/root/cloudcode/docker-compose.yml"] != "compose content" {
		t.Error("docker-compose.yml content mismatch")
	}
}

func TestUploadFiles_StopsOnError(t *testing.T) {
	callCount := 0
	mock := &MockSFTPClient{
		UploadFileFunc: func(content []byte, remotePath string) error {
			callCount++
			return errors.New("upload failed")
		},
	}

	files := map[string][]byte{
		"/root/a.txt": []byte("a"),
		"/root/b.txt": []byte("b"),
	}

	err := remote.UploadFiles(mock, files)
	if err == nil {
		t.Error("expected error")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call before stopping, got %d", callCount)
	}
}
