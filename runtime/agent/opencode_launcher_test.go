package agent

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenCodeLauncherAliases(t *testing.T) {
	t.Parallel()

	if OpenCodeLaunchErrorKindValidation == "" {
		t.Fatal("OpenCodeLaunchErrorKindValidation must be exported")
	}
	if OpenCodeLaunchErrorKindNotFound == "" {
		t.Fatal("OpenCodeLaunchErrorKindNotFound must be exported")
	}
	if OpenCodeLaunchErrorKindLaunchFailed == "" {
		t.Fatal("OpenCodeLaunchErrorKindLaunchFailed must be exported")
	}

	profilesSvc, err := NewFileAgentProfilesService(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	bindingsSvc, err := NewFileOpenCodeBindingService(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	launcher, err := NewOpenCodeLauncher(profilesSvc, bindingsSvc)
	require.NoError(t, err)
	require.NotNil(t, launcher)
}
