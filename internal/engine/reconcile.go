package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
	"github.com/pavlenkoa/vault-secrets-generator/internal/fetcher"
	"github.com/pavlenkoa/vault-secrets-generator/internal/vault"
)

// Engine handles the reconciliation of secrets.
type Engine struct {
	vaultClient *vault.Client
	resolver    *Resolver
	logger      *slog.Logger
}

// Options configures the engine behavior.
type Options struct {
	DryRun bool
	Force  bool // Force regeneration of generated secrets
}

// Result contains the outcome of a reconciliation.
type Result struct {
	Diff    *Diff
	Errors  []BlockError
	Applied bool
}

// BlockError represents an error in processing a block.
type BlockError struct {
	Block string
	Key   string
	Err   error
}

func (e BlockError) Error() string {
	if e.Key != "" {
		return fmt.Sprintf("%s/%s: %v", e.Block, e.Key, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Block, e.Err)
}

// vaultSecretReader implements VaultReader using the vault client.
type vaultSecretReader struct {
	client *vault.Client
}

// ReadSecret reads a secret from Vault.
func (r *vaultSecretReader) ReadSecret(ctx context.Context, path, key string) (string, error) {
	mount, subpath := parsePath(path)

	kv, err := vault.NewKVClient(r.client, mount, vault.KVVersionAuto)
	if err != nil {
		return "", fmt.Errorf("creating KV client: %w", err)
	}

	data, err := kv.Read(ctx, subpath)
	if err != nil {
		return "", fmt.Errorf("reading secret: %w", err)
	}

	if data == nil {
		return "", fmt.Errorf("secret not found: %s", path)
	}

	val, ok := data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s", key, path)
	}

	return fmt.Sprintf("%v", val), nil
}

// NewEngine creates a new reconciliation engine.
func NewEngine(vaultClient *vault.Client, fetchers *fetcher.Registry, defaults config.Defaults, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}

	// Create vault reader for vault() function
	vaultReader := &vaultSecretReader{client: vaultClient}

	return &Engine{
		vaultClient: vaultClient,
		resolver:    NewResolver(fetchers, vaultReader, defaults.Generate, defaults.Strategy),
		logger:      logger,
	}
}

// Reconcile processes the configuration and syncs secrets to Vault.
func (e *Engine) Reconcile(ctx context.Context, cfg *config.Config, opts Options) (*Result, error) {
	result := &Result{
		Diff: &Diff{},
	}

	for name, block := range cfg.Secrets {
		blockDiff, errors := e.processBlock(ctx, name, block, opts)
		result.Diff.Blocks = append(result.Diff.Blocks, blockDiff)
		result.Errors = append(result.Errors, errors...)
	}

	// Apply changes if not dry-run
	if !opts.DryRun && result.Diff.HasChanges() {
		applyErrors := e.applyChanges(ctx, cfg, result.Diff)
		result.Errors = append(result.Errors, applyErrors...)
		result.Applied = len(applyErrors) == 0
	}

	return result, nil
}

// processBlock processes a single secret block.
func (e *Engine) processBlock(ctx context.Context, name string, block config.SecretBlock, opts Options) (BlockDiff, []BlockError) {
	blockDiff := BlockDiff{
		Name:  name,
		Path:  block.Path,
		Prune: block.Prune,
	}
	var errors []BlockError

	e.logger.Debug("processing block", "name", name, "path", block.Path, "prune", block.Prune)

	// Parse mount and subpath from block.Path
	mount, subpath := parsePath(block.Path)

	// Create KV client for this block
	version := vault.KVVersion(block.Version)
	kv, err := vault.NewKVClient(e.vaultClient, mount, version)
	if err != nil {
		errors = append(errors, BlockError{Block: name, Err: fmt.Errorf("creating KV client: %w", err)})
		return blockDiff, errors
	}

	// Read current secrets from Vault
	current, err := kv.Read(ctx, subpath)
	if err != nil {
		errors = append(errors, BlockError{Block: name, Err: fmt.Errorf("reading current secrets: %w", err)})
		return blockDiff, errors
	}
	if current == nil {
		current = make(map[string]interface{})
	}

	// Convert current to string map
	currentStrings := make(map[string]string)
	for k, v := range current {
		currentStrings[k] = fmt.Sprintf("%v", v)
	}

	// Resolve desired values
	desired := make(map[string]string)
	sources := make(map[string]ValueSource)

	for key, value := range block.Data {
		existingValue := currentStrings[key]

		resolved, err := e.resolver.Resolve(ctx, value, existingValue, opts.Force)
		if err != nil {
			errors = append(errors, BlockError{Block: name, Key: key, Err: err})
			continue
		}

		desired[key] = resolved.Value
		sources[key] = resolved.Source

		e.logger.Debug("resolved secret",
			"block", name,
			"key", key,
			"source", resolved.Source,
			"strategy", resolved.Strategy,
			"changed", existingValue != resolved.Value,
		)
	}

	// Compute diff with prune option
	blockDiff.Changes = ComputeDiff(currentStrings, desired, sources, block.Prune)

	// Log warnings/info for unmanaged/deleted keys
	for _, change := range blockDiff.Changes {
		switch change.Change {
		case ChangeUnmanaged:
			e.logger.Warn("unmanaged key in Vault",
				"block", name,
				"key", change.Key,
				"hint", "this key exists in Vault but not in config",
			)
		case ChangeDelete:
			e.logger.Info("key will be pruned",
				"block", name,
				"key", change.Key,
			)
		}
	}

	return blockDiff, errors
}

// applyChanges writes the changes to Vault.
func (e *Engine) applyChanges(ctx context.Context, cfg *config.Config, diff *Diff) []BlockError {
	var errors []BlockError

	for _, blockDiff := range diff.Blocks {
		// Skip if no changes to apply
		hasChanges := false
		for _, change := range blockDiff.Changes {
			if change.Change == ChangeAdd || change.Change == ChangeUpdate || change.Change == ChangeDelete {
				hasChanges = true
				break
			}
		}
		if !hasChanges {
			continue
		}

		block, ok := cfg.Secrets[blockDiff.Name]
		if !ok {
			// Try to find by path (for secrets keyed by path)
			for _, b := range cfg.Secrets {
				if b.Path == blockDiff.Path {
					block = b
					ok = true
					break
				}
			}
		}
		if !ok {
			continue
		}

		mount, subpath := parsePath(block.Path)
		version := vault.KVVersion(block.Version)

		kv, err := vault.NewKVClient(e.vaultClient, mount, version)
		if err != nil {
			errors = append(errors, BlockError{Block: blockDiff.Name, Err: fmt.Errorf("creating KV client: %w", err)})
			continue
		}

		// Build the data to write
		data := make(map[string]interface{})
		for _, change := range blockDiff.Changes {
			switch change.Change {
			case ChangeAdd, ChangeUpdate, ChangeNone:
				data[change.Key] = change.NewValue
			case ChangeUnmanaged:
				// Keep unmanaged keys (prune is false)
				data[change.Key] = change.OldValue
			case ChangeDelete:
				// Don't include deleted keys (prune is true)
				// Key is intentionally omitted from data
			}
		}

		// Write to Vault
		e.logger.Info("writing secrets to vault",
			"block", blockDiff.Name,
			"path", blockDiff.Path,
			"keys", len(data),
			"prune", blockDiff.Prune,
		)

		if err := kv.Write(ctx, subpath, data); err != nil {
			errors = append(errors, BlockError{Block: blockDiff.Name, Err: fmt.Errorf("writing to vault: %w", err)})
		}
	}

	return errors
}

// parsePath splits a path like "secret/myapp" into mount "secret" and subpath "myapp".
func parsePath(path string) (mount, subpath string) {
	path = strings.Trim(path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// Plan computes what changes would be made without applying them.
func (e *Engine) Plan(ctx context.Context, cfg *config.Config, opts Options) (*Result, error) {
	opts.DryRun = true
	return e.Reconcile(ctx, cfg, opts)
}
