package vault

import (
	"context"
	"os"
	"testing"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
)

func TestKVVersion_String(t *testing.T) {
	tests := []struct {
		version  KVVersion
		expected int
	}{
		{KVVersionAuto, 0},
		{KVVersion1, 1},
		{KVVersion2, 2},
	}

	for _, tt := range tests {
		if int(tt.version) != tt.expected {
			t.Errorf("KVVersion %d != %d", tt.version, tt.expected)
		}
	}
}

func TestBuildReadPath_V1(t *testing.T) {
	kv := &KVClient{
		mount:   "secret",
		version: KVVersion1,
	}

	tests := []struct {
		path     string
		expected string
	}{
		{"myapp/config", "secret/myapp/config"},
		{"/myapp/config", "secret/myapp/config"},
		{"single", "secret/single"},
	}

	for _, tt := range tests {
		result := kv.buildReadPath(tt.path)
		if result != tt.expected {
			t.Errorf("buildReadPath(%q) = %q, want %q", tt.path, result, tt.expected)
		}
	}
}

func TestBuildReadPath_V2(t *testing.T) {
	kv := &KVClient{
		mount:   "secret",
		version: KVVersion2,
	}

	tests := []struct {
		path     string
		expected string
	}{
		{"myapp/config", "secret/data/myapp/config"},
		{"/myapp/config", "secret/data/myapp/config"},
		{"single", "secret/data/single"},
	}

	for _, tt := range tests {
		result := kv.buildReadPath(tt.path)
		if result != tt.expected {
			t.Errorf("buildReadPath(%q) = %q, want %q", tt.path, result, tt.expected)
		}
	}
}

func TestBuildDeletePath_V2(t *testing.T) {
	kv := &KVClient{
		mount:   "secret",
		version: KVVersion2,
	}

	path := "myapp/config"
	expected := "secret/data/myapp/config"
	result := kv.buildDeletePath(path)
	if result != expected {
		t.Errorf("buildDeletePath(%q) = %q, want %q", path, result, expected)
	}
}

// Integration tests - require a running Vault server
// Set VAULT_ADDR and VAULT_TOKEN to run these

func skipIfNoVault(t *testing.T) *Client {
	t.Helper()

	if os.Getenv("VAULT_ADDR") == "" || os.Getenv("VAULT_TOKEN") == "" {
		t.Skip("VAULT_ADDR or VAULT_TOKEN not set, skipping integration test")
	}

	cfg := config.VaultConfig{
		Auth: config.AuthConfig{
			Method: "token",
		},
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	return client
}

func TestIntegration_KVReadWrite(t *testing.T) {
	client := skipIfNoVault(t)
	ctx := context.Background()

	// Create KV client for "kv" mount (v2)
	kv, err := NewKVClient(client, "kv", KVVersion2)
	if err != nil {
		t.Fatalf("failed to create KV client: %v", err)
	}

	testPath := "vsg-test/integration"
	testData := map[string]interface{}{
		"username": "testuser",
		"password": "testpass123",
		"port":     "5432",
	}

	// Write
	err = kv.Write(ctx, testPath, testData)
	if err != nil {
		t.Fatalf("failed to write secret: %v", err)
	}

	// Read
	data, err := kv.Read(ctx, testPath)
	if err != nil {
		t.Fatalf("failed to read secret: %v", err)
	}

	if data == nil {
		t.Fatal("expected data, got nil")
	}

	if data["username"] != "testuser" {
		t.Errorf("expected username=testuser, got %v", data["username"])
	}

	if data["password"] != "testpass123" {
		t.Errorf("expected password=testpass123, got %v", data["password"])
	}

	// Clean up
	err = kv.Delete(ctx, testPath)
	if err != nil {
		t.Logf("warning: failed to delete test secret: %v", err)
	}
}

func TestIntegration_KVReadNonExistent(t *testing.T) {
	client := skipIfNoVault(t)
	ctx := context.Background()

	kv, err := NewKVClient(client, "kv", KVVersion2)
	if err != nil {
		t.Fatalf("failed to create KV client: %v", err)
	}

	data, err := kv.Read(ctx, "vsg-test/nonexistent-path-12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data != nil {
		t.Errorf("expected nil for non-existent path, got %v", data)
	}
}

func TestIntegration_KVPatch(t *testing.T) {
	client := skipIfNoVault(t)
	ctx := context.Background()

	kv, err := NewKVClient(client, "kv", KVVersion2)
	if err != nil {
		t.Fatalf("failed to create KV client: %v", err)
	}

	testPath := "vsg-test/patch-test"

	// Write initial data
	err = kv.Write(ctx, testPath, map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	})
	if err != nil {
		t.Fatalf("failed to write initial secret: %v", err)
	}

	// Patch with new key
	err = kv.Patch(ctx, testPath, map[string]interface{}{
		"key3": "value3",
	})
	if err != nil {
		t.Fatalf("failed to patch secret: %v", err)
	}

	// Read and verify
	data, err := kv.Read(ctx, testPath)
	if err != nil {
		t.Fatalf("failed to read secret: %v", err)
	}

	if data["key1"] != "value1" {
		t.Errorf("key1 should still be value1, got %v", data["key1"])
	}
	if data["key3"] != "value3" {
		t.Errorf("key3 should be value3, got %v", data["key3"])
	}

	// Clean up
	kv.Delete(ctx, testPath)
}

func TestIntegration_KVVersionDetection(t *testing.T) {
	client := skipIfNoVault(t)

	// Auto-detect version for kv mount
	kv, err := NewKVClient(client, "kv", KVVersionAuto)
	if err != nil {
		t.Fatalf("failed to create KV client: %v", err)
	}

	// Should detect v2 for standard kv mount
	if kv.Version() != KVVersion2 {
		t.Logf("detected KV version: %d (expected 2 for standard kv mount)", kv.Version())
	}
}

func TestIntegration_HealthCheck(t *testing.T) {
	client := skipIfNoVault(t)
	ctx := context.Background()

	err := client.CheckHealth(ctx)
	if err != nil {
		t.Errorf("health check failed: %v", err)
	}
}
