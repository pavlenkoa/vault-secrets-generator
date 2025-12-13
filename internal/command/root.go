package command

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

// Exit codes
const (
	ExitSuccess         = 0
	ExitConfigError     = 1
	ExitVaultError      = 2
	ExitFetchError      = 3
	ExitPartialFailure  = 4
)

var (
	// Global flags
	configFile string
	verbose    bool

	// Logger
	logger *slog.Logger
)

// rootCmd is the base command
var rootCmd = &cobra.Command{
	Use:   "vsg",
	Short: "Vault Secrets Generator",
	Long: `VSG is a CLI tool that generates and populates secrets in HashiCorp Vault
from various sources including Terraform state files, generated passwords,
and static values.

Use declarative YAML configuration for GitOps workflows.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Set up logging
		level := slog.LevelInfo
		if verbose {
			level = slog.LevelDebug
		}

		handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		})
		logger = slog.New(handler)
	},
}

// Execute runs the root command
func Execute() {
	ctx := context.Background()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(ExitConfigError)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file path (or set VSG_CONFIG)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}

// getConfigFile returns the config file path from flag or environment
func getConfigFile() (string, error) {
	if configFile != "" {
		return configFile, nil
	}

	if envConfig := os.Getenv("VSG_CONFIG"); envConfig != "" {
		return envConfig, nil
	}

	return "", fmt.Errorf("config file required: use --config or set VSG_CONFIG")
}

// getLogger returns the configured logger
func getLogger() *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}
