package engine

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
	"github.com/pavlenkoa/vault-secrets-generator/internal/fetcher"
	"github.com/pavlenkoa/vault-secrets-generator/internal/generator"
	"github.com/pavlenkoa/vault-secrets-generator/internal/parser"
)

// VaultReader reads secrets from Vault for the vault() function.
type VaultReader interface {
	ReadSecret(ctx context.Context, path, key string) (string, error)
}

// Resolver resolves secret values from various sources.
type Resolver struct {
	fetchers    *fetcher.Registry
	vaultReader VaultReader
	defaults    config.PasswordPolicy
	strategies  config.StrategyDefaults
}

// NewResolver creates a new value resolver.
func NewResolver(fetchers *fetcher.Registry, vaultReader VaultReader, defaults config.PasswordPolicy, strategies config.StrategyDefaults) *Resolver {
	return &Resolver{
		fetchers:    fetchers,
		vaultReader: vaultReader,
		defaults:    defaults,
		strategies:  strategies,
	}
}

// ResolveResult contains the resolved value and metadata.
type ResolveResult struct {
	Value     string
	Source    ValueSource
	Strategy  config.Strategy
	StaleHash bool   // True if hash doesn't verify but strategy=create prevented update
	FromKey   string // For hash types, the key that was hashed
}

// ValueSource indicates where a value came from.
type ValueSource string

// ValueSource constants indicate where a value originated from.
const (
	SourceStatic    ValueSource = "static"
	SourceGenerated ValueSource = "generated"
	SourceJSON      ValueSource = "json"
	SourceYAML      ValueSource = "yaml"
	SourceRaw       ValueSource = "raw"
	SourceVault     ValueSource = "vault"
	SourceCommand   ValueSource = "command"
	SourceExisting  ValueSource = "existing"
	SourceBcrypt    ValueSource = "bcrypt"
	SourceArgon2    ValueSource = "argon2"
	SourcePbkdf2    ValueSource = "pbkdf2"
)

// Resolve resolves a single value based on its type.
// existingValue is the current value in Vault (if any).
// force forces regeneration of generated secrets.
func (r *Resolver) Resolve(ctx context.Context, val config.Value, existingValue string, force bool) (*ResolveResult, error) {
	// Determine effective strategy
	strategy := val.Strategy
	if strategy == "" {
		strategy = r.getDefaultStrategy(val.Type)
	}

	switch val.Type {
	case config.ValueTypeStatic:
		return r.resolveStatic(val, existingValue, strategy)

	case config.ValueTypeGenerate:
		return r.resolveGenerate(val, existingValue, force, strategy)

	case config.ValueTypeJSON:
		return r.resolveJSON(ctx, val, existingValue, strategy)

	case config.ValueTypeYAML:
		return r.resolveYAML(ctx, val, existingValue, strategy)

	case config.ValueTypeRaw:
		return r.resolveRaw(ctx, val, existingValue, strategy)

	case config.ValueTypeVault:
		return r.resolveVault(ctx, val, existingValue, strategy)

	case config.ValueTypeCommand:
		return r.resolveCommand(ctx, val, existingValue, strategy)

	default:
		return nil, fmt.Errorf("unknown value type: %s", val.Type)
	}
}

// getDefaultStrategy returns the default strategy for a value type.
func (r *Resolver) getDefaultStrategy(valueType config.ValueType) config.Strategy {
	switch valueType {
	case config.ValueTypeGenerate:
		return r.strategies.Generate
	case config.ValueTypeJSON:
		return r.strategies.JSON
	case config.ValueTypeYAML:
		return r.strategies.YAML
	case config.ValueTypeRaw:
		return r.strategies.Raw
	case config.ValueTypeStatic:
		return r.strategies.Static
	case config.ValueTypeCommand:
		return r.strategies.Command
	case config.ValueTypeVault:
		return r.strategies.Vault
	case config.ValueTypeBcrypt:
		return r.strategies.Bcrypt
	case config.ValueTypeArgon2:
		return r.strategies.Argon2
	case config.ValueTypePbkdf2:
		return r.strategies.Pbkdf2
	default:
		return config.StrategyUpdate
	}
}

// resolveStatic returns a static value.
func (r *Resolver) resolveStatic(val config.Value, existingValue string, strategy config.Strategy) (*ResolveResult, error) {
	// Apply strategy
	if existingValue != "" && strategy == config.StrategyCreate && existingValue == val.Static {
		return &ResolveResult{
			Value:    existingValue,
			Source:   SourceExisting,
			Strategy: strategy,
		}, nil
	}

	return &ResolveResult{
		Value:    val.Static,
		Source:   SourceStatic,
		Strategy: strategy,
	}, nil
}

// resolveGenerate generates a password based on the policy.
func (r *Resolver) resolveGenerate(val config.Value, existingValue string, force bool, strategy config.Strategy) (*ResolveResult, error) {
	// If we have an existing value and not forcing and strategy is create, keep it
	if existingValue != "" && !force && strategy == config.StrategyCreate {
		return &ResolveResult{
			Value:    existingValue,
			Source:   SourceExisting,
			Strategy: strategy,
		}, nil
	}

	// Determine the policy to use
	policy := r.defaults
	if val.Generate != nil {
		// Merge custom policy with defaults
		policy = mergePolicy(r.defaults, *val.Generate)
	}

	password, err := generator.Generate(policy)
	if err != nil {
		return nil, fmt.Errorf("generating password: %w", err)
	}

	return &ResolveResult{
		Value:    password,
		Source:   SourceGenerated,
		Strategy: strategy,
	}, nil
}

// mergePolicy merges a custom policy with defaults.
// Custom values override defaults only if they are explicitly set.
func mergePolicy(defaults, custom config.PasswordPolicy) config.PasswordPolicy {
	result := defaults

	if custom.Length > 0 {
		result.Length = custom.Length
	}
	if custom.Digits > 0 {
		result.Digits = custom.Digits
	}
	// Symbols can be 0 intentionally, so we check differently
	// If the custom policy has any non-default fields set, use its Symbols value
	if custom.Length > 0 || custom.Digits > 0 || custom.SymbolCharacters != "" || custom.NoUpper || custom.AllowRepeat != nil {
		result.Symbols = custom.Symbols
	}
	if custom.SymbolCharacters != "" {
		result.SymbolCharacters = custom.SymbolCharacters
	}
	if custom.NoUpper {
		result.NoUpper = custom.NoUpper
	}
	if custom.AllowRepeat != nil {
		result.AllowRepeat = custom.AllowRepeat
	}

	return result
}

// resolveJSON fetches a JSON file and extracts a value.
func (r *Resolver) resolveJSON(ctx context.Context, val config.Value, existingValue string, strategy config.Strategy) (*ResolveResult, error) {
	// Apply strategy - if create and key exists, skip
	if existingValue != "" && strategy == config.StrategyCreate {
		return &ResolveResult{
			Value:    existingValue,
			Source:   SourceExisting,
			Strategy: strategy,
		}, nil
	}

	// Fetch the source file
	data, err := r.fetchers.Fetch(ctx, val.URL)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", val.URL, err)
	}

	// Extract value using JSON path
	extracted, err := parser.ExtractJSON(data, val.Query)
	if err != nil {
		return nil, fmt.Errorf("extracting JSON path %s: %w", val.Query, err)
	}

	return &ResolveResult{
		Value:    extracted,
		Source:   SourceJSON,
		Strategy: strategy,
	}, nil
}

// resolveYAML fetches a YAML file and extracts a value.
func (r *Resolver) resolveYAML(ctx context.Context, val config.Value, existingValue string, strategy config.Strategy) (*ResolveResult, error) {
	// Apply strategy - if create and key exists, skip
	if existingValue != "" && strategy == config.StrategyCreate {
		return &ResolveResult{
			Value:    existingValue,
			Source:   SourceExisting,
			Strategy: strategy,
		}, nil
	}

	// Fetch the source file
	data, err := r.fetchers.Fetch(ctx, val.URL)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", val.URL, err)
	}

	// Extract value using YAML path
	extracted, err := parser.ExtractYAML(data, val.Query)
	if err != nil {
		return nil, fmt.Errorf("extracting YAML path %s: %w", val.Query, err)
	}

	return &ResolveResult{
		Value:    extracted,
		Source:   SourceYAML,
		Strategy: strategy,
	}, nil
}

// resolveRaw fetches a file and returns its raw content.
func (r *Resolver) resolveRaw(ctx context.Context, val config.Value, existingValue string, strategy config.Strategy) (*ResolveResult, error) {
	// Apply strategy - if create and key exists, skip
	if existingValue != "" && strategy == config.StrategyCreate {
		return &ResolveResult{
			Value:    existingValue,
			Source:   SourceExisting,
			Strategy: strategy,
		}, nil
	}

	// Fetch the source file
	data, err := r.fetchers.Fetch(ctx, val.URL)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", val.URL, err)
	}

	return &ResolveResult{
		Value:    string(data),
		Source:   SourceRaw,
		Strategy: strategy,
	}, nil
}

// resolveVault reads a secret from another Vault path.
func (r *Resolver) resolveVault(ctx context.Context, val config.Value, existingValue string, strategy config.Strategy) (*ResolveResult, error) {
	// Apply strategy - if create and key exists, skip
	if existingValue != "" && strategy == config.StrategyCreate {
		return &ResolveResult{
			Value:    existingValue,
			Source:   SourceExisting,
			Strategy: strategy,
		}, nil
	}

	if r.vaultReader == nil {
		return nil, fmt.Errorf("vault reader not configured")
	}

	// Read from Vault
	value, err := r.vaultReader.ReadSecret(ctx, val.VaultPath, val.VaultKey)
	if err != nil {
		return nil, fmt.Errorf("reading from vault path %s key %s: %w", val.VaultPath, val.VaultKey, err)
	}

	return &ResolveResult{
		Value:    value,
		Source:   SourceVault,
		Strategy: strategy,
	}, nil
}

// resolveCommand executes a command and returns its output.
func (r *Resolver) resolveCommand(ctx context.Context, val config.Value, existingValue string, strategy config.Strategy) (*ResolveResult, error) {
	// Apply strategy - if create and key exists, skip
	if existingValue != "" && strategy == config.StrategyCreate {
		return &ResolveResult{
			Value:    existingValue,
			Source:   SourceExisting,
			Strategy: strategy,
		}, nil
	}

	// Execute the command using sh -c to support shell features
	// #nosec G204 -- Command is intentionally user-configured
	cmd := exec.CommandContext(ctx, "sh", "-c", val.Command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("executing command: %w (stderr: %s)", err, stderr.String())
	}

	// Trim trailing newlines from output
	output := strings.TrimRight(stdout.String(), "\n\r")

	return &ResolveResult{
		Value:    output,
		Source:   SourceCommand,
		Strategy: strategy,
	}, nil
}

// ResolveHash resolves a hash value (bcrypt, argon2, pbkdf2).
// sourceValue is the password to hash (from resolvedValues map).
// existingValue is the current hash in Vault (if any).
// force forces regeneration of the hash.
func (r *Resolver) ResolveHash(val config.Value, sourceValue, existingValue string, force bool) (*ResolveResult, error) {
	// Determine effective strategy
	strategy := val.Strategy
	if strategy == "" {
		strategy = r.getDefaultStrategy(val.Type)
	}

	switch val.Type {
	case config.ValueTypeBcrypt:
		return r.resolveBcrypt(val, sourceValue, existingValue, force, strategy)
	case config.ValueTypeArgon2:
		return r.resolveArgon2(val, sourceValue, existingValue, force, strategy)
	case config.ValueTypePbkdf2:
		return r.resolvePbkdf2(val, sourceValue, existingValue, force, strategy)
	default:
		return nil, fmt.Errorf("ResolveHash called with non-hash type: %s", val.Type)
	}
}

// resolveBcrypt generates a bcrypt hash of the source value.
func (r *Resolver) resolveBcrypt(val config.Value, sourceValue, existingValue string, force bool, strategy config.Strategy) (*ResolveResult, error) {
	// --force overrides everything: regenerate hash
	if force {
		hash, err := generator.HashBcrypt(sourceValue, *val.Bcrypt)
		if err != nil {
			return nil, fmt.Errorf("generating bcrypt hash: %w", err)
		}
		return &ResolveResult{
			Value:    hash,
			Source:   SourceBcrypt,
			Strategy: strategy,
		}, nil
	}

	// If hash doesn't exist, create it (both strategies)
	if existingValue == "" {
		hash, err := generator.HashBcrypt(sourceValue, *val.Bcrypt)
		if err != nil {
			return nil, fmt.Errorf("generating bcrypt hash: %w", err)
		}
		return &ResolveResult{
			Value:    hash,
			Source:   SourceBcrypt,
			Strategy: strategy,
		}, nil
	}

	// Hash exists - check if it verifies against current source
	verifies := generator.VerifyBcrypt(existingValue, sourceValue)

	if strategy == config.StrategyCreate {
		// create strategy: don't update, ever
		// Mark as stale if hash doesn't verify (caller should warn)
		return &ResolveResult{
			Value:     existingValue,
			Source:    SourceExisting,
			Strategy:  strategy,
			StaleHash: !verifies,
			FromKey:   val.Bcrypt.FromKey,
		}, nil
	}

	// strategy == StrategyUpdate
	if verifies {
		// Hash is valid, no update needed
		return &ResolveResult{
			Value:    existingValue,
			Source:   SourceExisting,
			Strategy: strategy,
		}, nil
	}

	// Hash is stale, regenerate
	hash, err := generator.HashBcrypt(sourceValue, *val.Bcrypt)
	if err != nil {
		return nil, fmt.Errorf("generating bcrypt hash: %w", err)
	}
	return &ResolveResult{
		Value:    hash,
		Source:   SourceBcrypt,
		Strategy: strategy,
	}, nil
}

// resolveArgon2 generates an argon2 hash of the source value.
func (r *Resolver) resolveArgon2(val config.Value, sourceValue, existingValue string, force bool, strategy config.Strategy) (*ResolveResult, error) {
	// --force overrides everything: regenerate hash
	if force {
		hash, err := generator.HashArgon2(sourceValue, *val.Argon2)
		if err != nil {
			return nil, fmt.Errorf("generating argon2 hash: %w", err)
		}
		return &ResolveResult{
			Value:    hash,
			Source:   SourceArgon2,
			Strategy: strategy,
		}, nil
	}

	// If hash doesn't exist, create it (both strategies)
	if existingValue == "" {
		hash, err := generator.HashArgon2(sourceValue, *val.Argon2)
		if err != nil {
			return nil, fmt.Errorf("generating argon2 hash: %w", err)
		}
		return &ResolveResult{
			Value:    hash,
			Source:   SourceArgon2,
			Strategy: strategy,
		}, nil
	}

	// Hash exists - check if it verifies against current source
	verifies := generator.VerifyArgon2(existingValue, sourceValue)

	if strategy == config.StrategyCreate {
		// create strategy: don't update, ever
		// Mark as stale if hash doesn't verify (caller should warn)
		return &ResolveResult{
			Value:     existingValue,
			Source:    SourceExisting,
			Strategy:  strategy,
			StaleHash: !verifies,
			FromKey:   val.Argon2.FromKey,
		}, nil
	}

	// strategy == StrategyUpdate
	if verifies {
		// Hash is valid, no update needed
		return &ResolveResult{
			Value:    existingValue,
			Source:   SourceExisting,
			Strategy: strategy,
		}, nil
	}

	// Hash is stale, regenerate
	hash, err := generator.HashArgon2(sourceValue, *val.Argon2)
	if err != nil {
		return nil, fmt.Errorf("generating argon2 hash: %w", err)
	}
	return &ResolveResult{
		Value:    hash,
		Source:   SourceArgon2,
		Strategy: strategy,
	}, nil
}

// resolvePbkdf2 generates a PBKDF2 hash of the source value.
func (r *Resolver) resolvePbkdf2(val config.Value, sourceValue, existingValue string, force bool, strategy config.Strategy) (*ResolveResult, error) {
	// --force overrides everything: regenerate hash
	if force {
		hash, err := generator.HashPbkdf2(sourceValue, *val.Pbkdf2)
		if err != nil {
			return nil, fmt.Errorf("generating pbkdf2 hash: %w", err)
		}
		return &ResolveResult{
			Value:    hash,
			Source:   SourcePbkdf2,
			Strategy: strategy,
		}, nil
	}

	// If hash doesn't exist, create it (both strategies)
	if existingValue == "" {
		hash, err := generator.HashPbkdf2(sourceValue, *val.Pbkdf2)
		if err != nil {
			return nil, fmt.Errorf("generating pbkdf2 hash: %w", err)
		}
		return &ResolveResult{
			Value:    hash,
			Source:   SourcePbkdf2,
			Strategy: strategy,
		}, nil
	}

	// Hash exists - check if it verifies against current source
	verifies := generator.VerifyPbkdf2(existingValue, sourceValue)

	if strategy == config.StrategyCreate {
		// create strategy: don't update, ever
		// Mark as stale if hash doesn't verify (caller should warn)
		return &ResolveResult{
			Value:     existingValue,
			Source:    SourceExisting,
			Strategy:  strategy,
			StaleHash: !verifies,
			FromKey:   val.Pbkdf2.FromKey,
		}, nil
	}

	// strategy == StrategyUpdate
	if verifies {
		// Hash is valid, no update needed
		return &ResolveResult{
			Value:    existingValue,
			Source:   SourceExisting,
			Strategy: strategy,
		}, nil
	}

	// Hash is stale, regenerate
	hash, err := generator.HashPbkdf2(sourceValue, *val.Pbkdf2)
	if err != nil {
		return nil, fmt.Errorf("generating pbkdf2 hash: %w", err)
	}
	return &ResolveResult{
		Value:    hash,
		Source:   SourcePbkdf2,
		Strategy: strategy,
	}, nil
}

// IsHashType returns true if the value type is a hash function.
func IsHashType(t config.ValueType) bool {
	return t == config.ValueTypeBcrypt || t == config.ValueTypeArgon2 || t == config.ValueTypePbkdf2
}

// GetHashFromKey returns the from_key for a hash value type.
func GetHashFromKey(val config.Value) string {
	switch val.Type {
	case config.ValueTypeBcrypt:
		if val.Bcrypt != nil {
			return val.Bcrypt.FromKey
		}
	case config.ValueTypeArgon2:
		if val.Argon2 != nil {
			return val.Argon2.FromKey
		}
	case config.ValueTypePbkdf2:
		if val.Pbkdf2 != nil {
			return val.Pbkdf2.FromKey
		}
	}
	return ""
}
