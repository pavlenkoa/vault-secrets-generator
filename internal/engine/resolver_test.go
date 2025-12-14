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
	strategies := config.DefaultStrategyDefaults()
	resolver := NewResolver(registry, nil, defaults, strategies)

	ctx := context.Background()

	val := config.Value{
		Type:   config.ValueTypeStatic,
		Static: "static-value",
	}

	result, err := resolver.Resolve(ctx, val, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Value != "static-value" {
		t.Errorf("expected 'static-value', got %q", result.Value)
	}
	if result.Source != SourceStatic {
		t.Errorf("expected SourceStatic, got %s", result.Source)
	}
}

func TestResolver_ResolveGenerate(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	strategies := config.DefaultStrategyDefaults()
	resolver := NewResolver(registry, nil, defaults, strategies)

	ctx := context.Background()

	val := config.Value{
		Type: config.ValueTypeGenerate,
	}

	result, err := resolver.Resolve(ctx, val, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Value) != defaults.Length {
		t.Errorf("expected length %d, got %d", defaults.Length, len(result.Value))
	}
	if result.Source != SourceGenerated {
		t.Errorf("expected SourceGenerated, got %s", result.Source)
	}
}

func TestResolver_ResolveGenerateWithParams(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	strategies := config.DefaultStrategyDefaults()
	resolver := NewResolver(registry, nil, defaults, strategies)

	ctx := context.Background()

	val := config.Value{
		Type: config.ValueTypeGenerate,
		Generate: &config.PasswordPolicy{
			Length:  16,
			Symbols: 0,
		},
	}

	result, err := resolver.Resolve(ctx, val, "", false)
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
	strategies := config.DefaultStrategyDefaults()
	resolver := NewResolver(registry, nil, defaults, strategies)

	ctx := context.Background()

	val := config.Value{
		Type: config.ValueTypeGenerate,
	}

	// With existing value and no force, should keep existing (default strategy is "create")
	result, err := resolver.Resolve(ctx, val, "existing-password", false)
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
	strategies := config.DefaultStrategyDefaults()
	resolver := NewResolver(registry, nil, defaults, strategies)

	ctx := context.Background()

	val := config.Value{
		Type: config.ValueTypeGenerate,
	}

	// With force, should generate new value
	result, err := resolver.Resolve(ctx, val, "existing-password", true)
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

func TestResolver_ResolveJSON(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	strategies := config.DefaultStrategyDefaults()
	resolver := NewResolver(registry, nil, defaults, strategies)

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

	val := config.Value{
		Type:  config.ValueTypeJSON,
		URL:   "s3://bucket/state.tfstate",
		Query: ".outputs.endpoint.value",
	}

	result, err := resolver.Resolve(ctx, val, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Value != "db.example.com" {
		t.Errorf("expected 'db.example.com', got %q", result.Value)
	}
	if result.Source != SourceJSON {
		t.Errorf("expected SourceJSON, got %s", result.Source)
	}
}

func TestResolver_ResolveJSONCaching(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	strategies := config.DefaultStrategyDefaults()
	resolver := NewResolver(registry, nil, defaults, strategies)

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

	val1 := config.Value{
		Type:  config.ValueTypeJSON,
		URL:   "s3://bucket/state.tfstate",
		Query: ".outputs.value1.value",
	}
	val2 := config.Value{
		Type:  config.ValueTypeJSON,
		URL:   "s3://bucket/state.tfstate",
		Query: ".outputs.value2.value",
	}

	// Resolve two values from the same source file
	_, err := resolver.Resolve(ctx, val1, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = resolver.Resolve(ctx, val2, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only fetch once due to caching in registry
	if fetchCount != 1 {
		t.Errorf("expected 1 fetch (cached), got %d", fetchCount)
	}
}

func TestResolver_ResolveCommand(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	strategies := config.DefaultStrategyDefaults()
	resolver := NewResolver(registry, nil, defaults, strategies)

	ctx := context.Background()

	val := config.Value{
		Type:    config.ValueTypeCommand,
		Command: "echo hello-world",
	}

	result, err := resolver.Resolve(ctx, val, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Value != "hello-world" {
		t.Errorf("expected 'hello-world', got %q", result.Value)
	}
	if result.Source != SourceCommand {
		t.Errorf("expected SourceCommand, got %s", result.Source)
	}
}

func TestResolver_ResolveGenerateWithUpdateStrategy(t *testing.T) {
	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	strategies := config.DefaultStrategyDefaults()
	resolver := NewResolver(registry, nil, defaults, strategies)

	ctx := context.Background()

	// Set strategy to update - should regenerate even with existing value
	val := config.Value{
		Type:     config.ValueTypeGenerate,
		Strategy: config.StrategyUpdate,
	}

	result, err := resolver.Resolve(ctx, val, "existing-password", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With update strategy, should generate new value
	if result.Value == "existing-password" {
		t.Error("expected new generated value with update strategy, got existing")
	}
	if result.Source != SourceGenerated {
		t.Errorf("expected SourceGenerated, got %s", result.Source)
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
