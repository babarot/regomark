//go:build !unix

package judge

import "os/exec"

// killGroup leaves the command as it is: on timeout, the command itself is
// killed, and WaitDelay bounds the wait for anything it started.
func killGroup(*exec.Cmd) {}
