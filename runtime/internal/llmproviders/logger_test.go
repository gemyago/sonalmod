package llmproviders

import (
	"io"
	"log/slog"
	"testing"
)

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewJSONHandler(io.Discard, nil)).With("test", t.Name())
}
