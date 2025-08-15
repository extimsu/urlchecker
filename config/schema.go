package config

// Config represents the complete configuration schema for the urlchecker application
type Config struct {
	// Basic URL configuration
	URLs []string `yaml:"urls,omitempty" json:"urls,omitempty"` // List of URLs to check
	File string   `yaml:"file,omitempty" json:"file,omitempty"` // Path to file containing URLs

	// Connection settings
	Port     string `yaml:"port,omitempty" json:"port,omitempty"`         // Port to check (default: "80")
	Protocol string `yaml:"protocol,omitempty" json:"protocol,omitempty"` // Protocol to use (default: "tcp")
	Timeout  string `yaml:"timeout,omitempty" json:"timeout,omitempty"`   // Connection timeout (default: "5s")

	// Output configuration
	JSONOutput bool `yaml:"json_output,omitempty" json:"json_output,omitempty"` // Enable JSON output format

	// Metrics and monitoring
	Metrics     bool `yaml:"metrics,omitempty" json:"metrics,omitempty"`           // Enable Prometheus metrics server
	MetricsPort int  `yaml:"metrics_port,omitempty" json:"metrics_port,omitempty"` // Port for metrics server (default: 9090)

	// Exporter mode configuration
	Exporter      bool   `yaml:"exporter,omitempty" json:"exporter,omitempty"`             // Enable exporter mode
	CheckInterval string `yaml:"check_interval,omitempty" json:"check_interval,omitempty"` // Interval between checks (default: "30s")
	Workers       int    `yaml:"workers,omitempty" json:"workers,omitempty"`               // Number of worker goroutines (default: 5)

	// Group configuration
	GroupName string                 `yaml:"group_name,omitempty" json:"group_name,omitempty"` // Default group name for URLs
	Groups    map[string]GroupConfig `yaml:"groups,omitempty" json:"groups,omitempty"`         // Per-group configurations

	// Response time thresholds
	WarningThreshold  string `yaml:"warning_threshold,omitempty" json:"warning_threshold,omitempty"`   // Warning threshold (default: "500ms")
	CriticalThreshold string `yaml:"critical_threshold,omitempty" json:"critical_threshold,omitempty"` // Critical threshold (default: "1s")

	// Retry configuration
	RetryCount int    `yaml:"retry_count,omitempty" json:"retry_count,omitempty"` // Number of retry attempts (default: 3)
	RetryDelay string `yaml:"retry_delay,omitempty" json:"retry_delay,omitempty"` // Initial delay between retries (default: "1s")

	// Circuit breaker configuration
	CircuitBreakerThreshold int    `yaml:"circuit_breaker_threshold,omitempty" json:"circuit_breaker_threshold,omitempty"` // Failure threshold (default: 5)
	CircuitBreakerTimeout   string `yaml:"circuit_breaker_timeout,omitempty" json:"circuit_breaker_timeout,omitempty"`     // Timeout before recovery (default: "60s")
}

// GroupConfig represents configuration for a specific group
type GroupConfig struct {
	URLs []string `yaml:"urls,omitempty" json:"urls,omitempty"` // URLs in this group

	// Per-group overrides
	WarningThreshold        string `yaml:"warning_threshold,omitempty" json:"warning_threshold,omitempty"`                 // Group-specific warning threshold
	CriticalThreshold       string `yaml:"critical_threshold,omitempty" json:"critical_threshold,omitempty"`               // Group-specific critical threshold
	RetryCount              int    `yaml:"retry_count,omitempty" json:"retry_count,omitempty"`                             // Group-specific retry count
	RetryDelay              string `yaml:"retry_delay,omitempty" json:"retry_delay,omitempty"`                             // Group-specific retry delay
	CircuitBreakerThreshold int    `yaml:"circuit_breaker_threshold,omitempty" json:"circuit_breaker_threshold,omitempty"` // Group-specific circuit breaker threshold
	CircuitBreakerTimeout   string `yaml:"circuit_breaker_timeout,omitempty" json:"circuit_breaker_timeout,omitempty"`     // Group-specific circuit breaker timeout
}

// DefaultConfig returns a configuration with sensible defaults matching CLI behavior
func DefaultConfig() *Config {
	return &Config{
		Port:                    "80",
		Protocol:                "tcp",
		Timeout:                 "5s",
		JSONOutput:              false,
		Metrics:                 false,
		MetricsPort:             9090,
		Exporter:                false,
		CheckInterval:           "30s",
		Workers:                 5,
		WarningThreshold:        "500ms",
		CriticalThreshold:       "1s",
		RetryCount:              3,
		RetryDelay:              "1s",
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   "60s",
		Groups:                  make(map[string]GroupConfig),
	}
}

// Merge merges another configuration into this one, with the other config taking precedence
func (c *Config) Merge(override *Config) {
	if override == nil {
		return
	}

	// Merge basic fields (override takes precedence)
	if len(override.URLs) > 0 {
		c.URLs = override.URLs
	}
	if override.File != "" {
		c.File = override.File
	}
	if override.Port != "" {
		c.Port = override.Port
	}
	if override.Protocol != "" {
		c.Protocol = override.Protocol
	}
	if override.Timeout != "" {
		c.Timeout = override.Timeout
	}

	// Merge output configuration
	if override.JSONOutput {
		c.JSONOutput = override.JSONOutput
	}

	// Merge metrics configuration
	if override.Metrics {
		c.Metrics = override.Metrics
	}
	if override.MetricsPort != 0 {
		c.MetricsPort = override.MetricsPort
	}

	// Merge exporter configuration
	if override.Exporter {
		c.Exporter = override.Exporter
	}
	if override.CheckInterval != "" {
		c.CheckInterval = override.CheckInterval
	}
	if override.Workers != 0 {
		c.Workers = override.Workers
	}

	// Merge group configuration
	if override.GroupName != "" {
		c.GroupName = override.GroupName
	}
	if len(override.Groups) > 0 {
		if c.Groups == nil {
			c.Groups = make(map[string]GroupConfig)
		}
		for name, group := range override.Groups {
			c.Groups[name] = group
		}
	}

	// Merge threshold configuration
	if override.WarningThreshold != "" {
		c.WarningThreshold = override.WarningThreshold
	}
	if override.CriticalThreshold != "" {
		c.CriticalThreshold = override.CriticalThreshold
	}

	// Merge retry configuration
	if override.RetryCount != 0 {
		c.RetryCount = override.RetryCount
	}
	if override.RetryDelay != "" {
		c.RetryDelay = override.RetryDelay
	}

	// Merge circuit breaker configuration
	if override.CircuitBreakerThreshold != 0 {
		c.CircuitBreakerThreshold = override.CircuitBreakerThreshold
	}
	if override.CircuitBreakerTimeout != "" {
		c.CircuitBreakerTimeout = override.CircuitBreakerTimeout
	}
}

// GetGroupConfig returns the configuration for a specific group, with defaults from the main config
func (c *Config) GetGroupConfig(groupName string) *GroupConfig {
	group, exists := c.Groups[groupName]
	if !exists {
		// Return a default group config with main config values
		return &GroupConfig{
			WarningThreshold:        c.WarningThreshold,
			CriticalThreshold:       c.CriticalThreshold,
			RetryCount:              c.RetryCount,
			RetryDelay:              c.RetryDelay,
			CircuitBreakerThreshold: c.CircuitBreakerThreshold,
			CircuitBreakerTimeout:   c.CircuitBreakerTimeout,
		}
	}

	// Merge with main config defaults for any unset values
	if group.WarningThreshold == "" {
		group.WarningThreshold = c.WarningThreshold
	}
	if group.CriticalThreshold == "" {
		group.CriticalThreshold = c.CriticalThreshold
	}
	if group.RetryCount == 0 {
		group.RetryCount = c.RetryCount
	}
	if group.RetryDelay == "" {
		group.RetryDelay = c.RetryDelay
	}
	if group.CircuitBreakerThreshold == 0 {
		group.CircuitBreakerThreshold = c.CircuitBreakerThreshold
	}
	if group.CircuitBreakerTimeout == "" {
		group.CircuitBreakerTimeout = c.CircuitBreakerTimeout
	}

	return &group
}
