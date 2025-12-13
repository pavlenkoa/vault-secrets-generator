package command

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Version information - set via ldflags at build time
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print the version, commit hash, and build date of vsg.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("vsg %s\n", Version)
		fmt.Printf("  commit:     %s\n", Commit)
		fmt.Printf("  built:      %s\n", BuildDate)
		fmt.Printf("  go version: %s\n", runtime.Version())
		fmt.Printf("  platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
