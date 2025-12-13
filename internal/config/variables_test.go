package config

import (
	"testing"
)

func TestSubstitute(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		expected string
	}{
		{
			name:     "single variable",
			input:    "kv/{env}",
			vars:     map[string]string{"env": "dev"},
			expected: "kv/dev",
		},
		{
			name:     "multiple variables",
			input:    "kv/{env}/{region}/data",
			vars:     map[string]string{"env": "prod", "region": "us-east-1"},
			expected: "kv/prod/us-east-1/data",
		},
		{
			name:     "repeated variable",
			input:    "{env}-{env}",
			vars:     map[string]string{"env": "test"},
			expected: "test-test",
		},
		{
			name:     "no variables",
			input:    "static/path",
			vars:     map[string]string{"env": "dev"},
			expected: "static/path",
		},
		{
			name:     "empty string",
			input:    "",
			vars:     map[string]string{"env": "dev"},
			expected: "",
		},
		{
			name:     "undefined variable unchanged",
			input:    "{undefined}",
			vars:     map[string]string{"env": "dev"},
			expected: "{undefined}",
		},
		{
			name:     "empty vars map",
			input:    "{env}",
			vars:     map[string]string{},
			expected: "{env}",
		},
		{
			name:     "nil vars map",
			input:    "{env}",
			vars:     nil,
			expected: "{env}",
		},
		{
			name:     "variable with underscore",
			input:    "{my_var}",
			vars:     map[string]string{"my_var": "value"},
			expected: "value",
		},
		{
			name:     "variable with numbers",
			input:    "{var123}",
			vars:     map[string]string{"var123": "value"},
			expected: "value",
		},
		{
			name:     "s3 uri with variable",
			input:    "s3://terraform-state/{env}/rds/terraform.tfstate#output.endpoint",
			vars:     map[string]string{"env": "staging"},
			expected: "s3://terraform-state/staging/rds/terraform.tfstate#output.endpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Substitute(tt.input, tt.vars)
			if result != tt.expected {
				t.Errorf("Substitute(%q, %v) = %q, want %q", tt.input, tt.vars, result, tt.expected)
			}
		})
	}
}

func TestFindUnresolved(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single unresolved",
			input:    "{undefined}",
			expected: []string{"{undefined}"},
		},
		{
			name:     "multiple unresolved",
			input:    "{one}/{two}",
			expected: []string{"{one}", "{two}"},
		},
		{
			name:     "no variables",
			input:    "static/path",
			expected: nil,
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findUnresolved(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("findUnresolved(%q) returned %d items, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("findUnresolved(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestSubstituteVariables_Config(t *testing.T) {
	cfg := &Config{
		Env: map[string]string{
			"env":    "production",
			"region": "eu-west-1",
		},
		Vault: VaultConfig{
			Address: "https://vault.{region}.example.com",
			Auth: AuthConfig{
				Role: "{env}-reader",
			},
		},
		Secrets: map[string]SecretBlock{
			"main": {
				Path: "kv/{env}",
				Data: map[string]string{
					"mysql/host":    "s3://bucket/{env}/state#output.host",
					"{env}/api_key": "generate",
				},
			},
		},
	}

	err := substituteVariables(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Vault.Address != "https://vault.eu-west-1.example.com" {
		t.Errorf("vault address not substituted: %s", cfg.Vault.Address)
	}

	if cfg.Vault.Auth.Role != "production-reader" {
		t.Errorf("auth role not substituted: %s", cfg.Vault.Auth.Role)
	}

	block := cfg.Secrets["main"]
	if block.Path != "kv/production" {
		t.Errorf("path not substituted: %s", block.Path)
	}

	if block.Data["mysql/host"] != "s3://bucket/production/state#output.host" {
		t.Errorf("data value not substituted: %s", block.Data["mysql/host"])
	}

	// Check key substitution
	if _, ok := block.Data["production/api_key"]; !ok {
		t.Error("data key not substituted")
	}
}
