package commands

import (
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/verb"
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
			verb.Set(verb.Debug)
		case flagVerbose:
			verb.Set(verb.Verbose)
		case flagQuiet:
			verb.Set(verb.Quiet)
		default:
			verb.Set(verb.Normal)
		}

		verb.PrintErrorf(verb.Verbose, "Version: %s\n", config.RunData.Version)
		verb.PrintErrorf(verb.Debug, "Verbosity: %s\n", verb.Get())
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
