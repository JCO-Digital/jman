package ansi

import "fmt"

const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	gray    = "\033[90m"
)

// Blue returns the string wrapped in blue ANSI codes.
func Blue(s any) string {
	return fmt.Sprintf("%s%v%s", blue, s, reset)
}

// Green returns the string wrapped in green ANSI codes.
func Green(s any) string {
	return fmt.Sprintf("%s%v%s", green, s, reset)
}

// Yellow returns the string wrapped in yellow ANSI codes.
func Yellow(s any) string {
	return fmt.Sprintf("%s%v%s", yellow, s, reset)
}

// Red returns the string wrapped in red ANSI codes.
func Red(s any) string {
	return fmt.Sprintf("%s%v%s", red, s, reset)
}

// Gray returns the string wrapped in gray ANSI codes.
func Gray(s any) string {
	return fmt.Sprintf("%s%v%s", gray, s, reset)
}

// Bold returns the string wrapped in bold ANSI codes.
func Bold(s any) string {
	return fmt.Sprintf("%s%v%s", bold, s, reset)
}

// Cyan returns the string wrapped in cyan ANSI codes.
func Cyan(s any) string {
	return fmt.Sprintf("%s%v%s", cyan, s, reset)
}

// Magenta returns the string wrapped in magenta ANSI codes.
func Magenta(s any) string {
	return fmt.Sprintf("%s%v%s", magenta, s, reset)
}
