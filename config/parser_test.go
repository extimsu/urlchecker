package config

import (
	"os"
	"testing"
)

func TestLoadConfigYAML(t *testing.T) {
	// Create a temporary YAML file
	yamlContent := `
urls:
  - "example.com"
  - "test.com"
port: "443"
protocol: "tcp"
timeout: "10s"
metrics: true
metrics_port: 9091
exporter: true
workers: 10
warning_threshold: "200ms"
critical_threshold: "500ms"
retry_count: 5
retry_delay: "2s"
circuit_breaker_threshold: 3
circuit_breaker_timeout: "30s"
groups:
  web:
    urls:
      - "web1.com"
      - "web2.com"
    warning_threshold: "100ms"
    retry_count: 3
  api:
    urls:
      - "api1.com"
    critical_threshold: "1s"
`

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(yamlContent)
	if err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Load the configuration
	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load YAML config: %v", err)
	}

	// Test basic fields
	if len(config.URLs) != 2 {
		t.Errorf("Expected 2 URLs, got %d", len(config.URLs))
	}
	if config.URLs[0] != "example.com" {
		t.Errorf("Expected first URL to be 'example.com', got '%s'", config.URLs[0])
	}
	if config.Port != "443" {
		t.Errorf("Expected port to be '443', got '%s'", config.Port)
	}
	if config.Protocol != "tcp" {
		t.Errorf("Expected protocol to be 'tcp', got '%s'", config.Protocol)
	}
	if config.Timeout != "10s" {
		t.Errorf("Expected timeout to be '10s', got '%s'", config.Timeout)
	}
	if !config.Metrics {
		t.Error("Expected metrics to be true")
	}
	if config.MetricsPort != 9091 {
		t.Errorf("Expected metrics port to be 9091, got %d", config.MetricsPort)
	}
	if !config.Exporter {
		t.Error("Expected exporter to be true")
	}
	if config.Workers != 10 {
		t.Errorf("Expected workers to be 10, got %d", config.Workers)
	}
	if config.WarningThreshold != "200ms" {
		t.Errorf("Expected warning threshold to be '200ms', got '%s'", config.WarningThreshold)
	}
	if config.CriticalThreshold != "500ms" {
		t.Errorf("Expected critical threshold to be '500ms', got '%s'", config.CriticalThreshold)
	}
	if config.RetryCount != 5 {
		t.Errorf("Expected retry count to be 5, got %d", config.RetryCount)
	}
	if config.RetryDelay != "2s" {
		t.Errorf("Expected retry delay to be '2s', got '%s'", config.RetryDelay)
	}
	if config.CircuitBreakerThreshold != 3 {
		t.Errorf("Expected circuit breaker threshold to be 3, got %d", config.CircuitBreakerThreshold)
	}
	if config.CircuitBreakerTimeout != "30s" {
		t.Errorf("Expected circuit breaker timeout to be '30s', got '%s'", config.CircuitBreakerTimeout)
	}

	// Test groups
	if len(config.Groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(config.Groups))
	}

	webGroup, exists := config.Groups["web"]
	if !exists {
		t.Error("Expected 'web' group to exist")
	} else {
		if len(webGroup.URLs) != 2 {
			t.Errorf("Expected 2 URLs in web group, got %d", len(webGroup.URLs))
		}
		if webGroup.WarningThreshold != "100ms" {
			t.Errorf("Expected web group warning threshold to be '100ms', got '%s'", webGroup.WarningThreshold)
		}
		if webGroup.RetryCount != 3 {
			t.Errorf("Expected web group retry count to be 3, got %d", webGroup.RetryCount)
		}
	}

	apiGroup, exists := config.Groups["api"]
	if !exists {
		t.Error("Expected 'api' group to exist")
	} else {
		if len(apiGroup.URLs) != 1 {
			t.Errorf("Expected 1 URL in api group, got %d", len(apiGroup.URLs))
		}
		if apiGroup.CriticalThreshold != "1s" {
			t.Errorf("Expected api group critical threshold to be '1s', got '%s'", apiGroup.CriticalThreshold)
		}
	}
}

func TestLoadConfigJSON(t *testing.T) {
	// Create a temporary JSON file
	jsonContent := `{
  "urls": ["example.com", "test.com"],
  "port": "443",
  "protocol": "tcp",
  "timeout": "10s",
  "metrics": true,
  "metrics_port": 9091,
  "exporter": true,
  "workers": 10,
  "warning_threshold": "200ms",
  "critical_threshold": "500ms",
  "retry_count": 5,
  "retry_delay": "2s",
  "circuit_breaker_threshold": 3,
  "circuit_breaker_timeout": "30s",
  "groups": {
    "web": {
      "urls": ["web1.com", "web2.com"],
      "warning_threshold": "100ms",
      "retry_count": 3
    },
    "api": {
      "urls": ["api1.com"],
      "critical_threshold": "1s"
    }
  }
}`

	tmpFile, err := os.CreateTemp("", "test-config-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(jsonContent)
	if err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Load the configuration
	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load JSON config: %v", err)
	}

	// Test basic fields (same as YAML test)
	if len(config.URLs) != 2 {
		t.Errorf("Expected 2 URLs, got %d", len(config.URLs))
	}
	if config.Port != "443" {
		t.Errorf("Expected port to be '443', got '%s'", config.Port)
	}
	if config.Protocol != "tcp" {
		t.Errorf("Expected protocol to be 'tcp', got '%s'", config.Protocol)
	}
	if config.Timeout != "10s" {
		t.Errorf("Expected timeout to be '10s', got '%s'", config.Timeout)
	}
	if !config.Metrics {
		t.Error("Expected metrics to be true")
	}
	if config.MetricsPort != 9091 {
		t.Errorf("Expected metrics port to be 9091, got %d", config.MetricsPort)
	}
	if !config.Exporter {
		t.Error("Expected exporter to be true")
	}
	if config.Workers != 10 {
		t.Errorf("Expected workers to be 10, got %d", config.Workers)
	}
	if config.WarningThreshold != "200ms" {
		t.Errorf("Expected warning threshold to be '200ms', got '%s'", config.WarningThreshold)
	}
	if config.CriticalThreshold != "500ms" {
		t.Errorf("Expected critical threshold to be '500ms', got '%s'", config.CriticalThreshold)
	}
	if config.RetryCount != 5 {
		t.Errorf("Expected retry count to be 5, got %d", config.RetryCount)
	}
	if config.RetryDelay != "2s" {
		t.Errorf("Expected retry delay to be '2s', got '%s'", config.RetryDelay)
	}
	if config.CircuitBreakerThreshold != 3 {
		t.Errorf("Expected circuit breaker threshold to be 3, got %d", config.CircuitBreakerThreshold)
	}
	if config.CircuitBreakerTimeout != "30s" {
		t.Errorf("Expected circuit breaker timeout to be '30s', got '%s'", config.CircuitBreakerTimeout)
	}

	// Test groups
	if len(config.Groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(config.Groups))
	}

	webGroup, exists := config.Groups["web"]
	if !exists {
		t.Error("Expected 'web' group to exist")
	} else {
		if len(webGroup.URLs) != 2 {
			t.Errorf("Expected 2 URLs in web group, got %d", len(webGroup.URLs))
		}
		if webGroup.WarningThreshold != "100ms" {
			t.Errorf("Expected web group warning threshold to be '100ms', got '%s'", webGroup.WarningThreshold)
		}
		if webGroup.RetryCount != 3 {
			t.Errorf("Expected web group retry count to be 3, got %d", webGroup.RetryCount)
		}
	}

	apiGroup, exists := config.Groups["api"]
	if !exists {
		t.Error("Expected 'api' group to exist")
	} else {
		if len(apiGroup.URLs) != 1 {
			t.Errorf("Expected 1 URL in api group, got %d", len(apiGroup.URLs))
		}
		if apiGroup.CriticalThreshold != "1s" {
			t.Errorf("Expected api group critical threshold to be '1s', got '%s'", apiGroup.CriticalThreshold)
		}
	}
}

func TestSaveConfig(t *testing.T) {
	// Create a test configuration
	config := &Config{
		URLs:                  []string{"example.com", "test.com"},
		Port:                  "443",
		Protocol:              "tcp",
		Timeout:               "10s",
		Metrics:               true,
		MetricsPort:           9091,
		Exporter:              true,
		Workers:               10,
		WarningThreshold:      "200ms",
		CriticalThreshold:     "500ms",
		RetryCount:            5,
		RetryDelay:            "2s",
		CircuitBreakerThreshold: 3,
		CircuitBreakerTimeout: "30s",
		Groups: map[string]GroupConfig{
			"web": {
				URLs:              []string{"web1.com", "web2.com"},
				WarningThreshold:  "100ms",
				RetryCount:        3,
			},
		},
	}

	// Test saving as YAML
	yamlFile := "test-config.yaml"
	defer os.Remove(yamlFile)

	err := SaveConfig(config, yamlFile)
	if err != nil {
		t.Fatalf("Failed to save YAML config: %v", err)
	}

	// Load it back and verify
	loadedConfig, err := LoadConfig(yamlFile)
	if err != nil {
		t.Fatalf("Failed to load saved YAML config: %v", err)
	}

	// Verify the loaded configuration matches the original
	if len(loadedConfig.URLs) != len(config.URLs) {
		t.Errorf("Expected %d URLs, got %d", len(config.URLs), len(loadedConfig.URLs))
	}
	if loadedConfig.Port != config.Port {
		t.Errorf("Expected port '%s', got '%s'", config.Port, loadedConfig.Port)
	}
	if loadedConfig.MetricsPort != config.MetricsPort {
		t.Errorf("Expected metrics port %d, got %d", config.MetricsPort, loadedConfig.MetricsPort)
	}

	// Test saving as JSON
	jsonFile := "test-config.json"
	defer os.Remove(jsonFile)

	err = SaveConfig(config, jsonFile)
	if err != nil {
		t.Fatalf("Failed to save JSON config: %v", err)
	}

	// Load it back and verify
	loadedConfig, err = LoadConfig(jsonFile)
	if err != nil {
		t.Fatalf("Failed to load saved JSON config: %v", err)
	}

	// Verify the loaded configuration matches the original
	if len(loadedConfig.URLs) != len(config.URLs) {
		t.Errorf("Expected %d URLs, got %d", len(config.URLs), len(loadedConfig.URLs))
	}
	if loadedConfig.Port != config.Port {
		t.Errorf("Expected port '%s', got '%s'", config.Port, loadedConfig.Port)
	}
	if loadedConfig.MetricsPort != config.MetricsPort {
		t.Errorf("Expected metrics port %d, got %d", config.MetricsPort, loadedConfig.MetricsPort)
	}
}

func TestDetectFormat(t *testing.T) {
	// Test YAML detection by extension
	yamlData := []byte("port: 80\nprotocol: tcp")
	format := detectFormat("config.yaml", yamlData)
	if format != "yaml" {
		t.Errorf("Expected YAML format for .yaml extension, got %s", format)
	}

	format = detectFormat("config.yml", yamlData)
	if format != "yaml" {
		t.Errorf("Expected YAML format for .yml extension, got %s", format)
	}

	// Test JSON detection by extension
	jsonData := []byte(`{"port": 80, "protocol": "tcp"}`)
	format = detectFormat("config.json", jsonData)
	if format != "json" {
		t.Errorf("Expected JSON format for .json extension, got %s", format)
	}

	// Test content-based detection
	format = detectFormat("config", jsonData)
	if format != "json" {
		t.Errorf("Expected JSON format for JSON content, got %s", format)
	}

	format = detectFormat("config", yamlData)
	if format != "yaml" {
		t.Errorf("Expected YAML format for YAML content, got %s", format)
	}
}

func TestValidateConfig(t *testing.T) {
	// Test valid configuration
	validConfig := &Config{
		Timeout:               "5s",
		WarningThreshold:      "500ms",
		CriticalThreshold:     "1s",
		RetryDelay:            "1s",
		CircuitBreakerTimeout: "60s",
		CheckInterval:         "30s",
		MetricsPort:           9090,
		Workers:               5,
		RetryCount:            3,
		CircuitBreakerThreshold: 5,
	}

	err := validateConfig(validConfig)
	if err != nil {
		t.Errorf("Expected valid config to pass validation, got error: %v", err)
	}

	// Test invalid timeout format
	invalidConfig := &Config{
		Timeout: "invalid",
	}

	err = validateConfig(invalidConfig)
	if err == nil {
		t.Error("Expected invalid timeout to fail validation")
	}

	// Test invalid metrics port
	invalidConfig = &Config{
		MetricsPort: 70000, // Invalid port
	}

	err = validateConfig(invalidConfig)
	if err == nil {
		t.Error("Expected invalid metrics port to fail validation")
	}

	// Test invalid worker count
	invalidConfig = &Config{
		Workers: 0, // Invalid worker count
	}

	err = validateConfig(invalidConfig)
	if err == nil {
		t.Error("Expected invalid worker count to fail validation")
	}

	// Test invalid retry count
	invalidConfig = &Config{
		RetryCount: 15, // Invalid retry count
	}

	err = validateConfig(invalidConfig)
	if err == nil {
		t.Error("Expected invalid retry count to fail validation")
	}

	// Test invalid circuit breaker threshold
	invalidConfig = &Config{
		CircuitBreakerThreshold: 0, // Invalid threshold
	}

	err = validateConfig(invalidConfig)
	if err == nil {
		t.Error("Expected invalid circuit breaker threshold to fail validation")
	}
}
