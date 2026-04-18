//go:build darwin

package clipboard

import (
	"log"
	"sync"
	"time"
)

// ChangeHandler is called when clipboard content changes
type ChangeHandler func(*Content)

// Monitor watches for clipboard changes using polling
type Monitor struct {
	interval        time.Duration
	lastChangeCount int
	lastChecksum    string
	onChange        ChangeHandler
	stopChan        chan struct{}
	doneChan        chan struct{} // closed by the goroutine when it exits
	running         bool
	mu              sync.Mutex
}

// NewMonitor creates a new clipboard monitor
func NewMonitor(interval time.Duration) *Monitor {
	return &Monitor{
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// OnChange sets the handler for clipboard changes
func (m *Monitor) OnChange(handler ChangeHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = handler
}

// Start begins monitoring the clipboard
func (m *Monitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.lastChangeCount = GetChangeCount()
	m.stopChan = make(chan struct{})
	m.doneChan = make(chan struct{})
	m.mu.Unlock()

	go func() {
		defer close(m.doneChan)
		m.run()
	}()
}

// Stop stops the clipboard monitor and waits for the polling goroutine to exit.
func (m *Monitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopChan)
	done := m.doneChan
	m.mu.Unlock()

	<-done // wait for the goroutine to fully exit before returning
}

// IsRunning returns true if the monitor is active
func (m *Monitor) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *Monitor) run() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkForChanges()
		case <-m.stopChan:
			return
		}
	}
}

func (m *Monitor) checkForChanges() {
	currentCount := GetChangeCount()

	m.mu.Lock()
	lastCount := m.lastChangeCount
	handler := m.onChange
	m.mu.Unlock()

	if currentCount == lastCount {
		return
	}

	// Update change count
	m.mu.Lock()
	m.lastChangeCount = currentCount
	m.mu.Unlock()

	// Read clipboard content
	content, err := Read()
	if err != nil {
		log.Printf("Error reading clipboard: %v", err)
		return
	}
	if content == nil {
		return
	}

	// Check if content actually changed (by checksum)
	m.mu.Lock()
	if content.Checksum == m.lastChecksum {
		m.mu.Unlock()
		return
	}
	m.lastChecksum = content.Checksum
	m.mu.Unlock()

	// Call handler
	if handler != nil {
		handler(content)
	}
}

// SetLastChecksum sets the last known checksum (used to prevent echo)
func (m *Monitor) SetLastChecksum(checksum string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastChecksum = checksum
}
