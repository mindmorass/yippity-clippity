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

	// Backend configuration
	BackendType string `mapstructure:"backend_type"` // "local", "s3", or "dropbox"

	// S3-specific settings
	S3Bucket string `mapstructure:"s3_bucket"`
	S3Prefix string `mapstructure:"s3_prefix"`
	S3Region string `mapstructure:"s3_region"`

	// Dropbox-specific settings (app credentials stored via environment or keychain)
	DropboxAppKey    string `mapstructure:"dropbox_app_key"`
	DropboxAppSecret string `mapstructure:"dropbox_app_secret"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		SharedLocation:   "",
		LaunchAtLogin:    false,
		BackendType:      "local",
		S3Bucket:         "",
		S3Prefix:         "",
		S3Region:         "",
		DropboxAppKey:    "",
		DropboxAppSecret: "",
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
	viper.SetDefault("backend_type", "local")
	viper.SetDefault("s3_bucket", "")
	viper.SetDefault("s3_prefix", "")
	viper.SetDefault("s3_region", "")
	viper.SetDefault("dropbox_app_key", "")
	viper.SetDefault("dropbox_app_secret", "")

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
	viper.Set("backend_type", config.BackendType)
	viper.Set("s3_bucket", config.S3Bucket)
	viper.Set("s3_prefix", config.S3Prefix)
	viper.Set("s3_region", config.S3Region)
	viper.Set("dropbox_app_key", config.DropboxAppKey)
	viper.Set("dropbox_app_secret", config.DropboxAppSecret)

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
