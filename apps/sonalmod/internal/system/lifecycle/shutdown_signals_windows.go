//go:build windows

package lifecycle

import "os"

func defaultShutdownSignals() []os.Signal {
	// Windows does not expose unix.SIGINT/SIGTERM; Ctrl+C maps to Interrupt.
	return []os.Signal{os.Interrupt}
}
