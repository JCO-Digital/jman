package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/monitor"
	"github.com/JCO-Digital/jman/internal/verbosity"
)

// Version is injected by the build flags
var Version = "dev"

var (
	flagVerbose bool
	flagDebug   bool
)

func main() {
	// Parse flags
	flag.BoolVar(&flagVerbose, "verbose", false, "Enable verbose output")
	flag.BoolVar(&flagVerbose, "v", false, "Enable verbose output (shorthand)")
	flag.BoolVar(&flagDebug, "debug", false, "Enable debug output")
	flag.BoolVar(&flagDebug, "d", false, "Enable debug output (shorthand)")
	flag.Parse()

	// Set verbosity
	if flagDebug {
		verbosity.Set(verbosity.Debug)
	} else if flagVerbose {
		verbosity.Set(verbosity.Verbose)
	} else {
		verbosity.Set(verbosity.Normal)
	}

	// Initialize config
	if err := config.Init(Version); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing config: %v\n", err)
		os.Exit(1)
	}

	// Run monitor
	if err := monitor.Run(); err != nil {
		verbosity.Printf(verbosity.Normal, "%v\n", err)
		os.Exit(1)
	}
}
