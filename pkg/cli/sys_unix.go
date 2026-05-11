//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

func setupDaemon(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func getTerminateSignal() os.Signal {
	return syscall.SIGTERM
}
