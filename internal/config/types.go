package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Config represents the root configuration structure.
type Config struct {
	// Vault contains connection and auth settings
	Vault VaultConfig `yaml:"vault"`

	// Defaults contains default settings for password generation
	Defaults Defaults `yaml:"defaults"`

	// Secrets contains secret block definitions keyed by name
	Secrets map[string]SecretBlock `yaml:"secrets"`
}

// VaultConfig contains Vault connection settings.
type VaultConfig struct {
	// Address is the Vault server URL
	Address string `yaml:"address"`

	// Namespace is the Vault namespace (enterprise feature)
	Namespace string `yaml:"namespace"`

	// Auth contains authentication settings
	Auth AuthConfig `yaml:"auth"`
}

// AuthConfig contains Vault authentication settings.
type AuthConfig struct {
	// Method is the auth method: token, kubernetes, approle
	Method string `yaml:"method"`

	// Token is used for token auth method
	Token string `yaml:"token"`

	// Role is used for kubernetes and approle auth methods
	Role string `yaml:"role"`

	// RoleID is used for approle auth method
	RoleID string `yaml:"role_id"`

	// SecretID is used for approle auth method
	SecretID string `yaml:"secret_id"`

	// MountPath is the auth mount path (default depends on method)
	MountPath string `yaml:"mount_path"`
}

// Defaults contains default settings.
type Defaults struct {
	// Generate contains default password generation policy
	Generate PasswordPolicy `yaml:"generate"`
}

// PasswordPolicy defines password generation parameters.
type PasswordPolicy struct {
	// Length is the total password length (default: 32)
	Length int `yaml:"length"`

	// Digits is the minimum number of digits (default: 5)
	Digits int `yaml:"digits"`

	// Symbols is the minimum number of symbols (default: 5)
	Symbols int `yaml:"symbols"`

	// SymbolCharacters is the set of allowed symbols (default: "-_$@")
	SymbolCharacters string `yaml:"symbolCharacters"`

	// NoUpper excludes uppercase letters when true (default: false)
	NoUpper bool `yaml:"noUpper"`

	// AllowRepeat allows repeated characters when true (default: true)
	AllowRepeat *bool `yaml:"allowRepeat"`
}

// SecretBlock represents a group of secrets at a Vault path.
type SecretBlock struct {
	// Path is the Vault path
	Path string `yaml:"path"`

	// Version is the KV engine version (1 or 2, auto-detected if not set)
	Version int `yaml:"version"`

	// Data contains secret key-value pairs
	Data map[string]Value `yaml:"data"`
}

// ValueType represents the type of a secret value.
type ValueType string

const (
	ValueTypeStatic   ValueType = "static"
	ValueTypeGenerate ValueType = "generate"
	ValueTypeSource   ValueType = "source"
	ValueTypeCommand  ValueType = "command"
)

// Value represents a secret value which can be static, generated, from a source, or from a command.
type Value struct {
	Type ValueType

	// For static values
	Static string

	// For generated values
	Generate *PasswordPolicy

	// For source values (remote JSON/YAML)
	Source   string
	JSONPath string
	YAMLPath string

	// For command values
	Command string
}

// UnmarshalYAML implements custom unmarshaling for Value.
func (v *Value) UnmarshalYAML(node *yaml.Node) error {
	// Handle scalar values (string)
	if node.Kind == yaml.ScalarNode {
		var s string
		if err := node.Decode(&s); err != nil {
			return err
		}

		if s == "generate" {
			v.Type = ValueTypeGenerate
			return nil
		}

		v.Type = ValueTypeStatic
		v.Static = s
		return nil
	}

	// Handle mapping values (object)
	if node.Kind == yaml.MappingNode {
		// Decode into a generic map first to check structure
		var raw map[string]yaml.Node
		if err := node.Decode(&raw); err != nil {
			return err
		}

		// Check for "generate" key
		if genNode, ok := raw["generate"]; ok {
			v.Type = ValueTypeGenerate
			var policy PasswordPolicy
			if err := genNode.Decode(&policy); err != nil {
				return fmt.Errorf("invalid generate policy: %w", err)
			}
			v.Generate = &policy
			return nil
		}

		// Check for "source" key (remote JSON/YAML)
		if sourceNode, ok := raw["source"]; ok {
			v.Type = ValueTypeSource
			if err := sourceNode.Decode(&v.Source); err != nil {
				return fmt.Errorf("invalid source: %w", err)
			}

			// Check for json or yaml path
			if jsonNode, ok := raw["json"]; ok {
				if err := jsonNode.Decode(&v.JSONPath); err != nil {
					return fmt.Errorf("invalid json path: %w", err)
				}
			}
			if yamlNode, ok := raw["yaml"]; ok {
				if err := yamlNode.Decode(&v.YAMLPath); err != nil {
					return fmt.Errorf("invalid yaml path: %w", err)
				}
			}

			if v.JSONPath == "" && v.YAMLPath == "" {
				return fmt.Errorf("source requires either 'json' or 'yaml' path")
			}

			return nil
		}

		// Check for "command" key
		if cmdNode, ok := raw["command"]; ok {
			v.Type = ValueTypeCommand
			if err := cmdNode.Decode(&v.Command); err != nil {
				return fmt.Errorf("invalid command: %w", err)
			}
			return nil
		}

		return fmt.Errorf("unknown value format: must be string, 'generate', or object with 'generate', 'source', or 'command' key")
	}

	return fmt.Errorf("invalid value type: expected string or object")
}

// DefaultPasswordPolicy returns the default password generation policy.
func DefaultPasswordPolicy() PasswordPolicy {
	allowRepeat := true
	return PasswordPolicy{
		Length:           32,
		Digits:           5,
		Symbols:          5,
		SymbolCharacters: "-_$@",
		NoUpper:          false,
		AllowRepeat:      &allowRepeat,
	}
}
