package unit

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
	"github.com/hwuu/cloudcode/internal/config"
	"github.com/hwuu/cloudcode/internal/deploy"
)

func TestSuspend_Success(t *testing.T) {
	dir := t.TempDir()
	state := &config.State{
		Version: "1.0",
		Status:  "running",
		Resources: config.Resources{
			ECS: config.ECSResource{ID: "i-test-001"},
			EIP: config.EIPResource{ID: "eip-test", IP: "1.2.3.4"},
		},
	}
	saveStateTo(t, dir, state)

	stopCalled := false
	describeCount := 0
	mockECS := &MockECSAPI{
		StopInstanceFunc: func(req *ecsclient.StopInstanceRequest) (*ecsclient.StopInstanceResponse, error) {
			stopCalled = true
			if req.StoppedMode == nil || *req.StoppedMode != "StopCharging" {
				t.Error("expected StopCharging mode")
			}
			return &ecsclient.StopInstanceResponse{}, nil
		},
		DescribeInstancesFunc: func(req *ecsclient.DescribeInstancesRequest) (*ecsclient.DescribeInstancesResponse, error) {
			describeCount++
			status := "Stopped"
			id := "i-test-001"
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
	}

	var buf bytes.Buffer
	prompter := config.NewPrompter(strings.NewReader("y\n"), &buf)
	s := &deploy.Suspender{
		ECS:          mockECS,
		Prompter:     prompter,
		Output:       &buf,
		Region:       "ap-southeast-1",
		StateDir:     dir,
		WaitInterval: time.Second,
		WaitTimeout:  5 * time.Second,
	}

	err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stopCalled {
		t.Error("expected StopInstance to be called")
	}

	// 验证 state 更新
	updated, err := loadStateFrom(t, dir)
	if err != nil {
		t.Fatalf("load state failed: %v", err)
	}
	if updated.Status != "suspended" {
		t.Errorf("expected status 'suspended', got '%s'", updated.Status)
	}
}

func TestSuspend_AlreadySuspended(t *testing.T) {
	dir := t.TempDir()
	state := &config.State{
		Version: "1.0",
		Status:  "suspended",
		Resources: config.Resources{
			ECS: config.ECSResource{ID: "i-test-001"},
		},
	}
	saveStateTo(t, dir, state)

	var buf bytes.Buffer
	prompter := config.NewPrompter(strings.NewReader(""), &buf)
	s := &deploy.Suspender{
		ECS:      &MockECSAPI{},
		Prompter: prompter,
		Output:   &buf,
		Region:   "ap-southeast-1",
		StateDir: dir,
	}

	err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "已处于停机状态") {
		t.Error("expected already suspended message")
	}
}

func TestSuspend_Cancelled(t *testing.T) {
	dir := t.TempDir()
	state := &config.State{
		Version: "1.0",
		Status:  "running",
		Resources: config.Resources{
			ECS: config.ECSResource{ID: "i-test-001"},
		},
	}
	saveStateTo(t, dir, state)

	var buf bytes.Buffer
	prompter := config.NewPrompter(strings.NewReader("n\n"), &buf)
	s := &deploy.Suspender{
		ECS:      &MockECSAPI{},
		Prompter: prompter,
		Output:   &buf,
		Region:   "ap-southeast-1",
		StateDir: dir,
	}

	err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "已取消") {
		t.Error("expected cancelled message")
	}
}

func TestResume_Success(t *testing.T) {
	dir := t.TempDir()
	state := &config.State{
		Version: "1.0",
		Status:  "suspended",
		Resources: config.Resources{
			ECS: config.ECSResource{ID: "i-test-001"},
			EIP: config.EIPResource{ID: "eip-test", IP: "1.2.3.4"},
		},
		CloudCode: config.CloudCodeConfig{Domain: "test.example.com"},
	}
	saveStateTo(t, dir, state)

	startCalled := false
	mockECS := &MockECSAPI{
		StartInstanceFunc: func(req *ecsclient.StartInstanceRequest) (*ecsclient.StartInstanceResponse, error) {
			startCalled = true
			return &ecsclient.StartInstanceResponse{}, nil
		},
		DescribeInstancesFunc: func(req *ecsclient.DescribeInstancesRequest) (*ecsclient.DescribeInstancesResponse, error) {
			status := "Running"
			id := "i-test-001"
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
	}

	var buf bytes.Buffer
	prompter := config.NewPrompter(strings.NewReader("y\n"), &buf)
	r := &deploy.Resumer{
		ECS:          mockECS,
		Prompter:     prompter,
		Output:       &buf,
		Region:       "ap-southeast-1",
		StateDir:     dir,
		WaitInterval: time.Second,
		WaitTimeout:  5 * time.Second,
	}

	err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !startCalled {
		t.Error("expected StartInstance to be called")
	}

	updated, err := loadStateFrom(t, dir)
	if err != nil {
		t.Fatalf("load state failed: %v", err)
	}
	if updated.Status != "running" {
		t.Errorf("expected status 'running', got '%s'", updated.Status)
	}
}

func TestResume_AlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	state := &config.State{
		Version: "1.0",
		Status:  "running",
		Resources: config.Resources{
			ECS: config.ECSResource{ID: "i-test-001"},
		},
	}
	saveStateTo(t, dir, state)

	var buf bytes.Buffer
	prompter := config.NewPrompter(strings.NewReader(""), &buf)
	r := &deploy.Resumer{
		ECS:      &MockECSAPI{},
		Prompter: prompter,
		Output:   &buf,
		Region:   "ap-southeast-1",
		StateDir: dir,
	}

	err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "已在运行中") {
		t.Error("expected already running message")
	}
}

// helpers
func saveStateTo(t *testing.T, dir string, state *config.State) {
	t.Helper()
	data, _ := json.Marshal(state)
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "state.json"), data, 0600)
}

func loadStateFrom(t *testing.T, dir string) (*config.State, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		return nil, err
	}
	var state config.State
	json.Unmarshal(data, &state)
	return &state, nil
}
