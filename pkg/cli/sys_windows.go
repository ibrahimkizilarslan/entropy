//go:build windows

package cli

import (
	"os"
	"os/exec"
)

func setupDaemon(cmd *exec.Cmd) {
	// Windows does not support Setsid. Background process handling is OS-specific.
}

func getTerminateSignal() os.Signal {
	// Windows does not have SIGTERM; os.Interrupt is the standard cross-platform graceful signal.
	return os.Interrupt
}
