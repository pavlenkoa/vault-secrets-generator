package engine

import (
	"context"
	"testing"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
	"github.com/pavlenkoa/vault-secrets-generator/internal/fetcher"
)

func TestResolver_ResolveStatic(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	resolver := NewResolver(registry, defaults)

	ctx := context.Background()

	result, err := resolver.Resolve(ctx, "static-value", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Value != "static-value" {
		t.Errorf("expected 'static-value', got %q", result.Value)
	}
	if result.Source != SourceStatic {
		t.Errorf("expected SourceStatic, got %s", result.Source)
	}
	if result.IsGenerated {
		t.Error("expected IsGenerated to be false")
	}
}

func TestResolver_ResolveGenerate(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	resolver := NewResolver(registry, defaults)

	ctx := context.Background()

	result, err := resolver.Resolve(ctx, "generate", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Value) != defaults.Length {
		t.Errorf("expected length %d, got %d", defaults.Length, len(result.Value))
	}
	if result.Source != SourceGenerated {
		t.Errorf("expected SourceGenerated, got %s", result.Source)
	}
	if !result.IsGenerated {
		t.Error("expected IsGenerated to be true")
	}
}

func TestResolver_ResolveGenerateWithParams(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	resolver := NewResolver(registry, defaults)

	ctx := context.Background()

	result, err := resolver.Resolve(ctx, "generate(length=16, symbols=0)", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Value) != 16 {
		t.Errorf("expected length 16, got %d", len(result.Value))
	}
}

func TestResolver_ResolveGenerateExistingNoForce(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	resolver := NewResolver(registry, defaults)

	ctx := context.Background()

	// With existing value and no force, should keep existing
	result, err := resolver.Resolve(ctx, "generate", "existing-password", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Value != "existing-password" {
		t.Errorf("expected 'existing-password', got %q", result.Value)
	}
	if result.Source != SourceExisting {
		t.Errorf("expected SourceExisting, got %s", result.Source)
	}
}

func TestResolver_ResolveGenerateExistingWithForce(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	resolver := NewResolver(registry, defaults)

	ctx := context.Background()

	// With force, should generate new value
	result, err := resolver.Resolve(ctx, "generate", "existing-password", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Value == "existing-password" {
		t.Error("expected new generated value, got existing")
	}
	if result.Source != SourceGenerated {
		t.Errorf("expected SourceGenerated, got %s", result.Source)
	}
}

func TestResolver_ResolveTerraformRef(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	resolver := NewResolver(registry, defaults)

	// Add a mock fetcher
	mockData := []byte(`{
		"version": 4,
		"outputs": {
			"endpoint": {
				"value": "db.example.com",
				"type": "string"
			}
		}
	}`)

	mockFetcher := &mockFetcherImpl{
		supports: func(uri string) bool { return true },
		fetch:    func(ctx context.Context, uri string) ([]byte, error) { return mockData, nil },
	}
	registry.Register(mockFetcher)

	ctx := context.Background()

	result, err := resolver.Resolve(ctx, "s3://bucket/state.tfstate#output.endpoint", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Value != "db.example.com" {
		t.Errorf("expected 'db.example.com', got %q", result.Value)
	}
	if result.Source != SourceTerraform {
		t.Errorf("expected SourceTerraform, got %s", result.Source)
	}
}

func TestResolver_ResolveTerraformRefCaching(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	resolver := NewResolver(registry, defaults)

	fetchCount := 0
	mockData := []byte(`{
		"version": 4,
		"outputs": {
			"value1": {"value": "one"},
			"value2": {"value": "two"}
		}
	}`)

	mockFetcher := &mockFetcherImpl{
		supports: func(uri string) bool { return true },
		fetch: func(ctx context.Context, uri string) ([]byte, error) {
			fetchCount++
			return mockData, nil
		},
	}
	registry.Register(mockFetcher)

	ctx := context.Background()

	// Resolve two values from the same state file
	_, err := resolver.Resolve(ctx, "s3://bucket/state.tfstate#output.value1", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = resolver.Resolve(ctx, "s3://bucket/state.tfstate#output.value2", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only fetch once due to caching
	if fetchCount != 1 {
		t.Errorf("expected 1 fetch (cached), got %d", fetchCount)
	}
}

// mockFetcherImpl implements fetcher.Fetcher for testing
type mockFetcherImpl struct {
	supports func(uri string) bool
	fetch    func(ctx context.Context, uri string) ([]byte, error)
}

func (m *mockFetcherImpl) Supports(uri string) bool {
	return m.supports(uri)
}

func (m *mockFetcherImpl) Fetch(ctx context.Context, uri string) ([]byte, error) {
	return m.fetch(ctx, uri)
}
