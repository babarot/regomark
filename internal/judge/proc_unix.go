//go:build unix

package judge

import (
	"os/exec"
	"syscall"
)

// killGroup makes the command the leader of its own process group and, on
// timeout, kills the whole group: a command run through sh -c leaves the
// real work to a child that killing sh alone would not stop.
func killGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
