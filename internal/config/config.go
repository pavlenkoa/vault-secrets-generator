package config

import (
	"fmt"
	"os"
)

// Load reads and parses a config file from the given path.
// The vars parameter provides CLI variable overrides for env() functions.
func Load(path string, vars Variables) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	return ParseHCL(data, path, vars)
}
