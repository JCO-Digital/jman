//go:build !linux

package agent

// lowerPriority does nothing outside Linux, where the agent doesn't run.
func lowerPriority() error { return nil }
