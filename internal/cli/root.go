package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// ErrConflicts is returned by pull/push when the run completed but left
// conflict markers in one or more files. main maps this to exit code 1.
var ErrConflicts = errors.New("conflicts")

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

	root.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "config file (default: .nodin.toml, walking up to ~/.config/nodin/config.toml)")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	root.AddCommand(
		newInitCmd(),
		newDoctorCmd(),
		newPullCmd(),
		newPushCmd(),
		newStatusCmd(),
		newDiffCmd(),
	)

	return root
}
