package engine

import (
	"context"
	"fmt"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
	"github.com/pavlenkoa/vault-secrets-generator/internal/fetcher"
	"github.com/pavlenkoa/vault-secrets-generator/internal/generator"
	"github.com/pavlenkoa/vault-secrets-generator/internal/tfstate"
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
	SourceTerraform ValueSource = "terraform"
	SourceGenerated ValueSource = "generated"
	SourceStatic    ValueSource = "static"
	SourceExisting  ValueSource = "existing"
)

// Resolve resolves a single value based on its type.
// If existingValue is provided and the value is a generate directive,
// it will return the existing value unless force is true.
func (r *Resolver) Resolve(ctx context.Context, value string, existingValue string, force bool) (*ResolveResult, error) {
	// Check if it's a terraform state reference
	if fetcher.IsTerraformStateRef(value) {
		resolved, err := r.resolveTerraformRef(ctx, value)
		if err != nil {
			return nil, err
		}
		return &ResolveResult{
			Value:  resolved,
			Source: SourceTerraform,
		}, nil
	}

	// Check if it's a generate directive
	if generator.IsGenerateValue(value) {
		// If we have an existing value and not forcing, keep it
		if existingValue != "" && !force {
			return &ResolveResult{
				Value:  existingValue,
				Source: SourceExisting,
			}, nil
		}

		resolved, err := r.resolveGenerate(value)
		if err != nil {
			return nil, err
		}
		return &ResolveResult{
			Value:       resolved,
			Source:      SourceGenerated,
			IsGenerated: true,
		}, nil
	}

	// Static value
	return &ResolveResult{
		Value:  value,
		Source: SourceStatic,
	}, nil
}

// resolveTerraformRef fetches and extracts a value from terraform state.
func (r *Resolver) resolveTerraformRef(ctx context.Context, ref string) (string, error) {
	// Parse the URI
	stateURI, outputPath, err := fetcher.ParseURI(ref)
	if err != nil {
		return "", err
	}

	// Fetch the state file (caching handled by registry)
	data, err := r.fetchers.Fetch(ctx, stateURI)
	if err != nil {
		return "", fmt.Errorf("fetching terraform state: %w", err)
	}

	// Extract the output
	value, err := tfstate.ExtractOutput(data, outputPath)
	if err != nil {
		return "", fmt.Errorf("extracting output %s: %w", outputPath, err)
	}

	return value, nil
}

// resolveGenerate generates a password based on the directive.
func (r *Resolver) resolveGenerate(value string) (string, error) {
	policy, err := generator.ParseGenerateValue(value, r.defaults)
	if err != nil {
		return "", fmt.Errorf("parsing generate directive: %w", err)
	}

	password, err := generator.Generate(policy)
	if err != nil {
		return "", fmt.Errorf("generating password: %w", err)
	}

	return password, nil
}
