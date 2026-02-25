// prompt.go 提供 CLI 交互式输入功能：文本输入、密码输入（掩码显示）、确认、选择。
// 同时包含密码哈希（Argon2id）和随机密钥生成工具。
package config

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/term"
)

// Argon2id 哈希参数，与 Authelia 配置保持一致
const (
	Argon2idIterations  = 1
	Argon2idSaltLength  = 16
	Argon2idParallelism = 8
	Argon2idMemory      = 64 // 单位 MiB，实际传给 argon2 时乘以 1024 转为 KiB
	Argon2idKeyLength   = 32

	SecretLength = 32 // 随机密钥长度（字节）
)

// Prompter 封装 CLI 交互式输入，通过 reader/writer 抽象支持 mock 测试
type Prompter struct {
	reader  io.Reader
	writer  io.Writer
	scanner *bufio.Scanner
}

// NewPrompter 创建 Prompter（指定输入输出流）
func NewPrompter(reader io.Reader, writer io.Writer) *Prompter {
	return &Prompter{
		reader:  reader,
		writer:  writer,
		scanner: bufio.NewScanner(reader),
	}
}

// NewDefaultPrompter 创建使用 stdin/stdout 的默认 Prompter
func NewDefaultPrompter() *Prompter {
	return &Prompter{
		reader: os.Stdin,
		writer: os.Stdout,
	}
}

// Prompt 显示提示信息并读取一行输入
func (p *Prompter) Prompt(message string) (string, error) {
	fmt.Fprint(p.writer, message)
	if !p.scanner.Scan() {
		if err := p.scanner.Err(); err != nil {
			return "", err
		}
		return "", nil
	}
	return strings.TrimSpace(p.scanner.Text()), nil
}

// PromptWithDefault 带默认值的输入提示，用户直接回车则使用默认值
func (p *Prompter) PromptWithDefault(message, defaultValue string) (string, error) {
	result, err := p.Prompt(fmt.Sprintf("%s [%s]: ", message, defaultValue))
	if err != nil {
		return "", err
	}
	if result == "" {
		return defaultValue, nil
	}
	return result, nil
}

// PromptPassword 密码输入，终端模式下每个字符显示为 *，支持退格删除。
// 非终端模式（如测试 mock）退化为普通文本读取。
func (p *Prompter) PromptPassword(message string) (string, error) {
	fmt.Fprint(p.writer, message)

	if f, ok := p.reader.(*os.File); ok {
		fd := int(f.Fd())
		password, err := readPassword(fd)
		if err != nil {
			return "", err
		}
		fmt.Fprintln(p.writer)
		return string(password), nil
	}

	if !p.scanner.Scan() {
		if err := p.scanner.Err(); err != nil {
			return "", err
		}
		return "", nil
	}
	return strings.TrimSpace(p.scanner.Text()), nil
}

// PromptConfirm 确认提示。defaultYes=true 时默认 yes [Y/n]，否则默认 no [y/N]
func (p *Prompter) PromptConfirm(message string, defaultYes bool) (bool, error) {
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	result, err := p.Prompt(fmt.Sprintf("%s %s: ", message, hint))
	if err != nil {
		return false, err
	}
	result = strings.ToLower(strings.TrimSpace(result))
	if result == "" {
		return defaultYes, nil
	}
	return result == "y", nil
}

// PromptSelect 显示选项列表，返回用户选择的索引（0-based）
func (p *Prompter) PromptSelect(message string, options []string) (int, error) {
	fmt.Fprintln(p.writer, message)
	for i, opt := range options {
		fmt.Fprintf(p.writer, "  %d) %s\n", i+1, opt)
	}

	result, err := p.Prompt("选择 [1]: ")
	if err != nil {
		return 0, err
	}

	if result == "" {
		return 0, nil
	}

	var choice int
	if _, err := fmt.Sscanf(result, "%d", &choice); err != nil {
		return 0, fmt.Errorf("invalid choice: %s", result)
	}

	if choice < 1 || choice > len(options) {
		return 0, fmt.Errorf("choice out of range: %d", choice)
	}

	return choice - 1, nil
}

// HashPassword 使用 Argon2id 算法哈希密码，返回 Authelia 兼容的格式：
// $argon2id$v=19$m=65536,t=1,p=8$<salt>$<hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, Argon2idSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		Argon2idIterations,
		Argon2idMemory*1024,
		Argon2idParallelism,
		Argon2idKeyLength,
	)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		Argon2idMemory*1024, Argon2idIterations, Argon2idParallelism, b64Salt, b64Hash), nil
}

// GenerateSecret 生成 32 字节随机密钥，base64 编码输出（用于 Authelia session/storage 密钥）
func GenerateSecret() (string, error) {
	bytes := make([]byte, SecretLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secret: %w", err)
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// readPassword 从终端读取密码，每输入一个字符显示 *，支持退格删除。
// 通过 term.MakeRaw 进入原始模式逐字符读取，退出时恢复终端状态。
func readPassword(fd int) ([]byte, error) {
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return term.ReadPassword(fd)
	}
	defer term.Restore(fd, oldState)

	var password []byte
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			break
		}
		ch := buf[0]
		switch {
		case ch == '\r' || ch == '\n':
			return password, nil
		case ch == 3: // Ctrl+C
			return nil, fmt.Errorf("interrupted")
		case ch == 127 || ch == 8: // Backspace / Delete
			if len(password) > 0 {
				password = password[:len(password)-1]
				os.Stdout.Write([]byte("\b \b"))
			}
		default:
			password = append(password, ch)
			os.Stdout.Write([]byte("*"))
		}
	}
	return password, nil
}

func readPasswordFallback() ([]byte, error) {
	reader := bufio.NewReader(os.Stdin)
	return reader.ReadBytes('\n')
}
