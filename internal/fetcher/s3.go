package fetcher

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Fetcher retrieves terraform state from AWS S3.
type S3Fetcher struct {
	client *s3.Client
}

// NewS3Fetcher creates a new S3 fetcher using the default AWS credential chain.
// This supports environment variables, shared credentials, IRSA, and EC2 instance roles.
func NewS3Fetcher(ctx context.Context) (*S3Fetcher, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	return &S3Fetcher{
		client: client,
	}, nil
}

// NewS3FetcherWithConfig creates a new S3 fetcher with a specific AWS config.
func NewS3FetcherWithConfig(cfg aws.Config) *S3Fetcher {
	return &S3Fetcher{
		client: s3.NewFromConfig(cfg),
	}
}

// Supports returns true for s3:// URIs.
func (f *S3Fetcher) Supports(uri string) bool {
	return strings.HasPrefix(uri, "s3://")
}

// Fetch retrieves the terraform state file from S3.
func (f *S3Fetcher) Fetch(ctx context.Context, uri string) ([]byte, error) {
	bucket, key, err := f.parseURI(uri)
	if err != nil {
		return nil, err
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := f.client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("fetching s3://%s/%s: %w", bucket, key, err)
	}
	//nolint:errcheck // Best effort close on defer
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("reading s3 object body: %w", err)
	}

	return data, nil
}

// parseURI extracts bucket and key from an s3:// URI.
// Format: s3://bucket/path/to/key
func (f *S3Fetcher) parseURI(uri string) (bucket, key string, err error) {
	if !strings.HasPrefix(uri, "s3://") {
		return "", "", fmt.Errorf("invalid S3 URI: %s", uri)
	}

	// Remove s3:// prefix
	path := strings.TrimPrefix(uri, "s3://")

	// Split into bucket and key
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid S3 URI format (expected s3://bucket/key): %s", uri)
	}

	return parts[0], parts[1], nil
}

// S3FetcherOption configures an S3Fetcher.
type S3FetcherOption func(*S3Fetcher)

// WithS3Client sets a custom S3 client.
func WithS3Client(client *s3.Client) S3FetcherOption {
	return func(f *S3Fetcher) {
		f.client = client
	}
}

// NewS3FetcherWithOptions creates a new S3 fetcher with options.
func NewS3FetcherWithOptions(ctx context.Context, opts ...S3FetcherOption) (*S3Fetcher, error) {
	fetcher, err := NewS3Fetcher(ctx)
	if err != nil {
		return nil, err
	}

	for _, opt := range opts {
		opt(fetcher)
	}

	return fetcher, nil
}
