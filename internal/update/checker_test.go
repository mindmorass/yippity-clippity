package update

import (
	"sync"
	"testing"
	"time"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{"1.2.0", "1.1.0", true},
		{"1.1.1", "1.1.0", true},
		{"2.0.0", "1.9.9", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.1.0", false},
		{"1.1.0", "dev", false},
		{"1.1.0", "", false},
		{"v1.2.0", "v1.1.0", true}, // v prefix
		{"1.0.0-alpha", "0.9.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.latest+">"+tt.current, func(t *testing.T) {
			got := isNewerVersion(tt.latest, tt.current)
			if got != tt.want {
				t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestChecker_ConcurrentCheckIfNeeded(t *testing.T) {
	// Verifies that concurrent calls to CheckIfNeeded do not race on
	// lastCheck / lastResult.  Run with -race to catch data races.
	checker := NewChecker("1.0.0")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// CheckIfNeeded may make an HTTP request; we only care that it
			// doesn't race.  We tolerate any error here.
			_, _ = checker.CheckIfNeeded()
		}()
	}
	wg.Wait()
}

func TestChecker_GetLastResultInitiallyNil(t *testing.T) {
	checker := NewChecker("1.0.0")
	if got := checker.GetLastResult(); got != nil {
		t.Errorf("GetLastResult() = %v, want nil before any check", got)
	}
}

func TestChecker_CheckIfNeededUsesCacheWithinInterval(t *testing.T) {
	checker := NewChecker("1.0.0")

	// Pre-populate the cache
	cached := &UpdateInfo{Available: false, CurrentVersion: "1.0.0", LatestVersion: "1.0.0"}
	checker.mu.Lock()
	checker.lastResult = cached
	checker.lastCheck = time.Now() // mark as just checked
	checker.mu.Unlock()

	result, err := checker.CheckIfNeeded()
	if err != nil {
		t.Fatalf("CheckIfNeeded() returned unexpected error: %v", err)
	}
	if result != cached {
		t.Error("CheckIfNeeded() should return the cached result within the check interval")
	}
}
