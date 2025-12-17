package engine

import (
	"testing"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
)

func TestParsePath(t *testing.T) {
	tests := []struct {
		path        string
		wantMount   string
		wantSubpath string
	}{
		{"kv/myapp", "kv", "myapp"},
		{"kv/myapp/config", "kv", "myapp/config"},
		{"secret/data/app", "secret", "data/app"},
		{"/kv/myapp/", "kv", "myapp"},
		{"kv", "kv", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			mount, subpath := parsePath(tt.path)
			if mount != tt.wantMount {
				t.Errorf("mount = %q, want %q", mount, tt.wantMount)
			}
			if subpath != tt.wantSubpath {
				t.Errorf("subpath = %q, want %q", subpath, tt.wantSubpath)
			}
		})
	}
}

func TestBlockError_Error(t *testing.T) {
	tests := []struct {
		err      BlockError
		expected string
	}{
		{
			err:      BlockError{Block: "main", Err: errTest},
			expected: "main: test error",
		},
		{
			err:      BlockError{Block: "main", Key: "db/password", Err: errTest},
			expected: "main/db/password: test error",
		},
	}

	for _, tt := range tests {
		result := tt.err.Error()
		if result != tt.expected {
			t.Errorf("Error() = %q, want %q", result, tt.expected)
		}
	}
}

var errTest = testError{}

type testError struct{}

func (testError) Error() string { return "test error" }

func TestShouldProcessBlock(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name     string
		block    config.SecretBlock
		opts     Options
		expected bool
	}{
		// Default behavior (enabled=true, no filters)
		{
			name:     "enabled=true (default), no filters",
			block:    config.SecretBlock{Name: "test", Enabled: nil},
			opts:     Options{},
			expected: true,
		},
		{
			name:     "enabled=true explicit, no filters",
			block:    config.SecretBlock{Name: "test", Enabled: &trueVal},
			opts:     Options{},
			expected: true,
		},
		{
			name:     "enabled=false, no filters",
			block:    config.SecretBlock{Name: "test", Enabled: &falseVal},
			opts:     Options{},
			expected: false,
		},

		// Target filtering
		{
			name:     "enabled=true, --target this",
			block:    config.SecretBlock{Name: "test", Enabled: &trueVal},
			opts:     Options{Target: []string{"test"}},
			expected: true,
		},
		{
			name:     "enabled=true, --target other",
			block:    config.SecretBlock{Name: "test", Enabled: &trueVal},
			opts:     Options{Target: []string{"other"}},
			expected: false,
		},
		{
			name:     "enabled=false, --target this (override)",
			block:    config.SecretBlock{Name: "test", Enabled: &falseVal},
			opts:     Options{Target: []string{"test"}},
			expected: true,
		},
		{
			name:     "enabled=false, --target other",
			block:    config.SecretBlock{Name: "test", Enabled: &falseVal},
			opts:     Options{Target: []string{"other"}},
			expected: false,
		},

		// Exclude filtering
		{
			name:     "enabled=true, --exclude this",
			block:    config.SecretBlock{Name: "test", Enabled: &trueVal},
			opts:     Options{Exclude: []string{"test"}},
			expected: false,
		},
		{
			name:     "enabled=true, --exclude other",
			block:    config.SecretBlock{Name: "test", Enabled: &trueVal},
			opts:     Options{Exclude: []string{"other"}},
			expected: true,
		},
		{
			name:     "enabled=false, --exclude this",
			block:    config.SecretBlock{Name: "test", Enabled: &falseVal},
			opts:     Options{Exclude: []string{"test"}},
			expected: false,
		},

		// Combined target and exclude
		{
			name:     "enabled=true, --target this, --exclude this",
			block:    config.SecretBlock{Name: "test", Enabled: &trueVal},
			opts:     Options{Target: []string{"test"}, Exclude: []string{"test"}},
			expected: false, // exclude takes precedence
		},
		{
			name:     "enabled=true, --target this --target other, --exclude other",
			block:    config.SecretBlock{Name: "test", Enabled: &trueVal},
			opts:     Options{Target: []string{"test", "other"}, Exclude: []string{"other"}},
			expected: true,
		},

		// Multiple targets
		{
			name:     "enabled=true, multiple targets including this",
			block:    config.SecretBlock{Name: "test", Enabled: &trueVal},
			opts:     Options{Target: []string{"foo", "test", "bar"}},
			expected: true,
		},

		// Multiple excludes
		{
			name:     "enabled=true, multiple excludes including this",
			block:    config.SecretBlock{Name: "test", Enabled: &trueVal},
			opts:     Options{Exclude: []string{"foo", "test", "bar"}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldProcessBlock(tt.block, tt.opts)
			if result != tt.expected {
				t.Errorf("shouldProcessBlock() = %v, want %v", result, tt.expected)
			}
		})
	}
}
