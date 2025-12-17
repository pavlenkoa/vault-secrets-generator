package engine

import (
	"context"
	"os"
	"testing"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
	"github.com/pavlenkoa/vault-secrets-generator/internal/fetcher"
	"github.com/pavlenkoa/vault-secrets-generator/internal/vault"
)

func skipIfNoVault(t *testing.T) *vault.Client {
	t.Helper()

	if os.Getenv("VAULT_ADDR") == "" || os.Getenv("VAULT_TOKEN") == "" {
		t.Skip("VAULT_ADDR or VAULT_TOKEN not set, skipping integration test")
	}

	cfg := config.VaultConfig{
		Auth: config.AuthConfig{
			Method: "token",
		},
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create Vault client: %v", err)
	}

	return client
}

func TestIntegration_Reconcile(t *testing.T) {
	vaultClient := skipIfNoVault(t)
	ctx := context.Background()

	// Set up fetcher registry
	registry := fetcher.NewRegistry()
	registry.Register(fetcher.NewLocalFetcher())

	// Create engine with config.Defaults
	defaults := config.Defaults{
		Mount:    "kv",
		Strategy: config.DefaultStrategyDefaults(),
		Generate: config.DefaultPasswordPolicy(),
	}
	engine := NewEngine(vaultClient, registry, defaults, nil)

	// Create test config using v2.0 structure
	cfg := &config.Config{
		Defaults: defaults,
		Secrets: map[string]config.SecretBlock{
			"integration-test": {
				Name:  "integration-test",
				Mount: "kv",
				Path:  "vsg-integration-test",
				Content: map[string]config.Value{
					"static_value": {
						Type:   config.ValueTypeStatic,
						Static: "hello-world",
					},
					"generated_pass": {
						Type: config.ValueTypeGenerate,
						Generate: &config.PasswordPolicy{
							Length:  16,
							Symbols: 0,
						},
					},
					"another_static": {
						Type:   config.ValueTypeStatic,
						Static: "test-value-123",
					},
				},
			},
		},
	}

	// Run reconciliation (dry-run first)
	result, err := engine.Plan(ctx, cfg, Options{})
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	if !result.Diff.HasChanges() {
		t.Log("No changes detected (secrets may already exist)")
	}

	adds, updates, deletes, unmanaged, _ := result.Diff.Summary()
	t.Logf("Plan: %d adds, %d updates, %d deletes, %d unmanaged", adds, updates, deletes, unmanaged)

	// Now apply
	result, err = engine.Reconcile(ctx, cfg, Options{})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			t.Errorf("Error: %v", e)
		}
		t.Fatal("Reconcile had errors")
	}

	// Verify secrets were written
	kv, err := vault.NewKVClient(vaultClient, "kv", vault.KVVersion2)
	if err != nil {
		t.Fatalf("failed to create KV client: %v", err)
	}

	data, err := kv.Read(ctx, "vsg-integration-test")
	if err != nil {
		t.Fatalf("failed to read secrets: %v", err)
	}

	if data == nil {
		t.Fatal("expected data, got nil")
	}

	if data["static_value"] != "hello-world" {
		t.Errorf("static_value = %v, want 'hello-world'", data["static_value"])
	}

	if data["another_static"] != "test-value-123" {
		t.Errorf("another_static = %v, want 'test-value-123'", data["another_static"])
	}

	if gen, ok := data["generated_pass"].(string); !ok || len(gen) != 16 {
		t.Errorf("generated_pass length = %d, want 16", len(gen))
	}

	// Run again - should have no changes (idempotent)
	result2, err := engine.Plan(ctx, cfg, Options{})
	if err != nil {
		t.Fatalf("Second Plan failed: %v", err)
	}

	adds2, updates2, _, _, _ := result2.Diff.Summary()
	if adds2 != 0 || updates2 != 0 {
		t.Errorf("Expected no changes on second run, got %d adds, %d updates", adds2, updates2)
	}

	// Clean up - use Destroy to fully remove (not just soft delete)
	err = kv.Destroy(ctx, "vsg-integration-test")
	if err != nil {
		t.Logf("Warning: failed to clean up test secret: %v", err)
	}
}

func TestIntegration_ReconcileWithForce(t *testing.T) {
	vaultClient := skipIfNoVault(t)
	ctx := context.Background()

	registry := fetcher.NewRegistry()
	defaults := config.Defaults{
		Mount:    "kv",
		Strategy: config.DefaultStrategyDefaults(),
		Generate: config.DefaultPasswordPolicy(),
	}
	engine := NewEngine(vaultClient, registry, defaults, nil)

	cfg := &config.Config{
		Defaults: defaults,
		Secrets: map[string]config.SecretBlock{
			"force-test": {
				Name:  "force-test",
				Mount: "kv",
				Path:  "vsg-force-test",
				Content: map[string]config.Value{
					"password": {
						Type: config.ValueTypeGenerate,
						Generate: &config.PasswordPolicy{
							Length: 20,
						},
					},
				},
			},
		},
	}

	// First run - create secret
	_, err := engine.Reconcile(ctx, cfg, Options{})
	if err != nil {
		t.Fatalf("First Reconcile failed: %v", err)
	}

	// Read the generated password
	kv, _ := vault.NewKVClient(vaultClient, "kv", vault.KVVersion2)
	data1, _ := kv.Read(ctx, "vsg-force-test")
	pass1 := data1["password"].(string)

	// Second run without force - password should be same
	_, err = engine.Reconcile(ctx, cfg, Options{})
	if err != nil {
		t.Fatalf("Second Reconcile failed: %v", err)
	}

	data2, _ := kv.Read(ctx, "vsg-force-test")
	pass2 := data2["password"].(string)

	if pass1 != pass2 {
		t.Error("Password changed without --force")
	}

	// Third run with force - password should change
	_, err = engine.Reconcile(ctx, cfg, Options{Force: true})
	if err != nil {
		t.Fatalf("Third Reconcile failed: %v", err)
	}

	data3, _ := kv.Read(ctx, "vsg-force-test")
	pass3 := data3["password"].(string)

	if pass1 == pass3 {
		t.Error("Password did not change with --force")
	}

	// Clean up - use Destroy to fully remove
	kv.Destroy(ctx, "vsg-force-test")
}

func TestIntegration_ReconcileMultipleBlocks(t *testing.T) {
	vaultClient := skipIfNoVault(t)
	ctx := context.Background()

	registry := fetcher.NewRegistry()
	defaults := config.Defaults{
		Mount:    "kv",
		Strategy: config.DefaultStrategyDefaults(),
		Generate: config.DefaultPasswordPolicy(),
	}
	engine := NewEngine(vaultClient, registry, defaults, nil)

	cfg := &config.Config{
		Defaults: defaults,
		Secrets: map[string]config.SecretBlock{
			"multi-test-1": {
				Name:  "multi-test-1",
				Mount: "kv",
				Path:  "vsg-multi-test-1",
				Content: map[string]config.Value{
					"key": {
						Type:   config.ValueTypeStatic,
						Static: "value1",
					},
				},
			},
			"multi-test-2": {
				Name:  "multi-test-2",
				Mount: "kv",
				Path:  "vsg-multi-test-2",
				Content: map[string]config.Value{
					"key": {
						Type:   config.ValueTypeStatic,
						Static: "value2",
					},
				},
			},
		},
	}

	// Process all blocks
	result, err := engine.Reconcile(ctx, cfg, Options{})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Should have processed both blocks
	if len(result.Diff.Blocks) != 2 {
		t.Errorf("Expected 2 blocks in diff, got %d", len(result.Diff.Blocks))
	}

	// Clean up - use Destroy to fully remove
	kv, _ := vault.NewKVClient(vaultClient, "kv", vault.KVVersion2)
	kv.Destroy(ctx, "vsg-multi-test-1")
	kv.Destroy(ctx, "vsg-multi-test-2")
}
