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

const (
	Argon2idIterations  = 1
	Argon2idSaltLength  = 16
	Argon2idParallelism = 8
	Argon2idMemory      = 64
	Argon2idKeyLength   = 32

	SecretLength = 32
)

type Prompter struct {
	reader io.Reader
	writer io.Writer
}

func NewPrompter(reader io.Reader, writer io.Writer) *Prompter {
	return &Prompter{
		reader: reader,
		writer: writer,
	}
}

func NewDefaultPrompter() *Prompter {
	return &Prompter{
		reader: os.Stdin,
		writer: os.Stdout,
	}
}

func (p *Prompter) Prompt(message string) (string, error) {
	fmt.Fprint(p.writer, message)
	scanner := bufio.NewScanner(p.reader)
	if !scanner.Scan() {
		return "", scanner.Err()
	}
	return strings.TrimSpace(scanner.Text()), nil
}

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

	scanner := bufio.NewScanner(p.reader)
	if !scanner.Scan() {
		return "", scanner.Err()
	}
	return strings.TrimSpace(scanner.Text()), nil
}

func (p *Prompter) PromptConfirm(message string) (bool, error) {
	result, err := p.Prompt(fmt.Sprintf("%s [y/N]: ", message))
	if err != nil {
		return false, err
	}
	return strings.ToLower(result) == "y", nil
}

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

func GenerateSecret() (string, error) {
	bytes := make([]byte, SecretLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secret: %w", err)
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

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
