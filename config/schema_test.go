package config

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Test default values
	if config.Port != "80" {
		t.Errorf("Expected default port to be '80', got '%s'", config.Port)
	}
	if config.Protocol != "tcp" {
		t.Errorf("Expected default protocol to be 'tcp', got '%s'", config.Protocol)
	}
	if config.Timeout != "5s" {
		t.Errorf("Expected default timeout to be '5s', got '%s'", config.Timeout)
	}
	if config.MetricsPort != 9090 {
		t.Errorf("Expected default metrics port to be 9090, got %d", config.MetricsPort)
	}
	if config.Workers != 5 {
		t.Errorf("Expected default workers to be 5, got %d", config.Workers)
	}
	if config.WarningThreshold != "500ms" {
		t.Errorf("Expected default warning threshold to be '500ms', got '%s'", config.WarningThreshold)
	}
	if config.CriticalThreshold != "1s" {
		t.Errorf("Expected default critical threshold to be '1s', got '%s'", config.CriticalThreshold)
	}
	if config.RetryCount != 3 {
		t.Errorf("Expected default retry count to be 3, got %d", config.RetryCount)
	}
	if config.RetryDelay != "1s" {
		t.Errorf("Expected default retry delay to be '1s', got '%s'", config.RetryDelay)
	}
	if config.CircuitBreakerThreshold != 0 || config.CircuitBreakerTimeout != "" {
		t.Error("Expected no circuit breaker defaults: the keys are deprecated")
	}
	if config.Groups == nil {
		t.Error("Expected default groups to be initialized")
	}
}

func TestConfigMerge(t *testing.T) {
	base := DefaultConfig()
	override := &Config{
		Port:     "443",
		Protocol: "udp",
		Timeout:  "10s",
		URLs:     []string{"example.com", "test.com"},
		Metrics:  true,
		Workers:  10,
	}

	base.Merge(override)

	// Test that override values take precedence
	if base.Port != "443" {
		t.Errorf("Expected port to be '443' after merge, got '%s'", base.Port)
	}
	if base.Protocol != "udp" {
		t.Errorf("Expected protocol to be 'udp' after merge, got '%s'", base.Protocol)
	}
	if base.Timeout != "10s" {
		t.Errorf("Expected timeout to be '10s' after merge, got '%s'", base.Timeout)
	}
	if len(base.URLs) != 2 {
		t.Errorf("Expected 2 URLs after merge, got %d", len(base.URLs))
	}
	if !base.Metrics {
		t.Error("Expected metrics to be true after merge")
	}
	if base.Workers != 10 {
		t.Errorf("Expected workers to be 10 after merge, got %d", base.Workers)
	}

	// Test that non-override values remain unchanged
	if base.MetricsPort != 9090 {
		t.Errorf("Expected metrics port to remain 9090, got %d", base.MetricsPort)
	}
}

func TestConfigMergeWithNil(t *testing.T) {
	base := DefaultConfig()
	originalPort := base.Port

	// Merge with nil should not change anything
	base.Merge(nil)

	if base.Port != originalPort {
		t.Errorf("Expected port to remain unchanged after nil merge, got '%s'", base.Port)
	}
}

func TestDeprecatedKeys(t *testing.T) {
	c := DefaultConfig()
	if keys := c.DeprecatedKeys(); len(keys) != 0 {
		t.Errorf("defaults: DeprecatedKeys() = %v, want none", keys)
	}
	c.CircuitBreakerThreshold = 3
	c.Groups = map[string]GroupConfig{"web": {URLs: []string{"a"}, RetryCount: 2}, "db": {URLs: []string{"b"}}}
	keys := c.DeprecatedKeys()
	if len(keys) != 2 || keys[0] != "circuit_breaker_threshold" || keys[1] != "groups.web overrides" {
		t.Errorf("DeprecatedKeys() = %v, want circuit_breaker_threshold and groups.web overrides", keys)
	}
}
