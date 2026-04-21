package workspacefs

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureRegistry struct {
	addCalls int
	tools    []agent.DefinedTool
}

func (c *captureRegistry) AddTools(tools ...agent.DefinedTool) {
	c.addCalls++
	c.tools = append(c.tools, tools...)
}

func TestWorkspaceRegistrationErrorsDoNotLeakAbsolutePaths(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	t.Run("missing_configured_directory", func(t *testing.T) {
		t.Parallel()
		reg := &captureRegistry{}
		missing := filepath.Join(t.TempDir(), "does-not-exist-"+fake.UUID().V4())
		absMissing, err := filepath.Abs(missing)
		require.NoError(t, err)
		clean := filepath.Clean(absMissing)
		id := fake.Lorem().Word()
		err = RegisterTools(reg,
			WithWorkspaces([]WorkspaceConfig{
				{Identifier: id, Description: fake.Lorem().Sentence(6), Path: missing},
			}),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), clean, "error must not include configured absolute path")
	})
}

func TestRegisterTools(t *testing.T) {
	fake := faker.New()

	type registryState struct {
		addCalls  int
		toolCount int
	}

	makeMockDeps := func() *captureRegistry {
		return &captureRegistry{}
	}

	t.Run("skips_AddTools_when_workspaces_nil_or_empty", func(t *testing.T) {
		tests := []struct {
			name string
			opts []RegisterToolsOpt
		}{
			{
				name: "nil_workspaces",
				opts: []RegisterToolsOpt{WithLogger(slog.New(slog.DiscardHandler))},
			},
			{
				name: "empty_workspaces",
				opts: []RegisterToolsOpt{WithWorkspaces(nil), WithLogger(slog.New(slog.DiscardHandler))},
			},
			{
				name: "empty_slice_literal",
				opts: []RegisterToolsOpt{
					WithWorkspaces([]WorkspaceConfig{}),
					WithLogger(slog.New(slog.DiscardHandler)),
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				reg := makeMockDeps()
				err := RegisterTools(reg, tt.opts...)
				require.Error(t, err)
				assert.Equal(t, registryState{}, registryState{reg.addCalls, len(reg.tools)})
			})
		}
	})

	t.Run("skips_AddTools_when_duplicate_identifier", func(t *testing.T) {
		reg := makeMockDeps()
		dir := t.TempDir()
		id := fake.Lorem().Word()
		err := RegisterTools(reg,
			WithWorkspaces([]WorkspaceConfig{
				{Identifier: id, Description: fake.Lorem().Sentence(6), Path: dir},
				{Identifier: id, Description: fake.Lorem().Sentence(6), Path: dir},
			}),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate")
		assert.Equal(t, registryState{}, registryState{reg.addCalls, len(reg.tools)})
	})

	t.Run("skips_AddTools_when_identifier_empty", func(t *testing.T) {
		reg := makeMockDeps()
		err := RegisterTools(reg,
			WithWorkspaces([]WorkspaceConfig{
				{Identifier: "   ", Description: fake.Lorem().Sentence(6), Path: t.TempDir()},
			}),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "identifier")
		assert.Equal(t, registryState{}, registryState{reg.addCalls, len(reg.tools)})
	})

	t.Run("skips_AddTools_when_description_empty", func(t *testing.T) {
		reg := makeMockDeps()
		err := RegisterTools(reg,
			WithWorkspaces([]WorkspaceConfig{
				{Identifier: fake.Lorem().Word(), Description: "  \t  ", Path: t.TempDir()},
			}),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "description")
		assert.Equal(t, registryState{}, registryState{reg.addCalls, len(reg.tools)})
	})

	t.Run("skips_AddTools_when_path_invalid", func(t *testing.T) {
		reg := makeMockDeps()
		missing := filepath.Join(t.TempDir(), "does-not-exist-"+fake.UUID().V4())
		err := RegisterTools(reg,
			WithWorkspaces([]WorkspaceConfig{
				{Identifier: fake.Lorem().Word(), Description: fake.Lorem().Sentence(6), Path: missing},
			}),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
		require.Error(t, err)
		assert.Equal(t, registryState{}, registryState{reg.addCalls, len(reg.tools)})
	})

	t.Run("debug_logs_validation_errors_with_slog_Default_when_logger_unset", func(t *testing.T) {
		var buf bytes.Buffer
		h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
		prev := slog.Default()
		slog.SetDefault(slog.New(h))
		t.Cleanup(func() { slog.SetDefault(prev) })

		reg := makeMockDeps()
		err := RegisterTools(reg, WithWorkspaces(nil))
		require.Error(t, err)
		assert.Contains(t, buf.String(), "workspacefs")
	})

	t.Run("debug_logs_validation_errors_with_slog_Default_when_WithLogger_nil", func(t *testing.T) {
		var buf bytes.Buffer
		h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
		prev := slog.Default()
		slog.SetDefault(slog.New(h))
		t.Cleanup(func() { slog.SetDefault(prev) })

		reg := makeMockDeps()
		err := RegisterTools(reg, WithWorkspaces([]WorkspaceConfig{}), WithLogger(nil))
		require.Error(t, err)
		assert.Contains(t, buf.String(), "workspacefs")
	})

	t.Run("AddTools_registers_expected_tools_when_one_valid_workspace", func(t *testing.T) {
		reg := makeMockDeps()
		root := t.TempDir()
		err := RegisterTools(reg,
			WithWorkspaces([]WorkspaceConfig{
				{Identifier: fake.Lorem().Word(), Description: fake.Lorem().Sentence(6), Path: root},
			}),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
		require.NoError(t, err)
		want := registryState{addCalls: 1, toolCount: ExpectedToolCount}
		got := registryState{reg.addCalls, len(reg.tools)}
		assert.Equal(t, want, got)
	})

	t.Run("AddTools_registers_expected_tools_when_multiple_valid_workspaces", func(t *testing.T) {
		reg := makeMockDeps()
		a := t.TempDir()
		b := t.TempDir()
		err := RegisterTools(reg,
			WithWorkspaces([]WorkspaceConfig{
				{Identifier: fake.Lorem().Word() + "-a", Description: fake.Lorem().Sentence(6), Path: a},
				{Identifier: fake.Lorem().Word() + "-b", Description: fake.Lorem().Sentence(6), Path: b},
			}),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
		require.NoError(t, err)
		want := registryState{addCalls: 1, toolCount: ExpectedToolCount}
		got := registryState{reg.addCalls, len(reg.tools)}
		assert.Equal(t, want, got)
	})

	t.Run("exec_disabled_by_default", func(t *testing.T) {
		reg := makeMockDeps()
		root := t.TempDir()
		err := RegisterTools(reg,
			WithWorkspaces([]WorkspaceConfig{
				{Identifier: fake.Lorem().Word(), Description: fake.Lorem().Sentence(6), Path: root},
			}),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
		require.NoError(t, err)
		// Without WithExec, only the base filesystem tools are registered.
		assert.Len(t, reg.tools, ExpectedToolCount)
	})

	t.Run("enabling_exec_registers_additional_tools", func(t *testing.T) {
		reg := makeMockDeps()
		root := t.TempDir()
		err := RegisterTools(reg,
			WithWorkspaces([]WorkspaceConfig{
				{Identifier: fake.Lorem().Word(), Description: fake.Lorem().Sentence(6), Path: root},
			}),
			WithExec(ExecOptions{}),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
		require.NoError(t, err)
		assert.Len(t, reg.tools, ExpectedToolCountWithExec)
	})

	t.Run("invalid_exec_option_max_output_bytes_zero_fails", func(t *testing.T) {
		reg := makeMockDeps()
		root := t.TempDir()
		err := RegisterTools(reg,
			WithWorkspaces([]WorkspaceConfig{
				{Identifier: fake.Lorem().Word(), Description: fake.Lorem().Sentence(6), Path: root},
			}),
			WithExec(ExecOptions{MaxOutputBytes: -1}),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MaxOutputBytes")
		assert.Equal(t, registryState{}, registryState{reg.addCalls, len(reg.tools)})
	})

	t.Run("invalid_exec_option_timeout_negative_fails", func(t *testing.T) {
		reg := makeMockDeps()
		root := t.TempDir()
		err := RegisterTools(reg,
			WithWorkspaces([]WorkspaceConfig{
				{Identifier: fake.Lorem().Word(), Description: fake.Lorem().Sentence(6), Path: root},
			}),
			WithExec(ExecOptions{DefaultTimeout: -1}),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DefaultTimeout")
		assert.Equal(t, registryState{}, registryState{reg.addCalls, len(reg.tools)})
	})

	t.Run("invalid_exec_option_max_jobs_negative_fails", func(t *testing.T) {
		reg := makeMockDeps()
		root := t.TempDir()
		err := RegisterTools(reg,
			WithWorkspaces([]WorkspaceConfig{
				{Identifier: fake.Lorem().Word(), Description: fake.Lorem().Sentence(6), Path: root},
			}),
			WithExec(ExecOptions{MaxConcurrentJobs: -1}),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MaxConcurrentJobs")
		assert.Equal(t, registryState{}, registryState{reg.addCalls, len(reg.tools)})
	})
}
