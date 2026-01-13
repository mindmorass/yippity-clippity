package sync

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/mindmorass/yippity-clippity/internal/backend"
	"github.com/mindmorass/yippity-clippity/internal/clipboard"
)

// Adaptive polling constants
const (
	MinPollInterval = 50 * time.Millisecond  // During active use
	MaxPollInterval = 500 * time.Millisecond // During idle
	ActivityWindow  = 30 * time.Second       // Time window to consider "active"
)

// RemoteChangeHandler is called when remote clipboard changes
type RemoteChangeHandler func(*clipboard.Content)

// Watcher monitors the shared location for changes
// Uses polling because fsnotify doesn't work on network filesystems
// Implements adaptive polling: faster during active use, slower when idle
type Watcher struct {
	backend      backend.Backend
	interval     time.Duration
	lastModTime  time.Time
	lastChecksum string
	onChange     RemoteChangeHandler
	stopChan     chan struct{}
	running      bool

	// Adaptive polling state
	lastActivity    time.Time
	currentInterval time.Duration

	mu sync.Mutex
}

// NewWatcher creates a new remote watcher
func NewWatcher(b backend.Backend, interval time.Duration) *Watcher {
	return &Watcher{
		backend:         b,
		interval:        interval,
		currentInterval: interval,
		stopChan:        make(chan struct{}),
	}
}

// SetBackend updates the backend instance
func (w *Watcher) SetBackend(b backend.Backend) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.backend = b
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

// NotifyActivity signals that clipboard activity occurred
// This triggers faster polling for better responsiveness
func (w *Watcher) NotifyActivity() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastActivity = time.Now()
}

// getAdaptiveInterval calculates the current polling interval based on activity
func (w *Watcher) getAdaptiveInterval() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()

	timeSinceActivity := time.Since(w.lastActivity)

	if timeSinceActivity < ActivityWindow {
		// Active: use fast polling
		w.currentInterval = MinPollInterval
	} else {
		// Idle: gradually increase to max interval
		// Linear interpolation from min to max over another activity window
		idleTime := timeSinceActivity - ActivityWindow
		if idleTime >= ActivityWindow {
			w.currentInterval = MaxPollInterval
		} else {
			ratio := float64(idleTime) / float64(ActivityWindow)
			w.currentInterval = MinPollInterval + time.Duration(ratio*float64(MaxPollInterval-MinPollInterval))
		}
	}

	return w.currentInterval
}

func (w *Watcher) run() {
	// Start with the configured interval
	w.mu.Lock()
	w.currentInterval = w.interval
	w.mu.Unlock()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Initial check
	w.checkForChanges()

	for {
		select {
		case <-ticker.C:
			w.checkForChanges()

			// Adjust ticker interval based on activity
			newInterval := w.getAdaptiveInterval()
			ticker.Reset(newInterval)
		case <-w.stopChan:
			return
		}
	}
}

func (w *Watcher) checkForChanges() {
	w.mu.Lock()
	b := w.backend
	handler := w.onChange
	w.mu.Unlock()

	if b == nil || b.GetLocation() == "" {
		return
	}

	ctx := context.Background()

	// Check if file exists and has been modified
	modTime, err := b.GetModTime(ctx)
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
	content, err := b.Read(ctx)
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
