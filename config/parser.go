package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads configuration from a file with automatic format detection
func LoadConfig(filePath string) (*Config, error) {
	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file %s: %w", filePath, err)
	}

	// Auto-detect format based on file extension
	format := detectFormat(filePath, data)

	// Keys the file leaves out keep their defaults.
	config := *DefaultConfig()

	switch format {
	case "yaml":
		err = yaml.Unmarshal(data, &config)
		if err != nil {
			return nil, fmt.Errorf("failed to parse YAML configuration file %s: %w", filePath, err)
		}
	case "json":
		err = json.Unmarshal(data, &config)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JSON configuration file %s: %w", filePath, err)
		}
	default:
		return nil, fmt.Errorf("unsupported configuration file format for %s", filePath)
	}

	// Validate the loaded configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("configuration validation failed for %s: %w", filePath, err)
	}

	return &config, nil
}

// SaveConfig saves configuration to a file in the specified format
func SaveConfig(config *Config, filePath string) error {
	var data []byte
	var err error

	// Determine format based on file extension
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".yaml", ".yml":
		data, err = yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML configuration: %w", err)
		}
	case ".json":
		data, err = json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON configuration: %w", err)
		}
	default:
		return fmt.Errorf("unsupported file extension: %s (use .yaml, .yml, or .json)", ext)
	}

	// Write the file
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write configuration file %s: %w", filePath, err)
	}

	return nil
}

// detectFormat determines the format of the configuration file
func detectFormat(filePath string, data []byte) string {
	// First, try to detect by file extension
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	}

	// If extension is ambiguous or missing, try to detect by content
	content := strings.TrimSpace(string(data))

	// Check if it starts with { or [ (JSON)
	if strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") {
		return "json"
	}

	// Check if it contains YAML indicators
	if strings.Contains(content, ":") && !strings.Contains(content, "{") && !strings.Contains(content, "}") {
		return "yaml"
	}

	// Default to YAML for ambiguous cases
	return "yaml"
}

// validateConfig checks the values a typo would break.
func validateConfig(config *Config) error {
	for key, v := range map[string]string{
		"timeout":            config.Timeout,
		"warning_threshold":  config.WarningThreshold,
		"critical_threshold": config.CriticalThreshold,
		"retry_delay":        config.RetryDelay,
		"check_interval":     config.CheckInterval,
		"ping_interval":      config.PingInterval,
	} {
		if v == "" {
			continue
		}
		if d, err := time.ParseDuration(v); err != nil || d < 0 {
			return fmt.Errorf("invalid %s %q (use a duration like 500ms, 5s or 1m)", key, v)
		}
	}
	if config.MetricsPort < 1 || config.MetricsPort > 65535 {
		return fmt.Errorf("invalid metrics_port %d (must be between 1 and 65535)", config.MetricsPort)
	}
	if config.Workers < 1 || config.Workers > 100 {
		return fmt.Errorf("invalid workers %d (must be between 1 and 100)", config.Workers)
	}
	if config.RetryCount < 0 || config.RetryCount > 10 {
		return fmt.Errorf("invalid retry_count %d (must be between 0 and 10)", config.RetryCount)
	}
	switch config.LogLevel {
	case "", "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log_level %q (want debug, info, warn or error)", config.LogLevel)
	}
	switch config.LogFormat {
	case "", "text", "json":
	default:
		return fmt.Errorf("invalid log_format %q (want text or json)", config.LogFormat)
	}
	return nil
}
