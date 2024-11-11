package config

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Device struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type GlobalConfig struct {
	Interval int    `yaml:"interval"`
	Port     int    `yaml:"port"`
	LogLevel string `yaml:"log_level"`
	Timeout  int    `yaml:"timeout"`
	Workers  int    `yaml:"workers"`
}

type Config struct {
	Global  GlobalConfig `yaml:"global"`
	Devices []Device     `yaml:"devices"`
}

// Default values for global config
var defaultGlobal = GlobalConfig{
	Interval: 60,
	Port:     2112,
	LogLevel: "info",
	Timeout:  5,
	Workers:  5,
}

var globalConfig *Config

// LoadConfig reads and validates the config file
func LoadConfig(filename string) (*Config, error) {
	// Read file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Parse YAML with defaults
	cfg := Config{
		Global: defaultGlobal,
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}

	// Validate
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func validate(cfg *Config) error {
	// Validate global config
	if cfg.Global.Interval < 1 {
		return fmt.Errorf("global.interval must be greater than 0")
	}
	if cfg.Global.Port < 1 || cfg.Global.Port > 65535 {
		return fmt.Errorf("global.port must be between 1 and 65535")
	}
	switch cfg.Global.LogLevel {
	case "debug", "info", "warn", "error":
		// valid log levels
	default:
		return fmt.Errorf("global.log_level must be one of: debug, info, warn, error")
	}
	if cfg.Global.Timeout < 1 || cfg.Global.Timeout > 30 {
		return fmt.Errorf("global.timeout must be between 1 and 30 seconds, got %d", cfg.Global.Timeout)
	}
	if cfg.Global.Workers < 1 || cfg.Global.Workers > 100 { // Arbitrary limit
		return fmt.Errorf("global.workers must be between 1 and 100")
	}

	// Validate devices
	if len(cfg.Devices) == 0 {
		return fmt.Errorf("no devices configured")
	}

	for i, dev := range cfg.Devices {
		// Check URL
		if dev.URL == "" {
			return fmt.Errorf("device %d: URL is required", i)
		}
		if _, err := url.Parse(dev.URL); err != nil {
			return fmt.Errorf("device %d: invalid URL: %w", i, err)
		}

		// Check credentials
		if dev.Username == "" {
			return fmt.Errorf("device %d: username is required", i)
		}
		if dev.Password == "" {
			return fmt.Errorf("device %d: password is required", i)
		}
	}

	return nil
}

func Set(cfg *Config) {
	globalConfig = cfg
}

func Get() *Config {
	if globalConfig == nil {
		return &Config{
			Global: defaultGlobal,
		}
	}
	return globalConfig
}

// Helper functions with proper types
func GetTimeout() time.Duration {
	return time.Duration(Get().Global.Timeout) * time.Second
}

func GetWorkers() int {
	return Get().Global.Workers
}
