package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/JCO-Digital/jman/internal/agent"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/verb"
)

var (
	flagVerbose    bool
	flagDebug      bool
	flagService    bool
	flagOnce       bool
	flagConfigPath string
)

func main() {
	os.Exit(run())
}

// run parses flags and executes jman-agent, returning the process exit code.
func run() int {
	flag.BoolVar(&flagVerbose, "verbose", false, "Enable verbose output")
	flag.BoolVar(&flagVerbose, "v", false, "Enable verbose output (shorthand)")
	flag.BoolVar(&flagDebug, "debug", false, "Enable debug output")
	flag.BoolVar(&flagDebug, "d", false, "Enable debug output (shorthand)")
	flag.BoolVar(&flagService, "service", false, "Run as a continuous service")
	flag.BoolVar(&flagService, "s", false, "Run as a continuous service (shorthand)")
	flag.BoolVar(&flagOnce, "once", false, "Run a single collection cycle and exit")
	flag.StringVar(&flagConfigPath, "config", "", "Path to config.toml (default: /etc/jman-agent/config.toml, or $XDG_CONFIG_HOME/jman-agent/config.toml)")
	flag.Parse()

	if flagDebug {
		verb.Set(verb.Debug)
	} else if flagVerbose {
		verb.Set(verb.Verbose)
	} else {
		verb.Set(verb.Normal)
	}

	configPath := flagConfigPath
	if configPath == "" {
		configPath = agent.DefaultConfigPath()
	}

	cfg, err := agent.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}

	if flagOnce {
		if err := agent.RunOnce(cfg); err != nil {
			verb.Printf(verb.Normal, "%v\n", err)
			return 1
		}
		return 0
	}

	if flagService {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		verb.LogPrintf(verb.Normal, "jman-agent service starting (version %s)...\n", config.AppVersion)
		if err := agent.RunService(ctx, cfg, config.AppVersion); err != nil && err != context.Canceled {
			verb.Printf(verb.Normal, "Service error: %v\n", err)
			return 1
		}
		verb.LogPrintf(verb.Normal, "jman-agent service stopped.\n")
		return 0
	}

	// Default to a single run if neither --service nor --once was given.
	if err := agent.RunOnce(cfg); err != nil {
		verb.Printf(verb.Normal, "%v\n", err)
		return 1
	}
	return 0
}
