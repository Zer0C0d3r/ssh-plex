// Package template provides command templating capabilities for ssh-plex.
package template

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"ssh-plex/internal/target"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// TemplateEngine provides command templating functionality
type TemplateEngine struct {
	templates map[string]*template.Template
}

// NewTemplateEngine creates a new template engine
func NewTemplateEngine() *TemplateEngine {
	return &TemplateEngine{
		templates: make(map[string]*template.Template),
	}
}

// RegisterTemplate registers a named template
func (te *TemplateEngine) RegisterTemplate(name, templateStr string) error {
	tmpl, err := template.New(name).Funcs(templateFuncs()).Parse(templateStr)
	if err != nil {
		return fmt.Errorf("failed to parse template '%s': %w", name, err)
	}

	te.templates[name] = tmpl
	return nil
}

// ExecuteTemplate executes a named template with target context
func (te *TemplateEngine) ExecuteTemplate(name string, target target.Target) (string, error) {
	tmpl, exists := te.templates[name]
	if !exists {
		return "", fmt.Errorf("template '%s' not found", name)
	}

	context := createTemplateContext(target)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, context); err != nil {
		return "", fmt.Errorf("failed to execute template '%s': %w", name, err)
	}

	return buf.String(), nil
}

// ExecuteInlineTemplate executes an inline template string
func (te *TemplateEngine) ExecuteInlineTemplate(templateStr string, target target.Target) (string, error) {
	tmpl, err := template.New("inline").Funcs(templateFuncs()).Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse inline template: %w", err)
	}

	context := createTemplateContext(target)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, context); err != nil {
		return "", fmt.Errorf("failed to execute inline template: %w", err)
	}

	return buf.String(), nil
}

// TemplateContext provides data available in templates
type TemplateContext struct {
	Host       string            `json:"host"`
	User       string            `json:"user"`
	Port       int               `json:"port"`
	Tags       []string          `json:"tags"`
	Properties map[string]string `json:"properties"`
}

// createTemplateContext creates a template context from a target
func createTemplateContext(target target.Target) TemplateContext {
	return TemplateContext{
		Host:       target.Host,
		User:       target.User,
		Port:       target.Port,
		Tags:       target.Tags,
		Properties: target.Properties,
	}
}

// templateFuncs returns custom template functions
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		// String functions
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"title":     cases.Title(language.English).String,
		"trim":      strings.TrimSpace,
		"replace":   strings.ReplaceAll,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,

		// Tag functions
		"hasTag": func(tags []string, tag string) bool {
			for _, t := range tags {
				if strings.EqualFold(t, tag) {
					return true
				}
			}
			return false
		},

		"hasAnyTag": func(tags []string, checkTags ...string) bool {
			tagMap := make(map[string]bool)
			for _, t := range tags {
				tagMap[strings.ToLower(t)] = true
			}
			for _, check := range checkTags {
				if tagMap[strings.ToLower(check)] {
					return true
				}
			}
			return false
		},

		"hasAllTags": func(tags []string, checkTags ...string) bool {
			tagMap := make(map[string]bool)
			for _, t := range tags {
				tagMap[strings.ToLower(t)] = true
			}
			for _, check := range checkTags {
				if !tagMap[strings.ToLower(check)] {
					return false
				}
			}
			return true
		},

		// Property functions
		"prop": func(props map[string]string, key string) string {
			return props[key]
		},

		"propDefault": func(props map[string]string, key, defaultValue string) string {
			if value, exists := props[key]; exists {
				return value
			}
			return defaultValue
		},

		// Conditional functions
		"if": func(condition bool, trueValue, falseValue string) string {
			if condition {
				return trueValue
			}
			return falseValue
		},

		// Host functions
		"hostShort": func(host string) string {
			if idx := strings.Index(host, "."); idx != -1 {
				return host[:idx]
			}
			return host
		},

		"hostDomain": func(host string) string {
			if idx := strings.Index(host, "."); idx != -1 {
				return host[idx+1:]
			}
			return ""
		},
	}
}

// PredefinedTemplates contains commonly used templates
var PredefinedTemplates = map[string]string{
	"system-info": `
echo "=== System Information for {{.Host}} ==="
echo "Hostname: $(hostname)"
echo "OS: $(uname -s)"
echo "Kernel: $(uname -r)"
echo "Architecture: $(uname -m)"
echo "Uptime: $(uptime)"
echo "Load: $(cat /proc/loadavg)"
echo "Memory: $(free -h | grep Mem)"
echo "Disk: $(df -h / | tail -1)"
`,

	"docker-status": `
{{if hasTag .Tags "docker"}}
echo "=== Docker Status on {{.Host}} ==="
docker version --format "Docker: {{.Server.Version}}"
echo "Running containers: $(docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}')"
echo "Images: $(docker images --format 'table {{.Repository}}\t{{.Tag}}\t{{.Size}}')"
{{else}}
echo "Docker not configured on {{.Host}}"
{{end}}
`,

	"service-check": `
{{$service := prop .Properties "service"}}
{{if $service}}
echo "=== Service Status: {{$service}} on {{.Host}} ==="
systemctl is-active {{$service}} && echo "✓ Active" || echo "✗ Inactive"
systemctl is-enabled {{$service}} && echo "✓ Enabled" || echo "✗ Disabled"
{{else}}
echo "No service specified for {{.Host}}"
{{end}}
`,

	"log-tail": `
{{$logfile := propDefault .Properties "logfile" "/var/log/syslog"}}
{{$lines := propDefault .Properties "lines" "50"}}
echo "=== Last {{$lines}} lines from {{$logfile}} on {{.Host}} ==="
tail -n {{$lines}} {{$logfile}}
`,

	"deployment-check": `
{{if hasTag .Tags "web"}}
echo "=== Web Server Status on {{.Host}} ==="
curl -s -o /dev/null -w "HTTP Status: %{http_code}, Response Time: %{time_total}s\n" http://localhost/health || echo "Health check failed"
{{end}}
{{if hasTag .Tags "database"}}
echo "=== Database Status on {{.Host}} ==="
{{if hasTag .Tags "mysql"}}
mysqladmin ping && echo "✓ MySQL is running" || echo "✗ MySQL is down"
{{else if hasTag .Tags "postgres"}}
pg_isready && echo "✓ PostgreSQL is running" || echo "✗ PostgreSQL is down"
{{end}}
{{end}}
`,
}

// LoadPredefinedTemplates loads all predefined templates into the engine
func (te *TemplateEngine) LoadPredefinedTemplates() error {
	for name, templateStr := range PredefinedTemplates {
		if err := te.RegisterTemplate(name, templateStr); err != nil {
			return fmt.Errorf("failed to load predefined template '%s': %w", name, err)
		}
	}
	return nil
}

// IsTemplate checks if a command string contains template syntax
func IsTemplate(command string) bool {
	return strings.Contains(command, "{{") && strings.Contains(command, "}}")
}

// ValidateTemplate validates a template string without executing it
func ValidateTemplate(templateStr string) error {
	_, err := template.New("validation").Funcs(templateFuncs()).Parse(templateStr)
	return err
}
