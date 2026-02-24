package unit

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/hwuu/cloudcode/internal/config"
)

func TestPrompter_Prompt(t *testing.T) {
	input := "test-input\n"
	output := &bytes.Buffer{}
	prompter := config.NewPrompter(strings.NewReader(input), output)

	result, err := prompter.Prompt("Enter value: ")
	if err != nil {
		t.Fatalf("Prompt failed: %v", err)
	}

	if result != "test-input" {
		t.Errorf("got %q, want %q", result, "test-input")
	}
}

func TestPrompter_PromptWithDefault(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		defaultValue string
		expected     string
	}{
		{
			name:         "user input",
			input:        "user-value\n",
			defaultValue: "default",
			expected:     "user-value",
		},
		{
			name:         "empty input uses default",
			input:        "\n",
			defaultValue: "default",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			prompter := config.NewPrompter(strings.NewReader(tt.input), output)

			result, err := prompter.PromptWithDefault("Enter value", tt.defaultValue)
			if err != nil {
				t.Fatalf("PromptWithDefault failed: %v", err)
			}

			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestPrompter_PromptConfirm(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultYes bool
		expected   bool
	}{
		{"yes lowercase", "y\n", false, true},
		{"yes uppercase", "Y\n", false, true},
		{"no", "n\n", false, false},
		{"empty default no", "\n", false, false},
		{"empty default yes", "\n", true, true},
		{"random", "foo\n", false, false},
		{"no override default yes", "n\n", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			prompter := config.NewPrompter(strings.NewReader(tt.input), output)

			result, err := prompter.PromptConfirm("Confirm?", tt.defaultYes)
			if err != nil {
				t.Fatalf("PromptConfirm failed: %v", err)
			}

			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestPrompter_PromptSelect(t *testing.T) {
	options := []string{"OpenAI", "Anthropic", "Custom"}

	tests := []struct {
		name        string
		input       string
		expectedIdx int
	}{
		{"select first", "1\n", 0},
		{"select second", "2\n", 1},
		{"select third", "3\n", 2},
		{"empty defaults to first", "\n", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			prompter := config.NewPrompter(strings.NewReader(tt.input), output)

			idx, err := prompter.PromptSelect("Choice:", options)
			if err != nil {
				t.Fatalf("PromptSelect failed: %v", err)
			}

			if idx != tt.expectedIdx {
				t.Errorf("got %d, want %d", idx, tt.expectedIdx)
			}
		})
	}
}

func TestPrompter_PromptSelect_Invalid(t *testing.T) {
	options := []string{"A", "B"}
	output := &bytes.Buffer{}
	prompter := config.NewPrompter(strings.NewReader("abc\n"), output)

	_, err := prompter.PromptSelect("Choice:", options)
	if err == nil {
		t.Error("expected error for invalid input")
	}
}

func TestPrompter_PromptSelect_OutOfRange(t *testing.T) {
	options := []string{"A", "B"}
	output := &bytes.Buffer{}
	prompter := config.NewPrompter(strings.NewReader("5\n"), output)

	_, err := prompter.PromptSelect("Choice:", options)
	if err == nil {
		t.Error("expected error for out of range")
	}
}

func TestHashPassword_Format(t *testing.T) {
	password := "test-password-123"

	hash, err := config.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Errorf("hash should start with $argon2id$v=19$, got %s", hash[:20])
	}

	if !strings.Contains(hash, "m=65536,t=1,p=8$") {
		t.Errorf("hash should contain memory/iterations/parallelism params, got %s", hash)
	}

	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Errorf("hash should have 6 parts separated by $, got %d", len(parts))
	}
}

func TestHashPassword_MemoryParameter(t *testing.T) {
	password := "test"

	hash, err := config.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if !strings.Contains(hash, "m=65536,") {
		t.Errorf("memory should be 65536 KiB (64 MiB), got hash: %s", hash)
	}
}

func TestHashPassword_UniqueSalts(t *testing.T) {
	password := "same-password"

	hash1, err := config.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword 1 failed: %v", err)
	}

	hash2, err := config.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword 2 failed: %v", err)
	}

	if hash1 == hash2 {
		t.Error("same password should produce different hashes due to random salt")
	}
}

func TestGenerateSecret_Length(t *testing.T) {
	secret, err := config.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret failed: %v", err)
	}

	decoded, err := decodeBase64(secret)
	if err != nil {
		t.Fatalf("failed to decode secret: %v", err)
	}

	if len(decoded) != config.SecretLength {
		t.Errorf("decoded secret length: got %d, want %d", len(decoded), config.SecretLength)
	}
}

func TestGenerateSecret_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		secret, err := config.GenerateSecret()
		if err != nil {
			t.Fatalf("GenerateSecret failed: %v", err)
		}
		if seen[secret] {
			t.Errorf("duplicate secret generated: %s", secret)
		}
		seen[secret] = true
	}
}

func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
