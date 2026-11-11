//go:build !windows

package appservice

import (
	"os/exec"
	"syscall"
)

// setupKill puts the child in its own process group and makes cancellation
// kill the whole group (npm → node → bundler children).
func setupKill(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
