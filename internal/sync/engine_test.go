//go:build darwin

package sync

import (
	"context"
	"testing"
	"time"

	"github.com/mindmorass/yippity-clippity/internal/backend"
	"github.com/mindmorass/yippity-clippity/internal/clipboard"
)

// stubBackend is an in-memory backend for testing.
type stubBackend struct {
	writes  []*clipboard.Content
	readErr error
	writeErr error
}

func (s *stubBackend) Write(_ context.Context, c *clipboard.Content) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.writes = append(s.writes, c)
	return nil
}
func (s *stubBackend) Read(_ context.Context) (*clipboard.Content, error) {
	return nil, s.readErr
}
func (s *stubBackend) GetModTime(_ context.Context) (time.Time, error) { return time.Time{}, nil }
func (s *stubBackend) GetChecksum(_ context.Context) (string, error)   { return "", nil }
func (s *stubBackend) Exists(_ context.Context) bool                   { return false }
func (s *stubBackend) Init(_ context.Context) error                    { return nil }
func (s *stubBackend) Close() error                                    { return nil }
func (s *stubBackend) Type() backend.BackendType                       { return backend.BackendLocal }
func (s *stubBackend) GetLocation() string                             { return "/tmp/test" }
func (s *stubBackend) SetLocation(_ string) error                      { return nil }

func makeContent(data []byte, contentType clipboard.ContentType) *clipboard.Content {
	return &clipboard.Content{
		ID:            "test-id",
		Timestamp:     time.Now().UTC(),
		SourceMachine: "other-host",
		SourceUser:    "user",
		ContentType:   contentType,
		MimeType:      "text/plain",
		Checksum:      "abc123",
		Size:          int64(len(data)),
		Data:          data,
	}
}

func TestEngine_SizeCheckDropsOversizedPayload(t *testing.T) {
	stub := &stubBackend{}
	engine := NewEngineWithBackend(stub)
	engine.running = true

	oversized := makeContent(make([]byte, MaxSyncPayloadBytes+1), clipboard.ContentTypeText)
	engine.onLocalClipboardChange(oversized)

	if len(stub.writes) != 0 {
		t.Errorf("expected no writes for oversized payload; got %d", len(stub.writes))
	}
}

func TestEngine_SizeCheckAllowsPayloadAtLimit(t *testing.T) {
	stub := &stubBackend{}
	engine := NewEngineWithBackend(stub)
	engine.running = true

	atLimit := makeContent(make([]byte, MaxSyncPayloadBytes), clipboard.ContentTypeText)
	engine.onLocalClipboardChange(atLimit)

	if len(stub.writes) != 1 {
		t.Errorf("expected exactly 1 write for payload at limit; got %d", len(stub.writes))
	}
}

func TestEngine_WriteRetryOnTransientError(t *testing.T) {
	callCount := 0
	stub := &stubBackend{}
	engine := NewEngineWithBackend(stub)
	engine.running = true

	// Override write to fail the first two calls then succeed.
	// We patch the backend inline via a wrapper type.
	type countingBackend struct {
		*stubBackend
		callCount *int
		failUntil int
	}
	cb := &countingBackend{stubBackend: stub, callCount: &callCount, failUntil: 2}

	engine.backend = cb

	content := makeContent([]byte("hello"), clipboard.ContentTypeText)
	engine.onLocalClipboardChange(content)

	// Even though writes 1 and 2 fail, write 3 should succeed.
	if len(stub.writes) != 1 {
		t.Errorf("expected 1 successful write after retries; got %d", len(stub.writes))
	}
}

// countingBackend wraps stubBackend and fails the first N writes.
type countingBackend struct {
	*stubBackend
	callCount *int
	failUntil int
}

func (c *countingBackend) Write(ctx context.Context, content *clipboard.Content) error {
	*c.callCount++
	if *c.callCount <= c.failUntil {
		return context.DeadlineExceeded // simulate transient error
	}
	return c.stubBackend.Write(ctx, content)
}

func TestEngine_WriteExhaustsRetries(t *testing.T) {
	// If the backend always fails, the engine should set StatusError.
	stub := &stubBackend{writeErr: context.DeadlineExceeded}
	engine := NewEngineWithBackend(stub)
	engine.running = true

	statusChanged := make(chan Status, 5)
	engine.OnStatusChange(func(s Status) {
		statusChanged <- s
	})

	content := makeContent([]byte("hello"), clipboard.ContentTypeText)
	engine.onLocalClipboardChange(content)

	select {
	case s := <-statusChanged:
		if s != StatusError {
			t.Errorf("expected StatusError after exhausted retries, got %v", s)
		}
	case <-time.After(5 * time.Second):
		t.Error("timed out waiting for StatusError")
	}
}
