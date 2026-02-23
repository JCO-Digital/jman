package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/vuln"
	"github.com/spf13/cobra"
)

var vulnCmd = &cobra.Command{
	Use:   "vuln [target] [threshold]",
	Short: "Scan for plugin vulnerabilities across all sites.",
	RunE: func(cmd *cobra.Command, args []string) error {
		target := ""
		if len(args) > 0 {
			target = args[0]
		}

		err := vuln.ScanVulnerabilities(target, args)
		if err != nil {
			return fmt.Errorf("error scanning vulnerabilities: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(vulnCmd)
}
