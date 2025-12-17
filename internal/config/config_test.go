package config

import (
	"testing"
)

func TestParseHCL_ValidConfig(t *testing.T) {
	hcl := `
vault {
  address = "https://vault.example.com"
  auth {
    method = "token"
  }
}

defaults {
  generate {
    length  = 32
    digits  = 5
    symbols = 5
  }
}

secret "dev-app" {
  path = "dev"

  content {
    api_key = generate()
    db_port = "5432"
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Vault.Address != "https://vault.example.com" {
		t.Errorf("unexpected vault address: %s", cfg.Vault.Address)
	}

	if len(cfg.Secrets) != 1 {
		t.Fatalf("expected 1 secret block, got %d", len(cfg.Secrets))
	}

	block, ok := cfg.Secrets["dev-app"]
	if !ok {
		t.Fatal("missing secret block for name 'dev-app'")
	}
	if block.Name != "dev-app" {
		t.Errorf("expected name=dev-app, got %s", block.Name)
	}
	if block.Mount != "secret" {
		t.Errorf("expected mount=secret (default), got %s", block.Mount)
	}
	if block.Path != "dev" {
		t.Errorf("expected path=dev, got %s", block.Path)
	}

	// Check value types
	if block.Content["api_key"].Type != ValueTypeGenerate {
		t.Errorf("expected api_key to be generate type, got %s", block.Content["api_key"].Type)
	}
	if block.Content["db_port"].Type != ValueTypeStatic {
		t.Errorf("expected db_port to be static type, got %s", block.Content["db_port"].Type)
	}
	if block.Content["db_port"].Static != "5432" {
		t.Errorf("expected db_port=5432, got %s", block.Content["db_port"].Static)
	}
}

func TestParseHCL_GenerateWithCustomPolicy(t *testing.T) {
	hcl := `
secret "test-secret" {
  path = "test"

  content {
    jwt_secret = generate({length = 64, symbols = 0})
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["test-secret"].Content["jwt_secret"]
	if val.Type != ValueTypeGenerate {
		t.Errorf("expected generate type, got %s", val.Type)
	}
	if val.Generate == nil {
		t.Fatal("expected generate policy to be set")
	}
	if val.Generate.Length != 64 {
		t.Errorf("expected length=64, got %d", val.Generate.Length)
	}
	if val.Generate.Symbols != 0 {
		t.Errorf("expected symbols=0, got %d", val.Generate.Symbols)
	}
}

func TestParseHCL_JSONFunction(t *testing.T) {
	hcl := `
secret "test-secret" {
  path = "test"

  content {
    db_host = json("s3://bucket/terraform.tfstate", ".outputs.db_host.value")
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["test-secret"].Content["db_host"]
	if val.Type != ValueTypeJSON {
		t.Errorf("expected json type, got %s", val.Type)
	}
	if val.URL != "s3://bucket/terraform.tfstate" {
		t.Errorf("unexpected url: %s", val.URL)
	}
	if val.Query != ".outputs.db_host.value" {
		t.Errorf("unexpected query: %s", val.Query)
	}
}

func TestParseHCL_YAMLFunction(t *testing.T) {
	hcl := `
secret "test-secret" {
  path = "test"

  content {
    config_value = yaml("file:///path/to/config.yaml", ".database.host")
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["test-secret"].Content["config_value"]
	if val.Type != ValueTypeYAML {
		t.Errorf("expected yaml type, got %s", val.Type)
	}
	if val.URL != "file:///path/to/config.yaml" {
		t.Errorf("unexpected url: %s", val.URL)
	}
	if val.Query != ".database.host" {
		t.Errorf("unexpected query: %s", val.Query)
	}
}

func TestParseHCL_RawFunction(t *testing.T) {
	hcl := `
secret "test-secret" {
  path = "test"

  content {
    ssh_key = raw("s3://bucket/key.pem")
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["test-secret"].Content["ssh_key"]
	if val.Type != ValueTypeRaw {
		t.Errorf("expected raw type, got %s", val.Type)
	}
	if val.URL != "s3://bucket/key.pem" {
		t.Errorf("unexpected url: %s", val.URL)
	}
}

func TestParseHCL_VaultFunction(t *testing.T) {
	hcl := `
secret "test-secret" {
  path = "test"

  content {
    shared_key = vault("secret/shared", "api_key")
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["test-secret"].Content["shared_key"]
	if val.Type != ValueTypeVault {
		t.Errorf("expected vault type, got %s", val.Type)
	}
	if val.VaultPath != "secret/shared" {
		t.Errorf("unexpected vault path: %s", val.VaultPath)
	}
	if val.VaultKey != "api_key" {
		t.Errorf("unexpected vault key: %s", val.VaultKey)
	}
}

func TestParseHCL_Command(t *testing.T) {
	hcl := `
secret "test-secret" {
  path = "test"

  content {
    hash = command("caddy hash-password --plaintext mypassword")
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["test-secret"].Content["hash"]
	if val.Type != ValueTypeCommand {
		t.Errorf("expected command type, got %s", val.Type)
	}
	if val.Command != `caddy hash-password --plaintext mypassword` {
		t.Errorf("unexpected command: %s", val.Command)
	}
}

func TestParseHCL_EnvFunction(t *testing.T) {
	hcl := `
secret "prod-app" {
  path = "prod/app"

  content {
    region = env("REGION")
  }
}
`

	vars := Variables{
		"REGION": "us-east-1",
	}

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	block, ok := cfg.Secrets["prod-app"]
	if !ok {
		t.Fatalf("missing secret block for name 'prod-app', got keys: %v", keys(cfg.Secrets))
	}
	if block.Path != "prod/app" {
		t.Errorf("expected path=prod/app, got %s", block.Path)
	}

	val := block.Content["region"]
	if val.Type != ValueTypeStatic {
		t.Errorf("expected static type for env(), got %s", val.Type)
	}
	if val.Static != "us-east-1" {
		t.Errorf("expected region=us-east-1, got %s", val.Static)
	}
}

func keys(m map[string]SecretBlock) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

func TestParseHCL_StrategyOverride(t *testing.T) {
	hcl := `
secret "test-secret" {
  path = "test"

  content {
    password = generate({strategy = "update"})
    db_host  = json("s3://bucket/state", ".db", {strategy = "create"})
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pass := cfg.Secrets["test-secret"].Content["password"]
	if pass.Strategy != StrategyUpdate {
		t.Errorf("expected password strategy=update, got %s", pass.Strategy)
	}

	dbHost := cfg.Secrets["test-secret"].Content["db_host"]
	if dbHost.Strategy != StrategyCreate {
		t.Errorf("expected db_host strategy=create, got %s", dbHost.Strategy)
	}
}

func TestParseHCL_Prune(t *testing.T) {
	hcl := `
secret "test-secret" {
  path  = "test"
  prune = true

  content {
    key = "value"
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Secrets["test-secret"].Prune {
		t.Error("expected prune=true")
	}
}

func TestParseHCL_DefaultValues(t *testing.T) {
	hcl := `
secret "test-secret" {
  path = "test"

  content {
    key = generate()
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Defaults.Generate.Length != 32 {
		t.Errorf("expected default length=32, got %d", cfg.Defaults.Generate.Length)
	}
	if cfg.Defaults.Generate.Digits != 5 {
		t.Errorf("expected default digits=5, got %d", cfg.Defaults.Generate.Digits)
	}
	if cfg.Defaults.Generate.Symbols != 5 {
		t.Errorf("expected default symbols=5, got %d", cfg.Defaults.Generate.Symbols)
	}
	if cfg.Defaults.Generate.SymbolCharacters != "-_$@" {
		t.Errorf("expected default symbolCharacters=-_$@, got %s", cfg.Defaults.Generate.SymbolCharacters)
	}
	if cfg.Defaults.Mount != "secret" {
		t.Errorf("expected default mount=secret, got %s", cfg.Defaults.Mount)
	}
}

func TestParseHCL_DefaultStrategies(t *testing.T) {
	hcl := `
defaults {
  strategy {
    generate = "update"
    json     = "create"
  }
}

secret "test-secret" {
  path = "test"

  content {
    key = generate()
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Defaults.Strategy.Generate != StrategyUpdate {
		t.Errorf("expected default generate strategy=update, got %s", cfg.Defaults.Strategy.Generate)
	}
	if cfg.Defaults.Strategy.JSON != StrategyCreate {
		t.Errorf("expected default json strategy=create, got %s", cfg.Defaults.Strategy.JSON)
	}
}

func TestParseHCL_NoSecrets(t *testing.T) {
	hcl := `
vault {
  address = "https://vault.example.com"
}
`

	_, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err == nil {
		t.Fatal("expected error for missing secrets")
	}
}

func TestParseHCL_InvalidHCL(t *testing.T) {
	hcl := `
not valid hcl here {{{
`

	_, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err == nil {
		t.Fatal("expected error for invalid HCL")
	}
}

func TestParseHCL_LengthTooSmall(t *testing.T) {
	hcl := `
defaults {
  generate {
    length  = 5
    digits  = 5
    symbols = 5
  }
}

secret "test-secret" {
  path = "test"

  content {
    key = generate()
  }
}
`

	_, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err == nil {
		t.Fatal("expected error for length too small")
	}
}

func TestParseHCL_MultipleSecrets(t *testing.T) {
	hcl := `
secret "app1" {
  path = "app1"

  content {
    key = "value1"
  }
}

secret "app2" {
  path = "app2"

  content {
    key = "value2"
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Secrets) != 2 {
		t.Errorf("expected 2 secret blocks, got %d", len(cfg.Secrets))
	}
}

func TestParseHCL_MountAndPath(t *testing.T) {
	hcl := `
secret "prod-db" {
  mount = "kv"
  path  = "prod/database"

  content {
    password = generate()
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	block := cfg.Secrets["prod-db"]
	if block.Mount != "kv" {
		t.Errorf("expected mount=kv, got %s", block.Mount)
	}
	if block.Path != "prod/database" {
		t.Errorf("expected path=prod/database, got %s", block.Path)
	}
	if block.FullPath() != "kv/prod/database" {
		t.Errorf("expected fullPath=kv/prod/database, got %s", block.FullPath())
	}
}

func TestParseHCL_DefaultMount(t *testing.T) {
	hcl := `
defaults {
  mount = "custom-kv"
}

secret "app" {
  path = "myapp"

  content {
    key = "value"
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Defaults.Mount != "custom-kv" {
		t.Errorf("expected defaults.mount=custom-kv, got %s", cfg.Defaults.Mount)
	}

	block := cfg.Secrets["app"]
	if block.Mount != "custom-kv" {
		t.Errorf("expected block mount=custom-kv (from defaults), got %s", block.Mount)
	}
}

func TestParseHCL_DuplicateName(t *testing.T) {
	hcl := `
secret "app" {
  path = "app1"

  content {
    key = "value1"
  }
}

secret "app" {
  path = "app2"

  content {
    key = "value2"
  }
}
`

	_, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err == nil {
		t.Fatal("expected error for duplicate secret name")
	}
}

func TestParseHCL_DuplicatePath(t *testing.T) {
	hcl := `
secret "app1" {
  path = "myapp"

  content {
    key = "value1"
  }
}

secret "app2" {
  path = "myapp"

  content {
    key = "value2"
  }
}
`

	_, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err == nil {
		t.Fatal("expected error for duplicate path")
	}
}

func TestParseHCL_MissingContent(t *testing.T) {
	hcl := `
secret "app" {
  path = "myapp"
}
`

	_, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err == nil {
		t.Fatal("expected error for missing content block")
	}
}

func TestParseHCL_EmptyContent(t *testing.T) {
	hcl := `
secret "app" {
  path = "myapp"

  content {
  }
}
`

	_, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err == nil {
		t.Fatal("expected error for empty content block")
	}
}

func TestParseHCL_PathInterpolation(t *testing.T) {
	hcl := `
secret "app" {
  path = "${env("ENV")}/app"

  content {
    key = "value"
  }
}
`

	vars := Variables{
		"ENV": "prod",
	}

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	block := cfg.Secrets["app"]
	if block.Path != "prod/app" {
		t.Errorf("expected path=prod/app (interpolated), got %s", block.Path)
	}
}

func TestParseHCL_Version(t *testing.T) {
	hcl := `
secret "app" {
  path    = "myapp"
  version = 2

  content {
    key = "value"
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	block := cfg.Secrets["app"]
	if block.Version != 2 {
		t.Errorf("expected version=2, got %d", block.Version)
	}
}

func TestParseHCL_DefaultVersion(t *testing.T) {
	hcl := `
defaults {
  version = 1
}

secret "app" {
  path = "myapp"

  content {
    key = "value"
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Defaults.Version != 1 {
		t.Errorf("expected defaults.version=1, got %d", cfg.Defaults.Version)
	}

	block := cfg.Secrets["app"]
	if block.Version != 1 {
		t.Errorf("expected block version=1 (from defaults), got %d", block.Version)
	}
}

func TestParseHCL_Enabled(t *testing.T) {
	hcl := `
secret "enabled-secret" {
  path    = "enabled"
  enabled = true

  content {
    key = "value"
  }
}

secret "disabled-secret" {
  path    = "disabled"
  enabled = false

  content {
    key = "value"
  }
}

secret "default-secret" {
  path = "default"

  content {
    key = "value"
  }
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// enabled = true
	enabledBlock := cfg.Secrets["enabled-secret"]
	if enabledBlock.Enabled == nil {
		t.Error("expected enabled to be set for enabled-secret")
	} else if !*enabledBlock.Enabled {
		t.Error("expected enabled=true for enabled-secret")
	}
	if !enabledBlock.IsEnabled() {
		t.Error("IsEnabled() should return true for enabled-secret")
	}

	// enabled = false
	disabledBlock := cfg.Secrets["disabled-secret"]
	if disabledBlock.Enabled == nil {
		t.Error("expected enabled to be set for disabled-secret")
	} else if *disabledBlock.Enabled {
		t.Error("expected enabled=false for disabled-secret")
	}
	if disabledBlock.IsEnabled() {
		t.Error("IsEnabled() should return false for disabled-secret")
	}

	// enabled not set (default: true)
	defaultBlock := cfg.Secrets["default-secret"]
	if defaultBlock.Enabled != nil {
		t.Error("expected enabled to be nil (not set) for default-secret")
	}
	if !defaultBlock.IsEnabled() {
		t.Error("IsEnabled() should return true (default) for default-secret")
	}
}

func TestSecretBlock_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		enabled  *bool
		expected bool
	}{
		{
			name:     "nil (default)",
			enabled:  nil,
			expected: true,
		},
		{
			name:     "true",
			enabled:  boolPtr(true),
			expected: true,
		},
		{
			name:     "false",
			enabled:  boolPtr(false),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := SecretBlock{Enabled: tt.enabled}
			if got := block.IsEnabled(); got != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
