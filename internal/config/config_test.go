package config

import (
	"testing"
)

func TestParse_ValidConfig(t *testing.T) {
	yaml := `
vault:
  address: https://vault.example.com
  auth:
    method: token

defaults:
  generate:
    length: 32
    digits: 5
    symbols: 5

secrets:
  main:
    path: secret/dev
    data:
      api_key: generate
      db_port: "5432"
`

	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Vault.Address != "https://vault.example.com" {
		t.Errorf("unexpected vault address: %s", cfg.Vault.Address)
	}

	block, ok := cfg.Secrets["main"]
	if !ok {
		t.Fatal("missing 'main' secret block")
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

func TestParse_GenerateWithCustomPolicy(t *testing.T) {
	yaml := `
secrets:
  test:
    path: secret/test
    data:
      jwt_secret:
        generate:
          length: 64
          symbols: 0
`

	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["test"].Data["jwt_secret"]
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

func TestParse_SourceWithJSON(t *testing.T) {
	yaml := `
secrets:
  test:
    path: secret/test
    data:
      db_host:
        source: s3://bucket/terraform.tfstate
        json: .outputs.db_host.value
`

	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["test"].Data["db_host"]
	if val.Type != ValueTypeSource {
		t.Errorf("expected source type, got %s", val.Type)
	}
	if val.Source != "s3://bucket/terraform.tfstate" {
		t.Errorf("unexpected source: %s", val.Source)
	}
	if val.JSONPath != ".outputs.db_host.value" {
		t.Errorf("unexpected json path: %s", val.JSONPath)
	}
}

func TestParse_SourceWithYAML(t *testing.T) {
	yaml := `
secrets:
  test:
    path: secret/test
    data:
      config_value:
        source: file:///path/to/config.yaml
        yaml: .database.host
`

	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["test"].Data["config_value"]
	if val.Type != ValueTypeSource {
		t.Errorf("expected source type, got %s", val.Type)
	}
	if val.Source != "file:///path/to/config.yaml" {
		t.Errorf("unexpected source: %s", val.Source)
	}
	if val.YAMLPath != ".database.host" {
		t.Errorf("unexpected yaml path: %s", val.YAMLPath)
	}
}

func TestParse_Command(t *testing.T) {
	yaml := `
secrets:
  test:
    path: secret/test
    data:
      hash:
        command: caddy hash-password --plaintext "mypassword"
`

	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := cfg.Secrets["test"].Data["hash"]
	if val.Type != ValueTypeCommand {
		t.Errorf("expected command type, got %s", val.Type)
	}
	if val.Command != `caddy hash-password --plaintext "mypassword"` {
		t.Errorf("unexpected command: %s", val.Command)
	}
}

func TestParse_SourceMissingPath(t *testing.T) {
	yaml := `
secrets:
  test:
    path: secret/test
    data:
      db_host:
        source: s3://bucket/terraform.tfstate
`

	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for source without json/yaml path")
	}
}

func TestParse_DefaultValues(t *testing.T) {
	yaml := `
secrets:
  test:
    path: secret/test
    data:
      key: generate
`

	cfg, err := Parse([]byte(yaml))
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
	if cfg.Defaults.Generate.AllowRepeat == nil || !*cfg.Defaults.Generate.AllowRepeat {
		t.Error("expected default allowRepeat=true")
	}
}

func TestParse_NoSecrets(t *testing.T) {
	yaml := `
vault:
  address: https://vault.example.com
`

	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing secrets")
	}
}

func TestParse_EmptyPath(t *testing.T) {
	yaml := `
secrets:
  test:
    data:
      key: "value"
`

	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestParse_EmptyData(t *testing.T) {
	yaml := `
secrets:
  test:
    path: secret/test
`

	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestParse_InvalidVersion(t *testing.T) {
	yaml := `
secrets:
  test:
    path: secret/test
    version: 3
    data:
      key: "value"
`

	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid version")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	yaml := `
not: valid: yaml: here
`

	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}

func TestParse_LengthTooSmall(t *testing.T) {
	yaml := `
defaults:
  generate:
    length: 5
    digits: 5
    symbols: 5

secrets:
  test:
    path: secret/test
    data:
      key: generate
`

	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for length too small")
	}
}

func TestParse_InvalidValueFormat(t *testing.T) {
	yaml := `
secrets:
  test:
    path: secret/test
    data:
      key:
        unknown_key: "value"
`

	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid value format")
	}
}
