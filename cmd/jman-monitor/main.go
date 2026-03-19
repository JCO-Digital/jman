package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/monitor"
	"github.com/JCO-Digital/jman/internal/verb"
)

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
		verb.Set(verb.Debug)
	} else if flagVerbose {
		verb.Set(verb.Verbose)
	} else {
		verb.Set(verb.Normal)
	}

	// Initialize config
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing config: %v\n", err)
		os.Exit(1)
	}

	// Run monitor
	if err := monitor.Run(); err != nil {
		verb.Printf(verb.Normal, "%v\n", err)
		os.Exit(1)
	}
}
