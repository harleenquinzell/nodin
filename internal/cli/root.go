package cli

import (
	"github.com/spf13/cobra"
)

// shared flag values, set by persistent flags on the root command.
var (
	cfgPath string
	verbose bool
)

// NewRootCmd builds and returns the root cobra command.
// version is embedded into --version output.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "nodin",
		Short:         "Bidirectional Notion ↔ markdown sync",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "config file (default: ~/.config/nodin/config.toml)")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	root.AddCommand(
		newInitCmd(),
		newDoctorCmd(),
		newPullCmd(),
	)

	return root
}
