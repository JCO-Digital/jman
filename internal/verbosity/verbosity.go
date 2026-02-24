package verbosity

import (
	"fmt"
	"os"
)

// Level represents the verbosity level of the application.
type Level int

const (
	// Quiet suppresses all non-essential output.
	Quiet Level = iota
	// Normal is the default verbosity level.
	Normal
	// Verbose enables additional informational output.
	Verbose
	// Debug enables detailed debug output.
	Debug
)

// String returns the human-readable name of the verbosity level.
func (l Level) String() string {
	switch l {
	case Quiet:
		return "quiet"
	case Normal:
		return "normal"
	case Verbose:
		return "verbose"
	case Debug:
		return "debug"
	default:
		return "unknown"
	}
}

// current holds the active verbosity level for the application.
var current = Normal

// Set updates the global verbosity level.
func Set(l Level) {
	current = l
}

// Get returns the current global verbosity level.
func Get() Level {
	return current
}

// IsQuiet returns true if the verbosity level is Quiet.
func IsQuiet() bool {
	return current == Quiet
}

// IsNormal returns true if the verbosity level is at least Normal.
func IsNormal() bool {
	return current >= Normal
}

// IsVerbose returns true if the verbosity level is at least Verbose.
func IsVerbose() bool {
	return current >= Verbose
}

// IsDebug returns true if the verbosity level is Debug.
func IsDebug() bool {
	return current >= Debug
}

// Print prints a message if the current verbosity level is at least the given level.
func Print(minLevel Level, a ...any) {
	if current >= minLevel {
		fmt.Print(a...)
	}
}

// Println prints a message with a newline if the current verbosity level is at least the given level.
func Println(minLevel Level, a ...any) {
	if current >= minLevel {
		fmt.Println(a...)
	}
}

// Printf prints a formatted message if the current verbosity level is at least the given level.
func Printf(minLevel Level, format string, a ...any) {
	if current >= minLevel {
		fmt.Printf(format, a...)
	}
}

// Error prints an error message if the current verbosity level is at least the given level.
func Error(minLevel Level, a ...any) {
	if current >= minLevel {
		fmt.Fprint(os.Stderr, a...)
	}
}

// Errorln prints an error message with a newline if the current verbosity level is at least the given level.
func Errorln(minLevel Level, a ...any) {
	if current >= minLevel {
		fmt.Fprintln(os.Stderr, a...)
	}
}

// Errorf prints a formatted error message if the current verbosity level is at least the given level.
func Errorf(minLevel Level, format string, a ...any) {
	if current >= minLevel {
		fmt.Fprintf(os.Stderr, format, a...)
	}
}
