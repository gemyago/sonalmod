package workspacefs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ReadTextFileRequest is the JSON input for workspacefs_read_text_file.
type ReadTextFileRequest struct {
	// Path is relative to the workspace (no leading slash, no ".." escape).
	Path string `json:"path"`
	// Workspace is the configured workspace identifier (required on every call).
	Workspace string `json:"workspace"`
	// Head, if set, returns only the first N lines (mutually exclusive with Tail).
	Head *int `json:"head,omitempty"`
	// Tail, if set, returns only the last N lines (mutually exclusive with Head).
	Tail *int `json:"tail,omitempty"`
}

// ReadTextFileResponse is the JSON result for workspacefs_read_text_file.
type ReadTextFileResponse struct {
	Content string `json:"content"`
}

// ReadMultipleFilesRequest is the JSON input for workspacefs_read_multiple_files.
type ReadMultipleFilesRequest struct {
	// Paths are relative to the workspace (same rules as read_text_file).
	Paths []string `json:"paths"`
	// Workspace is the configured workspace identifier (required on every call).
	Workspace string `json:"workspace"`
}

// ReadMultipleFilesResponse is the JSON result for workspacefs_read_multiple_files.
// Per-path failures do not abort the batch; each path yields one result entry in order.
type ReadMultipleFilesResponse struct {
	Results []ReadMultipleFileResult `json:"results"`
}

// ReadMultipleFileResult is one path outcome: either Content on success or Error on failure.
type ReadMultipleFileResult struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ReadTextFile reads UTF-8 text from a path under the selected workspace.
// Head and Tail are mutually exclusive; when both are nil, the full file is returned (subject to max bytes).
func (s *Service) ReadTextFile(ctx context.Context, in ReadTextFileRequest) (ReadTextFileResponse, error) {
	if err := ctx.Err(); err != nil {
		return ReadTextFileResponse{}, err
	}
	if in.Head != nil && in.Tail != nil {
		return ReadTextFileResponse{}, errors.New("workspacefs: head and tail are mutually exclusive")
	}
	if in.Head != nil && *in.Head < 0 {
		return ReadTextFileResponse{}, errors.New("workspacefs: head must be non-negative")
	}
	if in.Tail != nil && *in.Tail < 0 {
		return ReadTextFileResponse{}, errors.New("workspacefs: tail must be non-negative")
	}
	if strings.TrimSpace(in.Path) == "" {
		return ReadTextFileResponse{}, errors.New("workspacefs: path is required")
	}

	root, err := s.pickWorkspace(in.Workspace)
	if err != nil {
		return ReadTextFileResponse{}, err
	}
	rel, err := sanitizeRelativePath(in.Path)
	if err != nil {
		return ReadTextFileResponse{}, fmt.Errorf("workspacefs: %w", err)
	}

	f, err := root.Open(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ReadTextFileResponse{}, fmt.Errorf("workspacefs: %w", err)
		}
		return ReadTextFileResponse{}, fmt.Errorf("workspacefs: open %q: %w", rel, err)
	}
	defer func() { _ = f.Close() }()

	data, err := readAllLimited(f, s.maxReadBytes)
	if err != nil {
		return ReadTextFileResponse{}, err
	}
	if !utf8.Valid(data) {
		return ReadTextFileResponse{}, errors.New("workspacefs: file is not valid UTF-8 text")
	}

	text := string(data)
	if in.Head != nil {
		text = applyHead(text, *in.Head)
	} else if in.Tail != nil {
		text = applyTail(text, *in.Tail)
	}
	return ReadTextFileResponse{Content: text}, nil
}

// ReadMultipleFiles reads UTF-8 text from several paths under one workspace.
// Failures for individual paths are reported in the response and do not abort the batch.
func (s *Service) ReadMultipleFiles(
	ctx context.Context,
	in ReadMultipleFilesRequest,
) (ReadMultipleFilesResponse, error) {
	if err := ctx.Err(); err != nil {
		return ReadMultipleFilesResponse{}, err
	}
	root, err := s.pickWorkspace(in.Workspace)
	if err != nil {
		return ReadMultipleFilesResponse{}, err
	}

	results := make([]ReadMultipleFileResult, 0, len(in.Paths))
	for _, p := range in.Paths {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ReadMultipleFilesResponse{}, ctxErr
		}
		results = append(results, s.readMultipleOne(root, p))
	}
	return ReadMultipleFilesResponse{Results: results}, nil
}

func (s *Service) readMultipleOne(root *os.Root, path string) ReadMultipleFileResult {
	out := ReadMultipleFileResult{Path: path}
	if strings.TrimSpace(path) == "" {
		out.Error = "workspacefs: path is required"
		return out
	}
	rel, err := sanitizeRelativePath(path)
	if err != nil {
		out.Error = fmt.Sprintf("workspacefs: %v", err)
		return out
	}

	f, err := root.Open(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			out.Error = err.Error()
		} else {
			out.Error = fmt.Sprintf("workspacefs: open %q: %v", rel, err)
		}
		return out
	}
	defer func() { _ = f.Close() }()

	data, err := readAllLimited(f, s.maxReadBytes)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	if !utf8.Valid(data) {
		out.Error = "workspacefs: file is not valid UTF-8 text"
		return out
	}
	out.Content = string(data)
	return out
}

func (s *Service) pickWorkspace(workspaceField string) (*os.Root, error) {
	e, err := s.pickWorkspaceEntry(workspaceField)
	if err != nil {
		return nil, err
	}
	return e.root, nil
}

func (s *Service) pickWorkspaceEntry(workspaceField string) (*workspaceEntry, error) {
	if s == nil || len(s.entries) == 0 {
		return nil, errors.New("workspacefs: no workspaces configured")
	}
	ws := strings.TrimSpace(workspaceField)
	if ws == "" {
		return nil, errors.New("workspacefs: workspace is required")
	}
	e, ok := s.byID[ws]
	if !ok {
		return nil, fmt.Errorf("workspacefs: workspace %q is not configured", ws)
	}
	return e, nil
}

func sanitizeRelativePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if filepath.IsAbs(p) {
		return "", errors.New("path must be relative to workspace")
	}
	name := filepath.ToSlash(filepath.Clean(p))
	if name == ".." || strings.HasPrefix(name, "../") {
		return "", errors.New("path escapes workspace")
	}
	return name, nil
}

func readAllLimited(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("workspacefs: max read size must be positive")
	}
	lr := io.LimitReader(r, limit+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("workspacefs: read file: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("workspacefs: file exceeds maximum read size (%d bytes)", limit)
	}
	return data, nil
}

func applyHead(s string, n int) string {
	if n == 0 {
		return ""
	}
	lines := splitLines(s)
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:n], "\n")
}

func applyTail(s string, n int) string {
	if n == 0 {
		return ""
	}
	lines := splitLines(s)
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
