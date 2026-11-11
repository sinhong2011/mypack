//go:build windows

package appservice

import (
	"os/exec"
	"strconv"
)

// setupKill makes cancellation kill the whole process tree.
func setupKill(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
}
