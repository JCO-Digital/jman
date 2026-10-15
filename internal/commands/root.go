package commands

import (
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/mirror"
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
	Long:  `jman is a command-line utility for managing the WordPress sites in jman-api. Run "jman login <api-url>" to get started.`,
	Args:  cobra.NoArgs,
	// main prints errors, once.
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// The arguments were valid, so a later error isn't a usage mistake.
		cmd.SilenceUsage = true

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

		if !versionGateExempt[topLevelName(cmd)] {
			if err := mirror.CheckVersion(); err != nil {
				return err
			}
			mirror.UpdateNotice()
		}
		return nil
	},
}

// versionGateExempt are the commands that still run when jman-api requires
// a newer CLI: the ones needed to update, log in and out, and get help.
var versionGateExempt = map[string]bool{
	"update":                        true,
	"login":                         true,
	"logout":                        true,
	"help":                          true,
	"completion":                    true,
	cobra.ShellCompRequestCmd:       true,
	cobra.ShellCompNoDescRequestCmd: true,
}

// topLevelName returns the name of the top-level command cmd belongs to.
func topLevelName(cmd *cobra.Command) string {
	for cmd.HasParent() && cmd.Parent().HasParent() {
		cmd = cmd.Parent()
	}
	return cmd.Name()
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
