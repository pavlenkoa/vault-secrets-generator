package config

// Strategy defines how a value should be reconciled with Vault.
type Strategy string

const (
	// StrategyCreate only creates new keys, never updates existing ones.
	StrategyCreate Strategy = "create"
	// StrategyUpdate creates new keys and updates changed values.
	StrategyUpdate Strategy = "update"
)

// Config represents the root configuration structure.
type Config struct {
	// Vault contains connection and auth settings
	Vault VaultConfig

	// Defaults contains default settings for strategies and password generation
	Defaults Defaults

	// Secrets contains secret block definitions keyed by name
	Secrets map[string]SecretBlock
}

// VaultConfig contains Vault connection settings.
type VaultConfig struct {
	// Address is the Vault server URL
	Address string

	// Namespace is the Vault namespace (enterprise feature)
	Namespace string

	// Auth contains authentication settings
	Auth AuthConfig
}

// AuthConfig contains Vault authentication settings.
type AuthConfig struct {
	// Method is the auth method: token, kubernetes, approle
	Method string

	// Token is used for token auth method
	Token string

	// Role is used for kubernetes and approle auth methods
	Role string

	// RoleID is used for approle auth method
	RoleID string

	// SecretID is used for approle auth method
	SecretID string

	// MountPath is the auth mount path (default depends on method)
	MountPath string
}

// StrategyDefaults defines default strategies per value type.
type StrategyDefaults struct {
	Generate Strategy
	JSON     Strategy
	YAML     Strategy
	Raw      Strategy
	Static   Strategy
	Command  Strategy
	Vault    Strategy
}

// DefaultStrategyDefaults returns the default strategy configuration.
func DefaultStrategyDefaults() StrategyDefaults {
	return StrategyDefaults{
		Generate: StrategyCreate, // Don't regenerate existing passwords
		JSON:     StrategyUpdate, // Keep in sync with source
		YAML:     StrategyUpdate, // Keep in sync with source
		Raw:      StrategyUpdate, // Keep in sync with source
		Static:   StrategyUpdate, // Update if changed
		Command:  StrategyUpdate, // Re-run and update
		Vault:    StrategyUpdate, // Keep in sync with source
	}
}

// Defaults contains default settings.
type Defaults struct {
	// Strategy contains default strategies per value type
	Strategy StrategyDefaults

	// Generate contains default password generation policy
	Generate PasswordPolicy
}

// PasswordPolicy defines password generation parameters.
type PasswordPolicy struct {
	// Length is the total password length (default: 32)
	Length int

	// Digits is the minimum number of digits (default: 5)
	Digits int

	// Symbols is the minimum number of symbols (default: 5)
	Symbols int

	// SymbolCharacters is the set of allowed symbols (default: "-_$@")
	SymbolCharacters string

	// NoUpper excludes uppercase letters when true (default: false)
	NoUpper bool

	// AllowRepeat allows repeated characters when true (default: true)
	AllowRepeat *bool
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

// SecretBlock represents a group of secrets at a Vault path.
type SecretBlock struct {
	// Path is the Vault path
	Path string

	// Version is the KV engine version (1 or 2, auto-detected if not set)
	Version int

	// Prune deletes keys in Vault that are not defined in config
	Prune bool

	// Data contains secret key-value pairs
	Data map[string]Value
}

// ValueType represents the type of a secret value.
type ValueType string

const (
	ValueTypeStatic   ValueType = "static"
	ValueTypeGenerate ValueType = "generate"
	ValueTypeJSON     ValueType = "json"
	ValueTypeYAML     ValueType = "yaml"
	ValueTypeRaw      ValueType = "raw"
	ValueTypeVault    ValueType = "vault"
	ValueTypeCommand  ValueType = "command"
)

// Value represents a secret value which can be static, generated, fetched, or from a command.
type Value struct {
	// Type indicates the value type
	Type ValueType

	// Strategy overrides the default strategy for this value type
	Strategy Strategy

	// Static holds the value for static types
	Static string

	// Generate holds the password policy for generated values
	Generate *PasswordPolicy

	// URL is the source URL for json/yaml/raw types
	URL string

	// Query is the jq/yq path for json/yaml types
	Query string

	// VaultPath is the source path for vault type
	VaultPath string

	// VaultKey is the source key for vault type
	VaultKey string

	// Command is the shell command for command type
	Command string
}
