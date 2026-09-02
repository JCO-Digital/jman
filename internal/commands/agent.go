package commands

import (
	"fmt"
	"os"
	"strconv"

	"github.com/JCO-Digital/jman/internal/apiclient"
	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var agentTokenDescription string

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage jman-agent server tokens",
	Long: `Manage jman-agent server tokens via jman-api.

Agent tokens live in jman-api's own database, so these commands talk to
jman-api over HTTP rather than reading a local database file. Configure the
target with apiURL/apiUsername in config.toml (see README.md); you'll be
prompted for your jman-api password (and TOTP code, if configured).`,
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

		client, err := newAPIClient()
		if err != nil {
			return err
		}

		token, plaintext, err := client.CreateAgentToken(server.ID, server.Name, agentTokenDescription)
		if err != nil {
			return fmt.Errorf("failed to create agent token: %w", err)
		}
		saveAPISession(client)

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
		client, err := newAPIClient()
		if err != nil {
			return err
		}

		tokens, err := client.ListAgentTokens()
		if err != nil {
			return fmt.Errorf("failed to list agent tokens: %w", err)
		}
		saveAPISession(client)

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
			version := "unknown"
			if t.AgentVersion != nil {
				version = *t.AgentVersion
			}
			fmt.Printf("#%d  %s (server %d)  prefix=%s  %s  last_seen=%s  version=%s\n", t.ID, t.ServerName, t.ServerID, t.TokenPrefix, status, lastSeen, version)
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

		client, err := newAPIClient()
		if err != nil {
			return err
		}

		if err := client.RevokeAgentToken(id); err != nil {
			return err
		}
		saveAPISession(client)

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

// newAPIClient builds an apiclient.Client authenticated against jman-api,
// reusing a cached session (see apiclient.LoadSession) when possible and
// otherwise prompting interactively for a password (and TOTP code, if
// configured for the user).
func newAPIClient() (*apiclient.Client, error) {
	if config.Cfg.APIURL == "" {
		return nil, fmt.Errorf("apiURL is not configured — set it in config.toml (see README.md)")
	}
	if config.Cfg.APIUsername == "" {
		return nil, fmt.Errorf("apiUsername is not configured — set it in config.toml (see README.md)")
	}

	client := apiclient.New(config.Cfg.APIURL)

	if token, expiresAt, ok := apiclient.LoadSession(config.RunData.ConfigDir, config.Cfg.APIURL, config.Cfg.APIUsername); ok {
		client.SetToken(token, expiresAt)
		return client, nil
	}

	fmt.Printf("jman-api password for %s: ", config.Cfg.APIUsername)
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return nil, fmt.Errorf("failed to read password: %w", err)
	}

	fmt.Print("TOTP code (leave blank if not required): ")
	var totp string
	fmt.Scanln(&totp)

	if err := client.Login(config.Cfg.APIUsername, string(pw), totp); err != nil {
		return nil, fmt.Errorf("failed to authenticate with jman-api: %w", err)
	}

	return client, nil
}

// saveAPISession persists the client's current token so the next
// `jman agent token ...` invocation can skip the interactive login.
// Failures are non-fatal — worst case, the next invocation re-prompts.
func saveAPISession(client *apiclient.Client) {
	token, expiresAt := client.Token()
	_ = apiclient.SaveSession(config.RunData.ConfigDir, config.Cfg.APIURL, config.Cfg.APIUsername, token, expiresAt)
}

func init() {
	agentTokenCreateCmd.Flags().StringVar(&agentTokenDescription, "description", "", "Optional description for the token")
	agentTokenCmd.AddCommand(agentTokenCreateCmd, agentTokenListCmd, agentTokenRevokeCmd)
	agentCmd.AddCommand(agentTokenCmd)
	rootCmd.AddCommand(agentCmd)
}
