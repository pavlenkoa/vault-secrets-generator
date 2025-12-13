package tfstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetOutput_SimpleState(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "simple.tfstate"))
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	state, err := Parse(data)
	if err != nil {
		t.Fatalf("failed to parse state: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		expected string
		wantErr  bool
	}{
		{
			name:     "string output",
			path:     "output.endpoint",
			expected: "mydb.123456.us-east-1.rds.amazonaws.com",
		},
		{
			name:     "number output",
			path:     "output.port",
			expected: "5432",
		},
		{
			name:     "bool output",
			path:     "output.enabled",
			expected: "true",
		},
		{
			name:     "list output",
			path:     "output.tags",
			expected: "web,api,db",
		},
		{
			name:     "map output",
			path:     "output.config",
			expected: `{"max_connections":100,"timeout":"30s"}`,
		},
		{
			name:    "missing output",
			path:    "output.nonexistent",
			wantErr: true,
		},
		{
			name:    "invalid path prefix",
			path:    "invalid.endpoint",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetOutput(state, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("GetOutput(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGetOutput_WithModules(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "with_modules.tfstate"))
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	state, err := Parse(data)
	if err != nil {
		t.Fatalf("failed to parse state: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		expected string
		wantErr  bool
	}{
		{
			name:     "root output from values.outputs",
			path:     "output.vpc_id",
			expected: "vpc-12345678",
		},
		{
			name:     "root output from root_module.outputs",
			path:     "output.root_output",
			expected: "root-value",
		},
		{
			name:     "module output - rds endpoint",
			path:     "output.module.rds.endpoint",
			expected: "rds.internal.example.com",
		},
		{
			name:     "module output - rds password",
			path:     "output.module.rds.password",
			expected: "secret123",
		},
		{
			name:     "module output - redis endpoint",
			path:     "output.module.redis.endpoint",
			expected: "redis.internal.example.com",
		},
		{
			name:     "module output - redis port",
			path:     "output.module.redis.port",
			expected: "6379",
		},
		{
			name:    "missing module",
			path:    "output.module.nonexistent.value",
			wantErr: true,
		},
		{
			name:    "missing module output",
			path:    "output.module.rds.nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetOutput(state, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("GetOutput(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGetOutput_NestedModule(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "with_modules.tfstate"))
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	state, err := Parse(data)
	if err != nil {
		t.Fatalf("failed to parse state: %v", err)
	}

	// Test nested module: module.redis.module.cluster.primary_endpoint
	result, err := GetOutput(state, "output.module.redis.module.cluster.primary_endpoint")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "redis-primary.internal.example.com"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestExtractOutput(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "simple.tfstate"))
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	result, err := ExtractOutput(data, "output.endpoint")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "mydb.123456.us-east-1.rds.amazonaws.com"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestParse_InvalidJSON(t *testing.T) {
	_, err := Parse([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"string", "hello", "hello"},
		{"integer", float64(42), "42"},
		{"float", float64(3.14), "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"nil", nil, ""},
		{"list", []interface{}{"a", "b", "c"}, "a,b,c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatValue(tt.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("formatValue(%v) = %q, want %q", tt.value, result, tt.expected)
			}
		})
	}
}
