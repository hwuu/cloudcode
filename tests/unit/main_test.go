package unit

import (
	"bytes"
	"os/exec"
	"testing"
)

// 辅助函数：构建 CLI 二进制并返回路径
func buildBinary(t *testing.T) string {
	t.Helper()
	binary := t.TempDir() + "/cloudcode"
	cmd := exec.Command("go", "build", "-o", binary, "../../cmd/cloudcode")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("构建失败: %v\n%s", err, out)
	}
	return binary
}

// 辅助函数：运行 CLI 命令并返回输出
func runBinary(t *testing.T, binary string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binary, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	err := cmd.Run()
	if err != nil {
		t.Fatalf("运行失败: %v\n%s", err, stdout.String())
	}
	return stdout.String()
}

func TestRootCommandHelp(t *testing.T) {
	binary := buildBinary(t)
	output := runBinary(t, binary, "--help")

	expected := []string{"cloudcode", "deploy", "status", "destroy", "version"}
	for _, s := range expected {
		if !bytes.Contains([]byte(output), []byte(s)) {
			t.Errorf("help 输出缺少 %q\n实际输出:\n%s", s, output)
		}
	}
}

func TestVersionOutput(t *testing.T) {
	binary := buildBinary(t)
	output := runBinary(t, binary, "version")

	expected := []string{"cloudcode", "commit:", "built:", "go:"}
	for _, s := range expected {
		if !bytes.Contains([]byte(output), []byte(s)) {
			t.Errorf("version 输出缺少 %q\n实际输出:\n%s", s, output)
		}
	}
}

func TestSubcommandsRegistered(t *testing.T) {
	binary := buildBinary(t)
	output := runBinary(t, binary, "--help")

	subcommands := []string{"deploy", "status", "destroy", "version"}
	for _, cmd := range subcommands {
		if !bytes.Contains([]byte(output), []byte(cmd)) {
			t.Errorf("子命令 %q 未注册\n实际输出:\n%s", cmd, output)
		}
	}
}
