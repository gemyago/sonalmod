package workspacefs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// ListDirectoryRequest is the JSON input for workspacefs_list_directory.
type ListDirectoryRequest struct {
	// Path is relative to the workspace; use "." or omit for the workspace base.
	Path string `json:"path"`
	// Workspace is the configured workspace identifier (required on every call).
	Workspace string `json:"workspace"`
}

// ListDirectoryEntry is one entry returned by workspacefs_list_directory.
type ListDirectoryEntry struct {
	Name        string `json:"name"`
	IsDirectory bool   `json:"is_directory"`
}

// ListDirectoryResponse is the JSON result for workspacefs_list_directory.
type ListDirectoryResponse struct {
	Entries []ListDirectoryEntry `json:"entries"`
}

// ListDirectory returns a shallow listing of one directory under the selected workspace.
func (s *Service) ListDirectory(ctx context.Context, in ListDirectoryRequest) (ListDirectoryResponse, error) {
	if err := ctx.Err(); err != nil {
		return ListDirectoryResponse{}, err
	}

	root, err := s.pickWorkspace(in.Workspace)
	if err != nil {
		return ListDirectoryResponse{}, err
	}
	rel, err := sanitizeRelativePath(in.Path)
	if err != nil {
		return ListDirectoryResponse{}, fmt.Errorf("workspacefs: %w", err)
	}

	fsys := root.FS()
	fi, err := fs.Stat(fsys, rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ListDirectoryResponse{}, fmt.Errorf("workspacefs: %w", err)
		}
		return ListDirectoryResponse{}, fmt.Errorf("workspacefs: stat %q: %w", rel, err)
	}
	if !fi.IsDir() {
		return ListDirectoryResponse{}, errors.New("workspacefs: path is not a directory")
	}

	entries, err := fs.ReadDir(fsys, rel)
	if err != nil {
		return ListDirectoryResponse{}, fmt.Errorf("workspacefs: read directory %q: %w", rel, err)
	}
	if len(entries) > s.maxListEntries {
		return ListDirectoryResponse{}, fmt.Errorf(
			"workspacefs: directory listing exceeds maximum entries (%d)",
			s.maxListEntries,
		)
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	out := make([]ListDirectoryEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, ListDirectoryEntry{Name: e.Name(), IsDirectory: e.IsDir()})
	}
	return ListDirectoryResponse{Entries: out}, nil
}
