package command

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
	"github.com/pavlenkoa/vault-secrets-generator/internal/engine"
	"github.com/pavlenkoa/vault-secrets-generator/internal/fetcher"
	"github.com/pavlenkoa/vault-secrets-generator/internal/vault"
)

var (
	applyDryRun bool
	applyForce  bool
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply secrets to Vault",
	Long: `Apply reads the configuration file and syncs secrets to Vault.

For each secret defined in the configuration:
- Terraform state references are fetched and outputs extracted
- Generated passwords are created (only if they don't exist, unless --force)
- Static values are used as-is

Use --dry-run to see what changes would be made without applying them.`,
	Example: `  # Apply all secrets
  vsg apply --config config.yaml

  # Dry-run to see changes
  vsg apply --config config.yaml --dry-run

  # Force regeneration of generated secrets
  vsg apply --config config.yaml --force`,
	RunE: runApply,
}

func init() {
	rootCmd.AddCommand(applyCmd)

	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "show what would be done without making changes")
	applyCmd.Flags().BoolVar(&applyForce, "force", false, "force regeneration of generated secrets")
}

func runApply(cmd *cobra.Command, args []string) error {
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

	// Run reconciliation
	opts := engine.Options{
		DryRun: applyDryRun,
		Force:  applyForce,
	}

	result, err := eng.Reconcile(ctx, cfg, opts)
	if err != nil {
		return err
	}

	// Print diff
	if result.Diff.HasChanges() || verbose {
		fmt.Println(engine.FormatDiff(result.Diff))
	} else {
		fmt.Println("No changes required.")
	}

	// Handle errors
	if len(result.Errors) > 0 {
		fmt.Fprintln(os.Stderr, "\nErrors:")
		for _, e := range result.Errors {
			fmt.Fprintln(os.Stderr, " -", e.Error())
		}
		os.Exit(ExitPartialFailure)
	}

	// Report result
	if applyDryRun {
		adds, updates, _, _ := result.Diff.Summary()
		if adds+updates > 0 {
			fmt.Printf("\nDry-run complete. %d changes would be made.\n", adds+updates)
		}
	} else if result.Applied {
		fmt.Println("\nSecrets applied successfully.")
	}

	return nil
}

// setupFetchers creates and configures the fetcher registry
func setupFetchers(ctx context.Context) *fetcher.Registry {
	registry := fetcher.NewRegistry()

	// Local file fetcher
	registry.Register(fetcher.NewLocalFetcher())

	// S3 fetcher (optional - only if we might need it)
	s3Fetcher, err := fetcher.NewS3Fetcher(ctx)
	if err != nil {
		// Log but don't fail - S3 might not be needed
		getLogger().Debug("S3 fetcher not available", "error", err)
	} else {
		registry.Register(s3Fetcher)
	}

	return registry
}
