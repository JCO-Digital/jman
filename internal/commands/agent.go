package commands

import (
	"fmt"
	"strconv"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

var agentTokenDescription string

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage jman-agent server tokens",
}

var agentTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Create, list, and revoke per-server jman-agent tokens",
}

var agentTokenCreateCmd = &cobra.Command{
	Use:   "create <server-id-or-name>",
	Short: "Create a new agent token for a server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		server, err := resolveServer(args[0])
		if err != nil {
			return err
		}

		token, plaintext, err := db.CreateAgentToken(server.ID, server.Name, agentTokenDescription, "cli")
		if err != nil {
			return fmt.Errorf("failed to create agent token: %w", err)
		}

		fmt.Printf("Created agent token %d for server %s (%s).\n", token.ID, verb.Blue(server.Name), verb.Gray(strconv.Itoa(server.ID)))
		fmt.Printf("\n%s\n\n", verb.Yellow("This token will not be shown again. Copy it into the agent's config.toml now:"))
		fmt.Println(plaintext)
		return nil
	},
}

var agentTokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agent tokens",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		tokens, err := db.ListAgentTokens()
		if err != nil {
			return fmt.Errorf("failed to list agent tokens: %w", err)
		}
		if len(tokens) == 0 {
			fmt.Println("No agent tokens found.")
			return nil
		}
		for _, t := range tokens {
			status := verb.Green("active")
			if t.Revoked {
				status = verb.Red("revoked")
			}
			lastSeen := "never"
			if t.LastSeenAt != nil {
				lastSeen = *t.LastSeenAt
			}
			fmt.Printf("#%d  %s (server %d)  prefix=%s  %s  last_seen=%s\n", t.ID, t.ServerName, t.ServerID, t.TokenPrefix, status, lastSeen)
		}
		return nil
	},
}

var agentTokenRevokeCmd = &cobra.Command{
	Use:   "revoke <id>",
	Short: "Revoke an agent token",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid token ID: %s", args[0])
		}
		if err := db.RevokeAgentToken(id); err != nil {
			return err
		}
		fmt.Printf("Revoked agent token #%d.\n", id)
		return nil
	},
}

// resolveServer finds a cached server by numeric ID or exact name match.
func resolveServer(idOrName string) (*models.Server, error) {
	servers, err := cache.GetFastCachedServers()
	if err != nil {
		return nil, fmt.Errorf("failed to load servers: %w", err)
	}

	if id, err := strconv.Atoi(idOrName); err == nil {
		for _, s := range servers {
			if s.ID == id {
				return &s, nil
			}
		}
		return nil, fmt.Errorf("no server found with ID %d", id)
	}

	for _, s := range servers {
		if s.Name == idOrName {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("no server found matching %q", idOrName)
}

func init() {
	agentTokenCreateCmd.Flags().StringVar(&agentTokenDescription, "description", "", "Optional description for the token")
	agentTokenCmd.AddCommand(agentTokenCreateCmd, agentTokenListCmd, agentTokenRevokeCmd)
	agentCmd.AddCommand(agentTokenCmd)
	rootCmd.AddCommand(agentCmd)
}
