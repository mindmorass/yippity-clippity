package sync

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/mindmorass/yippity-clippity/internal/clipboard"
	"github.com/mindmorass/yippity-clippity/internal/storage"
)

// StatusHandler is called when sync status changes
type StatusHandler func(status Status)

// Status represents the current sync state
type Status int

const (
	StatusIdle Status = iota
	StatusSyncing
	StatusPaused
	StatusError
)

func (s Status) String() string {
	switch s {
	case StatusIdle:
		return "Idle"
	case StatusSyncing:
		return "Syncing"
	case StatusPaused:
		return "Paused"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// Engine coordinates clipboard synchronization
type Engine struct {
	storage          *storage.Storage
	clipboardMonitor *clipboard.Monitor
	remoteWatcher    *Watcher

	lastLocalContent  *clipboard.Content
	lastRemoteContent *clipboard.Content
	lastWriteChecksum string

	status         Status
	lastError      error
	lastSyncTime   time.Time
	onStatusChange StatusHandler

	paused  bool
	running bool
	mu      sync.Mutex
}

// NewEngine creates a new sync engine
func NewEngine(basePath string) *Engine {
	stor := storage.New(basePath)

	e := &Engine{
		storage:          stor,
		clipboardMonitor: clipboard.NewMonitor(100 * time.Millisecond),
		remoteWatcher:    NewWatcher(stor, 500*time.Millisecond),
		status:           StatusIdle,
	}

	// Set up callbacks
	e.clipboardMonitor.OnChange(e.onLocalClipboardChange)
	e.remoteWatcher.OnChange(e.onRemoteChange)

	return e
}

// SetSharedLocation updates the sync location
func (e *Engine) SetSharedLocation(path string) error {
	e.mu.Lock()
	wasRunning := e.running
	e.mu.Unlock()

	// Stop watcher while we change location
	if wasRunning {
		e.remoteWatcher.Stop()
	}

	e.mu.Lock()
	if err := e.storage.SetBasePath(path); err != nil {
		e.mu.Unlock()
		return err
	}
	e.remoteWatcher.SetStorage(e.storage)
	e.mu.Unlock()

	// Create directory if needed
	if err := e.storage.EnsureDir(); err != nil {
		return err
	}

	// Clean up any stale locks
	e.storage.CleanStaleLocks()

	// Restart watcher with new location
	if wasRunning && path != "" {
		e.remoteWatcher.Start()
	}

	log.Printf("Shared location set to: %s", path)
	return nil
}

// GetSharedLocation returns the current sync location
func (e *Engine) GetSharedLocation() string {
	return e.storage.GetBasePath()
}

// OnStatusChange sets the status change handler
func (e *Engine) OnStatusChange(handler StatusHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onStatusChange = handler
}

// Start begins the sync engine
func (e *Engine) Start() error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return nil
	}
	e.running = true
	e.paused = false
	e.mu.Unlock()

	// Start clipboard monitoring
	e.clipboardMonitor.Start()

	// Start remote watcher if location is set
	if e.storage.GetBasePath() != "" {
		e.remoteWatcher.Start()
	}

	e.setStatus(StatusSyncing)
	return nil
}

// Stop stops the sync engine
func (e *Engine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	e.mu.Unlock()

	e.clipboardMonitor.Stop()
	e.remoteWatcher.Stop()
	e.setStatus(StatusIdle)
}

// Pause pauses synchronization
func (e *Engine) Pause() {
	e.mu.Lock()
	e.paused = true
	e.mu.Unlock()
	e.setStatus(StatusPaused)
}

// Resume resumes synchronization
func (e *Engine) Resume() {
	e.mu.Lock()
	e.paused = false
	e.mu.Unlock()
	e.setStatus(StatusSyncing)
}

// IsPaused returns true if sync is paused
func (e *Engine) IsPaused() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.paused
}

// IsRunning returns true if engine is running
func (e *Engine) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// GetStatus returns the current status
func (e *Engine) GetStatus() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status
}

// GetLastSyncTime returns the last successful sync time
func (e *Engine) GetLastSyncTime() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastSyncTime
}

// GetLastError returns the last error
func (e *Engine) GetLastError() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastError
}

func (e *Engine) setStatus(status Status) {
	e.mu.Lock()
	e.status = status
	handler := e.onStatusChange
	e.mu.Unlock()

	if handler != nil {
		handler(status)
	}
}

func (e *Engine) onLocalClipboardChange(content *clipboard.Content) {
	e.mu.Lock()
	if e.paused || !e.running {
		e.mu.Unlock()
		return
	}

	// Skip if this is an echo of what we just applied from remote
	if e.lastWriteChecksum == content.Checksum {
		e.mu.Unlock()
		return
	}

	e.lastLocalContent = content
	e.mu.Unlock()

	// Write to shared location
	hostname, _ := os.Hostname()
	log.Printf("[%s] Local clipboard changed, writing to shared location", hostname)

	if err := e.storage.Write(content); err != nil {
		log.Printf("Failed to write clipboard: %v", err)
		e.mu.Lock()
		e.lastError = err
		e.mu.Unlock()
		e.setStatus(StatusError)
		return
	}

	e.mu.Lock()
	e.lastSyncTime = time.Now()
	e.lastError = nil
	e.mu.Unlock()
}

func (e *Engine) onRemoteChange(content *clipboard.Content) {
	e.mu.Lock()
	if e.paused || !e.running {
		e.mu.Unlock()
		return
	}

	// Skip if content is from this machine
	hostname, _ := os.Hostname()
	if content.SourceMachine == hostname {
		e.mu.Unlock()
		return
	}

	// Skip if we already have this content
	if e.lastRemoteContent != nil && e.lastRemoteContent.ID == content.ID {
		e.mu.Unlock()
		return
	}

	// Last-write-wins: only apply if remote is newer
	if e.lastLocalContent != nil && !content.Timestamp.After(e.lastLocalContent.Timestamp) {
		e.mu.Unlock()
		return
	}

	e.lastRemoteContent = content
	e.lastWriteChecksum = content.Checksum
	e.mu.Unlock()

	log.Printf("[%s] Remote clipboard changed from %s, applying locally", hostname, content.SourceMachine)

	// Apply to local clipboard
	if !clipboard.Write(content) {
		log.Printf("Failed to apply remote clipboard")
		return
	}

	// Update monitor's checksum to prevent echo
	e.clipboardMonitor.SetLastChecksum(content.Checksum)

	e.mu.Lock()
	e.lastSyncTime = time.Now()
	e.mu.Unlock()
}
