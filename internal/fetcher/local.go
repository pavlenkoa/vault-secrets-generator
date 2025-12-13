package fetcher

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// LocalFetcher retrieves terraform state from the local filesystem.
type LocalFetcher struct{}

// NewLocalFetcher creates a new local file fetcher.
func NewLocalFetcher() *LocalFetcher {
	return &LocalFetcher{}
}

// Supports returns true for file:// URIs.
func (f *LocalFetcher) Supports(uri string) bool {
	return strings.HasPrefix(uri, "file://")
}

// Fetch reads the terraform state file from the local filesystem.
func (f *LocalFetcher) Fetch(ctx context.Context, uri string) ([]byte, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	path, err := f.parsePath(uri)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", path, err)
	}

	return data, nil
}

// parsePath extracts the file path from a file:// URI.
func (f *LocalFetcher) parsePath(uri string) (string, error) {
	if !strings.HasPrefix(uri, "file://") {
		return "", fmt.Errorf("invalid file URI: %s", uri)
	}

	// file:///absolute/path or file://relative/path
	path := strings.TrimPrefix(uri, "file://")

	if path == "" {
		return "", fmt.Errorf("empty file path in URI: %s", uri)
	}

	return path, nil
}
