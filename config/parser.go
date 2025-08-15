package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	
	var config Config
	
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

// validateConfig performs basic validation on the loaded configuration
func validateConfig(config *Config) error {
	// Check for required fields or logical constraints
	if config.Timeout != "" {
		// Validate timeout format (basic check)
		if !strings.Contains(config.Timeout, "s") && !strings.Contains(config.Timeout, "ms") {
			return fmt.Errorf("invalid timeout format: %s (use format like '5s' or '500ms')", config.Timeout)
		}
	}

	if config.WarningThreshold != "" {
		if !strings.Contains(config.WarningThreshold, "s") && !strings.Contains(config.WarningThreshold, "ms") {
			return fmt.Errorf("invalid warning threshold format: %s (use format like '500ms' or '1s')", config.WarningThreshold)
		}
	}

	if config.CriticalThreshold != "" {
		if !strings.Contains(config.CriticalThreshold, "s") && !strings.Contains(config.CriticalThreshold, "ms") {
			return fmt.Errorf("invalid critical threshold format: %s (use format like '1s' or '2s')", config.CriticalThreshold)
		}
	}

	if config.RetryDelay != "" {
		if !strings.Contains(config.RetryDelay, "s") && !strings.Contains(config.RetryDelay, "ms") {
			return fmt.Errorf("invalid retry delay format: %s (use format like '1s' or '500ms')", config.RetryDelay)
		}
	}

	if config.CircuitBreakerTimeout != "" {
		if !strings.Contains(config.CircuitBreakerTimeout, "s") && !strings.Contains(config.CircuitBreakerTimeout, "ms") {
			return fmt.Errorf("invalid circuit breaker timeout format: %s (use format like '60s' or '1m')", config.CircuitBreakerTimeout)
		}
	}

	if config.CheckInterval != "" {
		if !strings.Contains(config.CheckInterval, "s") && !strings.Contains(config.CheckInterval, "ms") {
			return fmt.Errorf("invalid check interval format: %s (use format like '30s' or '1m')", config.CheckInterval)
		}
	}

	// Validate numeric ranges
	if config.MetricsPort < 1 || config.MetricsPort > 65535 {
		return fmt.Errorf("invalid metrics port: %d (must be between 1 and 65535)", config.MetricsPort)
	}

	if config.Workers < 1 || config.Workers > 100 {
		return fmt.Errorf("invalid worker count: %d (must be between 1 and 100)", config.Workers)
	}

	if config.RetryCount < 0 || config.RetryCount > 10 {
		return fmt.Errorf("invalid retry count: %d (must be between 0 and 10)", config.RetryCount)
	}

	if config.CircuitBreakerThreshold < 1 || config.CircuitBreakerThreshold > 100 {
		return fmt.Errorf("invalid circuit breaker threshold: %d (must be between 1 and 100)", config.CircuitBreakerThreshold)
	}

	return nil
}
