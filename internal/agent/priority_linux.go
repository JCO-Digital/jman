package agent

import (
	"fmt"
	"syscall"
)

// serviceNiceness is the CPU scheduling priority the agent runs at as a
// service, the same as its wp-cli and scan children.
const serviceNiceness = wpNiceness

// The I/O priority the agent runs at as a service: the lowest level of
// the best-effort class (see ioprio_set(2)). The idle class could starve
// a scan on a busy server until it times out.
const (
	ioprioWhoPgrp     = 2
	ioprioClassBE     = 2
	ioprioClassShift  = 13
	serviceIOPriority = ioprioClassBE<<ioprioClassShift | 7
)

// lowerPriority makes the agent and everything it starts (du, wp-cli,
// scans) yield CPU and disk to the sites it reports on. Linux keeps both
// priorities per thread, so they're set for the whole process group,
// which a service's agent leads; child processes and threads started
// later inherit them. An agent that doesn't lead its process group (run
// by hand from a script) leaves its priority alone rather than change its
// parent's.
func lowerPriority() error {
	if syscall.Getpgrp() != syscall.Getpid() {
		return fmt.Errorf("not a process group leader")
	}
	if err := syscall.Setpriority(syscall.PRIO_PGRP, 0, serviceNiceness); err != nil {
		return fmt.Errorf("failed to set the CPU priority: %w", err)
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOPRIO_SET, ioprioWhoPgrp, 0, serviceIOPriority); errno != 0 {
		return fmt.Errorf("failed to set the I/O priority: %w", errno)
	}
	return nil
}
