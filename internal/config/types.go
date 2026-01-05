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
	Bcrypt   Strategy
	Argon2   Strategy
	Pbkdf2   Strategy
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
		Bcrypt:   StrategyUpdate, // Keep in sync with source key
		Argon2:   StrategyUpdate, // Keep in sync with source key
		Pbkdf2:   StrategyUpdate, // Keep in sync with source key
	}
}

// Defaults contains default settings.
type Defaults struct {
	// Mount is the default KV mount path (default: "secret")
	Mount string

	// Version is the default KV engine version (1 or 2, auto-detect if 0)
	Version int

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

// BcryptConfig defines bcrypt hashing parameters.
type BcryptConfig struct {
	// FromKey is the key name to hash (must exist in same secret block)
	FromKey string

	// Cost is the bcrypt cost factor (default: 12)
	Cost int
}

// Argon2Config defines argon2 hashing parameters.
type Argon2Config struct {
	// FromKey is the key name to hash (must exist in same secret block)
	FromKey string

	// Variant is the argon2 variant: "id" or "i" (default: "id")
	Variant string

	// Memory is the memory size in KB (default: 65536 = 64MB)
	Memory uint32

	// Iterations is the number of iterations (default: 3)
	Iterations uint32

	// Parallelism is the degree of parallelism (default: 4)
	Parallelism uint8
}

// Pbkdf2Config defines PBKDF2 hashing parameters.
type Pbkdf2Config struct {
	// FromKey is the key name to hash (must exist in same secret block)
	FromKey string

	// Variant is the hash function: "sha256" or "sha512" (default: "sha512")
	Variant string

	// Iterations is the number of iterations (default: 310000)
	Iterations int
}

// SecretBlock represents a group of secrets at a Vault path.
type SecretBlock struct {
	// Name is the block label/identifier (for display and lookup)
	Name string

	// Mount is the KV mount path (defaults to defaults.mount, then "secret")
	Mount string

	// Path is the path within the mount (supports interpolation)
	Path string

	// Version is the KV engine version (1 or 2, auto-detected if not set)
	Version int

	// Prune deletes keys in Vault that are not defined in config
	Prune bool

	// Enabled controls whether this secret block is processed (default: true)
	// When false, the block is skipped unless explicitly targeted via --target flag
	Enabled *bool

	// Content contains secret key-value pairs (moved from direct attributes in v1.x)
	Content map[string]Value
}

// IsEnabled returns true if this secret block should be processed.
// Defaults to true if Enabled is not set.
func (s *SecretBlock) IsEnabled() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

// FullPath returns the complete Vault path as mount/path.
func (s *SecretBlock) FullPath() string {
	if s.Path == "" {
		return s.Mount
	}
	return s.Mount + "/" + s.Path
}

// ValueType represents the type of a secret value.
type ValueType string

// ValueType constants define the supported value types.
const (
	ValueTypeStatic   ValueType = "static"
	ValueTypeGenerate ValueType = "generate"
	ValueTypeJSON     ValueType = "json"
	ValueTypeYAML     ValueType = "yaml"
	ValueTypeRaw      ValueType = "raw"
	ValueTypeVault    ValueType = "vault"
	ValueTypeCommand  ValueType = "command"
	ValueTypeBcrypt   ValueType = "bcrypt"
	ValueTypeArgon2   ValueType = "argon2"
	ValueTypePbkdf2   ValueType = "pbkdf2"
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

	// Bcrypt holds the bcrypt hashing configuration
	Bcrypt *BcryptConfig

	// Argon2 holds the argon2 hashing configuration
	Argon2 *Argon2Config

	// Pbkdf2 holds the PBKDF2 hashing configuration
	Pbkdf2 *Pbkdf2Config
}
