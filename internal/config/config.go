package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a config file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	return Parse(data)
}

// Parse parses config from YAML bytes.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}

	// Apply defaults
	applyDefaults(&cfg)

	// Validate
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// applyDefaults sets default values for unset fields.
func applyDefaults(cfg *Config) {
	defaults := DefaultPasswordPolicy()

	if cfg.Defaults.Generate.Length == 0 {
		cfg.Defaults.Generate.Length = defaults.Length
	}
	if cfg.Defaults.Generate.Digits == 0 {
		cfg.Defaults.Generate.Digits = defaults.Digits
	}
	if cfg.Defaults.Generate.Symbols == 0 {
		cfg.Defaults.Generate.Symbols = defaults.Symbols
	}
	if cfg.Defaults.Generate.SymbolCharacters == "" {
		cfg.Defaults.Generate.SymbolCharacters = defaults.SymbolCharacters
	}
	if cfg.Defaults.Generate.AllowRepeat == nil {
		cfg.Defaults.Generate.AllowRepeat = defaults.AllowRepeat
	}
}

// validate checks the config for errors.
func validate(cfg *Config) error {
	if len(cfg.Secrets) == 0 {
		return fmt.Errorf("no secrets defined")
	}

	for name, block := range cfg.Secrets {
		if block.Path == "" {
			return fmt.Errorf("secret block %q: path is required", name)
		}
		if len(block.Data) == 0 {
			return fmt.Errorf("secret block %q: no data defined", name)
		}
		if block.Version != 0 && block.Version != 1 && block.Version != 2 {
			return fmt.Errorf("secret block %q: version must be 1 or 2", name)
		}
	}

	if cfg.Defaults.Generate.Length < 1 {
		return fmt.Errorf("defaults.generate.length must be at least 1")
	}

	minRequired := cfg.Defaults.Generate.Digits + cfg.Defaults.Generate.Symbols
	if !cfg.Defaults.Generate.NoUpper {
		// Need at least some room for letters
		minRequired++
	}
	if cfg.Defaults.Generate.Length < minRequired {
		return fmt.Errorf("defaults.generate.length (%d) is too small for required digits (%d) and symbols (%d)",
			cfg.Defaults.Generate.Length, cfg.Defaults.Generate.Digits, cfg.Defaults.Generate.Symbols)
	}

	return nil
}
