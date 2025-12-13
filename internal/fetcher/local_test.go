package fetcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFetcher_Supports(t *testing.T) {
	f := NewLocalFetcher()

	tests := []struct {
		uri      string
		expected bool
	}{
		{"file:///path/to/state.tfstate", true},
		{"file://relative/path.tfstate", true},
		{"s3://bucket/path.tfstate", false},
		{"gcs://bucket/path.tfstate", false},
		{"http://example.com/state.tfstate", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			result := f.Supports(tt.uri)
			if result != tt.expected {
				t.Errorf("Supports(%q) = %v, want %v", tt.uri, result, tt.expected)
			}
		})
	}
}

func TestLocalFetcher_Fetch(t *testing.T) {
	f := NewLocalFetcher()
	ctx := context.Background()

	// Create a temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.tfstate")
	content := []byte(`{"version":4,"outputs":{}}`)
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	// Test successful fetch
	data, err := f.Fetch(ctx, "file://"+tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("got %q, want %q", string(data), string(content))
	}
}

func TestLocalFetcher_Fetch_NotFound(t *testing.T) {
	f := NewLocalFetcher()
	ctx := context.Background()

	_, err := f.Fetch(ctx, "file:///nonexistent/path/state.tfstate")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLocalFetcher_Fetch_InvalidURI(t *testing.T) {
	f := NewLocalFetcher()
	ctx := context.Background()

	_, err := f.Fetch(ctx, "s3://bucket/path")
	if err == nil {
		t.Error("expected error for invalid URI scheme")
	}
}

func TestLocalFetcher_Fetch_EmptyPath(t *testing.T) {
	f := NewLocalFetcher()
	ctx := context.Background()

	_, err := f.Fetch(ctx, "file://")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestLocalFetcher_Fetch_Cancelled(t *testing.T) {
	f := NewLocalFetcher()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := f.Fetch(ctx, "file:///any/path")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
