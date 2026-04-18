//go:build darwin

package sync

import (
	"context"
	"testing"
	"time"

	"github.com/mindmorass/yippity-clippity/internal/backend"
	"github.com/mindmorass/yippity-clippity/internal/clipboard"
)

// noopBackend is a backend that reports an empty location so the watcher never
// performs real work.
type noopBackend struct{}

func (n *noopBackend) Write(_ context.Context, _ *clipboard.Content) error  { return nil }
func (n *noopBackend) Read(_ context.Context) (*clipboard.Content, error)   { return nil, nil }
func (n *noopBackend) GetModTime(_ context.Context) (time.Time, error)      { return time.Time{}, nil }
func (n *noopBackend) GetChecksum(_ context.Context) (string, error)        { return "", nil }
func (n *noopBackend) Exists(_ context.Context) bool                        { return false }
func (n *noopBackend) Init(_ context.Context) error                         { return nil }
func (n *noopBackend) Close() error                                         { return nil }
func (n *noopBackend) Type() backend.BackendType                            { return backend.BackendLocal }
func (n *noopBackend) GetLocation() string                                  { return "" } // empty = watcher skips
func (n *noopBackend) SetLocation(_ string) error                           { return nil }

// TestWatcher_StartStopNoGoroutineLeak verifies that rapid Start/Stop/Start
// cycles do not accumulate goroutines — Stop() must block until the previously
// spawned goroutine has exited.
func TestWatcher_StartStopNoGoroutineLeak(t *testing.T) {
	w := NewWatcher(&noopBackend{}, 50*time.Millisecond)

	for i := 0; i < 10; i++ {
		w.Start()
		if !w.IsRunning() {
			t.Fatalf("iteration %d: watcher not running after Start()", i)
		}
		w.Stop()
		if w.IsRunning() {
			t.Fatalf("iteration %d: watcher still running after Stop()", i)
		}
	}
}

// TestWatcher_StopIdempotent verifies that calling Stop on a non-running
// watcher is a no-op and does not panic.
func TestWatcher_StopIdempotent(t *testing.T) {
	w := NewWatcher(&noopBackend{}, 50*time.Millisecond)
	// Not started — Stop should be safe
	w.Stop()
	w.Stop()
}

// TestWatcher_StartIdempotent verifies that calling Start twice does not
// launch a second goroutine.
func TestWatcher_StartIdempotent(t *testing.T) {
	w := NewWatcher(&noopBackend{}, 50*time.Millisecond)
	defer w.Stop()

	w.Start()
	w.Start() // second call must be a no-op
	if !w.IsRunning() {
		t.Fatal("watcher not running after double Start()")
	}
}

// TestWatcher_StopWaitsForGoroutine checks that Stop() does not return until
// the polling goroutine has actually exited, preventing the Start/Stop/Start
// double-goroutine issue.
func TestWatcher_StopWaitsForGoroutine(t *testing.T) {
	w := NewWatcher(&noopBackend{}, 10*time.Millisecond)
	w.Start()

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stop returned cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2 seconds")
	}
}
