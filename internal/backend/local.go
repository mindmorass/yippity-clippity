package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mindmorass/yippity-clippity/internal/clipboard"
	"github.com/mindmorass/yippity-clippity/internal/storage"
)

const (
	// DirName is the hidden directory name for clipboard sync
	DirName = ".yippity-clippity"

	// CurrentFile is the filename for current clipboard state
	CurrentFile = "current.clip"

	// LockFile is the filename for the write lock
	LockFile = "current.clip.lock"

	// LockTimeout is how long a lock is valid
	LockTimeout = 10 * time.Second

	// FilePermissions for clipboard files
	FilePermissions = 0600

	// DirPermissions for the sync directory
	DirPermissions = 0700
)

// LockInfo represents lock file contents
type LockInfo struct {
	Holder     string    `json:"holder"`
	PID        int       `json:"pid"`
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// LocalBackend implements Backend for local filesystem storage
type LocalBackend struct {
	basePath string
}

// NewLocalBackend creates a new local filesystem backend
func NewLocalBackend(basePath string) *LocalBackend {
	return &LocalBackend{basePath: basePath}
}

// Type returns the backend type
func (b *LocalBackend) Type() BackendType {
	return BackendLocal
}

// GetLocation returns the current base path
func (b *LocalBackend) GetLocation() string {
	return b.basePath
}

// SetLocation updates the base path with validation
func (b *LocalBackend) SetLocation(location string) error {
	if location == "" {
		b.basePath = ""
		return nil
	}

	// Clean and validate path
	cleanPath := filepath.Clean(location)

	// Ensure path is absolute
	if !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("path must be absolute: %s", location)
	}

	// Check for path traversal attempts
	if cleanPath != location && filepath.Base(cleanPath) == ".." {
		return fmt.Errorf("invalid path: %s", location)
	}

	b.basePath = cleanPath
	return nil
}

// syncDir returns the full path to the sync directory
func (b *LocalBackend) syncDir() string {
	return filepath.Join(b.basePath, DirName)
}

// clipPath returns the full path to the clipboard file
func (b *LocalBackend) clipPath() string {
	return filepath.Join(b.syncDir(), CurrentFile)
}

// lockPath returns the full path to the lock file
func (b *LocalBackend) lockPath() string {
	return filepath.Join(b.syncDir(), LockFile)
}

// Init creates the sync directory if it doesn't exist
func (b *LocalBackend) Init(ctx context.Context) error {
	if b.basePath == "" {
		return ErrNotConfigured
	}

	// Check if base path exists
	if _, err := os.Stat(b.basePath); os.IsNotExist(err) {
		return fmt.Errorf("location does not exist: %s", b.basePath)
	}

	// Create sync directory
	if err := os.MkdirAll(b.syncDir(), DirPermissions); err != nil {
		return err
	}

	// Clean up any stale locks
	b.cleanStaleLocks()

	return nil
}

// Close releases resources (no-op for local backend)
func (b *LocalBackend) Close() error {
	return nil
}

// Write stores clipboard content to the shared location
func (b *LocalBackend) Write(ctx context.Context, content *clipboard.Content) error {
	if b.basePath == "" {
		return ErrNotConfigured
	}

	if err := b.Init(ctx); err != nil {
		return err
	}

	// Try to acquire lock
	if err := b.acquireLock(); err != nil {
		return err
	}
	defer b.releaseLock()

	// Encode content using shared format
	data, err := storage.Encode(content)
	if err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}

	// Write to temp file first (atomic write)
	tempPath := b.clipPath() + ".tmp"
	if err := os.WriteFile(tempPath, data, FilePermissions); err != nil {
		return fmt.Errorf("write temp file failed: %w", err)
	}

	// Rename to final location (atomic on POSIX)
	if err := os.Rename(tempPath, b.clipPath()); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("rename failed: %w", err)
	}

	return nil
}

// Read retrieves clipboard content from the shared location
func (b *LocalBackend) Read(ctx context.Context) (*clipboard.Content, error) {
	if b.basePath == "" {
		return nil, ErrNotConfigured
	}

	data, err := os.ReadFile(b.clipPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read failed: %w", err)
	}

	content, err := storage.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	return content, nil
}

// GetModTime returns the modification time of the clipboard file
func (b *LocalBackend) GetModTime(ctx context.Context) (time.Time, error) {
	info, err := os.Stat(b.clipPath())
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// GetChecksum returns the checksum without reading full content
// For local backend, we read and decode the file but could optimize later
func (b *LocalBackend) GetChecksum(ctx context.Context) (string, error) {
	content, err := b.Read(ctx)
	if err != nil {
		return "", err
	}
	if content == nil {
		return "", ErrNotFound
	}
	return content.Checksum, nil
}

// Exists returns true if the clipboard file exists
func (b *LocalBackend) Exists(ctx context.Context) bool {
	_, err := os.Stat(b.clipPath())
	return err == nil
}

// acquireLock attempts to acquire the write lock using atomic operations
func (b *LocalBackend) acquireLock() error {
	lockPath := b.lockPath()
	hostname, _ := os.Hostname()

	// Prepare lock info
	lockInfo := LockInfo{
		Holder:     hostname,
		PID:        os.Getpid(),
		AcquiredAt: time.Now(),
		ExpiresAt:  time.Now().Add(LockTimeout),
	}

	data, err := json.Marshal(lockInfo)
	if err != nil {
		return err
	}

	// Try to create lock file exclusively (atomic operation)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, FilePermissions)
	if err == nil {
		// Successfully created new lock
		defer f.Close()
		_, err = f.Write(data)
		return err
	}

	if !os.IsExist(err) {
		return err
	}

	// Lock file exists - check if it's stale or owned by us
	existingData, readErr := os.ReadFile(lockPath)
	if readErr != nil {
		// Can't read lock file, try to remove and retry once
		os.Remove(lockPath)
		return b.acquireLockOnce(data)
	}

	var existingLock LockInfo
	if json.Unmarshal(existingData, &existingLock) != nil {
		// Corrupted lock file, remove and retry
		os.Remove(lockPath)
		return b.acquireLockOnce(data)
	}

	// Check if we own this lock
	if existingLock.Holder == hostname && existingLock.PID == os.Getpid() {
		// We own it, update expiry
		return os.WriteFile(lockPath, data, FilePermissions)
	}

	// Check if lock is expired
	if time.Now().After(existingLock.ExpiresAt) {
		// Expired, remove and retry
		os.Remove(lockPath)
		return b.acquireLockOnce(data)
	}

	// Lock is held by another process and not expired
	return ErrLocked
}

// acquireLockOnce attempts to create lock file once (helper to avoid infinite recursion)
func (b *LocalBackend) acquireLockOnce(data []byte) error {
	f, err := os.OpenFile(b.lockPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, FilePermissions)
	if err != nil {
		if os.IsExist(err) {
			return ErrLocked
		}
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// releaseLock releases the write lock
func (b *LocalBackend) releaseLock() {
	os.Remove(b.lockPath())
}

// cleanStaleLocks removes expired lock files
func (b *LocalBackend) cleanStaleLocks() {
	lockPath := b.lockPath()
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return
	}

	var lockInfo LockInfo
	if json.Unmarshal(data, &lockInfo) != nil {
		return
	}

	if time.Now().After(lockInfo.ExpiresAt) {
		os.Remove(lockPath)
	}
}
