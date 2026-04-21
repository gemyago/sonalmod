//go:build windows

package workspacefs

import "os/exec"

func configureExecCommand(cmd *exec.Cmd) {}

func terminateCmdProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
