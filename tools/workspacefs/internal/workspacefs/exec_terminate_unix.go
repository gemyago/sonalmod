//go:build !windows

package workspacefs

import (
	"os/exec"
	"syscall"
)

// configureExecCommand puts the shell in its own process group so that
// terminating the leader kills child processes (e.g. sleep spawned by sh -c),
// allowing [exec.Cmd.Wait] and os/exec I/O goroutines to complete promptly.
func configureExecCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func terminateCmdProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
