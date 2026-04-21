//go:build !release

package internal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	genkit "github.com/firebase/genkit/go/genkit"
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
)

// openTestLogFile will open a log file in a project root directory.
func openTestLogFile() *os.File {
	_, filename, _, _ := runtime.Caller(0) // Will be current file
	testFilePath := filepath.Join(filename, "..", "..", "agent-test.log")
	f, err := os.OpenFile(testFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		err = fmt.Errorf("fail to open log file %q for test logging: %w", testFilePath, err)
		panic(err)
	}
	return f
}

var testOutput = openTestLogFile() //nolint:gochecknoglobals //it's ok for tests

func RootTestLogger() *slog.Logger {
	return slog.New(
		slog.NewJSONHandler(testOutput, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)
}

// FakeGenkitInstance is a test helper that provides a controllable genkit init function.
// Each call to the returned InitFunc increments an internal counter and returns a zero *genkit.Genkit.
// Access InitCount() to inspect how many times the init function was called.
type FakeGenkitInstance struct {
	mu    sync.Mutex
	count int
}

// NewFakeGenkitInstance creates a new FakeGenkitInstance for use in tests.
func NewFakeGenkitInstance() *FakeGenkitInstance {
	return &FakeGenkitInstance{}
}

// InitFunc returns a genkitInitFuncType-compatible function that records calls.
func (f *FakeGenkitInstance) InitFunc() func(context.Context, lp.ProviderConfig) (*genkit.Genkit, error) {
	return func(_ context.Context, _ lp.ProviderConfig) (*genkit.Genkit, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.count++
		return &genkit.Genkit{}, nil
	}
}

// InitCount returns the number of times InitFunc was invoked.
func (f *FakeGenkitInstance) InitCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count
}
