package sync

import (
	"log"
	"sync"
	"time"

	"github.com/mindmorass/yippity-clippity/internal/clipboard"
	"github.com/mindmorass/yippity-clippity/internal/storage"
)

// RemoteChangeHandler is called when remote clipboard changes
type RemoteChangeHandler func(*clipboard.Content)

// Watcher monitors the shared location for changes
// Uses polling because fsnotify doesn't work on network filesystems
type Watcher struct {
	storage      *storage.Storage
	interval     time.Duration
	lastModTime  time.Time
	lastChecksum string
	onChange     RemoteChangeHandler
	stopChan     chan struct{}
	running      bool
	mu           sync.Mutex
}

// NewWatcher creates a new remote watcher
func NewWatcher(stor *storage.Storage, interval time.Duration) *Watcher {
	return &Watcher{
		storage:  stor,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// SetStorage updates the storage instance
func (w *Watcher) SetStorage(stor *storage.Storage) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.storage = stor
}

// OnChange sets the handler for remote changes
func (w *Watcher) OnChange(handler RemoteChangeHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onChange = handler
}

// Start begins watching for remote changes
func (w *Watcher) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.stopChan = make(chan struct{})
	w.mu.Unlock()

	go w.run()
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.running {
		return
	}
	w.running = false
	close(w.stopChan)
}

// IsRunning returns true if watcher is active
func (w *Watcher) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (w *Watcher) run() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Initial check
	w.checkForChanges()

	for {
		select {
		case <-ticker.C:
			w.checkForChanges()
		case <-w.stopChan:
			return
		}
	}
}

func (w *Watcher) checkForChanges() {
	w.mu.Lock()
	stor := w.storage
	handler := w.onChange
	w.mu.Unlock()

	if stor == nil || stor.GetBasePath() == "" {
		return
	}

	// Check if file exists and has been modified
	modTime, err := stor.GetModTime()
	if err != nil {
		return // File doesn't exist yet
	}

	w.mu.Lock()
	if !modTime.After(w.lastModTime) {
		w.mu.Unlock()
		return
	}
	w.lastModTime = modTime
	w.mu.Unlock()

	// Read the content
	content, err := stor.Read()
	if err != nil {
		log.Printf("Failed to read remote clipboard: %v", err)
		return
	}
	if content == nil {
		return
	}

	// Check if content actually changed
	w.mu.Lock()
	if content.Checksum == w.lastChecksum {
		w.mu.Unlock()
		return
	}
	w.lastChecksum = content.Checksum
	w.mu.Unlock()

	// Notify handler
	if handler != nil {
		handler(content)
	}
}

// SetLastChecksum sets the last known checksum (used to prevent initial echo)
func (w *Watcher) SetLastChecksum(checksum string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastChecksum = checksum
}
