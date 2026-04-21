package workspacefs

import (
	"log/slog"

	"github.com/gemyago/sonalmod/runtime/agent"
	ifs "github.com/gemyago/sonalmod/tools/workspacefs/internal/workspacefs"
)

func workspacefsAgentTools(svc *ifs.Service, _ *slog.Logger) []agent.DefinedTool {
	tools := []agent.DefinedTool{
		readTextFileTool(svc),
		readMultipleFilesTool(svc),
		writeFileTool(svc),
		editFileTool(svc),
		listDirectoryTool(svc),
		directoryTreeTool(svc),
		searchFilesTool(svc),
		getFileInfoTool(svc),
		listWorkspacesTool(svc),
	}
	if svc.IsExecEnabled() {
		tools = append(tools, execCommandTool(svc), execJobOutputTool(svc), execKillJobTool(svc))
	}
	return tools
}

func editFileTool(svc *ifs.Service) agent.ToolDef[ifs.EditFileRequest, ifs.EditFileResponse] {
	return agent.NewToolDef[ifs.EditFileRequest, ifs.EditFileResponse](
		"workspacefs_edit_file",
		"Replace the first occurrence of old_text with new_text in a UTF-8 file. Requires workspace (identifier from workspacefs_list_workspaces). Path is relative to that workspace. If old_text appears multiple times, only the first occurrence is replaced.",
		func(tc *agent.ToolContext, in ifs.EditFileRequest) (ifs.EditFileResponse, error) {
			return svc.EditFile(tc, in)
		},
	)
}

func writeFileTool(svc *ifs.Service) agent.ToolDef[ifs.WriteFileRequest, ifs.WriteFileResponse] {
	return agent.NewToolDef[ifs.WriteFileRequest, ifs.WriteFileResponse](
		"workspacefs_write_file",
		"Create or overwrite a UTF-8 text file. Requires workspace (identifier from workspacefs_list_workspaces). Path must be relative to that workspace. Truncates existing files; parent directories are created as needed.",
		func(tc *agent.ToolContext, in ifs.WriteFileRequest) (ifs.WriteFileResponse, error) {
			return svc.WriteFile(tc, in)
		},
	)
}

func readTextFileTool(svc *ifs.Service) agent.ToolDef[ifs.ReadTextFileRequest, ifs.ReadTextFileResponse] {
	return agent.NewToolDef[ifs.ReadTextFileRequest, ifs.ReadTextFileResponse](
		"workspacefs_read_text_file",
		"Read a UTF-8 text file. Requires workspace (identifier from workspacefs_list_workspaces). Path must be relative to that workspace. Optional head or tail (mutually exclusive) return line windows.",
		func(tc *agent.ToolContext, in ifs.ReadTextFileRequest) (ifs.ReadTextFileResponse, error) {
			return svc.ReadTextFile(tc, in)
		},
	)
}

func readMultipleFilesTool(
	svc *ifs.Service,
) agent.ToolDef[ifs.ReadMultipleFilesRequest, ifs.ReadMultipleFilesResponse] {
	return agent.NewToolDef[ifs.ReadMultipleFilesRequest, ifs.ReadMultipleFilesResponse](
		"workspacefs_read_multiple_files",
		"Read several UTF-8 text files under one workspace in one call. Requires workspace (identifier from workspacefs_list_workspaces). Paths are relative to that workspace. Per-path failures (missing file, invalid path, etc.) are returned in results and do not stop other paths from being read.",
		func(tc *agent.ToolContext, in ifs.ReadMultipleFilesRequest) (ifs.ReadMultipleFilesResponse, error) {
			return svc.ReadMultipleFiles(tc, in)
		},
	)
}

func listDirectoryTool(svc *ifs.Service) agent.ToolDef[ifs.ListDirectoryRequest, ifs.ListDirectoryResponse] {
	return agent.NewToolDef[ifs.ListDirectoryRequest, ifs.ListDirectoryResponse](
		"workspacefs_list_directory",
		"List entries in a single directory. Requires workspace (identifier from workspacefs_list_workspaces). Path must be relative to that workspace. Does not recurse into subdirectories.",
		func(tc *agent.ToolContext, in ifs.ListDirectoryRequest) (ifs.ListDirectoryResponse, error) {
			return svc.ListDirectory(tc, in)
		},
	)
}

func directoryTreeTool(svc *ifs.Service) agent.ToolDef[ifs.DirectoryTreeRequest, ifs.DirectoryTreeResponse] {
	// DirectoryTreeNode is recursive (Children []*DirectoryTreeNode), so schema
	// inference would fail with a cycle error. Provide a hand-written schema using
	// $defs/$ref to represent the self-referential structure.
	outputSchema := []byte(`{
		"type": "object",
		"properties": {
			"root": {"$ref": "#/$defs/node"}
		},
		"$defs": {
			"node": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"is_directory": {"type": "boolean"},
					"children": {
						"type": "array",
						"items": {"$ref": "#/$defs/node"}
					}
				},
				"required": ["name", "is_directory"]
			}
		}
	}`)
	return agent.NewToolDef[ifs.DirectoryTreeRequest, ifs.DirectoryTreeResponse](
		"workspacefs_directory_tree",
		"Return a directory tree. Requires workspace (identifier from workspacefs_list_workspaces). Path must be relative to that workspace. max_depth limits recursion depth (minimum 1; omitted uses a default cap).",
		func(tc *agent.ToolContext, in ifs.DirectoryTreeRequest) (ifs.DirectoryTreeResponse, error) {
			return svc.DirectoryTree(tc, in)
		},
	).WithOutputSchema(outputSchema)
}

func searchFilesTool(svc *ifs.Service) agent.ToolDef[ifs.SearchFilesRequest, ifs.SearchFilesResponse] {
	return agent.NewToolDef[ifs.SearchFilesRequest, ifs.SearchFilesResponse](
		"workspacefs_search_files",
		"Find files matching a glob pattern (path.Match semantics, forward slashes). Requires workspace (identifier from workspacefs_list_workspaces). Path is the directory to search relative to that workspace; omit or use '.' for the workspace base. Returns paths relative to that workspace.",
		func(tc *agent.ToolContext, in ifs.SearchFilesRequest) (ifs.SearchFilesResponse, error) {
			return svc.SearchFiles(tc, in)
		},
	)
}

func getFileInfoTool(svc *ifs.Service) agent.ToolDef[ifs.GetFileInfoRequest, ifs.GetFileInfoResponse] {
	return agent.NewToolDef[ifs.GetFileInfoRequest, ifs.GetFileInfoResponse](
		"workspacefs_get_file_info",
		"Return file metadata (size, modification time, name, directory flag). Requires workspace (identifier from workspacefs_list_workspaces). Path must be relative to that workspace.",
		func(tc *agent.ToolContext, in ifs.GetFileInfoRequest) (ifs.GetFileInfoResponse, error) {
			return svc.GetFileInfo(tc, in)
		},
	)
}

func listWorkspacesTool(svc *ifs.Service) agent.ToolDef[struct{}, ifs.ListWorkspacesResponse] {
	return agent.NewToolDef[struct{}, ifs.ListWorkspacesResponse](
		"workspacefs_list_workspaces",
		"List configured workspaces by identifier and short description. Use these identifiers as the workspace selector in other workspacefs tools. No host filesystem paths are returned.",
		func(_ *agent.ToolContext, _ struct{}) (ifs.ListWorkspacesResponse, error) {
			return ifs.ListWorkspacesResponse{Workspaces: svc.ListWorkspaces()}, nil
		},
	)
}

func execCommandTool(svc *ifs.Service) agent.ToolDef[ifs.ExecCommandRequest, ifs.ExecCommandResponse] {
	return agent.NewToolDef[ifs.ExecCommandRequest, ifs.ExecCommandResponse](
		"workspacefs_exec_command",
		"Execute a shell command within a workspace. Requires workspace (identifier from workspacefs_list_workspaces). working_directory is optional and relative to the workspace. Set background to true for long-running commands; poll status and output with workspacefs_exec_job_output and stop with workspacefs_exec_kill_job.",
		func(tc *agent.ToolContext, in ifs.ExecCommandRequest) (ifs.ExecCommandResponse, error) {
			return svc.ExecCommand(tc, in)
		},
	)
}

func execJobOutputTool(svc *ifs.Service) agent.ToolDef[ifs.ExecJobOutputRequest, ifs.ExecJobOutputResponse] {
	return agent.NewToolDef[ifs.ExecJobOutputRequest, ifs.ExecJobOutputResponse](
		"workspacefs_exec_job_output",
		"Poll output and status for a background command. Requires workspace (identifier from workspacefs_list_workspaces) and jobId returned by workspacefs_exec_command.",
		func(tc *agent.ToolContext, in ifs.ExecJobOutputRequest) (ifs.ExecJobOutputResponse, error) {
			return svc.ExecJobOutput(tc, in)
		},
	)
}

func execKillJobTool(svc *ifs.Service) agent.ToolDef[ifs.ExecKillJobRequest, ifs.ExecKillJobResponse] {
	return agent.NewToolDef[ifs.ExecKillJobRequest, ifs.ExecKillJobResponse](
		"workspacefs_exec_kill_job",
		"Terminate an active background command. Requires workspace (identifier from workspacefs_list_workspaces) and jobId returned by workspacefs_exec_command.",
		func(tc *agent.ToolContext, in ifs.ExecKillJobRequest) (ifs.ExecKillJobResponse, error) {
			return svc.ExecKillJob(tc, in)
		},
	)
}
