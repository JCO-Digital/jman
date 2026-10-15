package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// usageArgs validates a command's positional arguments against its Use
// line, where <name> is required, [name] is optional, and a trailing "..."
// inside either allows any number. A mismatch is reported with the usage
// line instead of cobra's generic "accepts N arg(s)".
func usageArgs(cmd *cobra.Command, args []string) error {
	required, optional, variadic := parseUse(cmd.Use)
	switch {
	case len(args) < len(required):
		return usageError(cmd, "missing "+strings.Join(required[len(args):], " and "))
	case !variadic && len(args) > len(required)+len(optional):
		return usageError(cmd, fmt.Sprintf("too many arguments: %q", strings.Join(args[len(required)+len(optional):], " ")))
	}
	return nil
}

// parseUse splits the argument part of a Use line into required and
// optional argument names, and reports whether the last one repeats.
func parseUse(use string) (required, optional []string, variadic bool) {
	fields := strings.Fields(use)
	if len(fields) == 0 {
		return nil, nil, false
	}
	for _, f := range fields[1:] {
		variadic = strings.Contains(f, "...")
		switch {
		case strings.HasPrefix(f, "<"):
			required = append(required, f)
		case strings.HasPrefix(f, "["):
			optional = append(optional, f)
		}
	}
	return required, optional, variadic
}

// usageError returns err with the command's usage line and where to find
// more help.
func usageError(cmd *cobra.Command, problem string) error {
	return fmt.Errorf("%s\n%s", problem, usageHint(cmd))
}

func usageHint(cmd *cobra.Command) string {
	return fmt.Sprintf("Usage: %s\nRun \"%s --help\" for details.", cmd.UseLine(), cmd.CommandPath())
}
