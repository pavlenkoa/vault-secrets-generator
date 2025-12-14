package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pavlenkoa/vault-secrets-generator/internal/vault"
)

var (
	deleteForce bool
	deleteHard  bool
	deleteFull  bool
	deleteKeys  string
)

var deleteCmd = &cobra.Command{
	Use:   "delete <path>",
	Short: "Delete secrets from Vault",
	Long: `Delete removes secrets from Vault at the specified path.

By default, performs a soft delete (KV v2 keeps version history, recoverable).

Delete modes (KV v2):
  (default)  Soft delete - recoverable via 'vault kv undelete'
  --hard     Destroy version data permanently (metadata remains)
  --full     Remove all versions and metadata completely

For KV v1, all deletes are permanent.

Use --keys to delete specific keys only (writes new version without those keys).

This is a destructive operation and requires confirmation unless --force is used.`,
	Example: `  # Soft delete (recoverable in KV v2)
  vsg delete secret/myapp

  # Delete specific keys only
  vsg delete secret/myapp --keys old_key,deprecated_key

  # Destroy version data permanently
  vsg delete secret/myapp --hard

  # Remove all versions and metadata
  vsg delete secret/myapp --full

  # Delete without confirmation
  vsg delete secret/myapp --full --force`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete,
}

func init() {
	rootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "skip confirmation prompt")
	deleteCmd.Flags().BoolVar(&deleteHard, "hard", false, "destroy version data permanently (KV v2 only)")
	deleteCmd.Flags().BoolVar(&deleteFull, "full", false, "remove all versions and metadata (KV v2 only)")
	deleteCmd.Flags().StringVar(&deleteKeys, "keys", "", "comma-separated list of keys to delete")
}

func runDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	log := getLogger()

	path := args[0]

	// Validate flags
	if deleteHard && deleteFull {
		return fmt.Errorf("cannot use --hard and --full together")
	}

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

		fmt.Print("\nAre you sure? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
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
