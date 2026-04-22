package agent

import "testing"

func TestOpenCodeLauncherAliases(t *testing.T) {
	t.Parallel()

	if NewOpenCodeLauncher == nil {
		t.Fatal("NewOpenCodeLauncher must be exported")
	}
	if OpenCodeLaunchErrorKindValidation == "" {
		t.Fatal("OpenCodeLaunchErrorKindValidation must be exported")
	}
	if OpenCodeLaunchErrorKindNotFound == "" {
		t.Fatal("OpenCodeLaunchErrorKindNotFound must be exported")
	}
	if OpenCodeLaunchErrorKindLaunchFailed == "" {
		t.Fatal("OpenCodeLaunchErrorKindLaunchFailed must be exported")
	}
}
