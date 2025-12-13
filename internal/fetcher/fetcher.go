package fetcher

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Fetcher retrieves terraform state files from various backends.
type Fetcher interface {
	// Fetch retrieves the terraform state file and returns its contents.
	Fetch(ctx context.Context, uri string) ([]byte, error)

	// Supports returns true if this fetcher handles the given URI scheme.
	Supports(uri string) bool
}

// Registry manages multiple fetchers and routes requests to the appropriate one.
type Registry struct {
	fetchers []Fetcher
	cache    map[string][]byte
	mu       sync.RWMutex
}

// NewRegistry creates a new fetcher registry.
func NewRegistry() *Registry {
	return &Registry{
		cache: make(map[string][]byte),
	}
}

// Register adds a fetcher to the registry.
func (r *Registry) Register(f Fetcher) {
	r.fetchers = append(r.fetchers, f)
}

// Fetch retrieves content from the given URI using the appropriate fetcher.
// Results are cached for the lifetime of the registry.
func (r *Registry) Fetch(ctx context.Context, uri string) ([]byte, error) {
	// Extract the state file URI (before the #output... part)
	stateURI := uri
	if idx := strings.Index(uri, "#"); idx != -1 {
		stateURI = uri[:idx]
	}

	// Check cache
	r.mu.RLock()
	if data, ok := r.cache[stateURI]; ok {
		r.mu.RUnlock()
		return data, nil
	}
	r.mu.RUnlock()

	// Find appropriate fetcher
	for _, f := range r.fetchers {
		if f.Supports(stateURI) {
			data, err := f.Fetch(ctx, stateURI)
			if err != nil {
				return nil, err
			}

			// Cache the result
			r.mu.Lock()
			r.cache[stateURI] = data
			r.mu.Unlock()

			return data, nil
		}
	}

	return nil, fmt.Errorf("no fetcher supports URI: %s", stateURI)
}

// ClearCache clears the fetch cache.
func (r *Registry) ClearCache() {
	r.mu.Lock()
	r.cache = make(map[string][]byte)
	r.mu.Unlock()
}

// ParseURI splits a terraform state reference into the state file URI and output path.
// Format: scheme://path/to/state.tfstate#output.name
func ParseURI(uri string) (stateURI string, outputPath string, err error) {
	idx := strings.Index(uri, "#")
	if idx == -1 {
		return "", "", fmt.Errorf("invalid terraform state reference: missing #output.* path: %s", uri)
	}

	stateURI = uri[:idx]
	outputPath = uri[idx+1:]

	if !strings.HasPrefix(outputPath, "output.") {
		return "", "", fmt.Errorf("invalid output path: must start with 'output.': %s", outputPath)
	}

	return stateURI, outputPath, nil
}

// IsTerraformStateRef returns true if the value looks like a terraform state reference.
func IsTerraformStateRef(value string) bool {
	// Check for known schemes
	schemes := []string{"s3://", "gcs://", "file://"}
	for _, scheme := range schemes {
		if strings.HasPrefix(value, scheme) && strings.Contains(value, "#output.") {
			return true
		}
	}
	return false
}
