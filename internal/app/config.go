package app

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	// ConfigFileName is the config file name (without extension)
	ConfigFileName = "config"

	// ConfigDir is the directory for config files
	ConfigDir = ".yippity-clippity"
)

// Config holds application configuration
type Config struct {
	SharedLocation string `mapstructure:"shared_location"`
	LaunchAtLogin  bool   `mapstructure:"launch_at_login"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		SharedLocation: "",
		LaunchAtLogin:  false,
	}
}

// LoadConfig loads configuration from file
func LoadConfig() (*Config, error) {
	configDir := getConfigDir()

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}

	viper.SetConfigName(ConfigFileName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configDir)

	// Set defaults
	viper.SetDefault("shared_location", "")
	viper.SetDefault("launch_at_login", false)

	// Try to read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, use defaults
			return DefaultConfig(), nil
		}
		return nil, err
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveConfig saves configuration to file
func SaveConfig(config *Config) error {
	configDir := getConfigDir()

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}

	viper.Set("shared_location", config.SharedLocation)
	viper.Set("launch_at_login", config.LaunchAtLogin)

	configPath := filepath.Join(configDir, ConfigFileName+".yaml")
	return viper.WriteConfigAs(configPath)
}

func getConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ConfigDir)
}
