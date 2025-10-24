// Package config provides configuration management for ssh-plex.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config represents the application configuration structure
type Config struct {
	Hosts        string        `mapstructure:"hosts"`       // Comma-separated host specifications
	HostFile     string        `mapstructure:"hostfile"`    // Path to file containing host specifications
	Concurrency  string        `mapstructure:"concurrency"` // Concurrency limit ("auto" or number)
	Retries      int           `mapstructure:"retries"`     // Maximum retry attempts per target
	Timeout      time.Duration `mapstructure:"timeout"`     // Total execution timeout
	CmdTimeout   time.Duration `mapstructure:"cmd-timeout"` // Per-command timeout
	Output       string        `mapstructure:"output"`      // Output format (streamed, buffered, json)
	Quiet        bool          `mapstructure:"quiet"`       // Suppress non-error output
	DryRun       bool          `mapstructure:"dry-run"`     // Show execution plan without connecting
	LogLevel     string        `mapstructure:"log-level"`   // Log level (info, error)
	LogFormat    string        `mapstructure:"log-format"`  // Log format (json, text)
	ShowProgress bool          `mapstructure:"progress"`    // Show progress bar
	ShowStats    bool          `mapstructure:"stats"`       // Show real-time statistics
}

// Manager defines the interface for configuration management
type Manager interface {
	// Load reads configuration from all sources (files, env vars, CLI flags)
	Load() (*Config, error)

	// SetDefaults establishes default configuration values
	SetDefaults()

	// Validate ensures configuration values are valid and consistent
	Validate(config *Config) error
}

// ViperManager implements the Manager interface using Viper
type ViperManager struct {
	v *viper.Viper
}

// NewManager creates a new configuration manager
func NewManager() Manager {
	return &ViperManager{
		v: viper.New(),
	}
}

// SetDefaults establishes default configuration values
func (m *ViperManager) SetDefaults() {
	m.v.SetDefault("concurrency", "10")
	m.v.SetDefault("retries", 0)
	m.v.SetDefault("timeout", time.Duration(0)) // No timeout by default
	m.v.SetDefault("cmd-timeout", 60*time.Second)
	m.v.SetDefault("output", "streamed")
	m.v.SetDefault("quiet", false)
	m.v.SetDefault("dry-run", false)
	m.v.SetDefault("log-level", "info")
	m.v.SetDefault("log-format", "text")
	m.v.SetDefault("progress", false)
	m.v.SetDefault("stats", false)
}

// Load reads configuration from all sources with proper precedence
func (m *ViperManager) Load() (*Config, error) {
	// Set defaults first
	m.SetDefaults()

	// Configure config file locations and formats
	m.v.SetConfigName("config")

	// Add config paths in precedence order (current dir highest, system lowest)
	m.v.AddConfigPath(".") // Current directory (highest precedence)

	// Add user config path
	if homeDir, err := os.UserHomeDir(); err == nil {
		userConfigDir := filepath.Join(homeDir, ".config", "ssh-plex")
		m.v.AddConfigPath(userConfigDir)
	}

	// Add system config path (lowest precedence)
	m.v.AddConfigPath("/etc/ssh-plex/")

	// Set up environment variable handling
	m.v.SetEnvPrefix("SSH_PLEX")
	m.v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	m.v.AutomaticEnv()

	// Try to read config file with multiple formats
	formats := []string{"yaml", "yml", "json", "toml"}

	for _, format := range formats {
		m.v.SetConfigType(format)
		if err := m.v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, fmt.Errorf("error reading %s config file: %w", format, err)
			}
		} else {
			// Config file found and loaded successfully
			break
		}
	}

	// Unmarshal into Config struct
	var config Config
	if err := m.v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Validate the configuration
	if err := m.Validate(&config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &config, nil
}

// Validate ensures configuration values are valid and consistent
func (m *ViperManager) Validate(config *Config) error {
	// Validate concurrency
	if config.Concurrency != "auto" {
		if concurrency, err := strconv.Atoi(config.Concurrency); err != nil {
			return fmt.Errorf("invalid concurrency value '%s': must be 'auto' or a positive integer", config.Concurrency)
		} else if concurrency <= 0 {
			return fmt.Errorf("concurrency must be positive, got %d", concurrency)
		}
	}

	// Validate retries
	if config.Retries < 0 {
		return fmt.Errorf("retries must be non-negative, got %d", config.Retries)
	}

	// Validate timeouts
	if config.Timeout < 0 {
		return fmt.Errorf("timeout must be non-negative, got %v", config.Timeout)
	}
	if config.CmdTimeout <= 0 {
		return fmt.Errorf("cmd-timeout must be positive, got %v", config.CmdTimeout)
	}

	// Validate output format
	validOutputs := map[string]bool{
		"streamed": true,
		"buffered": true,
		"json":     true,
	}
	if !validOutputs[config.Output] {
		return fmt.Errorf("invalid output format '%s': must be one of 'streamed', 'buffered', or 'json'", config.Output)
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"info":  true,
		"error": true,
	}
	if !validLogLevels[config.LogLevel] {
		return fmt.Errorf("invalid log level '%s': must be one of 'info' or 'error'", config.LogLevel)
	}

	// Validate log format
	validLogFormats := map[string]bool{
		"json": true,
		"text": true,
	}
	if !validLogFormats[config.LogFormat] {
		return fmt.Errorf("invalid log format '%s': must be one of 'json' or 'text'", config.LogFormat)
	}

	return nil
}

// BindFlags binds CLI flags to configuration keys
func (m *ViperManager) BindFlags(flagSet interface{}) error {
	// This method will be used by the CLI layer to bind command-line flags
	// The actual implementation will depend on the CLI framework being used
	return nil
}

// LoadFromEnv loads configuration specifically from environment variables
// This method demonstrates explicit environment variable handling
func (m *ViperManager) LoadFromEnv() (*Config, error) {
	m.SetDefaults()

	// Set up environment variable handling with SSH_PLEX_ prefix
	m.v.SetEnvPrefix("SSH_PLEX")
	m.v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	m.v.AutomaticEnv()

	// Explicitly bind environment variables for better type conversion
	envVars := map[string]string{
		"hosts":       "SSH_PLEX_HOSTS",
		"hostfile":    "SSH_PLEX_HOSTFILE",
		"concurrency": "SSH_PLEX_CONCURRENCY",
		"retries":     "SSH_PLEX_RETRIES",
		"timeout":     "SSH_PLEX_TIMEOUT",
		"cmd-timeout": "SSH_PLEX_CMD_TIMEOUT",
		"output":      "SSH_PLEX_OUTPUT",
		"quiet":       "SSH_PLEX_QUIET",
		"dry-run":     "SSH_PLEX_DRY_RUN",
		"log-level":   "SSH_PLEX_LOG_LEVEL",
		"log-format":  "SSH_PLEX_LOG_FORMAT",
		"progress":    "SSH_PLEX_PROGRESS",
		"stats":       "SSH_PLEX_STATS",
	}

	for key, envVar := range envVars {
		if value := os.Getenv(envVar); value != "" {
			if err := m.setConfigValue(key, value); err != nil {
				return nil, fmt.Errorf("error setting %s from environment: %w", envVar, err)
			}
		}
	}

	var config Config
	if err := m.v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config from environment: %w", err)
	}

	if err := m.Validate(&config); err != nil {
		return nil, fmt.Errorf("environment configuration validation failed: %w", err)
	}

	return &config, nil
}

// setConfigValue sets a configuration value with proper type conversion
func (m *ViperManager) setConfigValue(key, value string) error {
	switch key {
	case "retries":
		if intVal, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid integer value for %s: %s", key, value)
		} else {
			m.v.Set(key, intVal)
		}
	case "timeout", "cmd-timeout":
		if duration, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("invalid duration value for %s: %s", key, value)
		} else {
			m.v.Set(key, duration)
		}
	case "quiet", "dry-run", "progress", "stats":
		if boolVal, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid boolean value for %s: %s", key, value)
		} else {
			m.v.Set(key, boolVal)
		}
	default:
		// String values
		m.v.Set(key, value)
	}
	return nil
}

// GetEnvVarNames returns a list of all supported environment variable names
func GetEnvVarNames() []string {
	return []string{
		"SSH_PLEX_HOSTS",
		"SSH_PLEX_HOSTFILE",
		"SSH_PLEX_CONCURRENCY",
		"SSH_PLEX_RETRIES",
		"SSH_PLEX_TIMEOUT",
		"SSH_PLEX_CMD_TIMEOUT",
		"SSH_PLEX_OUTPUT",
		"SSH_PLEX_QUIET",
		"SSH_PLEX_DRY_RUN",
		"SSH_PLEX_LOG_LEVEL",
		"SSH_PLEX_LOG_FORMAT",
		"SSH_PLEX_PROGRESS",
		"SSH_PLEX_STATS",
	}
}
