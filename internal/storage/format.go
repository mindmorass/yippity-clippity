package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/mindmorass/yippity-clippity/internal/clipboard"
)

const (
	// MagicBytes identifies a yippity-clippity clipboard file
	MagicBytes = "YCLP"

	// CurrentVersion is the current file format version
	CurrentVersion uint32 = 1

	// MaxHeaderSize limits header size to prevent memory issues
	MaxHeaderSize = 1024 * 1024 // 1 MB

	// MaxPayloadSize limits payload size
	MaxPayloadSize = 100 * 1024 * 1024 // 100 MB
)

var (
	ErrInvalidMagic     = errors.New("invalid magic bytes")
	ErrInvalidVersion   = errors.New("unsupported file format version")
	ErrHeaderTooLarge   = errors.New("header size exceeds maximum")
	ErrPayloadTooLarge  = errors.New("payload size exceeds maximum")
	ErrChecksumMismatch = errors.New("checksum verification failed")
	ErrInvalidHeader    = errors.New("invalid header format")
)

// FileHeader represents the JSON metadata in the file header
type FileHeader struct {
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	SourceMachine string `json:"source_machine"`
	SourceUser    string `json:"source_user"`
	ContentType   string `json:"content_type"`
	MimeType      string `json:"mime_type"`
	Checksum      string `json:"checksum"`
	Size          int64  `json:"size"`
}

// Encode serializes clipboard content to the .clip format
func Encode(content *clipboard.Content) ([]byte, error) {
	if content == nil {
		return nil, errors.New("content is nil")
	}

	// Create header
	header := FileHeader{
		ID:            content.ID,
		Timestamp:     content.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
		SourceMachine: content.SourceMachine,
		SourceUser:    content.SourceUser,
		ContentType:   string(content.ContentType),
		MimeType:      content.MimeType,
		Checksum:      content.Checksum,
		Size:          content.Size,
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}

	// Calculate total size
	// 4 (magic) + 4 (version) + 4 (header length) + header + payload
	totalSize := 12 + len(headerBytes) + len(content.Data)
	buf := bytes.NewBuffer(make([]byte, 0, totalSize))

	// Write magic bytes
	buf.WriteString(MagicBytes)

	// Write version (big-endian)
	if err := binary.Write(buf, binary.BigEndian, CurrentVersion); err != nil {
		return nil, err
	}

	// Write header length (big-endian)
	if err := binary.Write(buf, binary.BigEndian, uint32(len(headerBytes))); err != nil {
		return nil, err
	}

	// Write header
	buf.Write(headerBytes)

	// Write payload
	buf.Write(content.Data)

	return buf.Bytes(), nil
}

// Decode deserializes the .clip format to clipboard content
func Decode(data []byte) (*clipboard.Content, error) {
	if len(data) < 12 {
		return nil, ErrInvalidMagic
	}

	reader := bytes.NewReader(data)

	// Read and verify magic bytes
	magic := make([]byte, 4)
	if _, err := io.ReadFull(reader, magic); err != nil {
		return nil, err
	}
	if string(magic) != MagicBytes {
		return nil, ErrInvalidMagic
	}

	// Read version
	var version uint32
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil {
		return nil, err
	}
	if version > CurrentVersion {
		return nil, ErrInvalidVersion
	}

	// Read header length
	var headerLen uint32
	if err := binary.Read(reader, binary.BigEndian, &headerLen); err != nil {
		return nil, err
	}
	if headerLen > MaxHeaderSize {
		return nil, ErrHeaderTooLarge
	}

	// Read header
	headerBytes := make([]byte, headerLen)
	if _, err := io.ReadFull(reader, headerBytes); err != nil {
		return nil, err
	}

	var header FileHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrInvalidHeader
	}

	// Validate payload size
	if header.Size > MaxPayloadSize {
		return nil, ErrPayloadTooLarge
	}

	// Read payload
	payload := make([]byte, header.Size)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}

	// Verify checksum
	checksum := sha256.Sum256(payload)
	if hex.EncodeToString(checksum[:]) != header.Checksum {
		return nil, ErrChecksumMismatch
	}

	// Parse timestamp
	timestamp, err := parseTimestamp(header.Timestamp)
	if err != nil {
		return nil, err
	}

	return &clipboard.Content{
		ID:            header.ID,
		Timestamp:     timestamp,
		SourceMachine: header.SourceMachine,
		SourceUser:    header.SourceUser,
		ContentType:   clipboard.ContentType(header.ContentType),
		MimeType:      header.MimeType,
		Checksum:      header.Checksum,
		Size:          header.Size,
		Data:          payload,
	}, nil
}

func parseTimestamp(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
	}
	for _, format := range formats {
		t, err := time.Parse(format, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("unable to parse timestamp: " + s)
}
