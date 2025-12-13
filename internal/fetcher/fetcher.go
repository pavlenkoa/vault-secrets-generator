package fetcher

import (
	"context"
	"fmt"
	"sync"
)

// Fetcher retrieves files from various backends.
type Fetcher interface {
	// Fetch retrieves the file and returns its contents.
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
	// Check cache
	r.mu.RLock()
	if data, ok := r.cache[uri]; ok {
		r.mu.RUnlock()
		return data, nil
	}
	r.mu.RUnlock()

	// Find appropriate fetcher
	for _, f := range r.fetchers {
		if f.Supports(uri) {
			data, err := f.Fetch(ctx, uri)
			if err != nil {
				return nil, err
			}

			// Cache the result
			r.mu.Lock()
			r.cache[uri] = data
			r.mu.Unlock()

			return data, nil
		}
	}

	return nil, fmt.Errorf("no fetcher supports URI: %s", uri)
}

// ClearCache clears the fetch cache.
func (r *Registry) ClearCache() {
	r.mu.Lock()
	r.cache = make(map[string][]byte)
	r.mu.Unlock()
}
