package main

import (
	"fmt"
	"os"

	"github.com/JCO-Digital/jman/internal/commands"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
)

func main() {
	os.Exit(run())
}

// run executes the CLI and returns the process exit code. Unlike calling
// os.Exit directly from main, this lets deferred cleanup (closing the
// database) run before the process exits on an error path.
func run() int {
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing config: %v\n", err)
		return 1
	}

	if err := db.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing database: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := commands.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}
