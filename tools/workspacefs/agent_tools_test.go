package workspacefs

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	ifs "github.com/gemyago/sonalmod/tools/workspacefs/internal/workspacefs"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testToolContext(t *testing.T) *agent.ToolContext {
	return &agent.ToolContext{Context: t.Context()}
}

func TestWorkspaceModelVisibleErrorsDoNotLeakHostPaths(t *testing.T) {
	t.Parallel()

	t.Run("unknown_workspace_tool_error_references_identifier_only", func(t *testing.T) {
		t.Parallel()
		p := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		svc, err := ifs.NewService([]ifs.WorkspaceMount{{
			Identifier:  "configured-ws",
			Description: "desc",
			Path:        clean,
		}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		read := readTextFileTool(svc)
		_, err = read.Handler(
			testToolContext(t),
			ifs.ReadTextFileRequest{Path: p, Workspace: "wrong-id"},
		)
		require.Error(t, err)
		require.ErrorContains(t, err, `wrong-id`)
		require.ErrorContains(t, err, "not configured")
		assert.NotContains(t, err.Error(), clean)
	})
}

func TestListWorkspacesTool(t *testing.T) {
	t.Parallel()

	t.Run("name_and_handler_returns_identifiers_and_descriptions_without_paths", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		id := "user-docs"
		desc := "User-facing documentation tree"

		svc, err := ifs.NewService([]ifs.WorkspaceMount{{Identifier: id, Description: desc, Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		td := listWorkspacesTool(svc)
		assert.Equal(t, "workspacefs_list_workspaces", td.Name)

		res, err := td.Handler(testToolContext(t), struct{}{})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 1)
		assert.Equal(t, id, res.Workspaces[0].Identifier)
		assert.Equal(t, desc, res.Workspaces[0].Description)

		b, err := json.Marshal(res)
		require.NoError(t, err)
		assert.NotContains(t, string(b), clean, "response must not leak configured absolute paths")

		var round ifs.ListWorkspacesResponse
		require.NoError(t, json.Unmarshal(b, &round))
		assert.Equal(t, res.Workspaces, round.Workspaces)
	})

	t.Run("multiple_workspaces_in_configuration_order", func(t *testing.T) {
		t.Parallel()
		dirA := t.TempDir()
		dirB := t.TempDir()
		absA, err := filepath.Abs(dirA)
		require.NoError(t, err)
		absB, err := filepath.Abs(dirB)
		require.NoError(t, err)
		cleanA := filepath.Clean(absA)
		cleanB := filepath.Clean(absB)

		svc, err := ifs.NewService([]ifs.WorkspaceMount{
			{Identifier: "a", Description: "first workspace", Path: cleanA},
			{Identifier: "b", Description: "second workspace", Path: cleanB},
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		td := listWorkspacesTool(svc)
		res, err := td.Handler(testToolContext(t), struct{}{})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 2)
		assert.Equal(t, "a", res.Workspaces[0].Identifier)
		assert.Equal(t, "first workspace", res.Workspaces[0].Description)
		assert.Equal(t, "b", res.Workspaces[1].Identifier)
		assert.Equal(t, "second workspace", res.Workspaces[1].Description)

		raw, err := json.Marshal(res)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), cleanA)
		assert.NotContains(t, string(raw), cleanB)
	})
}

func TestGetFileInfoTool(t *testing.T) {
	t.Parallel()

	t.Run("name_and_handler_returns_metadata", func(t *testing.T) {
		t.Parallel()
		meta := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, meta), []byte("x"), 0o644))

		svc, err := ifs.NewService([]ifs.WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		td := getFileInfoTool(svc)
		assert.Equal(t, "workspacefs_get_file_info", td.Name)

		res, err := td.Handler(testToolContext(t), ifs.GetFileInfoRequest{Path: meta, Workspace: "w"})
		require.NoError(t, err)
		assert.Equal(t, int64(1), res.Size)
		assert.Equal(t, meta, res.Name)
		assert.False(t, res.IsDirectory)

		b, err := json.Marshal(res)
		require.NoError(t, err)
		var round ifs.GetFileInfoResponse
		require.NoError(t, json.Unmarshal(b, &round))
		assert.Equal(t, res.Size, round.Size)
		assert.Equal(t, res.Name, round.Name)
		assert.Equal(t, res.IsDirectory, round.IsDirectory)
		assert.Equal(t, res.ModTime, round.ModTime)
	})
}

func TestListDirectoryTool(t *testing.T) {
	t.Parallel()

	t.Run("name_and_handler_lists_directory", func(t *testing.T) {
		t.Parallel()
		x := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, x), []byte("x"), 0o644))

		svc, err := ifs.NewService([]ifs.WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		td := listDirectoryTool(svc)
		assert.Equal(t, "workspacefs_list_directory", td.Name)

		res, err := td.Handler(testToolContext(t), ifs.ListDirectoryRequest{Path: ".", Workspace: "w"})
		require.NoError(t, err)
		require.Len(t, res.Entries, 1)
		assert.Equal(t, x, res.Entries[0].Name)

		b, err := json.Marshal(res)
		require.NoError(t, err)
		var round ifs.ListDirectoryResponse
		require.NoError(t, json.Unmarshal(b, &round))
		assert.Equal(t, res.Entries, round.Entries)
	})
}

func TestDirectoryTreeTool(t *testing.T) {
	t.Parallel()

	t.Run("name_and_handler_returns_tree", func(t *testing.T) {
		t.Parallel()
		sub := faker.New().UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.Mkdir(filepath.Join(root, sub), 0o755))

		svc, err := ifs.NewService([]ifs.WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		td := directoryTreeTool(svc)
		assert.Equal(t, "workspacefs_directory_tree", td.Name)

		d := 1
		res, err := td.Handler(testToolContext(t), ifs.DirectoryTreeRequest{Path: ".", Workspace: "w", MaxDepth: &d})
		require.NoError(t, err)
		require.NotNil(t, res.Root)
		assert.Equal(t, ".", res.Root.Name)

		b, err := json.Marshal(res)
		require.NoError(t, err)
		var round ifs.DirectoryTreeResponse
		require.NoError(t, json.Unmarshal(b, &round))
		assert.Equal(t, res.Root.Name, round.Root.Name)
	})
}

func TestWriteFileTool(t *testing.T) {
	t.Parallel()

	t.Run("name_and_handler_writes_file", func(t *testing.T) {
		t.Parallel()
		out := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)

		svc, err := ifs.NewService([]ifs.WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		td := writeFileTool(svc)
		assert.Equal(t, "workspacefs_write_file", td.Name)

		res, err := td.Handler(testToolContext(t), ifs.WriteFileRequest{Path: out, Content: "hello", Workspace: "w"})
		require.NoError(t, err)
		assert.Equal(t, 5, res.BytesWritten)

		b, err := os.ReadFile(filepath.Join(root, out))
		require.NoError(t, err)
		assert.Equal(t, "hello", string(b))

		raw, err := json.Marshal(res)
		require.NoError(t, err)
		var round ifs.WriteFileResponse
		require.NoError(t, json.Unmarshal(raw, &round))
		assert.Equal(t, res.BytesWritten, round.BytesWritten)
	})
}

func TestEditFileTool(t *testing.T) {
	t.Parallel()

	t.Run("name_and_handler_edits_file", func(t *testing.T) {
		t.Parallel()
		edit := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, edit), []byte("one two"), 0o644))

		svc, err := ifs.NewService([]ifs.WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		td := editFileTool(svc)
		assert.Equal(t, "workspacefs_edit_file", td.Name)

		res, err := td.Handler(testToolContext(t), ifs.EditFileRequest{
			Workspace: "w",
			Path:      edit,
			OldText:   "two",
			NewText:   "three",
		})
		require.NoError(t, err)
		assert.Positive(t, res.BytesWritten)
		assert.Equal(t, 1, res.MatchCount)

		after, err := os.ReadFile(filepath.Join(root, edit))
		require.NoError(t, err)
		assert.Equal(t, "one three", string(after))

		raw, err := json.Marshal(res)
		require.NoError(t, err)
		var round ifs.EditFileResponse
		require.NoError(t, json.Unmarshal(raw, &round))
		assert.Equal(t, res.BytesWritten, round.BytesWritten)
		assert.Equal(t, res.MatchCount, round.MatchCount)
	})
}

func TestMultiWorkspaceToolHandlers(t *testing.T) {
	t.Parallel()

	t.Run("two_workspaces_read_text_file_requires_workspace_then_selects", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		onlyA := fake.UUID().V4() + ".txt"
		onlyB := fake.UUID().V4() + ".txt"
		dirA := t.TempDir()
		dirB := t.TempDir()
		absA, err := filepath.Abs(dirA)
		require.NoError(t, err)
		absB, err := filepath.Abs(dirB)
		require.NoError(t, err)
		cleanA := filepath.Clean(absA)
		cleanB := filepath.Clean(absB)
		require.NoError(t, os.WriteFile(filepath.Join(dirA, onlyA), []byte("from A"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dirB, onlyB), []byte("from B"), 0o644))

		svc, err := ifs.NewService([]ifs.WorkspaceMount{
			{Identifier: "a", Description: "test", Path: cleanA},
			{Identifier: "b", Description: "test", Path: cleanB},
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		read := readTextFileTool(svc)
		_, err = read.Handler(testToolContext(t), ifs.ReadTextFileRequest{Path: onlyA})
		require.Error(t, err)
		require.ErrorContains(t, err, "workspace is required")

		resA, err := read.Handler(testToolContext(t), ifs.ReadTextFileRequest{Path: onlyA, Workspace: "a"})
		require.NoError(t, err)
		assert.Equal(t, "from A", resA.Content)

		resB, err := read.Handler(testToolContext(t), ifs.ReadTextFileRequest{Path: onlyB, Workspace: "b"})
		require.NoError(t, err)
		assert.Equal(t, "from B", resB.Content)

		_, err = read.Handler(testToolContext(t), ifs.ReadTextFileRequest{Path: onlyB, Workspace: "a"})
		require.Error(t, err)
		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("two_workspaces_write_file_and_list_directory", func(t *testing.T) {
		t.Parallel()
		wname := faker.New().UUID().V4() + ".txt"
		dirA := t.TempDir()
		dirB := t.TempDir()
		absA, err := filepath.Abs(dirA)
		require.NoError(t, err)
		absB, err := filepath.Abs(dirB)
		require.NoError(t, err)
		cleanA := filepath.Clean(absA)
		cleanB := filepath.Clean(absB)

		svc, err := ifs.NewService([]ifs.WorkspaceMount{
			{Identifier: "a", Description: "test", Path: cleanA},
			{Identifier: "b", Description: "test", Path: cleanB},
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		w := writeFileTool(svc)
		_, err = w.Handler(testToolContext(t), ifs.WriteFileRequest{Path: wname, Content: "w"})
		require.Error(t, err)
		require.ErrorContains(t, err, "workspace is required")

		_, err = w.Handler(testToolContext(t), ifs.WriteFileRequest{Path: wname, Content: "in B", Workspace: "b"})
		require.NoError(t, err)
		b, err := os.ReadFile(filepath.Join(dirB, wname))
		require.NoError(t, err)
		assert.Equal(t, "in B", string(b))
		_, err = os.ReadFile(filepath.Join(dirA, wname))
		require.Error(t, err)

		listTool := listDirectoryTool(svc)
		ld, err := listTool.Handler(testToolContext(t), ifs.ListDirectoryRequest{Path: ".", Workspace: "b"})
		require.NoError(t, err)
		names := make([]string, 0, len(ld.Entries))
		for _, e := range ld.Entries {
			names = append(names, e.Name)
		}
		assert.Contains(t, names, wname)
	})

	t.Run("two_workspaces_read_multiple_files", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		x := fake.UUID().V4() + ".txt"
		y := fake.UUID().V4() + ".txt"
		dirA := t.TempDir()
		dirB := t.TempDir()
		absA, err := filepath.Abs(dirA)
		require.NoError(t, err)
		absB, err := filepath.Abs(dirB)
		require.NoError(t, err)
		cleanA := filepath.Clean(absA)
		cleanB := filepath.Clean(absB)
		require.NoError(t, os.WriteFile(filepath.Join(dirA, x), []byte("xa"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dirB, y), []byte("yb"), 0o644))

		svc, err := ifs.NewService([]ifs.WorkspaceMount{
			{Identifier: "a", Description: "test", Path: cleanA},
			{Identifier: "b", Description: "test", Path: cleanB},
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		batch := readMultipleFilesTool(svc)
		_, err = batch.Handler(testToolContext(t), ifs.ReadMultipleFilesRequest{Paths: []string{x}})
		require.Error(t, err)
		require.ErrorContains(t, err, "workspace is required")

		res, err := batch.Handler(
			testToolContext(t),
			ifs.ReadMultipleFilesRequest{Paths: []string{x, y}, Workspace: "a"},
		)
		require.NoError(t, err)
		require.Len(t, res.Results, 2)
		assert.Equal(t, "xa", res.Results[0].Content)
		require.NotEmpty(t, res.Results[1].Error)

		res2, err := batch.Handler(
			testToolContext(t),
			ifs.ReadMultipleFilesRequest{Paths: []string{y}, Workspace: "b"},
		)
		require.NoError(t, err)
		require.Len(t, res2.Results, 1)
		assert.Equal(t, "yb", res2.Results[0].Content)
	})

	t.Run("single_workspace_tools_require_workspace_field", func(t *testing.T) {
		t.Parallel()
		solo := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, solo), []byte("solo"), 0o644))

		svc, err := ifs.NewService([]ifs.WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		read := readTextFileTool(svc)
		_, err = read.Handler(testToolContext(t), ifs.ReadTextFileRequest{Path: solo})
		require.Error(t, err)
		require.ErrorContains(t, err, "workspace is required")

		res, err := read.Handler(testToolContext(t), ifs.ReadTextFileRequest{Path: solo, Workspace: "w"})
		require.NoError(t, err)
		assert.Equal(t, "solo", res.Content)

		listTool := listDirectoryTool(svc)
		ld, err := listTool.Handler(testToolContext(t), ifs.ListDirectoryRequest{Path: ".", Workspace: "w"})
		require.NoError(t, err)
		assert.NotEmpty(t, ld.Entries)
	})
}

func TestReadTextFileTool(t *testing.T) {
	t.Parallel()

	t.Run("name_and_handler_reads_file", func(t *testing.T) {
		t.Parallel()
		note := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, note), []byte("hi"), 0o644))

		svc, err := ifs.NewService([]ifs.WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		td := readTextFileTool(svc)
		assert.Equal(t, "workspacefs_read_text_file", td.Name)

		res, err := td.Handler(testToolContext(t), ifs.ReadTextFileRequest{Path: note, Workspace: "w"})
		require.NoError(t, err)
		assert.Equal(t, "hi", res.Content)

		b, err := json.Marshal(res)
		require.NoError(t, err)
		var round ifs.ReadTextFileResponse
		require.NoError(t, json.Unmarshal(b, &round))
		assert.Equal(t, res.Content, round.Content)
	})
}

func TestExecCommandTool(t *testing.T) {
	t.Parallel()

	makeExecSvc := func(t *testing.T) (*ifs.Service, string) {
		t.Helper()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		svc, err := ifs.NewService(
			[]ifs.WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}},
			ifs.WithExecEnabled(ifs.ExecConfig{
				MaxOutputBytes:    1024 * 1024,
				DefaultTimeout:    10_000_000_000, // 10s in nanoseconds
				MaxConcurrentJobs: 10,
			}),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })
		return svc, clean
	}

	t.Run("tool_name_is_workspacefs_exec_command", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeExecSvc(t)
		td := execCommandTool(svc)
		assert.Equal(t, "workspacefs_exec_command", td.Name)
	})

	t.Run("description_mentions_workspace_and_background", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeExecSvc(t)
		td := execCommandTool(svc)
		assert.Contains(t, td.Description, "workspace")
		assert.Contains(t, td.Description, "background")
	})

	t.Run("handler_runs_foreground_command_and_returns_output", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		svc, _ := makeExecSvc(t)
		word := fake.Lorem().Word()
		td := execCommandTool(svc)

		res, err := td.Handler(testToolContext(t), ifs.ExecCommandRequest{
			Workspace: "w",
			Command:   "echo " + word,
		})
		require.NoError(t, err)
		assert.Equal(t, 0, res.ExitCode)
		assert.Contains(t, res.Stdout, word)
		assert.False(t, res.TimedOut)

		b, err := json.Marshal(res)
		require.NoError(t, err)
		var round ifs.ExecCommandResponse
		require.NoError(t, json.Unmarshal(b, &round))
		assert.Equal(t, res.ExitCode, round.ExitCode)
		assert.Equal(t, res.Stdout, round.Stdout)
	})

	t.Run("handler_returns_error_when_workspace_missing", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeExecSvc(t)
		td := execCommandTool(svc)

		_, err := td.Handler(testToolContext(t), ifs.ExecCommandRequest{
			Command: "echo hello",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace")
	})

	t.Run("handler_error_does_not_leak_host_path", func(t *testing.T) {
		t.Parallel()
		svc, hostPath := makeExecSvc(t)
		td := execCommandTool(svc)

		_, err := td.Handler(testToolContext(t), ifs.ExecCommandRequest{
			Workspace:        "w",
			Command:          "echo hello",
			WorkingDirectory: "/absolute/path",
		})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), hostPath)
	})

	t.Run("handler_returns_error_for_unknown_workspace", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		svc, _ := makeExecSvc(t)
		td := execCommandTool(svc)

		_, err := td.Handler(testToolContext(t), ifs.ExecCommandRequest{
			Workspace: fake.UUID().V4(),
			Command:   "echo hello",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace")
	})

	t.Run("handler_starts_background_job_and_returns_job_id", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeExecSvc(t)
		td := execCommandTool(svc)

		res, err := td.Handler(testToolContext(t), ifs.ExecCommandRequest{
			Workspace:  "w",
			Command:    "sleep 10",
			Background: true,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, res.JobID)
		assert.True(t, res.Running)
	})
}

func TestExecJobOutputTool(t *testing.T) {
	t.Parallel()

	makeExecSvc := func(t *testing.T) *ifs.Service {
		t.Helper()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		svc, err := ifs.NewService(
			[]ifs.WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}},
			ifs.WithExecEnabled(ifs.ExecConfig{
				MaxOutputBytes:    1024 * 1024,
				DefaultTimeout:    10_000_000_000,
				MaxConcurrentJobs: 10,
			}),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })
		return svc
	}

	t.Run("tool_name_is_workspacefs_exec_job_output", func(t *testing.T) {
		t.Parallel()
		svc := makeExecSvc(t)
		td := execJobOutputTool(svc)
		assert.Equal(t, "workspacefs_exec_job_output", td.Name)
	})

	t.Run("description_mentions_workspace_and_jobId", func(t *testing.T) {
		t.Parallel()
		svc := makeExecSvc(t)
		td := execJobOutputTool(svc)
		assert.Contains(t, td.Description, "workspace")
		assert.Contains(t, td.Description, "jobId")
	})

	t.Run("handler_polls_running_job_status", func(t *testing.T) {
		t.Parallel()
		svc := makeExecSvc(t)

		start, err := svc.ExecCommand(testToolContext(t).Context, ifs.ExecCommandRequest{
			Workspace:  "w",
			Command:    "sleep 30",
			Background: true,
		})
		require.NoError(t, err)
		require.NotEmpty(t, start.JobID)

		td := execJobOutputTool(svc)
		res, err := td.Handler(testToolContext(t), ifs.ExecJobOutputRequest{
			Workspace: "w",
			JobID:     start.JobID,
		})
		require.NoError(t, err)
		assert.True(t, res.Running)

		b, err := json.Marshal(res)
		require.NoError(t, err)
		var round ifs.ExecJobOutputResponse
		require.NoError(t, json.Unmarshal(b, &round))
		assert.Equal(t, res.Running, round.Running)
	})

	t.Run("handler_returns_error_for_unknown_job", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		svc := makeExecSvc(t)
		td := execJobOutputTool(svc)

		_, err := td.Handler(testToolContext(t), ifs.ExecJobOutputRequest{
			Workspace: "w",
			JobID:     fake.UUID().V4(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "job")
	})
}

func TestExecKillJobTool(t *testing.T) {
	t.Parallel()

	makeExecSvc := func(t *testing.T) *ifs.Service {
		t.Helper()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		svc, err := ifs.NewService(
			[]ifs.WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}},
			ifs.WithExecEnabled(ifs.ExecConfig{
				MaxOutputBytes:    1024 * 1024,
				DefaultTimeout:    30_000_000_000,
				MaxConcurrentJobs: 10,
			}),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })
		return svc
	}

	t.Run("tool_name_is_workspacefs_exec_kill_job", func(t *testing.T) {
		t.Parallel()
		svc := makeExecSvc(t)
		td := execKillJobTool(svc)
		assert.Equal(t, "workspacefs_exec_kill_job", td.Name)
	})

	t.Run("description_mentions_workspace_and_jobId", func(t *testing.T) {
		t.Parallel()
		svc := makeExecSvc(t)
		td := execKillJobTool(svc)
		assert.Contains(t, td.Description, "workspace")
		assert.Contains(t, td.Description, "jobId")
	})

	t.Run("handler_kills_active_job", func(t *testing.T) {
		t.Parallel()
		svc := makeExecSvc(t)

		start, err := svc.ExecCommand(testToolContext(t).Context, ifs.ExecCommandRequest{
			Workspace:  "w",
			Command:    "sleep 30",
			Background: true,
		})
		require.NoError(t, err)
		require.NotEmpty(t, start.JobID)

		td := execKillJobTool(svc)
		res, err := td.Handler(testToolContext(t), ifs.ExecKillJobRequest{
			Workspace: "w",
			JobID:     start.JobID,
		})
		require.NoError(t, err)
		assert.True(t, res.Killed)

		b, err := json.Marshal(res)
		require.NoError(t, err)
		var round ifs.ExecKillJobResponse
		require.NoError(t, json.Unmarshal(b, &round))
		assert.Equal(t, res.Killed, round.Killed)
	})

	t.Run("handler_returns_not_killed_for_unknown_job", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		svc := makeExecSvc(t)
		td := execKillJobTool(svc)

		res, err := td.Handler(testToolContext(t), ifs.ExecKillJobRequest{
			Workspace: "w",
			JobID:     fake.UUID().V4(),
		})
		require.NoError(t, err)
		assert.False(t, res.Killed)
	})
}

func TestExecToolsModelVisibleErrors(t *testing.T) {
	t.Parallel()

	t.Run("exec_command_unknown_workspace_references_identifier_only", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		svc, err := ifs.NewService(
			[]ifs.WorkspaceMount{{Identifier: "configured-ws", Description: "desc", Path: clean}},
			ifs.WithExecEnabled(ifs.ExecConfig{
				MaxOutputBytes:    1024 * 1024,
				DefaultTimeout:    10_000_000_000,
				MaxConcurrentJobs: 10,
			}),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		td := execCommandTool(svc)
		_, err = td.Handler(testToolContext(t), ifs.ExecCommandRequest{
			Workspace: "wrong-id",
			Command:   "echo hello",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wrong-id")
		assert.NotContains(t, err.Error(), clean)
	})

	t.Run("exec_command_working_directory_error_does_not_expose_host_path", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		svc, err := ifs.NewService(
			[]ifs.WorkspaceMount{{Identifier: "w", Description: "desc", Path: clean}},
			ifs.WithExecEnabled(ifs.ExecConfig{
				MaxOutputBytes:    1024 * 1024,
				DefaultTimeout:    10_000_000_000,
				MaxConcurrentJobs: 10,
			}),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		td := execCommandTool(svc)
		_, err = td.Handler(testToolContext(t), ifs.ExecCommandRequest{
			Workspace:        "w",
			Command:          "echo hello",
			WorkingDirectory: "/absolute/escaped/path",
		})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), clean)
	})
}
