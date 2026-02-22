package unit

import (
	"strings"
	"testing"

	tmpl "github.com/hwuu/cloudcode/internal/template"
)

func testData() *tmpl.TemplateData {
	return &tmpl.TemplateData{
		Domain:               "opencode.example.com",
		Username:             "admin",
		HashedPassword:       "$argon2id$v=19$m=65536,t=1,p=8$salt$hash",
		Email:                "admin@example.com",
		SessionSecret:        "test-session-secret",
		StorageEncryptionKey: "test-storage-key",
		OpenAIAPIKey:         "sk-test-key",
		OpenAIBaseURL:        "https://api.openai.com/v1",
		AnthropicAPIKey:      "sk-ant-test",
		Version:              "0.2.0-dev",
	}
}

func TestStaticFiles_NonEmpty(t *testing.T) {
	for _, name := range tmpl.StaticFileList() {
		content, err := tmpl.GetStaticFile(name)
		if err != nil {
			t.Errorf("GetStaticFile(%s) failed: %v", name, err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("static file %s is empty", name)
		}
	}
}

func TestTemplateFiles_NonEmpty(t *testing.T) {
	data := testData()
	for _, name := range tmpl.TemplateFileList() {
		content, err := tmpl.RenderTemplate(name, data)
		if err != nil {
			t.Errorf("RenderTemplate(%s) failed: %v", name, err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("rendered template %s is empty", name)
		}
	}
}

func TestRenderCaddyfile(t *testing.T) {
	data := testData()
	content, err := tmpl.RenderTemplate("templates/Caddyfile.tmpl", data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "opencode.example.com") {
		t.Error("Caddyfile should contain domain")
	}
	if !strings.Contains(s, "reverse_proxy authelia:9091") {
		t.Error("Caddyfile should contain authelia reverse proxy")
	}
	if !strings.Contains(s, "reverse_proxy opencode:4096") {
		t.Error("Caddyfile should contain opencode reverse proxy")
	}
	if !strings.Contains(s, "forward_auth") {
		t.Error("Caddyfile should contain forward_auth")
	}
}

func TestRenderEnv(t *testing.T) {
	data := testData()
	content, err := tmpl.RenderTemplate("templates/env.tmpl", data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "OPENAI_API_KEY=sk-test-key") {
		t.Error("env should contain OpenAI API key")
	}
	if !strings.Contains(s, "OPENAI_BASE_URL=https://api.openai.com/v1") {
		t.Error("env should contain OpenAI base URL")
	}
	if !strings.Contains(s, "ANTHROPIC_API_KEY=sk-ant-test") {
		t.Error("env should contain Anthropic API key")
	}
}

func TestRenderEnv_OptionalFieldsEmpty(t *testing.T) {
	data := &tmpl.TemplateData{
		OpenAIAPIKey: "sk-test-key",
	}
	content, err := tmpl.RenderTemplate("templates/env.tmpl", data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "OPENAI_API_KEY=sk-test-key") {
		t.Error("env should contain OpenAI API key")
	}
	if strings.Contains(s, "OPENAI_BASE_URL=") {
		t.Error("env should not contain empty OPENAI_BASE_URL")
	}
	if strings.Contains(s, "ANTHROPIC_API_KEY=") {
		t.Error("env should not contain empty ANTHROPIC_API_KEY")
	}
}

func TestRenderAutheliaConfig(t *testing.T) {
	data := testData()
	content, err := tmpl.RenderTemplate("templates/authelia/configuration.yml.tmpl", data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	s := string(content)
	checks := []struct {
		substr string
		desc   string
	}{
		{"secret: 'test-session-secret'", "session secret"},
		{"encryption_key: 'test-storage-key'", "storage encryption key"},
		{"domain: opencode.example.com", "domain in session cookies"},
		{"authelia_url: https://auth.opencode.example.com", "authelia URL"},
		{"policy: two_factor", "two_factor policy"},
		{"domain: auth.opencode.example.com", "auth subdomain"},
		{"display_name: CloudCode", "webauthn display name"},
	}

	for _, c := range checks {
		if !strings.Contains(s, c.substr) {
			t.Errorf("authelia config should contain %s (%q)", c.desc, c.substr)
		}
	}
}

func TestRenderAutheliaUsersDB(t *testing.T) {
	data := testData()
	content, err := tmpl.RenderTemplate("templates/authelia/users_database.yml.tmpl", data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "admin:") {
		t.Error("users_database should contain username")
	}
	if !strings.Contains(s, `password: "$argon2id$v=19$m=65536,t=1,p=8$salt$hash"`) {
		t.Error("users_database should contain hashed password")
	}
	if !strings.Contains(s, `email: "admin@example.com"`) {
		t.Error("users_database should contain email")
	}
}

func TestRenderDockerCompose(t *testing.T) {
	data := testData()
	content, err := tmpl.RenderTemplate("templates/docker-compose.yml.tmpl", data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "caddy:") {
		t.Error("docker-compose should contain caddy service")
	}
	if !strings.Contains(s, "authelia:") {
		t.Error("docker-compose should contain authelia service")
	}
	if !strings.Contains(s, "opencode:") {
		t.Error("docker-compose should contain opencode service")
	}
	if !strings.Contains(s, "ghcr.io/hwuu/cloudcode-opencode:"+data.Version) {
		t.Error("docker-compose should contain versioned image tag")
	}
}

func TestRenderAll(t *testing.T) {
	data := testData()
	files, err := tmpl.RenderAll(data)
	if err != nil {
		t.Fatalf("RenderAll failed: %v", err)
	}

	expectedPaths := []string{
		"~/cloudcode/docker-compose.yml",
		"~/cloudcode/Caddyfile",
		"~/cloudcode/.env",
		"~/cloudcode/authelia/configuration.yml",
		"~/cloudcode/authelia/users_database.yml",
	}

	if len(files) != len(expectedPaths) {
		t.Errorf("expected %d files, got %d", len(expectedPaths), len(files))
	}

	for _, path := range expectedPaths {
		content, ok := files[path]
		if !ok {
			t.Errorf("missing file: %s", path)
			continue
		}
		if len(content) == 0 {
			t.Errorf("file %s is empty", path)
		}
	}
}

func TestRenderTemplate_NotFound(t *testing.T) {
	data := testData()
	_, err := tmpl.RenderTemplate("templates/nonexistent.tmpl", data)
	if err == nil {
		t.Error("expected error for nonexistent template")
	}
}

func TestGetStaticFile_NotFound(t *testing.T) {
	_, err := tmpl.GetStaticFile("templates/nonexistent.yml")
	if err == nil {
		t.Error("expected error for nonexistent static file")
	}
}
