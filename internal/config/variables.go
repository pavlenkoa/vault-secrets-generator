package config

import (
	"fmt"
	"regexp"
	"strings"
)

// variablePattern matches {variable} patterns.
var variablePattern = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// substituteVariables replaces {var} patterns throughout the config.
func substituteVariables(cfg *Config) error {
	if len(cfg.Env) == 0 {
		return nil
	}

	// Substitute in vault config
	cfg.Vault.Address = substituteString(cfg.Vault.Address, cfg.Env)
	cfg.Vault.Namespace = substituteString(cfg.Vault.Namespace, cfg.Env)
	cfg.Vault.Auth.Role = substituteString(cfg.Vault.Auth.Role, cfg.Env)
	cfg.Vault.Auth.MountPath = substituteString(cfg.Vault.Auth.MountPath, cfg.Env)

	// Substitute in secret blocks
	for name, block := range cfg.Secrets {
		block.Path = substituteString(block.Path, cfg.Env)

		newData := make(map[string]string, len(block.Data))
		for key, value := range block.Data {
			newKey := substituteString(key, cfg.Env)
			newValue := substituteString(value, cfg.Env)
			newData[newKey] = newValue
		}
		block.Data = newData
		cfg.Secrets[name] = block
	}

	// Validate no unresolved variables remain
	return checkUnresolvedVariables(cfg)
}

// substituteString replaces all {var} patterns in s with values from vars.
func substituteString(s string, vars map[string]string) string {
	if s == "" {
		return s
	}

	return variablePattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name from {name}
		varName := match[1 : len(match)-1]
		if value, ok := vars[varName]; ok {
			return value
		}
		// Return original if not found (will be caught by validation)
		return match
	})
}

// checkUnresolvedVariables returns an error if any {var} patterns remain.
func checkUnresolvedVariables(cfg *Config) error {
	var unresolved []string

	// Check vault config
	unresolved = append(unresolved, findUnresolved(cfg.Vault.Address)...)
	unresolved = append(unresolved, findUnresolved(cfg.Vault.Namespace)...)
	unresolved = append(unresolved, findUnresolved(cfg.Vault.Auth.Role)...)
	unresolved = append(unresolved, findUnresolved(cfg.Vault.Auth.MountPath)...)

	// Check secret blocks
	for _, block := range cfg.Secrets {
		unresolved = append(unresolved, findUnresolved(block.Path)...)
		for key, value := range block.Data {
			unresolved = append(unresolved, findUnresolved(key)...)
			unresolved = append(unresolved, findUnresolved(value)...)
		}
	}

	if len(unresolved) > 0 {
		// Deduplicate
		seen := make(map[string]bool)
		var unique []string
		for _, v := range unresolved {
			if !seen[v] {
				seen[v] = true
				unique = append(unique, v)
			}
		}
		return fmt.Errorf("unresolved variables: %s", strings.Join(unique, ", "))
	}

	return nil
}

// findUnresolved returns all unresolved {var} patterns in s.
func findUnresolved(s string) []string {
	matches := variablePattern.FindAllString(s, -1)
	return matches
}

// Substitute replaces {var} patterns in a string using the provided variables.
// This is exported for use by other packages that need variable substitution.
func Substitute(s string, vars map[string]string) string {
	return substituteString(s, vars)
}
