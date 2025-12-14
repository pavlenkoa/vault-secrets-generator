package command

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
)

// Exit codes
const (
	ExitSuccess        = 0
	ExitConfigError    = 1
	ExitVaultError     = 2
	ExitFetchError     = 3
	ExitPartialFailure = 4
)

var (
	// Global flags
	configFile string
	verbose    bool
	cliVars    []string

	// Logger
	logger *slog.Logger
)

// rootCmd is the base command
var rootCmd = &cobra.Command{
	Use:   "vsg",
	Short: "Vault Secrets Generator",
	Long: `VSG is a CLI tool that generates and populates secrets in HashiCorp Vault
from various sources including remote files (Terraform state, configs),
generated passwords, commands, and static values.

Use declarative HCL configuration for GitOps workflows.`,
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
	rootCmd.PersistentFlags().StringArrayVar(&cliVars, "var", nil, "set variable KEY=VALUE (can be repeated)")
}

// parseVars converts --var flags to a Variables map.
// CLI vars take priority over environment variables.
func parseVars() config.Variables {
	vars := make(config.Variables)
	for _, v := range cliVars {
		if parts := strings.SplitN(v, "=", 2); len(parts) == 2 {
			vars[parts[0]] = parts[1]
		}
	}
	return vars
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

// parsePath splits a path like "kv/myapp" into mount "kv" and subpath "myapp".
func parsePath(path string) (mount, subpath string) {
	path = trimSlashes(path)
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			return path[:i], path[i+1:]
		}
	}
	return path, ""
}

func trimSlashes(s string) string {
	start := 0
	end := len(s)
	for start < end && s[start] == '/' {
		start++
	}
	for end > start && s[end-1] == '/' {
		end--
	}
	return s[start:end]
}
