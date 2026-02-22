// Package template 管理配置文件模板的渲染和静态文件的读取。
// 使用 go:embed 嵌入 templates/ 目录下的所有文件，编译后无需外部文件依赖。
//
// 文件分两类：
//   - 模板文件（.tmpl）：使用 Go text/template 渲染，注入域名/密码/API Key 等变量
//   - 静态文件：原样输出（docker-compose.yml、Dockerfile.opencode）
//
// RenderAll 将所有文件渲染后映射到 ECS 上的目标路径，供 SFTP 上传。
package template

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed all:templates
var templateFS embed.FS

// TemplateData 包含所有模板渲染所需的字段
type TemplateData struct {
	Domain               string // 域名
	Username             string // 管理员用户名
	HashedPassword       string // Argon2id 哈希后的密码
	Email                string // 管理员邮箱
	SessionSecret        string // Authelia session 密钥
	StorageEncryptionKey string // Authelia storage 加密密钥
	OpenAIAPIKey         string // OpenAI API Key
	OpenAIBaseURL        string // OpenAI Base URL（可选）
	AnthropicAPIKey      string // Anthropic API Key（可选）
	Version              string // Docker 镜像版本号
}

// 模板文件（需要渲染）
var templateFiles = []string{
	"templates/Caddyfile.tmpl",
	"templates/env.tmpl",
	"templates/authelia/configuration.yml.tmpl",
	"templates/authelia/users_database.yml.tmpl",
	"templates/docker-compose.yml.tmpl",
}

// 静态文件（原样输出）
var staticFiles = []string{}

// RenderTemplate 渲染指定模板文件，返回渲染后的内容
func RenderTemplate(name string, data *TemplateData) ([]byte, error) {
	content, err := templateFS.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("failed to read template %s: %w", name, err)
	}

	tmpl, err := template.New(name).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to render template %s: %w", name, err)
	}

	return buf.Bytes(), nil
}

// GetStaticFile 返回静态文件的原始内容
func GetStaticFile(name string) ([]byte, error) {
	content, err := templateFS.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("failed to read static file %s: %w", name, err)
	}
	return content, nil
}

// RenderAll 渲染所有文件，返回 ECS 目标路径 → 内容 的映射
func RenderAll(data *TemplateData) (map[string][]byte, error) {
	result := make(map[string][]byte)

	// 文件映射：模板源文件 → ECS 目标路径（参考 design-oc.md 5.1.6）
	templateMapping := map[string]string{
		"templates/Caddyfile.tmpl":                       "~/cloudcode/Caddyfile",
		"templates/env.tmpl":                             "~/cloudcode/.env",
		"templates/authelia/configuration.yml.tmpl":      "~/cloudcode/authelia/configuration.yml",
		"templates/authelia/users_database.yml.tmpl":     "~/cloudcode/authelia/users_database.yml",
		"templates/docker-compose.yml.tmpl":              "~/cloudcode/docker-compose.yml",
	}

	staticMapping := map[string]string{}

	for src, dst := range templateMapping {
		content, err := RenderTemplate(src, data)
		if err != nil {
			return nil, err
		}
		result[dst] = content
	}

	for src, dst := range staticMapping {
		content, err := GetStaticFile(src)
		if err != nil {
			return nil, err
		}
		result[dst] = content
	}

	return result, nil
}

// TemplateFileList 返回所有模板文件路径
func TemplateFileList() []string {
	return templateFiles
}

// StaticFileList 返回所有静态文件路径
func StaticFileList() []string {
	return staticFiles
}
