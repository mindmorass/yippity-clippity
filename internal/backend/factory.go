package backend

import (
	"fmt"
)

// New creates a new backend based on the configuration
func New(cfg *Config) (Backend, error) {
	if cfg == nil {
		cfg = &Config{Type: BackendLocal}
	}

	switch cfg.Type {
	case BackendLocal, "":
		b := NewLocalBackend(cfg.Location)
		return b, nil

	case BackendS3:
		b := NewS3Backend(cfg.S3Bucket, cfg.S3Prefix, cfg.S3Region)
		return b, nil

	case BackendDropbox:
		b := NewDropboxBackend(cfg.DropboxAppKey, cfg.DropboxAppSecret)
		return b, nil

	default:
		return nil, fmt.Errorf("unknown backend type: %s", cfg.Type)
	}
}

// NewDefault creates a local backend with no path configured
func NewDefault() Backend {
	return NewLocalBackend("")
}
