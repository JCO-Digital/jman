package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/pquerna/otp/totp"
	"github.com/spf13/cobra"
)

var totpSetupUsername string

var totpSetupCmd = &cobra.Command{
	Use:   "totp-setup",
	Short: "Generate and configure TOTP for a user",
	Long: `Generates a new TOTP secret for the specified user, prints the base32 secret
and an otpauth:// URI suitable for QR code generation, and updates users.toml.`,
	RunE: runTOTPSetup,
}

func init() {
	totpSetupCmd.Flags().StringVar(&totpSetupUsername, "username", "", "username to configure TOTP for (required)")
	_ = totpSetupCmd.MarkFlagRequired("username")
	rootCmd.AddCommand(totpSetupCmd)
}

func runTOTPSetup(cmd *cobra.Command, args []string) error {
	configDir := config.RunData.ConfigDir
	filePath := filepath.Join(configDir, "users.toml")

	cfg, err := config.LoadUsersConfig(configDir)
	if err != nil {
		return fmt.Errorf("failed to load users config: %w", err)
	}

	user := config.FindUser(&cfg, totpSetupUsername)
	if user == nil {
		return fmt.Errorf("user %q not found in %s", totpSetupUsername, filePath)
	}

	// Warn if the user already has a TOTP secret and ask for confirmation.
	if user.TOTPSecret != "" {
		fmt.Fprintf(os.Stderr, "WARNING: User %q already has a TOTP secret configured. This will replace it.\n", totpSetupUsername)
		fmt.Fprint(os.Stderr, "Continue? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(answer)
		if !strings.EqualFold(answer, "y") {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil
		}
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "jman-api",
		AccountName: totpSetupUsername,
	})
	if err != nil {
		return fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	// Update the user entry in the loaded config (FindUser returns a pointer
	// into the slice, so this mutates cfg.Users directly).
	user.TOTPSecret = key.Secret()

	if err := config.SaveUsersConfig(configDir, cfg); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "TOTP configured for user %q in %s\n\n", totpSetupUsername, filePath)
	fmt.Printf("Secret:  %s\n", key.Secret())
	fmt.Printf("URI:     %s\n", key.URL())
	fmt.Fprintln(os.Stderr, "\nAdd this secret to your authenticator app (Google Authenticator, Authy, etc.).")
	fmt.Fprintln(os.Stderr, "The URI above can be encoded as a QR code for easy scanning.")

	return nil
}
