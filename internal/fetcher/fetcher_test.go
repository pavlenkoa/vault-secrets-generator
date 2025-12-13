package fetcher

import (
	"context"
	"testing"
)

func TestRegistry_Caching(t *testing.T) {
	registry := NewRegistry()

	// Create a mock fetcher that counts calls
	callCount := 0
	mockFetcher := &mockFetcher{
		supports: func(uri string) bool { return true },
		fetch: func(ctx context.Context, uri string) ([]byte, error) {
			callCount++
			return []byte(`{"key":"value"}`), nil
		},
	}
	registry.Register(mockFetcher)

	ctx := context.Background()

	// First fetch
	_, err := registry.Fetch(ctx, "test://state.json")
	if err != nil {
		t.Fatalf("first fetch error: %v", err)
	}

	// Second fetch with same URI should use cache
	_, err = registry.Fetch(ctx, "test://state.json")
	if err != nil {
		t.Fatalf("second fetch error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 fetch call (cached), got %d", callCount)
	}

	// Fetch with different URI should not use cache
	_, err = registry.Fetch(ctx, "test://other.json")
	if err != nil {
		t.Fatalf("third fetch error: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 fetch calls, got %d", callCount)
	}

	// Clear cache and fetch again
	registry.ClearCache()
	_, err = registry.Fetch(ctx, "test://state.json")
	if err != nil {
		t.Fatalf("fourth fetch error: %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected 3 fetch calls after cache clear, got %d", callCount)
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
