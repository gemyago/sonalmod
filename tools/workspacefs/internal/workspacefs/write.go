package workspacefs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// WriteFileRequest is the JSON input for workspacefs_write_file.
type WriteFileRequest struct {
	// Path is relative to the workspace (no leading slash, no ".." escape).
	Path string `json:"path"`
	// Workspace is the configured workspace identifier (required on every call).
	Workspace string `json:"workspace"`
	// Content is UTF-8 text written to the file (create or full overwrite; see module docs).
	Content string `json:"content"`
}

// WriteFileResponse is the JSON result for workspacefs_write_file.
type WriteFileResponse struct {
	BytesWritten int `json:"bytes_written"`
}

const (
	writeFilePerm = 0o644
	writeDirPerm  = 0o755
)

// WriteFile creates or overwrites a UTF-8 text file at a path under the selected workspace.
// Parent directories are created as needed. Existing files are truncated (full replace).
func (s *Service) WriteFile(ctx context.Context, in WriteFileRequest) (WriteFileResponse, error) {
	if err := ctx.Err(); err != nil {
		return WriteFileResponse{}, err
	}
	if strings.TrimSpace(in.Path) == "" {
		return WriteFileResponse{}, errors.New("workspacefs: path is required")
	}
	if !utf8.ValidString(in.Content) {
		return WriteFileResponse{}, errors.New("workspacefs: content must be valid UTF-8")
	}

	root, err := s.pickWorkspace(in.Workspace)
	if err != nil {
		return WriteFileResponse{}, err
	}
	rel, err := sanitizeRelativePath(in.Path)
	if err != nil {
		return WriteFileResponse{}, fmt.Errorf("workspacefs: %w", err)
	}

	data := []byte(in.Content)
	dir := filepath.Dir(rel)
	if dir != "." && dir != "" {
		if err = root.MkdirAll(dir, writeDirPerm); err != nil {
			return WriteFileResponse{}, fmt.Errorf("workspacefs: mkdir %q: %w", dir, err)
		}
	}
	if err = root.WriteFile(rel, data, writeFilePerm); err != nil {
		return WriteFileResponse{}, fmt.Errorf("workspacefs: write %q: %w", rel, err)
	}
	return WriteFileResponse{BytesWritten: len(data)}, nil
}
