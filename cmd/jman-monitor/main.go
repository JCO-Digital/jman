package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/monitor"
	"github.com/JCO-Digital/jman/internal/verb"
)

var (
	flagVerbose bool
	flagDebug   bool
	flagService bool
)

func main() {
	os.Exit(run())
}

// run parses flags and executes jman-monitor, returning the process exit
// code. Unlike calling os.Exit directly, this lets deferred cleanup (closing
// the database, stopping the signal handler) run before the process exits.
func run() int {
	// Parse flags
	flag.BoolVar(&flagVerbose, "verbose", false, "Enable verbose output")
	flag.BoolVar(&flagVerbose, "v", false, "Enable verbose output (shorthand)")
	flag.BoolVar(&flagDebug, "debug", false, "Enable debug output")
	flag.BoolVar(&flagDebug, "d", false, "Enable debug output (shorthand)")
	flag.BoolVar(&flagService, "service", false, "Run as a continuous service")
	flag.BoolVar(&flagService, "s", false, "Run as a continuous service (shorthand)")
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
		return 1
	}

	// Initialize database
	if err := db.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing database: %v\n", err)
		return 1
	}
	defer db.Close()

	if flagService {
		// Run as service with graceful shutdown
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		verb.LogPrintf(verb.Normal, "jman-monitor service starting...\n")
		if err := monitor.RunService(ctx); err != nil && err != context.Canceled {
			verb.Printf(verb.Normal, "Service error: %v\n", err)
			return 1
		}
		verb.LogPrintf(verb.Normal, "jman-monitor service stopped.\n")
		return 0
	}

	// Run once (legacy/manual mode)
	if err := monitor.RunOnce(); err != nil {
		verb.Printf(verb.Normal, "%v\n", err)
		return 1
	}
	return 0
}
