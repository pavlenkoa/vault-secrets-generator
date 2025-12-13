package vault

import (
	"os"
	"testing"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
)

func TestNewClient_TokenAuth(t *testing.T) {
	// Skip if no vault server available
	if os.Getenv("VAULT_ADDR") == "" {
		t.Skip("VAULT_ADDR not set, skipping integration test")
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

	if client.Token() == "" {
		t.Error("expected token to be set")
	}
}

func TestNewClient_MissingToken(t *testing.T) {
	// Clear any existing token
	originalToken := os.Getenv("VAULT_TOKEN")
	os.Unsetenv("VAULT_TOKEN")
	defer func() {
		if originalToken != "" {
			os.Setenv("VAULT_TOKEN", originalToken)
		}
	}()

	cfg := config.VaultConfig{
		Address: "http://localhost:8200",
		Auth: config.AuthConfig{
			Method: "token",
		},
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("expected error for missing token")
	}
}

func TestNewClient_InvalidAuthMethod(t *testing.T) {
	cfg := config.VaultConfig{
		Address: "http://localhost:8200",
		Auth: config.AuthConfig{
			Method: "invalid",
		},
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("expected error for invalid auth method")
	}
}

func TestNewClient_AddressFromConfig(t *testing.T) {
	// Set a token to avoid auth error
	originalToken := os.Getenv("VAULT_TOKEN")
	os.Setenv("VAULT_TOKEN", "test-token")
	defer func() {
		if originalToken != "" {
			os.Setenv("VAULT_TOKEN", originalToken)
		} else {
			os.Unsetenv("VAULT_TOKEN")
		}
	}()

	cfg := config.VaultConfig{
		Address: "http://custom-vault:8200",
		Auth: config.AuthConfig{
			Method: "token",
		},
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if client.Address() != "http://custom-vault:8200" {
		t.Errorf("expected address http://custom-vault:8200, got %s", client.Address())
	}
}

func TestNewClient_WithNamespace(t *testing.T) {
	originalToken := os.Getenv("VAULT_TOKEN")
	os.Setenv("VAULT_TOKEN", "test-token")
	defer func() {
		if originalToken != "" {
			os.Setenv("VAULT_TOKEN", originalToken)
		} else {
			os.Unsetenv("VAULT_TOKEN")
		}
	}()

	cfg := config.VaultConfig{
		Address:   "http://localhost:8200",
		Namespace: "admin",
		Auth: config.AuthConfig{
			Method: "token",
		},
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if client.namespace != "admin" {
		t.Errorf("expected namespace admin, got %s", client.namespace)
	}
}
