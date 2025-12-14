package vault

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/vault/api"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
)

// Client wraps the Vault API client with convenience methods.
type Client struct {
	client    *api.Client
	namespace string
}

// NewClient creates a new Vault client from the given configuration.
func NewClient(cfg config.VaultConfig) (*Client, error) {
	// Create Vault API config
	vaultCfg := api.DefaultConfig()

	// Set address from config or environment
	if cfg.Address != "" {
		vaultCfg.Address = cfg.Address
	}
	// api.DefaultConfig() already reads VAULT_ADDR

	// Create the client
	client, err := api.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("creating vault client: %w", err)
	}

	// Set namespace if specified
	if cfg.Namespace != "" {
		client.SetNamespace(cfg.Namespace)
	}

	// Authenticate
	if err := authenticate(client, cfg.Auth); err != nil {
		return nil, fmt.Errorf("authenticating to vault: %w", err)
	}

	return &Client{
		client:    client,
		namespace: cfg.Namespace,
	}, nil
}

// authenticate sets up authentication based on the config.
func authenticate(client *api.Client, auth config.AuthConfig) error {
	switch auth.Method {
	case "token", "":
		return authenticateToken(client, auth)
	case "kubernetes":
		return authenticateKubernetes(client, auth)
	case "approle":
		return authenticateAppRole(client, auth)
	default:
		return fmt.Errorf("unsupported auth method: %s", auth.Method)
	}
}

// authenticateToken sets up token authentication.
func authenticateToken(client *api.Client, auth config.AuthConfig) error {
	token := auth.Token
	if token == "" {
		token = os.Getenv("VAULT_TOKEN")
	}
	if token == "" {
		return fmt.Errorf("no token provided: set VAULT_TOKEN or specify in config")
	}

	client.SetToken(token)
	return nil
}

// authenticateKubernetes performs Kubernetes service account authentication.
func authenticateKubernetes(client *api.Client, auth config.AuthConfig) error {
	if auth.Role == "" {
		return fmt.Errorf("kubernetes auth requires role")
	}

	// Read the service account token
	jwt, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return fmt.Errorf("reading service account token: %w", err)
	}

	mountPath := auth.MountPath
	if mountPath == "" {
		mountPath = "kubernetes"
	}

	// Login
	path := fmt.Sprintf("auth/%s/login", mountPath)
	secret, err := client.Logical().Write(path, map[string]interface{}{
		"role": auth.Role,
		"jwt":  string(jwt),
	})
	if err != nil {
		return fmt.Errorf("kubernetes auth login: %w", err)
	}

	if secret == nil || secret.Auth == nil {
		return fmt.Errorf("kubernetes auth: no auth info returned")
	}

	client.SetToken(secret.Auth.ClientToken)
	return nil
}

// authenticateAppRole performs AppRole authentication.
func authenticateAppRole(client *api.Client, auth config.AuthConfig) error {
	roleID := auth.RoleID
	if roleID == "" {
		roleID = os.Getenv("VAULT_ROLE_ID")
	}
	if roleID == "" {
		return fmt.Errorf("approle auth requires role_id")
	}

	secretID := auth.SecretID
	if secretID == "" {
		secretID = os.Getenv("VAULT_SECRET_ID")
	}
	if secretID == "" {
		return fmt.Errorf("approle auth requires secret_id")
	}

	mountPath := auth.MountPath
	if mountPath == "" {
		mountPath = "approle"
	}

	// Login
	path := fmt.Sprintf("auth/%s/login", mountPath)
	secret, err := client.Logical().Write(path, map[string]interface{}{
		"role_id":   roleID,
		"secret_id": secretID,
	})
	if err != nil {
		return fmt.Errorf("approle auth login: %w", err)
	}

	if secret == nil || secret.Auth == nil {
		return fmt.Errorf("approle auth: no auth info returned")
	}

	client.SetToken(secret.Auth.ClientToken)
	return nil
}

// Logical returns the underlying logical client for direct API access.
func (c *Client) Logical() *api.Logical {
	return c.client.Logical()
}

// Token returns the current client token.
func (c *Client) Token() string {
	return c.client.Token()
}

// Address returns the Vault server address.
func (c *Client) Address() string {
	return c.client.Address()
}

// CheckHealth verifies the client can connect to Vault.
func (c *Client) CheckHealth(ctx context.Context) error {
	// Use sys/health which doesn't require auth
	resp, err := c.client.Sys().Health()
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	if !resp.Initialized {
		return fmt.Errorf("vault is not initialized")
	}

	if resp.Sealed {
		return fmt.Errorf("vault is sealed")
	}

	return nil
}

// NewClientFromEnv creates a new Vault client using environment variables.
// Uses VAULT_ADDR for address and VAULT_TOKEN for authentication.
func NewClientFromEnv(addr, namespace string) (*Client, error) {
	// Create Vault API config
	vaultCfg := api.DefaultConfig()
	if addr != "" {
		vaultCfg.Address = addr
	}

	// Create the client
	client, err := api.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("creating vault client: %w", err)
	}

	// Set namespace if specified
	if namespace != "" {
		client.SetNamespace(namespace)
	}

	// Get token from environment
	token := os.Getenv("VAULT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("VAULT_TOKEN environment variable is required")
	}
	client.SetToken(token)

	return &Client{
		client:    client,
		namespace: namespace,
	}, nil
}
