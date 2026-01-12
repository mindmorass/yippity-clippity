package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mindmorass/yippity-clippity/internal/clipboard"
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

var (
	ErrLocked       = errors.New("clipboard file is locked by another process")
	ErrNoLocation   = errors.New("no shared location configured")
	ErrLocationNotExist = errors.New("shared location does not exist")
)

// LockInfo represents lock file contents
type LockInfo struct {
	Holder     string    `json:"holder"`
	PID        int       `json:"pid"`
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// Storage handles reading and writing clipboard files
type Storage struct {
	basePath string
}

// New creates a new Storage instance
func New(basePath string) *Storage {
	return &Storage{basePath: basePath}
}

// SetBasePath updates the shared location with path validation
func (s *Storage) SetBasePath(path string) error {
	if path == "" {
		s.basePath = ""
		return nil
	}

	// Clean and validate path
	cleanPath := filepath.Clean(path)

	// Ensure path is absolute
	if !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("path must be absolute: %s", path)
	}

	// Check for path traversal attempts
	if cleanPath != path && filepath.Base(cleanPath) == ".." {
		return fmt.Errorf("invalid path: %s", path)
	}

	s.basePath = cleanPath
	return nil
}

// GetBasePath returns the current base path
func (s *Storage) GetBasePath() string {
	return s.basePath
}

// syncDir returns the full path to the sync directory
func (s *Storage) syncDir() string {
	return filepath.Join(s.basePath, DirName)
}

// clipPath returns the full path to the clipboard file
func (s *Storage) clipPath() string {
	return filepath.Join(s.syncDir(), CurrentFile)
}

// lockPath returns the full path to the lock file
func (s *Storage) lockPath() string {
	return filepath.Join(s.syncDir(), LockFile)
}

// EnsureDir creates the sync directory if it doesn't exist
func (s *Storage) EnsureDir() error {
	if s.basePath == "" {
		return ErrNoLocation
	}

	// Check if base path exists
	if _, err := os.Stat(s.basePath); os.IsNotExist(err) {
		return ErrLocationNotExist
	}

	return os.MkdirAll(s.syncDir(), DirPermissions)
}

// Write writes clipboard content to the shared location
func (s *Storage) Write(content *clipboard.Content) error {
	if err := s.EnsureDir(); err != nil {
		return err
	}

	// Try to acquire lock
	if err := s.acquireLock(); err != nil {
		return err
	}
	defer s.releaseLock()

	// Encode content
	data, err := Encode(content)
	if err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}

	// Write to temp file first (atomic write)
	tempPath := s.clipPath() + ".tmp"
	if err := os.WriteFile(tempPath, data, FilePermissions); err != nil {
		return fmt.Errorf("write temp file failed: %w", err)
	}

	// Rename to final location (atomic on POSIX)
	if err := os.Rename(tempPath, s.clipPath()); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("rename failed: %w", err)
	}

	return nil
}

// Read reads clipboard content from the shared location
func (s *Storage) Read() (*clipboard.Content, error) {
	if s.basePath == "" {
		return nil, ErrNoLocation
	}

	data, err := os.ReadFile(s.clipPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read failed: %w", err)
	}

	content, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	return content, nil
}

// GetModTime returns the modification time of the clipboard file
func (s *Storage) GetModTime() (time.Time, error) {
	info, err := os.Stat(s.clipPath())
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// Exists returns true if the clipboard file exists
func (s *Storage) Exists() bool {
	_, err := os.Stat(s.clipPath())
	return err == nil
}

// acquireLock attempts to acquire the write lock using atomic operations
// to prevent TOCTOU race conditions
func (s *Storage) acquireLock() error {
	lockPath := s.lockPath()
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
		return s.acquireLockOnce(data)
	}

	var existingLock LockInfo
	if json.Unmarshal(existingData, &existingLock) != nil {
		// Corrupted lock file, remove and retry
		os.Remove(lockPath)
		return s.acquireLockOnce(data)
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
		return s.acquireLockOnce(data)
	}

	// Lock is held by another process and not expired
	return ErrLocked
}

// acquireLockOnce attempts to create lock file once (helper to avoid infinite recursion)
func (s *Storage) acquireLockOnce(data []byte) error {
	f, err := os.OpenFile(s.lockPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, FilePermissions)
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
func (s *Storage) releaseLock() {
	os.Remove(s.lockPath())
}

// CleanStaleLocks removes expired lock files
func (s *Storage) CleanStaleLocks() {
	lockPath := s.lockPath()
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
