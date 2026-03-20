package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

var hashpwCmd = &cobra.Command{
	Use:   "hashpw",
	Short: "Hash a password with bcrypt",
	Long:  "Prompts for a password interactively and prints the bcrypt hash to stdout.\nUseful for manually constructing users.toml entries.",
	Args:  cobra.NoArgs,
	RunE:  runHashpw,
}

func init() {
	rootCmd.AddCommand(hashpwCmd)
}

func runHashpw(cmd *cobra.Command, args []string) error {
	fmt.Fprint(os.Stderr, "Password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	if len(pw) == 0 {
		return fmt.Errorf("password cannot be empty")
	}

	hash, err := bcrypt.GenerateFromPassword(pw, 12)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	fmt.Println(string(hash))
	return nil
}
