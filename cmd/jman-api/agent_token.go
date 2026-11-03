package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/spf13/cobra"
)

var (
	agentTokenServer      string
	agentTokenDescription string
)

var agentTokenCmd = &cobra.Command{
	Use:   "agent-token",
	Short: "Create a jman-agent token for a server",
	Long: `Creates a jman-agent token for a server in api.db and prints it to stdout.
It is the scripted equivalent of creating one on the jman-ui Settings page.

The token is shown only once. The server is given by its UUID or exact name
and must be a physical (not logical) server.`,
	Args: cobra.NoArgs,
	RunE: runAgentToken,
}

func init() {
	agentTokenCmd.Flags().StringVar(&agentTokenServer, "server", "", "server UUID or name (required)")
	agentTokenCmd.Flags().StringVar(&agentTokenDescription, "description", "", "description shown in the token list")
	_ = agentTokenCmd.MarkFlagRequired("server")
	rootCmd.AddCommand(agentTokenCmd)
}

func runAgentToken(cmd *cobra.Command, args []string) error {
	servers, err := db.ListManagedServers()
	if err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}
	want := strings.TrimSpace(agentTokenServer)
	var server *models.ManagedServer
	for i := range servers {
		if strings.EqualFold(servers[i].ID, want) || servers[i].Name == want {
			if server != nil {
				return fmt.Errorf("more than one server is named %q; pass its UUID", want)
			}
			server = &servers[i]
		}
	}
	if server == nil {
		return fmt.Errorf("server %q not found", want)
	}
	if server.IsLogical {
		return fmt.Errorf("jman-agent cannot run on a logical server")
	}

	_, plaintext, err := db.CreateAgentToken(server.ID, server.Name, agentTokenDescription, "jman-api agent-token")
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created an agent token for %s (%s).\n", server.Name, server.ID)
	fmt.Println(plaintext)
	return nil
}
