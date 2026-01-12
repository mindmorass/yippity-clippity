package clipboard

import (
	"time"
)

// ContentType represents the type of clipboard content
type ContentType string

const (
	ContentTypeText  ContentType = "text"
	ContentTypeImage ContentType = "image"
)

// Content represents clipboard data with metadata
type Content struct {
	ID            string      `json:"id"`
	Timestamp     time.Time   `json:"timestamp"`
	SourceMachine string      `json:"source_machine"`
	SourceUser    string      `json:"source_user"`
	ContentType   ContentType `json:"content_type"`
	MimeType      string      `json:"mime_type"`
	Checksum      string      `json:"checksum"`
	Size          int64       `json:"size"`
	Data          []byte      `json:"-"` // Payload data, not serialized in header
}

// IsText returns true if content is text-based
func (c *Content) IsText() bool {
	return c.ContentType == ContentTypeText
}

// IsImage returns true if content is an image
func (c *Content) IsImage() bool {
	return c.ContentType == ContentTypeImage
}
