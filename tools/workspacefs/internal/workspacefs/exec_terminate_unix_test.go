//go:build !windows

package workspacefs

import (
	"os/exec"
	"testing"
)

func TestTerminateCmdProcess_noopsWithoutProcess(t *testing.T) {
	t.Parallel()
	terminateCmdProcess(nil)
	terminateCmdProcess(&exec.Cmd{})
}
