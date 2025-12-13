package config

// Config represents the root configuration structure.
type Config struct {
	// Env contains variables for substitution throughout the config
	Env map[string]string `yaml:"env"`

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
	// Path is the Vault path (supports variable substitution)
	Path string `yaml:"path"`

	// Version is the KV engine version (1 or 2, auto-detected if not set)
	Version int `yaml:"version"`

	// Data contains secret key-value pairs
	Data map[string]string `yaml:"data"`
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
