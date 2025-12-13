package engine

import (
	"testing"
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
