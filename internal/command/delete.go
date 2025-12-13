package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
	"github.com/pavlenkoa/vault-secrets-generator/internal/vault"
)

var (
	deleteAll         bool
	deleteForce       bool
	deletePermanently bool
)

var deleteCmd = &cobra.Command{
	Use:   "delete [block-name]",
	Short: "Delete secrets from Vault",
	Long: `Delete removes secrets from Vault that are defined in the configuration.

If a block name is provided, only that block's secrets are deleted.
If --all is specified, all blocks defined in the config are deleted.

By default, performs a soft delete (KV v2 keeps version history).
Use --permanently to remove secrets and all version history.

This is a destructive operation and requires confirmation unless --force is used.`,
	Example: `  # Delete a specific block's secrets (soft delete)
  vsg delete main --config config.yaml

  # Permanently delete (removes all versions)
  vsg delete main --config config.yaml --permanently

  # Delete all secrets defined in config
  vsg delete --all --config config.yaml --permanently

  # Delete without confirmation
  vsg delete main --config config.yaml --permanently --force`,
	RunE: runDelete,
}

func init() {
	rootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().BoolVar(&deleteAll, "all", false, "delete all blocks defined in config")
	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "skip confirmation prompt")
	deleteCmd.Flags().BoolVar(&deletePermanently, "permanently", false, "permanently delete (removes all versions)")
}

func runDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	log := getLogger()

	// Validate args
	if len(args) == 0 && !deleteAll {
		return fmt.Errorf("specify a block name or use --all")
	}
	if len(args) > 0 && deleteAll {
		return fmt.Errorf("cannot specify block name with --all")
	}

	blockName := ""
	if len(args) > 0 {
		blockName = args[0]
	}

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

	// Validate block exists
	if blockName != "" {
		if _, ok := cfg.Secrets[blockName]; !ok {
			return fmt.Errorf("block %q not found in config", blockName)
		}
	}

	// Create Vault client
	log.Debug("connecting to vault", "address", cfg.Vault.Address)

	vaultClient, err := vault.NewClient(cfg.Vault)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: failed to connect to Vault:", err)
		os.Exit(ExitVaultError)
	}

	// Collect blocks to delete
	var blocksToDelete []config.SecretBlock
	var blockNames []string

	if deleteAll {
		for name, block := range cfg.Secrets {
			blocksToDelete = append(blocksToDelete, block)
			blockNames = append(blockNames, name)
		}
	} else {
		blocksToDelete = append(blocksToDelete, cfg.Secrets[blockName])
		blockNames = append(blockNames, blockName)
	}

	// Confirm deletion
	if !deleteForce {
		action := "deleted"
		if deletePermanently {
			action = "permanently deleted"
		}
		fmt.Printf("The following secrets will be %s:\n", action)
		for i, block := range blocksToDelete {
			fmt.Printf("  - %s (%s)\n", blockNames[i], block.Path)
		}
		fmt.Print("\nAre you sure? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Delete secrets
	var errors []error
	for i, block := range blocksToDelete {
		mount, subpath := parsePath(block.Path)
		version := vault.KVVersion(block.Version)

		kv, err := vault.NewKVClient(vaultClient, mount, version)
		if err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", blockNames[i], err))
			continue
		}

		action := "deleting"
		if deletePermanently {
			action = "permanently deleting"
		}
		log.Info(action+" secret", "block", blockNames[i], "path", block.Path)

		if deletePermanently {
			err = kv.Destroy(ctx, subpath)
		} else {
			err = kv.Delete(ctx, subpath)
		}
		if err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", blockNames[i], err))
			continue
		}

		result := "Deleted"
		if deletePermanently {
			result = "Permanently deleted"
		}
		fmt.Printf("%s: %s (%s)\n", result, blockNames[i], block.Path)
	}

	if len(errors) > 0 {
		fmt.Fprintln(os.Stderr, "\nErrors:")
		for _, e := range errors {
			fmt.Fprintln(os.Stderr, " -", e.Error())
		}
		os.Exit(ExitPartialFailure)
	}

	return nil
}
