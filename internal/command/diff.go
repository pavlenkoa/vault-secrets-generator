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
	diffOutput string
	diffOnly   string
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show differences between current and desired state",
	Long: `Diff compares the current secrets in Vault with the desired state
defined in the configuration file and shows what changes would be made.

This is equivalent to 'apply --dry-run' but with more output options.`,
	Example: `  # Show diff in text format
  vsg diff --config config.yaml

  # Show diff in JSON format
  vsg diff --config config.yaml --output json

  # Show diff for a specific block
  vsg diff --config config.yaml --only main`,
	RunE: runDiff,
}

func init() {
	rootCmd.AddCommand(diffCmd)

	diffCmd.Flags().StringVarP(&diffOutput, "output", "o", "text", "output format: text, json")
	diffCmd.Flags().StringVar(&diffOnly, "only", "", "only process this secret block")
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

	cfg, err := config.Load(cfgPath)
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
	eng := engine.NewEngine(vaultClient, registry, cfg.Defaults.Generate, log)

	// Run plan (dry-run)
	opts := engine.Options{
		DryRun: true,
		Only:   diffOnly,
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
