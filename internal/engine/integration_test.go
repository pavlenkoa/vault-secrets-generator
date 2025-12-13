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

	// Create engine
	defaults := config.DefaultPasswordPolicy()
	engine := NewEngine(vaultClient, registry, defaults, nil)

	// Create test config
	cfg := &config.Config{
		Secrets: map[string]config.SecretBlock{
			"test": {
				Path: "kv/vsg-integration-test",
				Data: map[string]string{
					"static_value":   "hello-world",
					"generated_pass": "generate(length=16, symbols=0)",
					"another_static": "test-value-123",
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

	adds, updates, deletes, _ := result.Diff.Summary()
	t.Logf("Plan: %d adds, %d updates, %d deletes", adds, updates, deletes)

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

	adds2, updates2, _, _ := result2.Diff.Summary()
	if adds2 != 0 || updates2 != 0 {
		t.Errorf("Expected no changes on second run, got %d adds, %d updates", adds2, updates2)
	}

	// Clean up
	err = kv.Delete(ctx, "vsg-integration-test")
	if err != nil {
		t.Logf("Warning: failed to clean up test secret: %v", err)
	}
}

func TestIntegration_ReconcileWithForce(t *testing.T) {
	vaultClient := skipIfNoVault(t)
	ctx := context.Background()

	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	engine := NewEngine(vaultClient, registry, defaults, nil)

	cfg := &config.Config{
		Secrets: map[string]config.SecretBlock{
			"test": {
				Path: "kv/vsg-force-test",
				Data: map[string]string{
					"password": "generate(length=20)",
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

	// Clean up
	kv.Delete(ctx, "vsg-force-test")
}

func TestIntegration_ReconcileOnlyBlock(t *testing.T) {
	vaultClient := skipIfNoVault(t)
	ctx := context.Background()

	registry := fetcher.NewRegistry()
	defaults := config.DefaultPasswordPolicy()
	engine := NewEngine(vaultClient, registry, defaults, nil)

	cfg := &config.Config{
		Secrets: map[string]config.SecretBlock{
			"block1": {
				Path: "kv/vsg-only-test-1",
				Data: map[string]string{"key": "value1"},
			},
			"block2": {
				Path: "kv/vsg-only-test-2",
				Data: map[string]string{"key": "value2"},
			},
		},
	}

	// Only process block1
	result, err := engine.Reconcile(ctx, cfg, Options{Only: "block1"})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Should only have processed block1
	if len(result.Diff.Blocks) != 1 {
		t.Errorf("Expected 1 block in diff, got %d", len(result.Diff.Blocks))
	}

	if result.Diff.Blocks[0].Name != "block1" {
		t.Errorf("Expected block1, got %s", result.Diff.Blocks[0].Name)
	}

	// Clean up
	kv, _ := vault.NewKVClient(vaultClient, "kv", vault.KVVersion2)
	kv.Delete(ctx, "vsg-only-test-1")
	kv.Delete(ctx, "vsg-only-test-2")
}
