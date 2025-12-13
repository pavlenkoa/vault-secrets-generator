package config

import (
	"testing"
)

func TestParse_ValidConfig(t *testing.T) {
	yaml := `
env:
  env: dev
  region: eu-west-1

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
    path: kv/{env}
    data:
      app/key: generate
      app/static: "hello"
`

	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Env["env"] != "dev" {
		t.Errorf("expected env=dev, got %s", cfg.Env["env"])
	}

	if cfg.Vault.Address != "https://vault.example.com" {
		t.Errorf("unexpected vault address: %s", cfg.Vault.Address)
	}

	// Check variable substitution happened
	block, ok := cfg.Secrets["main"]
	if !ok {
		t.Fatal("missing 'main' secret block")
	}
	if block.Path != "kv/dev" {
		t.Errorf("expected path=kv/dev, got %s", block.Path)
	}
}

func TestParse_DefaultValues(t *testing.T) {
	yaml := `
secrets:
  test:
    path: kv/test
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
      key: value
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
    path: kv/test
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
    path: kv/test
    version: 3
    data:
      key: value
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

func TestParse_UnresolvedVariable(t *testing.T) {
	yaml := `
env:
  env: dev

secrets:
  test:
    path: kv/{env}/{undefined}
    data:
      key: value
`

	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for unresolved variable")
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
    path: kv/test
    data:
      key: generate
`

	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for length too small")
	}
}
