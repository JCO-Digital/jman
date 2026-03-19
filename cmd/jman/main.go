package main

import (
	"fmt"
	"os"

	"github.com/JCO-Digital/jman/internal/commands"
	"github.com/JCO-Digital/jman/internal/config"
)

func main() {
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing config: %v\n", err)
		os.Exit(1)
	}

	if err := commands.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
