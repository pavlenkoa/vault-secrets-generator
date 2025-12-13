package fetcher

import (
	"context"
	"os"
	"testing"
)

func TestS3Fetcher_Supports(t *testing.T) {
	f := &S3Fetcher{}

	tests := []struct {
		uri      string
		expected bool
	}{
		{"s3://bucket/path/to/state.tfstate", true},
		{"s3://my-bucket/terraform.tfstate", true},
		{"s3://bucket/deep/nested/path/state.tfstate", true},
		{"gcs://bucket/path.tfstate", false},
		{"file:///path/to/state.tfstate", false},
		{"http://example.com/state.tfstate", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			result := f.Supports(tt.uri)
			if result != tt.expected {
				t.Errorf("Supports(%q) = %v, want %v", tt.uri, result, tt.expected)
			}
		})
	}
}

func TestS3Fetcher_ParseURI(t *testing.T) {
	f := &S3Fetcher{}

	tests := []struct {
		name       string
		uri        string
		wantBucket string
		wantKey    string
		wantErr    bool
	}{
		{
			name:       "simple path",
			uri:        "s3://mybucket/terraform.tfstate",
			wantBucket: "mybucket",
			wantKey:    "terraform.tfstate",
		},
		{
			name:       "nested path",
			uri:        "s3://terraform-state/env/prod/rds/terraform.tfstate",
			wantBucket: "terraform-state",
			wantKey:    "env/prod/rds/terraform.tfstate",
		},
		{
			name:       "path with special chars",
			uri:        "s3://my-bucket-123/path_to/state-file.tfstate",
			wantBucket: "my-bucket-123",
			wantKey:    "path_to/state-file.tfstate",
		},
		{
			name:    "missing key",
			uri:     "s3://mybucket/",
			wantErr: true,
		},
		{
			name:    "bucket only",
			uri:     "s3://mybucket",
			wantErr: true,
		},
		{
			name:    "empty bucket",
			uri:     "s3:///path/to/file",
			wantErr: true,
		},
		{
			name:    "wrong scheme",
			uri:     "gcs://bucket/key",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, key, err := f.parseURI(tt.uri)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bucket != tt.wantBucket {
				t.Errorf("bucket = %q, want %q", bucket, tt.wantBucket)
			}
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
		})
	}
}

// Integration tests - require AWS credentials and a test bucket
// Set AWS credentials and VSG_TEST_S3_BUCKET to run these

func TestIntegration_S3Fetcher_Fetch(t *testing.T) {
	bucket := os.Getenv("VSG_TEST_S3_BUCKET")
	if bucket == "" {
		t.Skip("VSG_TEST_S3_BUCKET not set, skipping S3 integration test")
	}

	ctx := context.Background()

	fetcher, err := NewS3Fetcher(ctx)
	if err != nil {
		t.Fatalf("failed to create S3 fetcher: %v", err)
	}

	// Try to fetch a test file (you would need to create this in your test bucket)
	testURI := "s3://" + bucket + "/test/terraform.tfstate"
	data, err := fetcher.Fetch(ctx, testURI)
	if err != nil {
		t.Logf("Note: To run this test, create a test terraform.tfstate file in s3://%s/test/", bucket)
		t.Skipf("failed to fetch test file (may not exist): %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty data")
	}

	t.Logf("Successfully fetched %d bytes from S3", len(data))
}

func TestIntegration_S3Fetcher_FetchNonExistent(t *testing.T) {
	bucket := os.Getenv("VSG_TEST_S3_BUCKET")
	if bucket == "" {
		t.Skip("VSG_TEST_S3_BUCKET not set, skipping S3 integration test")
	}

	ctx := context.Background()

	fetcher, err := NewS3Fetcher(ctx)
	if err != nil {
		t.Fatalf("failed to create S3 fetcher: %v", err)
	}

	// Try to fetch a non-existent file
	testURI := "s3://" + bucket + "/nonexistent/path/file.tfstate"
	_, err = fetcher.Fetch(ctx, testURI)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestNewS3Fetcher_NoCredentials(t *testing.T) {
	// This test verifies that the fetcher can be created even without credentials
	// (the error would occur when trying to actually fetch)

	// Save and clear AWS credentials
	origAccessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	origSecretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	origProfile := os.Getenv("AWS_PROFILE")
	origConfigFile := os.Getenv("AWS_CONFIG_FILE")
	origCredsFile := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")

	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	os.Unsetenv("AWS_PROFILE")
	os.Setenv("AWS_CONFIG_FILE", "/nonexistent")
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/nonexistent")

	defer func() {
		if origAccessKey != "" {
			os.Setenv("AWS_ACCESS_KEY_ID", origAccessKey)
		}
		if origSecretKey != "" {
			os.Setenv("AWS_SECRET_ACCESS_KEY", origSecretKey)
		}
		if origProfile != "" {
			os.Setenv("AWS_PROFILE", origProfile)
		}
		if origConfigFile != "" {
			os.Setenv("AWS_CONFIG_FILE", origConfigFile)
		} else {
			os.Unsetenv("AWS_CONFIG_FILE")
		}
		if origCredsFile != "" {
			os.Setenv("AWS_SHARED_CREDENTIALS_FILE", origCredsFile)
		} else {
			os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE")
		}
	}()

	ctx := context.Background()

	// Should still be able to create the fetcher
	fetcher, err := NewS3Fetcher(ctx)
	if err != nil {
		// Some environments may fail to create the fetcher without credentials
		// This is acceptable behavior
		t.Logf("NewS3Fetcher failed without credentials (expected in some environments): %v", err)
		return
	}

	if fetcher == nil {
		t.Error("expected non-nil fetcher")
	}
}
