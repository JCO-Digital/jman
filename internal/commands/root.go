package commands

import (
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/spf13/cobra"
)

var (
	flagQuiet   bool
	flagVerbose bool
	flagDebug   bool
)

var rootCmd = &cobra.Command{
	Use:   "jman",
	Short: "A CLI tool for managing WordPress projects",
	Long:  `jman is a command-line utility designed to manage WordPress sites hosted on SpinupWP.`,
	Args:  cobra.NoArgs,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

		switch {
		case flagDebug:
			verbosity.Set(verbosity.Debug)
		case flagVerbose:
			verbosity.Set(verbosity.Verbose)
		case flagQuiet:
			verbosity.Set(verbosity.Quiet)
		default:
			verbosity.Set(verbosity.Normal)
		}

		verbosity.PrintErrorf(verbosity.Verbose, "Version: %s\n", config.RunData.Version)
		verbosity.PrintErrorf(verbosity.Debug, "Verbosity: %s\n", verbosity.Get())
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "suppress all non-essential output")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "enable additional informational output")
	rootCmd.PersistentFlags().BoolVarP(&flagDebug, "debug", "d", false, "enable detailed debug output")
	rootCmd.MarkFlagsMutuallyExclusive("quiet", "verbose", "debug")

	rootCmd.Version = config.AppVersion

	// Hide the default completion command
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
}
