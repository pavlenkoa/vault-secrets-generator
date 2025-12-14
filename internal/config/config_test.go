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

secret "secret/dev" {
  api_key = generate()
  db_port = "5432"
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

	block, ok := cfg.Secrets["secret/dev"]
	if !ok {
		t.Fatal("missing secret block for path 'secret/dev'")
	}
	if block.Path != "secret/dev" {
		t.Errorf("expected path=secret/dev, got %s", block.Path)
	}

	// Check value types
	if block.Data["api_key"].Type != ValueTypeGenerate {
		t.Errorf("expected api_key to be generate type, got %s", block.Data["api_key"].Type)
	}
	if block.Data["db_port"].Type != ValueTypeStatic {
		t.Errorf("expected db_port to be static type, got %s", block.Data["db_port"].Type)
	}
	if block.Data["db_port"].Static != "5432" {
		t.Errorf("expected db_port=5432, got %s", block.Data["db_port"].Static)
	}
}

func TestParseHCL_GenerateWithCustomPolicy(t *testing.T) {
	hcl := `
secret "secret/test" {
  jwt_secret = generate({length = 64, symbols = 0})
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["secret/test"].Data["jwt_secret"]
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
secret "secret/test" {
  db_host = json("s3://bucket/terraform.tfstate", ".outputs.db_host.value")
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["secret/test"].Data["db_host"]
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
secret "secret/test" {
  config_value = yaml("file:///path/to/config.yaml", ".database.host")
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["secret/test"].Data["config_value"]
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
secret "secret/test" {
  ssh_key = raw("s3://bucket/key.pem")
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["secret/test"].Data["ssh_key"]
	if val.Type != ValueTypeRaw {
		t.Errorf("expected raw type, got %s", val.Type)
	}
	if val.URL != "s3://bucket/key.pem" {
		t.Errorf("unexpected url: %s", val.URL)
	}
}

func TestParseHCL_VaultFunction(t *testing.T) {
	hcl := `
secret "secret/test" {
  shared_key = vault("secret/shared", "api_key")
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["secret/test"].Data["shared_key"]
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
secret "secret/test" {
  hash = command("caddy hash-password --plaintext mypassword")
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["secret/test"].Data["hash"]
	if val.Type != ValueTypeCommand {
		t.Errorf("expected command type, got %s", val.Type)
	}
	if val.Command != `caddy hash-password --plaintext mypassword` {
		t.Errorf("unexpected command: %s", val.Command)
	}
}

func TestParseHCL_EnvFunction(t *testing.T) {
	// Note: HCL template interpolation uses ${} syntax
	hcl := `
secret "secret/prod/app" {
  region = env("REGION")
}
`

	vars := Variables{
		"REGION": "us-east-1",
	}

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	block, ok := cfg.Secrets["secret/prod/app"]
	if !ok {
		t.Fatalf("missing secret block for path 'secret/prod/app', got keys: %v", keys(cfg.Secrets))
	}
	if block.Path != "secret/prod/app" {
		t.Errorf("expected path=secret/prod/app, got %s", block.Path)
	}

	val := block.Data["region"]
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
secret "secret/test" {
  password = generate({strategy = "update"})
  db_host  = json("s3://bucket/state", ".db", {strategy = "create"})
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pass := cfg.Secrets["secret/test"].Data["password"]
	if pass.Strategy != StrategyUpdate {
		t.Errorf("expected password strategy=update, got %s", pass.Strategy)
	}

	dbHost := cfg.Secrets["secret/test"].Data["db_host"]
	if dbHost.Strategy != StrategyCreate {
		t.Errorf("expected db_host strategy=create, got %s", dbHost.Strategy)
	}
}

func TestParseHCL_Prune(t *testing.T) {
	hcl := `
secret "secret/test" {
  prune = true
  key   = "value"
}
`

	cfg, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Secrets["secret/test"].Prune {
		t.Error("expected prune=true")
	}
}

func TestParseHCL_DefaultValues(t *testing.T) {
	hcl := `
secret "secret/test" {
  key = generate()
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
}

func TestParseHCL_DefaultStrategies(t *testing.T) {
	hcl := `
defaults {
  strategy {
    generate = "update"
    json     = "create"
  }
}

secret "secret/test" {
  key = generate()
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

secret "secret/test" {
  key = generate()
}
`

	_, err := ParseHCL([]byte(hcl), "test.hcl", nil)
	if err == nil {
		t.Fatal("expected error for length too small")
	}
}

func TestParseHCL_MultipleSecrets(t *testing.T) {
	hcl := `
secret "secret/app1" {
  key = "value1"
}

secret "secret/app2" {
  key = "value2"
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
