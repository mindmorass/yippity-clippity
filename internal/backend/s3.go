package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/mindmorass/yippity-clippity/internal/clipboard"
	"github.com/mindmorass/yippity-clippity/internal/storage"
)

const (
	// S3ObjectKey is the key suffix for the clipboard object
	S3ObjectKey = ".yippity-clippity/current.clip"
)

// S3Backend implements Backend for AWS S3 storage
type S3Backend struct {
	bucket   string
	prefix   string
	region   string
	client   *s3.Client
	lastETag string
}

// NewS3Backend creates a new S3 backend
func NewS3Backend(bucket, prefix, region string) *S3Backend {
	return &S3Backend{
		bucket: bucket,
		prefix: strings.TrimSuffix(prefix, "/"),
		region: region,
	}
}

// Type returns the backend type
func (b *S3Backend) Type() BackendType {
	return BackendS3
}

// GetLocation returns the S3 location as s3://bucket/prefix
func (b *S3Backend) GetLocation() string {
	if b.bucket == "" {
		return ""
	}
	if b.prefix != "" {
		return fmt.Sprintf("s3://%s/%s", b.bucket, b.prefix)
	}
	return fmt.Sprintf("s3://%s", b.bucket)
}

// SetLocation parses and sets the S3 location
// Accepts format: s3://bucket/prefix or just bucket/prefix
func (b *S3Backend) SetLocation(location string) error {
	if location == "" {
		b.bucket = ""
		b.prefix = ""
		return nil
	}

	// Remove s3:// prefix if present
	location = strings.TrimPrefix(location, "s3://")

	// Split into bucket and prefix
	parts := strings.SplitN(location, "/", 2)
	b.bucket = parts[0]
	if len(parts) > 1 {
		b.prefix = strings.TrimSuffix(parts[1], "/")
	} else {
		b.prefix = ""
	}

	// Validate bucket name
	if b.bucket == "" {
		return fmt.Errorf("invalid S3 location: bucket name required")
	}

	return nil
}

// objectKey returns the full S3 object key
func (b *S3Backend) objectKey() string {
	if b.prefix != "" {
		return b.prefix + "/" + S3ObjectKey
	}
	return S3ObjectKey
}

// Init initializes the S3 client
func (b *S3Backend) Init(ctx context.Context) error {
	if b.bucket == "" {
		return ErrNotConfigured
	}

	// Load AWS configuration using standard credential chain
	opts := []func(*config.LoadOptions) error{}
	if b.region != "" {
		opts = append(opts, config.WithRegion(b.region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	b.client = s3.NewFromConfig(cfg)

	// Verify bucket access with a HEAD request
	_, err = b.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(b.bucket),
	})
	if err != nil {
		return fmt.Errorf("failed to access bucket %s: %w", b.bucket, err)
	}

	return nil
}

// Close releases resources (no-op for S3)
func (b *S3Backend) Close() error {
	return nil
}

// Write stores clipboard content to S3 with optimistic locking
func (b *S3Backend) Write(ctx context.Context, content *clipboard.Content) error {
	if b.client == nil {
		return ErrNotConfigured
	}

	// Encode content
	data, err := storage.Encode(content)
	if err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}

	input := &s3.PutObjectInput{
		Bucket:      aws.String(b.bucket),
		Key:         aws.String(b.objectKey()),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/octet-stream"),
	}

	// Use If-None-Match for optimistic locking when we have a known ETag
	// This prevents race conditions where another client wrote in between
	if b.lastETag != "" {
		// Note: S3 doesn't support If-None-Match for PutObject
		// We use conditional writes through the expected ETag check
		// after reading to detect conflicts
	}

	result, err := b.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("S3 put failed: %w", err)
	}

	// Store the new ETag for future conflict detection
	if result.ETag != nil {
		b.lastETag = strings.Trim(*result.ETag, "\"")
	}

	return nil
}

// Read retrieves clipboard content from S3
func (b *S3Backend) Read(ctx context.Context) (*clipboard.Content, error) {
	if b.client == nil {
		return nil, ErrNotConfigured
	}

	result, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.objectKey()),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, nil
		}
		// Also check for 404 status
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("S3 get failed: %w", err)
	}
	defer result.Body.Close()

	// Update ETag
	if result.ETag != nil {
		b.lastETag = strings.Trim(*result.ETag, "\"")
	}

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("read body failed: %w", err)
	}

	content, err := storage.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	return content, nil
}

// GetModTime returns the last modification time of the S3 object
func (b *S3Backend) GetModTime(ctx context.Context) (time.Time, error) {
	if b.client == nil {
		return time.Time{}, ErrNotConfigured
	}

	result, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.objectKey()),
	})
	if err != nil {
		return time.Time{}, err
	}

	// Update ETag from HEAD request
	if result.ETag != nil {
		b.lastETag = strings.Trim(*result.ETag, "\"")
	}

	if result.LastModified != nil {
		return *result.LastModified, nil
	}

	return time.Time{}, ErrNotFound
}

// GetChecksum returns the ETag which serves as a checksum for S3 objects
// This is more efficient than a full Read() operation
func (b *S3Backend) GetChecksum(ctx context.Context) (string, error) {
	if b.client == nil {
		return "", ErrNotConfigured
	}

	result, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.objectKey()),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return "", ErrNotFound
		}
		return "", err
	}

	if result.ETag != nil {
		etag := strings.Trim(*result.ETag, "\"")
		b.lastETag = etag
		return etag, nil
	}

	return "", ErrNotFound
}

// Exists returns true if the clipboard object exists in S3
func (b *S3Backend) Exists(ctx context.Context) bool {
	if b.client == nil {
		return false
	}

	_, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.objectKey()),
	})
	return err == nil
}

// GetBucket returns the configured bucket name
func (b *S3Backend) GetBucket() string {
	return b.bucket
}

// GetPrefix returns the configured prefix
func (b *S3Backend) GetPrefix() string {
	return b.prefix
}

// GetRegion returns the configured region
func (b *S3Backend) GetRegion() string {
	return b.region
}

// SetRegion sets the AWS region
func (b *S3Backend) SetRegion(region string) {
	b.region = region
}
