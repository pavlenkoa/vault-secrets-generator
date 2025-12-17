package command

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
	"github.com/pavlenkoa/vault-secrets-generator/internal/engine"
	"github.com/pavlenkoa/vault-secrets-generator/internal/vault"
)

var (
	diffOutput  string
	diffTarget  []string
	diffExclude []string
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show differences between current and desired state",
	Long: `Diff compares the current secrets in Vault with the desired state
defined in the configuration file and shows what changes would be made.

This is equivalent to 'apply --dry-run' but with more output options.
Use --target to diff specific secrets by label.
Use --exclude to skip specific secrets by label.`,
	Example: `  # Show diff in text format
  vsg diff --config config.hcl

  # Show diff with variable override
  vsg diff --config config.hcl --var ENV=prod

  # Show diff in JSON format
  vsg diff --config config.hcl --output json

  # Diff specific secrets by label
  vsg diff --config config.hcl --target prod-app
  vsg diff --config config.hcl -t prod-app -t prod-db

  # Diff all except specific secrets
  vsg diff --config config.hcl --exclude broken-secret`,
	RunE: runDiff,
}

func init() {
	rootCmd.AddCommand(diffCmd)

	diffCmd.Flags().StringVarP(&diffOutput, "output", "o", "text", "output format: text, json")
	diffCmd.Flags().StringSliceVarP(&diffTarget, "target", "t", nil, "target specific secrets by label (comma-separated or repeated)")
	diffCmd.Flags().StringSliceVarP(&diffExclude, "exclude", "e", nil, "exclude secrets by label (comma-separated or repeated)")
}

func runDiff(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	log := getLogger()

	// Load config
	cfgPath, err := getConfigFile()
	if err != nil {
		return err
	}

	log.Debug("loading config", "path", cfgPath)

	vars := parseVars()
	cfg, err := config.Load(cfgPath, vars)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Create Vault client
	log.Debug("connecting to vault", "address", cfg.Vault.Address)

	vaultClient, err := vault.NewClient(cfg.Vault)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: failed to connect to Vault:", err)
		os.Exit(ExitVaultError)
	}

	// Check Vault health
	if err := vaultClient.CheckHealth(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Error: Vault health check failed:", err)
		os.Exit(ExitVaultError)
	}

	// Set up fetchers
	registry := setupFetchers(ctx)

	// Create engine
	eng := engine.NewEngine(vaultClient, registry, cfg.Defaults, log)

	// Run plan (dry-run)
	opts := engine.Options{
		DryRun:  true,
		Target:  diffTarget,
		Exclude: diffExclude,
	}

	result, err := eng.Plan(ctx, cfg, opts)
	if err != nil {
		return err
	}

	// Output diff
	switch diffOutput {
	case "json":
		jsonOutput, err := result.Diff.ToJSON()
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(jsonOutput)

	case "text":
		if verbose {
			fmt.Println(engine.FormatDiffVerbose(result.Diff))
		} else {
			fmt.Println(engine.FormatDiff(result.Diff))
		}

	default:
		return fmt.Errorf("unknown output format: %s (use 'text' or 'json')", diffOutput)
	}

	// Handle errors
	if len(result.Errors) > 0 {
		fmt.Fprintln(os.Stderr, "\nErrors:")
		for _, e := range result.Errors {
			fmt.Fprintln(os.Stderr, " -", e.Error())
		}
		os.Exit(ExitPartialFailure)
	}

	// Exit with non-zero if there are changes (useful for CI)
	if result.Diff.HasChanges() {
		os.Exit(1)
	}

	return nil
}
