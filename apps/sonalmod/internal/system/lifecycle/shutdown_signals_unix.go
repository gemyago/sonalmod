//go:build !windows

package lifecycle

import (
	"os"

	"golang.org/x/sys/unix"
)

func defaultShutdownSignals() []os.Signal {
	return []os.Signal{unix.SIGINT, unix.SIGTERM}
}
