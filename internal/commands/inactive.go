package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var inactiveCmd = &cobra.Command{
	Use:   "inactive [target]",
	Short: "List sites that don't have an active MainWP Child connection.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := ""
		if len(args) > 0 {
			target = args[0]
		}

		if target != "" {
			sites, err := search.PromptSearch(target)
			if err != nil {
				return err
			}
			if len(sites) == 0 {
				fmt.Println("No sites matched.")
				return nil
			}

			var inactive []string
			for _, site := range sites {
				fmt.Printf("\nChecking %s (%s)\n", site.Name, site.ServerName)
				active := wpcli.IsActiveMainwp(site.SSH, site.Path)
				if active {
					fmt.Println("Already active")
				} else {
					fmt.Println("Not active, or connection error.")
					inactive = append(inactive, fmt.Sprintf("%s (%s)", site.Name, site.ServerName))
				}
			}

			if len(inactive) > 0 {
				fmt.Println("\nInactive sites:")
				for _, s := range inactive {
					fmt.Println(s)
				}
			}

		} else {
			// In the original TS code, it was: promptSearch(data.target)
			// We'll enforce a target or search query since `promptSearch` requires one.
			return fmt.Errorf("please provide a target to search for")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(inactiveCmd)
}
