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

// Resolver resolves secret values from various sources.
type Resolver struct {
	fetchers *fetcher.Registry
	defaults config.PasswordPolicy
}

// NewResolver creates a new value resolver.
func NewResolver(fetchers *fetcher.Registry, defaults config.PasswordPolicy) *Resolver {
	return &Resolver{
		fetchers: fetchers,
		defaults: defaults,
	}
}

// ResolveResult contains the resolved value and metadata.
type ResolveResult struct {
	Value       string
	Source      ValueSource
	IsGenerated bool
}

// ValueSource indicates where a value came from.
type ValueSource string

const (
	SourceStatic    ValueSource = "static"
	SourceGenerated ValueSource = "generated"
	SourceRemote    ValueSource = "remote"
	SourceCommand   ValueSource = "command"
	SourceExisting  ValueSource = "existing"
)

// Resolve resolves a single value based on its type.
// If existingValue is provided and the value is a generate directive,
// it will return the existing value unless force is true.
func (r *Resolver) Resolve(ctx context.Context, val config.Value, existingValue string, force bool) (*ResolveResult, error) {
	switch val.Type {
	case config.ValueTypeStatic:
		return &ResolveResult{
			Value:  val.Static,
			Source: SourceStatic,
		}, nil

	case config.ValueTypeGenerate:
		return r.resolveGenerate(val, existingValue, force)

	case config.ValueTypeSource:
		return r.resolveSource(ctx, val)

	case config.ValueTypeCommand:
		return r.resolveCommand(ctx, val)

	default:
		return nil, fmt.Errorf("unknown value type: %s", val.Type)
	}
}

// resolveGenerate generates a password based on the policy.
func (r *Resolver) resolveGenerate(val config.Value, existingValue string, force bool) (*ResolveResult, error) {
	// If we have an existing value and not forcing, keep it
	if existingValue != "" && !force {
		return &ResolveResult{
			Value:  existingValue,
			Source: SourceExisting,
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
		Value:       password,
		Source:      SourceGenerated,
		IsGenerated: true,
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

// resolveSource fetches data from a remote source and extracts a value.
func (r *Resolver) resolveSource(ctx context.Context, val config.Value) (*ResolveResult, error) {
	// Fetch the source file (caching handled by registry)
	data, err := r.fetchers.Fetch(ctx, val.Source)
	if err != nil {
		return nil, fmt.Errorf("fetching source %s: %w", val.Source, err)
	}

	var extracted string

	if val.JSONPath != "" {
		extracted, err = parser.ExtractJSON(data, val.JSONPath)
		if err != nil {
			return nil, fmt.Errorf("extracting JSON path %s: %w", val.JSONPath, err)
		}
	} else if val.YAMLPath != "" {
		extracted, err = parser.ExtractYAML(data, val.YAMLPath)
		if err != nil {
			return nil, fmt.Errorf("extracting YAML path %s: %w", val.YAMLPath, err)
		}
	} else {
		return nil, fmt.Errorf("source requires either 'json' or 'yaml' path")
	}

	return &ResolveResult{
		Value:  extracted,
		Source: SourceRemote,
	}, nil
}

// resolveCommand executes a command and returns its output.
func (r *Resolver) resolveCommand(ctx context.Context, val config.Value) (*ResolveResult, error) {
	// Execute the command using sh -c to support shell features
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
		Value:  output,
		Source: SourceCommand,
	}, nil
}
