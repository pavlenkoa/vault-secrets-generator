package fetcher

import (
	"context"
	"testing"
)

func TestParseURI(t *testing.T) {
	tests := []struct {
		name           string
		uri            string
		wantStateURI   string
		wantOutputPath string
		wantErr        bool
	}{
		{
			name:           "s3 uri",
			uri:            "s3://bucket/path/terraform.tfstate#output.endpoint",
			wantStateURI:   "s3://bucket/path/terraform.tfstate",
			wantOutputPath: "output.endpoint",
		},
		{
			name:           "gcs uri",
			uri:            "gcs://bucket/path/terraform.tfstate#output.value",
			wantStateURI:   "gcs://bucket/path/terraform.tfstate",
			wantOutputPath: "output.value",
		},
		{
			name:           "file uri",
			uri:            "file:///path/to/terraform.tfstate#output.name",
			wantStateURI:   "file:///path/to/terraform.tfstate",
			wantOutputPath: "output.name",
		},
		{
			name:           "module output",
			uri:            "s3://bucket/state.tfstate#output.module.rds.endpoint",
			wantStateURI:   "s3://bucket/state.tfstate",
			wantOutputPath: "output.module.rds.endpoint",
		},
		{
			name:    "missing hash",
			uri:     "s3://bucket/path/terraform.tfstate",
			wantErr: true,
		},
		{
			name:    "invalid output prefix",
			uri:     "s3://bucket/path/terraform.tfstate#endpoint",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateURI, outputPath, err := ParseURI(tt.uri)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if stateURI != tt.wantStateURI {
				t.Errorf("stateURI = %q, want %q", stateURI, tt.wantStateURI)
			}
			if outputPath != tt.wantOutputPath {
				t.Errorf("outputPath = %q, want %q", outputPath, tt.wantOutputPath)
			}
		})
	}
}

func TestIsTerraformStateRef(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"s3://bucket/path/terraform.tfstate#output.endpoint", true},
		{"gcs://bucket/path/terraform.tfstate#output.value", true},
		{"file:///path/to/terraform.tfstate#output.name", true},
		{"s3://bucket/path/terraform.tfstate#output.module.rds.endpoint", true},
		{"generate", false},
		{"generate(length=64)", false},
		{"static-value", false},
		{"s3://bucket/path/terraform.tfstate", false}, // missing #output
		{"http://example.com#output.value", false},    // wrong scheme
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			result := IsTerraformStateRef(tt.value)
			if result != tt.expected {
				t.Errorf("IsTerraformStateRef(%q) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestRegistry_Caching(t *testing.T) {
	registry := NewRegistry()

	// Create a mock fetcher that counts calls
	callCount := 0
	mockFetcher := &mockFetcher{
		supports: func(uri string) bool { return true },
		fetch: func(ctx context.Context, uri string) ([]byte, error) {
			callCount++
			return []byte(`{"outputs":{}}`), nil
		},
	}
	registry.Register(mockFetcher)

	ctx := context.Background()

	// First fetch
	_, err := registry.Fetch(ctx, "test://state.tfstate#output.value")
	if err != nil {
		t.Fatalf("first fetch error: %v", err)
	}

	// Second fetch should use cache
	_, err = registry.Fetch(ctx, "test://state.tfstate#output.other")
	if err != nil {
		t.Fatalf("second fetch error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 fetch call (cached), got %d", callCount)
	}

	// Clear cache and fetch again
	registry.ClearCache()
	_, err = registry.Fetch(ctx, "test://state.tfstate#output.value")
	if err != nil {
		t.Fatalf("third fetch error: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 fetch calls after cache clear, got %d", callCount)
	}
}

func TestRegistry_NoFetcher(t *testing.T) {
	registry := NewRegistry()

	_, err := registry.Fetch(context.Background(), "unknown://path")
	if err == nil {
		t.Error("expected error for unsupported URI")
	}
}

// mockFetcher is a test helper
type mockFetcher struct {
	supports func(uri string) bool
	fetch    func(ctx context.Context, uri string) ([]byte, error)
}

func (m *mockFetcher) Supports(uri string) bool {
	return m.supports(uri)
}

func (m *mockFetcher) Fetch(ctx context.Context, uri string) ([]byte, error) {
	return m.fetch(ctx, uri)
}
