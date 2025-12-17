package command

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
	"github.com/pavlenkoa/vault-secrets-generator/internal/vault"
)

var (
	deleteForce   bool
	deleteHard    bool
	deleteFull    bool
	deleteKeys    string
	deleteTarget  []string
	deleteExclude []string
	deleteAll     bool
)

var deleteCmd = &cobra.Command{
	Use:   "delete [path]",
	Short: "Delete secrets from Vault",
	Long: `Delete removes secrets from Vault.

Two modes:
  Path mode:   vsg delete <path> [flags]
  Config mode: vsg delete --config <file> (--target <labels> | --all) [flags]

Path mode deletes a secret at the specified path directly.
Config mode deletes secrets defined in the config file.

Delete modes (KV v2):
  (default)  Soft delete - recoverable via 'vault kv undelete'
  --hard     Destroy version data permanently (metadata remains)
  --full     Remove all versions and metadata completely

For KV v1, all deletes are permanent.

Use --keys to delete specific keys only (writes new version without those keys).

This is a destructive operation and requires confirmation unless --force is used.`,
	Example: `  # Path mode - delete specific path
  vsg delete secret/myapp
  vsg delete secret/myapp --hard
  vsg delete secret/myapp --keys old_key,deprecated_key

  # Config mode - delete secrets from config
  vsg delete --config config.hcl --target prod-app
  vsg delete --config config.hcl --target prod-app,prod-db --hard
  vsg delete --config config.hcl --all
  vsg delete --config config.hcl --all --exclude keep-this --force`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDelete,
}

func init() {
	rootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "skip confirmation prompt")
	deleteCmd.Flags().BoolVar(&deleteHard, "hard", false, "destroy version data permanently (KV v2 only)")
	deleteCmd.Flags().BoolVar(&deleteFull, "full", false, "remove all versions and metadata (KV v2 only)")
	deleteCmd.Flags().StringVar(&deleteKeys, "keys", "", "comma-separated list of keys to delete (path mode only)")
	deleteCmd.Flags().StringSliceVarP(&deleteTarget, "target", "t", nil, "target secrets by label (config mode, comma-separated or repeated)")
	deleteCmd.Flags().StringSliceVarP(&deleteExclude, "exclude", "e", nil, "exclude secrets by label (config mode, comma-separated or repeated)")
	deleteCmd.Flags().BoolVar(&deleteAll, "all", false, "delete all secrets in config (config mode)")
}

func runDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	log := getLogger()

	// Determine mode: path mode vs config mode
	hasPath := len(args) > 0
	hasConfigMode := len(deleteTarget) > 0 || deleteAll || len(deleteExclude) > 0

	// Validate mutually exclusive modes
	if hasPath && hasConfigMode {
		return fmt.Errorf("cannot mix path mode and config mode flags (--target, --all, --exclude)")
	}

	if !hasPath && !hasConfigMode {
		return fmt.Errorf("either provide a path or use config mode (--config with --target or --all)")
	}

	// Config mode requires --config
	if hasConfigMode && configFile == "" {
		return fmt.Errorf("config mode requires --config flag")
	}

	// Config mode requires --target or --all
	if hasConfigMode && len(deleteTarget) == 0 && !deleteAll {
		return fmt.Errorf("config mode requires --target or --all flag")
	}

	// --keys is only for path mode
	if deleteKeys != "" && hasConfigMode {
		return fmt.Errorf("--keys flag is only available in path mode")
	}

	// --exclude requires --all
	if len(deleteExclude) > 0 && !deleteAll {
		return fmt.Errorf("--exclude requires --all flag")
	}

	// Validate delete mode flags
	if deleteHard && deleteFull {
		return fmt.Errorf("cannot use --hard and --full together")
	}

	if hasPath {
		return runDeletePathMode(ctx, log, args[0])
	}

	return runDeleteConfigMode(ctx, log)
}

// runDeletePathMode handles path-based deletion
func runDeletePathMode(ctx context.Context, log *slog.Logger, path string) error {
	// Parse path
	mount, subpath := parsePath(path)
	if subpath == "" {
		return fmt.Errorf("invalid path %q: must include mount and subpath (e.g., secret/myapp)", path)
	}

	// Get Vault address from environment
	vaultAddr := os.Getenv("VAULT_ADDR")
	if vaultAddr == "" {
		return fmt.Errorf("VAULT_ADDR environment variable is required")
	}

	// Get optional namespace
	namespace := os.Getenv("VAULT_NAMESPACE")

	log.Debug("connecting to vault", "address", vaultAddr)

	vaultClient, err := vault.NewClientFromEnv(vaultAddr, namespace)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: failed to connect to Vault:", err)
		os.Exit(ExitVaultError)
	}

	// Create KV client (auto-detect version)
	kv, err := vault.NewKVClient(vaultClient, mount, vault.KVVersionAuto)
	if err != nil {
		return fmt.Errorf("creating KV client: %w", err)
	}

	// Determine action description
	var action string
	switch {
	case deleteKeys != "":
		action = fmt.Sprintf("delete keys [%s] from", deleteKeys)
	case deleteFull:
		action = "permanently remove all versions of"
	case deleteHard:
		action = "destroy version data of"
	default:
		action = "soft delete"
	}

	// Confirm deletion
	if !deleteForce {
		fmt.Printf("The following secret will be %s:\n", action)
		fmt.Printf("  Path: %s\n", path)

		if deleteKeys != "" {
			fmt.Printf("  Keys: %s\n", deleteKeys)
		}
		if deleteFull {
			fmt.Println("  WARNING: This will remove ALL versions and metadata!")
		} else if deleteHard {
			fmt.Println("  WARNING: This will permanently destroy version data!")
		}

		if !confirmAction() {
			fmt.Println("Canceled.")
			return nil
		}
	}

	// Perform deletion
	log.Info("deleting secret", "path", path, "action", action)

	switch {
	case deleteKeys != "":
		keys := strings.Split(deleteKeys, ",")
		for i := range keys {
			keys[i] = strings.TrimSpace(keys[i])
		}
		err = kv.DeleteKeys(ctx, subpath, keys)
		if err == nil {
			fmt.Printf("Deleted keys [%s] from %s\n", deleteKeys, path)
		}

	case deleteFull:
		err = kv.DestroyMetadata(ctx, subpath)
		if err == nil {
			fmt.Printf("Permanently removed all versions of %s\n", path)
		}

	case deleteHard:
		err = kv.DestroyVersions(ctx, subpath)
		if err == nil {
			fmt.Printf("Destroyed version data of %s\n", path)
		}

	default:
		err = kv.Delete(ctx, subpath)
		if err == nil {
			fmt.Printf("Soft deleted %s (recoverable in KV v2)\n", path)
		}
	}

	if err != nil {
		return fmt.Errorf("deleting secret: %w", err)
	}

	return nil
}

// runDeleteConfigMode handles config-based deletion
func runDeleteConfigMode(ctx context.Context, log *slog.Logger) error {
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

	// Build list of secrets to delete
	secretsToDelete := make([]config.SecretBlock, 0, len(cfg.Secrets))
	for name, block := range cfg.Secrets {
		// If using --target, only include targeted secrets
		if len(deleteTarget) > 0 {
			targeted := false
			for _, t := range deleteTarget {
				if t == name {
					targeted = true
					break
				}
			}
			if !targeted {
				continue
			}
		}

		// If using --all with --exclude, skip excluded secrets
		if deleteAll && len(deleteExclude) > 0 {
			excluded := false
			for _, e := range deleteExclude {
				if e == name {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		secretsToDelete = append(secretsToDelete, block)
	}

	if len(secretsToDelete) == 0 {
		fmt.Println("No secrets to delete.")
		return nil
	}

	// Determine action description
	var action string
	switch {
	case deleteFull:
		action = "permanently remove all versions of"
	case deleteHard:
		action = "destroy version data of"
	default:
		action = "soft delete"
	}

	// Confirm deletion
	if !deleteForce {
		fmt.Printf("The following %d secret(s) will be %s:\n", len(secretsToDelete), action)
		for _, block := range secretsToDelete {
			fmt.Printf("  - %s (%s)\n", block.Name, block.FullPath())
		}

		if deleteFull {
			fmt.Println("\nWARNING: This will remove ALL versions and metadata!")
		} else if deleteHard {
			fmt.Println("\nWARNING: This will permanently destroy version data!")
		}

		if !confirmAction() {
			fmt.Println("Canceled.")
			return nil
		}
	}

	// Get Vault address from config or environment
	vaultAddr := cfg.Vault.Address
	if vaultAddr == "" {
		vaultAddr = os.Getenv("VAULT_ADDR")
	}
	if vaultAddr == "" {
		return fmt.Errorf("VAULT_ADDR not set in config or environment")
	}

	namespace := cfg.Vault.Namespace
	if namespace == "" {
		namespace = os.Getenv("VAULT_NAMESPACE")
	}

	log.Debug("connecting to vault", "address", vaultAddr)

	vaultClient, err := vault.NewClientFromEnv(vaultAddr, namespace)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: failed to connect to Vault:", err)
		os.Exit(ExitVaultError)
	}

	// Delete each secret
	var errors []error
	for _, block := range secretsToDelete {
		version := vault.KVVersion(block.Version)
		kv, err := vault.NewKVClient(vaultClient, block.Mount, version)
		if err != nil {
			errors = append(errors, fmt.Errorf("%s: creating KV client: %w", block.Name, err))
			continue
		}

		log.Info("deleting secret", "name", block.Name, "path", block.FullPath(), "action", action)

		switch {
		case deleteFull:
			err = kv.DestroyMetadata(ctx, block.Path)
			if err == nil {
				fmt.Printf("Permanently removed all versions of %s (%s)\n", block.Name, block.FullPath())
			}

		case deleteHard:
			err = kv.DestroyVersions(ctx, block.Path)
			if err == nil {
				fmt.Printf("Destroyed version data of %s (%s)\n", block.Name, block.FullPath())
			}

		default:
			err = kv.Delete(ctx, block.Path)
			if err == nil {
				fmt.Printf("Soft deleted %s (%s)\n", block.Name, block.FullPath())
			}
		}

		if err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", block.Name, err))
		}
	}

	// Report errors
	if len(errors) > 0 {
		fmt.Fprintln(os.Stderr, "\nErrors:")
		for _, e := range errors {
			fmt.Fprintln(os.Stderr, " -", e.Error())
		}
		os.Exit(ExitPartialFailure)
	}

	return nil
}

// confirmAction prompts the user for confirmation
func confirmAction() bool {
	fmt.Print("\nAre you sure? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}
