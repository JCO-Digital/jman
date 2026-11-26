package agent

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// TestLowerPriority runs lowerPriority in a child process that leads its
// own process group, as a service's agent does, and has it report the
// priorities it ended up with.
func TestLowerPriority(t *testing.T) {
	if os.Getenv("JMAN_PRIORITY_CHILD") == "1" {
		if err := lowerPriority(); err != nil {
			os.Stdout.WriteString("error " + err.Error())
			os.Exit(0)
		}
		// getpriority's raw syscall returns 20 - nice.
		prio, _, errno := syscall.Syscall(syscall.SYS_GETPRIORITY, syscall.PRIO_PROCESS, 0, 0)
		if errno != 0 {
			os.Stdout.WriteString("error " + errno.Error())
			os.Exit(0)
		}
		io, _, errno := syscall.Syscall(syscall.SYS_IOPRIO_GET, 1, 0, 0)
		if errno != 0 {
			os.Stdout.WriteString("error " + errno.Error())
			os.Exit(0)
		}
		os.Stdout.WriteString(strconv.Itoa(20-int(prio)) + " " + strconv.Itoa(int(io)))
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestLowerPriority$")
	cmd.Env = append(os.Environ(), "JMAN_PRIORITY_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if want := strconv.Itoa(serviceNiceness) + " " + strconv.Itoa(serviceIOPriority); !strings.HasPrefix(got, want) {
		t.Errorf("child priorities = %q, want %q (nice, ioprio)", got, want)
	}

	// Not leading its process group, as under go test, it changes nothing.
	if syscall.Getpgrp() != syscall.Getpid() {
		if err := lowerPriority(); err == nil {
			t.Error("lowered the priority without leading the process group")
		}
	}
}
