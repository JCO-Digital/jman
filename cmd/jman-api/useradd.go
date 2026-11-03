package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JCO-Digital/jman/internal/api"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

var (
	useraddUsername    string
	useraddDisplayName string
	useraddLevel       string
	useraddPwStdin     bool
)

var useraddCmd = &cobra.Command{
	Use:   "useradd",
	Short: "Add a new user to users.toml",
	Long: `Prompts for a password interactively and appends a new user entry to users.toml.

If users.toml does not yet exist, a new file will be created with a
randomly generated JWT secret.

With --password-stdin, the password is read from the first line of stdin
instead, for scripts (such as make local-dev).`,
	RunE: runUseradd,
}

func init() {
	useraddCmd.Flags().StringVar(&useraddUsername, "username", "", "username for the new user (required)")
	useraddCmd.Flags().StringVar(&useraddDisplayName, "display-name", "", "display name for the new user (required)")
	useraddCmd.Flags().StringVar(&useraddLevel, "level", "basic", "user level (basic, edit, admin, execute)")
	useraddCmd.Flags().BoolVar(&useraddPwStdin, "password-stdin", false, "read the password from stdin instead of prompting")
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
		cfg = config.NewUsersConfig(hex.EncodeToString(secret))
		fmt.Fprintf(os.Stderr, "No users.toml found — a new file will be created at %s\n", filePath)
	} else {
		loaded, err := config.LoadUsersConfig(configDir)
		if err != nil {
			return fmt.Errorf("failed to load existing users config: %w", err)
		}
		cfg = loaded
	}

	useraddUsername = api.NormalizeUsername(useraddUsername)
	if err := api.ValidateUsername(useraddUsername); err != nil {
		return err
	}

	// Reject duplicate usernames.
	if config.FindUser(&cfg, useraddUsername) != nil {
		return fmt.Errorf("user %q already exists in users.toml", useraddUsername)
	}

	pw1, err := readNewPassword()
	if err != nil {
		return err
	}

	if err := api.ValidatePasswordStrength(string(pw1)); err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword(pw1, api.BcryptCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	level := config.UserLevel(useraddLevel)
	switch level {
	case config.LevelBasic, config.LevelEdit, config.LevelAdmin, config.LevelExecute:
		// Valid
	case "":
		level = config.LevelBasic
	default:
		return fmt.Errorf("invalid level %q: must be basic, edit, admin, or execute", useraddLevel)
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

// readNewPassword reads the new user's password: from the first line of
// stdin with --password-stdin, otherwise twice from the terminal with echo
// disabled.
func readNewPassword() ([]byte, error) {
	if useraddPwStdin {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return nil, fmt.Errorf("failed to read password from stdin: %w", err)
		}
		return []byte(strings.TrimRight(line, "\r\n")), nil
	}

	fmt.Fprint(os.Stderr, "Password: ")
	pw1, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to read password: %w", err)
	}

	fmt.Fprint(os.Stderr, "Confirm password: ")
	pw2, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to read password confirmation: %w", err)
	}

	if string(pw1) != string(pw2) {
		return nil, fmt.Errorf("passwords do not match")
	}
	return pw1, nil
}
