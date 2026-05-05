package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

var (
	useraddUsername    string
	useraddDisplayName string
	useraddLevel       string
)

var useraddCmd = &cobra.Command{
	Use:   "useradd",
	Short: "Add a new user to users.toml",
	Long: `Prompts for a password interactively and appends a new user entry to users.toml.

If users.toml does not yet exist, a new file will be created with a
randomly generated JWT secret.`,
	RunE: runUseradd,
}

func init() {
	useraddCmd.Flags().StringVar(&useraddUsername, "username", "", "username for the new user (required)")
	useraddCmd.Flags().StringVar(&useraddDisplayName, "display-name", "", "display name for the new user (required)")
	useraddCmd.Flags().StringVar(&useraddLevel, "level", "basic", "user level (basic, edit, execute)")
	_ = useraddCmd.MarkFlagRequired("username")
	_ = useraddCmd.MarkFlagRequired("display-name")
	rootCmd.AddCommand(useraddCmd)
}

func runUseradd(cmd *cobra.Command, args []string) error {
	configDir := config.RunData.ConfigDir
	filePath := filepath.Join(configDir, "users.toml")

	var cfg config.UsersConfig

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// No existing file — generate a fresh config with a random JWT secret.
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return fmt.Errorf("failed to generate JWT secret: %w", err)
		}
		cfg = config.UsersConfig{
			JWTSecret:          hex.EncodeToString(secret),
			TokenLifetimeHours: 24,
		}
		fmt.Fprintf(os.Stderr, "No users.toml found — a new file will be created at %s\n", filePath)
	} else {
		loaded, err := config.LoadUsersConfig(configDir)
		if err != nil {
			return fmt.Errorf("failed to load existing users config: %w", err)
		}
		cfg = loaded
	}

	// Reject duplicate usernames.
	if config.FindUser(&cfg, useraddUsername) != nil {
		return fmt.Errorf("user %q already exists in users.toml", useraddUsername)
	}

	// Prompt for password with echo disabled.
	fmt.Fprint(os.Stderr, "Password: ")
	pw1, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	fmt.Fprint(os.Stderr, "Confirm password: ")
	pw2, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to read password confirmation: %w", err)
	}

	if string(pw1) != string(pw2) {
		return fmt.Errorf("passwords do not match")
	}

	if len(pw1) == 0 {
		return fmt.Errorf("password cannot be empty")
	}

	hash, err := bcrypt.GenerateFromPassword(pw1, 12)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	level := config.UserLevel(useraddLevel)
	switch level {
	case config.LevelBasic, config.LevelEdit, config.LevelExecute:
		// Valid
	case "":
		level = config.LevelBasic
	default:
		return fmt.Errorf("invalid level %q: must be basic, edit, or execute", useraddLevel)
	}

	cfg.Users = append(cfg.Users, config.UserEntry{
		Username:     useraddUsername,
		PasswordHash: string(hash),
		DisplayName:  useraddDisplayName,
		TOTPSecret:   "",
		Level:        level,
	})

	if err := config.SaveUsersConfig(configDir, cfg); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "User %q added successfully to %s\n", useraddUsername, filePath)
	return nil
}
