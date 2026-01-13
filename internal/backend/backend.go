package backend

import (
	"context"
	"errors"
	"time"

	"github.com/mindmorass/yippity-clippity/internal/clipboard"
)

// BackendType identifies the type of storage backend
type BackendType string

const (
	BackendLocal   BackendType = "local"
	BackendS3      BackendType = "s3"
	BackendDropbox BackendType = "dropbox"
)

// Common errors
var (
	ErrNotConfigured = errors.New("backend not configured")
	ErrNotFound      = errors.New("clipboard data not found")
	ErrLocked        = errors.New("resource is locked by another process")
	ErrConflict      = errors.New("write conflict detected")
)

// Backend defines the interface for clipboard storage backends
type Backend interface {
	// Write stores clipboard content
	Write(ctx context.Context, content *clipboard.Content) error

	// Read retrieves clipboard content
	Read(ctx context.Context) (*clipboard.Content, error)

	// GetModTime returns the last modification time
	GetModTime(ctx context.Context) (time.Time, error)

	// GetChecksum returns a lightweight checksum for change detection
	// This should be cheaper than a full Read() operation
	GetChecksum(ctx context.Context) (string, error)

	// Exists returns true if clipboard data exists
	Exists(ctx context.Context) bool

	// Init initializes the backend (creates directories, validates credentials, etc.)
	Init(ctx context.Context) error

	// Close releases any resources held by the backend
	Close() error

	// Type returns the backend type
	Type() BackendType

	// GetLocation returns a human-readable location string
	GetLocation() string

	// SetLocation updates the backend location/path
	SetLocation(location string) error
}

// Config holds configuration for creating backends
type Config struct {
	Type     BackendType
	Location string // For local: filesystem path

	// S3-specific
	S3Bucket string
	S3Prefix string
	S3Region string

	// Dropbox-specific
	DropboxAppKey    string
	DropboxAppSecret string
}
