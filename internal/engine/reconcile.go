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
	DryRun   bool
	Force    bool // Force regeneration of generated secrets
	FailFast bool // Stop on first error
	Only     string // Only process this block
	Key      string // Only process this key (requires Only)
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

// NewEngine creates a new reconciliation engine.
func NewEngine(vaultClient *vault.Client, fetchers *fetcher.Registry, defaults config.PasswordPolicy, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}

	return &Engine{
		vaultClient: vaultClient,
		resolver:    NewResolver(fetchers, defaults),
		logger:      logger,
	}
}

// Reconcile processes the configuration and syncs secrets to Vault.
func (e *Engine) Reconcile(ctx context.Context, cfg *config.Config, opts Options) (*Result, error) {
	result := &Result{
		Diff: &Diff{},
	}

	for name, block := range cfg.Secrets {
		// Filter by block name if specified
		if opts.Only != "" && opts.Only != name {
			continue
		}

		blockDiff, errors := e.processBlock(ctx, name, block, opts)
		result.Diff.Blocks = append(result.Diff.Blocks, blockDiff)
		result.Errors = append(result.Errors, errors...)

		if opts.FailFast && len(errors) > 0 {
			return result, fmt.Errorf("failed processing block %s: %w", name, errors[0].Err)
		}
	}

	// Apply changes if not dry-run
	if !opts.DryRun && result.Diff.HasChanges() {
		applyErrors := e.applyChanges(ctx, cfg, result.Diff, opts)
		result.Errors = append(result.Errors, applyErrors...)
		result.Applied = len(applyErrors) == 0
	}

	return result, nil
}

// processBlock processes a single secret block.
func (e *Engine) processBlock(ctx context.Context, name string, block config.SecretBlock, opts Options) (BlockDiff, []BlockError) {
	blockDiff := BlockDiff{
		Name: name,
		Path: block.Path,
	}
	var errors []BlockError

	e.logger.Debug("processing block", "name", name, "path", block.Path)

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
		// Filter by key if specified
		if opts.Key != "" && opts.Key != key {
			continue
		}

		existingValue := currentStrings[key]

		resolved, err := e.resolver.Resolve(ctx, value, existingValue, opts.Force)
		if err != nil {
			errors = append(errors, BlockError{Block: name, Key: key, Err: err})
			if opts.FailFast {
				break
			}
			continue
		}

		desired[key] = resolved.Value
		sources[key] = resolved.Source

		e.logger.Debug("resolved secret",
			"block", name,
			"key", key,
			"source", resolved.Source,
			"changed", existingValue != resolved.Value,
		)
	}

	// Compute diff
	blockDiff.Changes = ComputeDiff(currentStrings, desired, sources)

	return blockDiff, errors
}

// applyChanges writes the changes to Vault.
func (e *Engine) applyChanges(ctx context.Context, cfg *config.Config, diff *Diff, opts Options) []BlockError {
	var errors []BlockError

	for _, blockDiff := range diff.Blocks {
		// Skip if no changes
		hasChanges := false
		for _, change := range blockDiff.Changes {
			if change.Change != ChangeNone {
				hasChanges = true
				break
			}
		}
		if !hasChanges {
			continue
		}

		block, ok := cfg.Secrets[blockDiff.Name]
		if !ok {
			continue
		}

		mount, subpath := parsePath(block.Path)
		version := vault.KVVersion(block.Version)

		kv, err := vault.NewKVClient(e.vaultClient, mount, version)
		if err != nil {
			errors = append(errors, BlockError{Block: blockDiff.Name, Err: fmt.Errorf("creating KV client: %w", err)})
			if opts.FailFast {
				return errors
			}
			continue
		}

		// Build the data to write
		data := make(map[string]interface{})
		for _, change := range blockDiff.Changes {
			switch change.Change {
			case ChangeAdd, ChangeUpdate, ChangeNone:
				data[change.Key] = change.NewValue
			case ChangeDelete:
				// Don't include deleted keys
			}
		}

		// Write to Vault
		e.logger.Info("writing secrets to vault",
			"block", blockDiff.Name,
			"path", blockDiff.Path,
			"keys", len(data),
		)

		if err := kv.Write(ctx, subpath, data); err != nil {
			errors = append(errors, BlockError{Block: blockDiff.Name, Err: fmt.Errorf("writing to vault: %w", err)})
			if opts.FailFast {
				return errors
			}
		}
	}

	return errors
}

// parsePath splits a path like "kv/myapp" into mount "kv" and subpath "myapp".
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
