package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "jman",
	Short: "A CLI tool for managing WordPress projects",
	Long:  `jman is a command-line utility designed to manage WordPress sites hosted on SpinupWP, with additional support for MainWP integration.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Version: %s\n", config.RunData.Version)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Flags can be defined here
}
